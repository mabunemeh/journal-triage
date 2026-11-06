package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mabunemeh/journal-triage/internal/triage"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		input    string
		code     int
		out, err string
	}{
		{"stdin", []string{"-"}, `{"PRIORITY":"3","MESSAGE":"failed"}`, 0, "ERR", ""},
		{"empty", []string{"-"}, "", 0, "0 matched / 0 records", ""},
		{"skipped", []string{"-"}, `{}`, 1, "1 skipped", ""},
		{"binary message", []string{"-"}, `{"PRIORITY":"3","MESSAGE":[104,105]}`, 0, "decoded: 1", ""},
		{"malformed", []string{"-"}, "{}\n[", 3, "", "line 2"},
		{"missing file", []string{"missing-file.jsonl"}, "", 3, "", "cannot open input"},
		{"no argument", nil, "", 2, "", "usage:"},
		{"bad flag", []string{"--wat"}, "", 2, "", "flag provided"},
		{"bad integer", []string{"--limit", "no", "-"}, "", 2, "", "invalid value"},
		{"bad format", []string{"--format", "xml", "-"}, "", 2, "", "usage:"},
		{"negative limit", []string{"--limit", "-1", "-"}, "", 2, "", "usage:"},
		{"zero limit", []string{"--limit", "0", "-"}, "", 2, "", "usage:"},
		{"priority high", []string{"--max-priority", "8", "-"}, "", 2, "", "usage:"},
		{"priority low", []string{"--max-priority", "-1", "-"}, "", 2, "", "usage:"},
		{"flags after input", []string{"-", "--format", "json"}, "", 2, "", "usage:"},
		{"version", []string{"--version"}, "", 0, "journal-triage 0.1.0", ""},
		{"help", []string{"--help"}, "", 0, "", "Usage of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(tt.input), &out, &stderr)
			if code != tt.code || !strings.Contains(out.String(), tt.out) || !strings.Contains(stderr.String(), tt.err) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			if tt.code >= 2 && out.Len() != 0 {
				t.Fatal("failed input emitted a partial report")
			}
		})
	}
}

func TestFileAndJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf{\"PRIORITY\":7,\"MESSAGE\":\"debug\"}\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"--format", "json", "--max-priority", "7", path}, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("exit=%d: %s", code, stderr.String())
	}
	var r triage.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != 1 || r.Matched != 1 || r.Groups[0].Severity != "debug" {
		t.Fatalf("%+v", r)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteFailures(t *testing.T) {
	for _, args := range [][]string{{"-"}, {"--format", "json", "-"}, {"--version"}} {
		var stderr bytes.Buffer
		if code := Run(args, strings.NewReader(""), brokenWriter{}, &stderr); code != 3 {
			t.Fatal(code)
		}
	}
	if code := Run(nil, strings.NewReader(""), &bytes.Buffer{}, brokenWriter{}); code != 3 {
		t.Fatal(code)
	}
}
