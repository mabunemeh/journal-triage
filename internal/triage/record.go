package triage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// record holds the raw values of the only fields the summary reads. Values
// alias the input line, so they must be converted before the next line is read.
type record struct {
	priority, message, unit, identifier json.RawMessage
}

var errNotObject = errors.New("expected a JSON object")

// parseRecord extracts the wanted top-level fields from one JSON object.
//
// Decoding every field into a map is the slowest part of a run, and decoding
// into a struct would also match keys case-insensitively ("message" as
// MESSAGE). Instead the line is validated, then walked once. Keys match
// exactly after unescaping and, as with a map, a repeated key keeps its last
// value.
func parseRecord(data []byte) (record, error) {
	if len(data) == 0 || data[0] != '{' || !json.Valid(data) {
		var probe any
		if err := json.Unmarshal(data, &probe); err != nil {
			return record{}, fmt.Errorf("%w: %w", errNotObject, err)
		}
		return record{}, errNotObject
	}
	var fields record
	for i := skipSpace(data, 1); data[i] != '}'; {
		start := i
		i = skipString(data, i)
		name := data[start+1 : i-1]
		if bytes.IndexByte(name, '\\') >= 0 {
			var unescaped string
			if err := json.Unmarshal(data[start:i], &unescaped); err != nil {
				return record{}, fmt.Errorf("%w: %w", errNotObject, err)
			}
			name = []byte(unescaped)
		}
		i = skipSpace(data, skipSpace(data, i)+1) // the colon
		start = i
		i = skipValue(data, i)
		value := json.RawMessage(data[start:i])
		switch string(name) {
		case "PRIORITY":
			fields.priority = value
		case "MESSAGE":
			fields.message = value
		case "_SYSTEMD_UNIT":
			fields.unit = value
		case "SYSLOG_IDENTIFIER":
			fields.identifier = value
		}
		if i = skipSpace(data, i); data[i] == ',' {
			i = skipSpace(data, i+1)
		}
	}
	return fields, nil
}

// The helpers below assume valid JSON and return the index after what they skip.

func skipSpace(data []byte, i int) int {
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\n' || data[i] == '\r') {
		i++
	}
	return i
}

func skipString(data []byte, i int) int {
	for i++; data[i] != '"'; i++ {
		if data[i] == '\\' {
			i++
		}
	}
	return i + 1
}

func skipValue(data []byte, i int) int {
	switch data[i] {
	case '"':
		return skipString(data, i)
	case '{', '[':
		for depth := 0; ; {
			switch data[i] {
			case '"':
				i = skipString(data, i)
				continue
			case '{', '[':
				depth++
			case '}', ']':
				if depth--; depth == 0 {
					return i + 1
				}
			}
			i++
		}
	default:
		for i < len(data) && bytes.IndexByte([]byte(",}] \t\r\n"), data[i]) < 0 {
			i++
		}
		return i
	}
}
