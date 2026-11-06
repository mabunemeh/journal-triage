package triage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

var seeds = []string{
	`{"PRIORITY":"3","MESSAGE":"failed","_SYSTEMD_UNIT":"a.service"}`,
	`{"PRIORITY":3,"MESSAGE":[104,105],"SYSLOG_IDENTIFIER":"kernel"}`,
	`{ "MESSAGE" : "a" , "MESSAGE" : null , "X" : { "MESSAGE" : [ "b" , { } ] } }`,
	`{"MESSAGE":"escaped key","message":"lower","PRIORITY":"\"3\""}`,
	`{"A":"\\\"}","B":[1,-2.5e3,true,false,null,"]"],"PRIORITY":["3"]}`,
	`{}`, `[]`, `null`, `{bad}`, `{"a":1,}`, "\xff",
}

// mapRecord is the previous decoder: every field into a map, exact keys.
func mapRecord(data []byte) (record, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return record{}, false
	}
	return record{fields["PRIORITY"], fields["MESSAGE"], fields["_SYSTEMD_UNIT"], fields["SYSLOG_IDENTIFIER"]}, true
}

// FuzzParseRecord checks that the single-pass parser accepts exactly what the
// map decoder accepts and extracts byte-identical values.
func FuzzParseRecord(f *testing.F) {
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = bytes.TrimSpace(data) // as Summarize does before parsing
		got, err := parseRecord(data)
		want, ok := mapRecord(data)
		if (err == nil) != ok {
			t.Fatalf("parseRecord error %v; map decoder ok=%v; input %q", err, ok, data)
		}
		for i, pair := range [][2]json.RawMessage{
			{got.priority, want.priority}, {got.message, want.message},
			{got.unit, want.unit}, {got.identifier, want.identifier},
		} {
			if !bytes.Equal(pair[0], pair[1]) || (pair[0] == nil) != (pair[1] == nil) {
				t.Fatalf("field %d: got %q, want %q; input %q", i, pair[0], pair[1], data)
			}
		}
	})
}

// FuzzSummarize checks that every record is accounted for exactly once.
func FuzzSummarize(f *testing.F) {
	for _, seed := range seeds {
		f.Add(seed + "\n" + seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		r, err := Summarize(strings.NewReader(input), 7, 1)
		if err != nil {
			return
		}
		if r.Records != r.Matched+r.Filtered+r.Skipped.Priority+r.Skipped.Message ||
			r.BinaryMessages > r.Matched || r.TotalGroups != len(r.Groups)+r.OmittedGroups ||
			(r.Matched > 0) != (r.TotalGroups > 0) {
			t.Fatalf("inconsistent report %+v for %q", r, input)
		}
		_ = Text(r)
	})
}

// BenchmarkSummarize uses records shaped like `journalctl -o json --all` output.
func BenchmarkSummarize(b *testing.B) {
	var input strings.Builder
	for i := range 1000 {
		fields := map[string]any{
			"__CURSOR":             "s=0123456789abcdef0123456789abcdef;i=1a2b;b=0123456789abcdef0123456789abcdef;m=1;t=2;x=3",
			"__REALTIME_TIMESTAMP": "1700000000000000", "__MONOTONIC_TIMESTAMP": "123456789",
			"_BOOT_ID": strings.Repeat("a", 32), "_MACHINE_ID": strings.Repeat("b", 32), "_HOSTNAME": "host",
			"_TRANSPORT": "journal", "_UID": "0", "_GID": "0", "_PID": "1234", "_COMM": "svc", "_EXE": "/usr/bin/svc",
			"_CMDLINE": "/usr/bin/svc --config /etc/svc.conf", "_SYSTEMD_CGROUP": "/system.slice/svc.service",
			"_SYSTEMD_SLICE": "system.slice", "_SYSTEMD_INVOCATION_ID": strings.Repeat("c", 32),
			"CODE_FILE": "src/svc.c", "CODE_LINE": "42", "CODE_FUNC": "handle", "SYSLOG_FACILITY": "3",
			"PRIORITY": fmt.Sprint(i % 8), "_SYSTEMD_UNIT": fmt.Sprintf("svc%d.service", i%10),
			"MESSAGE": fmt.Sprintf("Request failed with status %d", 500+i%5),
		}
		line, err := json.Marshal(fields)
		if err != nil {
			b.Fatal(err)
		}
		input.Write(line)
		input.WriteByte('\n')
	}
	data := input.String()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Summarize(strings.NewReader(data), 4, 20); err != nil {
			b.Fatal(err)
		}
	}
}
