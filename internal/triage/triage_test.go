package triage

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDemo(t *testing.T) {
	data, err := os.ReadFile("../../examples/demo.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Summarize(strings.NewReader(string(data)), 4, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{
		{"kernel", 2, "crit", "Synthetic storage failure", 1},
		{"api-demo.service", 3, "err", "Database connection timed out", 3},
		{"worker-demo.service", 4, "warning", "Job queue is growing", 1},
	}
	if report.Records != 6 || report.Matched != 5 || report.Filtered != 1 ||
		report.Skipped != (Skipped{}) || report.TotalGroups != 3 || !reflect.DeepEqual(report.Groups, want) {
		t.Fatalf("unexpected report: %+v", report)
	}
	wantText, err := os.ReadFile("testdata/demo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := Text(report); got != string(wantText) {
		t.Fatalf("text mismatch:\n%s", got)
	}
}

func TestPriorityAndMessageFields(t *testing.T) {
	tests := []struct {
		name, input                                  string
		matched, filtered, priority, message, binary int
	}{
		{"integer", `{"PRIORITY":0,"MESSAGE":""}`, 1, 0, 0, 0, 0},
		{"string", `{"PRIORITY":"4","MESSAGE":"warning"}`, 1, 0, 0, 0, 0},
		{"filtered before message", `{"PRIORITY":"5","MESSAGE":null}`, 0, 1, 0, 0, 0},
		{"no priority", `{"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"null priority", `{"PRIORITY":null,"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"bool priority", `{"PRIORITY":true,"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"float priority", `{"PRIORITY":3.5,"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"out of range", `{"PRIORITY":"8","MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"negative", `{"PRIORITY":-1,"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"multiple priorities", `{"PRIORITY":["3","4"],"MESSAGE":"hello"}`, 0, 0, 1, 0, 0},
		{"no message", `{"PRIORITY":"3"}`, 0, 0, 0, 1, 0},
		{"null message", `{"PRIORITY":"3","MESSAGE":null}`, 0, 0, 0, 1, 0},
		{"binary message", `{"PRIORITY":"3","MESSAGE":[104,105]}`, 1, 0, 0, 0, 1},
		{"binary invalid UTF-8", `{"PRIORITY":"3","MESSAGE":[255,9]}`, 1, 0, 0, 0, 1},
		{"empty byte array", `{"PRIORITY":"3","MESSAGE":[]}`, 0, 0, 0, 1, 0},
		{"byte out of range", `{"PRIORITY":"3","MESSAGE":[256]}`, 0, 0, 0, 1, 0},
		{"fractional byte", `{"PRIORITY":"3","MESSAGE":[1.5]}`, 0, 0, 0, 1, 0},
		{"multiple messages", `{"PRIORITY":"3","MESSAGE":["a","b"]}`, 0, 0, 0, 1, 0},
		{"multiple binary messages", `{"PRIORITY":"3","MESSAGE":[[104],[105]]}`, 0, 0, 0, 1, 0},
		{"lowercase key ignored", `{"PRIORITY":"3","message":"hello"}`, 0, 0, 0, 1, 0},
		{"escaped key", `{"PRIORITY":"3","MESS\u0041GE":"hello"}`, 1, 0, 0, 0, 0},
		{"last duplicate wins", `{"PRIORITY":"3","MESSAGE":"a","MESSAGE":null}`, 0, 0, 0, 1, 0},
		{"nested keys ignored", `{"X":{"MESSAGE":"a","PRIORITY":"3"},"Y":["MESSAGE"]}`, 0, 0, 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Summarize(strings.NewReader(tt.input), 4, 20)
			if err != nil {
				t.Fatal(err)
			}
			if r.Records != 1 || r.Matched != tt.matched || r.Filtered != tt.filtered ||
				r.Skipped.Priority != tt.priority || r.Skipped.Message != tt.message || r.BinaryMessages != tt.binary {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestOrderingAndLimit(t *testing.T) {
	input := `{"PRIORITY":"3","MESSAGE":"b","_SYSTEMD_UNIT":"a"}
{"PRIORITY":"3","MESSAGE":"a","_SYSTEMD_UNIT":"z"}
{"PRIORITY":"3","MESSAGE":"a","_SYSTEMD_UNIT":"a"}
{"PRIORITY":"3","MESSAGE":"b","_SYSTEMD_UNIT":"z"}
{"PRIORITY":"3","MESSAGE":"b","_SYSTEMD_UNIT":"z"}
{"PRIORITY":"2","MESSAGE":"severe"}`
	r, err := Summarize(strings.NewReader(input), 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{{"(unknown)", 2, "crit", "severe", 1}, {"z", 3, "err", "b", 2},
		{"a", 3, "err", "a", 1}, {"a", 3, "err", "b", 1}}
	if r.TotalGroups != 5 || r.OmittedGroups != 1 || r.Matched != 6 || !reflect.DeepEqual(r.Groups, want) {
		t.Fatalf("%+v", r)
	}
	// A lower cutoff must filter records rather than silently change their severity.
	r, err = Summarize(strings.NewReader(input), 2, 20)
	if err != nil || r.Matched != 1 || r.Filtered != 5 {
		t.Fatalf("%+v, %v", r, err)
	}
}

func TestBinaryMessages(t *testing.T) {
	// journalctl writes this ANSI-coloured message as a byte array.
	input := `{"PRIORITY":"3","MESSAGE":[27,91,51,49,109,102,97,105,108,27,91,48,109,9,255],"_SYSTEMD_UNIT":"a"}
{"PRIORITY":"3","MESSAGE":[104,105],"_SYSTEMD_UNIT":"a"}
{"PRIORITY":"3","MESSAGE":"hi","_SYSTEMD_UNIT":"a"}`
	r, err := Summarize(strings.NewReader(input), 4, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{{"a", 3, "err", "hi", 2}, {"a", 3, "err", "\x1b[31mfail\x1b[0m\t\uFFFD", 1}}
	if r.Matched != 3 || r.BinaryMessages != 2 || r.Skipped != (Skipped{}) || !reflect.DeepEqual(r.Groups, want) {
		t.Fatalf("%+v", r)
	}
	got := Text(r)
	if strings.ContainsAny(got, "\x1b\t") || !strings.Contains(got, `"\x1b[31mfail\x1b[0m\t\ufffd"`) ||
		!strings.Contains(got, "Byte-array messages decoded: 2") {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestUnitFallbackAndExactGrouping(t *testing.T) {
	input := `{"PRIORITY":3,"MESSAGE":"x","_SYSTEMD_UNIT":"svc","SYSLOG_IDENTIFIER":"ignored"}
{"PRIORITY":3,"MESSAGE":"x","_SYSTEMD_UNIT":null,"SYSLOG_IDENTIFIER":"fallback"}
{"PRIORITY":3,"MESSAGE":"x","_SYSTEMD_UNIT":[],"SYSLOG_IDENTIFIER":false}
{"PRIORITY":4,"MESSAGE":"x","_SYSTEMD_UNIT":"svc"}
{"PRIORITY":3,"MESSAGE":"x ","_SYSTEMD_UNIT":"svc"}`
	r, err := Summarize(strings.NewReader(input), 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalGroups != 5 || r.Groups[0].Unit != "(unknown)" || r.Groups[1].Unit != "fallback" || r.Groups[2].Unit != "svc" {
		t.Fatalf("%+v", r)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestInputErrors(t *testing.T) {
	for _, input := range []string{"[]", "null", "3", "true", `"{}"`, `{bad}`, `{"a":1,}`, `{"a":}`, "{} {}", "{}\n[",
		"\xff", `{"MESS\uZZZZ":1}`, strings.Repeat("x", MaxLineBytes+1)} {
		r, err := Summarize(strings.NewReader(input), 4, 20)
		if err == nil || r.Records != 0 {
			t.Fatalf("expected error, got %+v, %v", r, err)
		}
	}
	if _, err := Summarize(brokenReader{}, 4, 20); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatal(err)
	}
	for _, args := range [][2]int{{-1, 20}, {8, 20}, {4, 0}} {
		if _, err := Summarize(strings.NewReader(""), args[0], args[1]); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
}

func TestEmptyBOMAndLongLines(t *testing.T) {
	r, err := Summarize(strings.NewReader("\xef\xbb\xbf\r\n \n"), 4, 20)
	if err != nil || r.Records != 0 || r.Groups == nil {
		t.Fatalf("%+v, %v", r, err)
	}
	data, err := json.Marshal(map[string]any{"PRIORITY": 3, "MESSAGE": strings.Repeat("x", 100000)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = Summarize(strings.NewReader(string(data)), 4, 20)
	if err != nil || r.Matched != 1 {
		t.Fatalf("%+v, %v", r, err)
	}
}

func TestTextEscapesAndSkipped(t *testing.T) {
	r, err := Summarize(strings.NewReader("{\"PRIORITY\":3,\"MESSAGE\":\"\\u001b[31m\\n\\u202e\",\"_SYSTEMD_UNIT\":\"\\t\"}\n{}"), 4, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := Text(r)
	if strings.ContainsAny(got, "\x1b\t\u202e") || !strings.Contains(got, `\u202e`) || !strings.Contains(got, "1 missing/unsupported priority") {
		t.Fatalf("unsafe or incomplete output: %q", got)
	}
}
