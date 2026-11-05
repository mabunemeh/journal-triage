// Package triage summarizes newline-delimited journal JSON without contacting a host.
package triage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxLineBytes bounds a single input record, including its newline.
const MaxLineBytes = 8 * 1024 * 1024

var priorities = [...]string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"}

// Group is an exact match of service, severity and message.
type Group struct {
	Unit     string `json:"unit"`
	Priority int    `json:"priority"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
}

// Skipped counts records whose priority or message could not be interpreted.
type Skipped struct {
	Priority int `json:"priority"`
	Message  int `json:"message"`
}

// Report accounts for all records, including filtered and unsupported entries.
type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Records       int     `json:"records"`
	Matched       int     `json:"matched"`
	Filtered      int     `json:"filtered"`
	Skipped       Skipped `json:"skipped"`
	TotalGroups   int     `json:"total_groups"`
	OmittedGroups int     `json:"omitted_groups"`
	Groups        []Group `json:"groups"`
}

type key struct {
	unit     string
	priority int
	message  string
}

func priority(raw json.RawMessage) (int, bool) {
	value := string(raw)
	if len(value) == 3 && value[0] == '"' && value[2] == '"' {
		value = value[1:2]
	}
	if len(value) != 1 || value[0] < '0' || value[0] > '7' {
		return 0, false
	}
	return int(value[0] - '0'), true
}

func text(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func unit(record map[string]json.RawMessage) string {
	for _, field := range []string{"_SYSTEMD_UNIT", "SYSLOG_IDENTIFIER"} {
		if value, ok := text(record[field]); ok && value != "" {
			return value
		}
	}
	return "(unknown)"
}

// Summarize ranks groups by severity, descending count, unit and message.
// Blank lines are ignored. Malformed input returns no partial report.
// Memory grows with unique groups; limit only bounds the displayed groups.
func Summarize(input io.Reader, maxPriority, limit int) (Report, error) {
	if maxPriority < 0 || maxPriority > 7 || limit < 1 {
		return Report{}, fmt.Errorf("priority must be 0..7 and limit must be positive")
	}
	report := Report{SchemaVersion: 1, Groups: []Group{}}
	counts := make(map[key]int)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes)
	line := 0
	for scanner.Scan() {
		line++
		data := scanner.Bytes()
		if line == 1 {
			data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}
		if !utf8.Valid(data) {
			return Report{}, fmt.Errorf("line %d: invalid UTF-8", line)
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(data, &record); err != nil {
			return Report{}, fmt.Errorf("line %d: expected a JSON object: %w", line, err)
		}
		if record == nil {
			return Report{}, fmt.Errorf("line %d: expected a JSON object", line)
		}
		report.Records++
		level, ok := priority(record["PRIORITY"])
		if !ok {
			report.Skipped.Priority++
			continue
		}
		if level > maxPriority {
			report.Filtered++
			continue
		}
		message, ok := text(record["MESSAGE"])
		if !ok {
			report.Skipped.Message++
			continue
		}
		report.Matched++
		counts[key{unit(record), level, message}]++
	}
	if err := scanner.Err(); err != nil {
		return Report{}, fmt.Errorf("read input near line %d (maximum record size 8 MiB): %w", line+1, err)
	}
	for item, count := range counts {
		report.Groups = append(report.Groups, Group{
			Unit: item.unit, Priority: item.priority, Severity: priorities[item.priority],
			Message: item.message, Count: count,
		})
	}
	sort.Slice(report.Groups, func(i, j int) bool {
		a, b := report.Groups[i], report.Groups[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		return a.Message < b.Message
	})
	report.TotalGroups = len(report.Groups)
	if len(report.Groups) > limit {
		report.OmittedGroups = len(report.Groups) - limit
		report.Groups = report.Groups[:limit]
	}
	return report, nil
}

// Text quotes log-supplied fields to keep terminal control characters inert.
func Text(report Report) string {
	lines := []string{
		fmt.Sprintf("Journal triage: %d matched / %d records; %d filtered; %d skipped",
			report.Matched, report.Records, report.Filtered, report.Skipped.Priority+report.Skipped.Message),
		fmt.Sprintf("Groups: %d total; showing %d", report.TotalGroups, len(report.Groups)),
	}
	if report.Skipped.Priority+report.Skipped.Message > 0 {
		lines = append(lines, fmt.Sprintf("Skipped: %d missing/unsupported priority; %d missing/non-text message",
			report.Skipped.Priority, report.Skipped.Message))
	}
	for _, group := range report.Groups {
		lines = append(lines, fmt.Sprintf("%-7s %5dx %s %s", strings.ToUpper(group.Severity),
			group.Count, strconv.QuoteToASCII(group.Unit), strconv.QuoteToASCII(group.Message)))
	}
	return strings.Join(lines, "\n") + "\n"
}
