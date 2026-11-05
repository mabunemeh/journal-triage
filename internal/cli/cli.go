// Package cli implements the command-line interface with injectable input/output.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mabunemeh/journal-triage/internal/triage"
)

const version = "0.1.0"

func diagnostic(stderr io.Writer, code int, message string) int {
	if _, err := fmt.Fprintln(stderr, "journal-triage: "+message); err != nil {
		return 3
	}
	return code
}

// Run returns 0 on success, 1 for skipped evidence, 2 for usage and 3 for I/O/input errors.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("journal-triage [flags] FILE|-", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	maxPriority := flags.Int("max-priority", 4, "include priorities 0 through this number (0..7)")
	limit := flags.Int("limit", 20, "maximum groups to display")
	showVersion := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, "journal-triage "+version); err != nil {
			return 3
		}
		return 0
	}
	if flags.NArg() != 1 || (*format != "text" && *format != "json") ||
		*maxPriority < 0 || *maxPriority > 7 || *limit < 1 {
		return diagnostic(stderr, 2, "usage: journal-triage [--format text|json] [--max-priority 0..7] [--limit N] FILE|-")
	}
	input := stdin
	var file *os.File
	if flags.Arg(0) != "-" {
		var err error
		file, err = os.Open(flags.Arg(0)) // #nosec G304 -- the user explicitly selects the input file.
		if err != nil {
			return diagnostic(stderr, 3, "cannot open input file")
		}
		input = file
	}
	report, err := triage.Summarize(input, *maxPriority, *limit)
	if file != nil {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return diagnostic(stderr, 3, err.Error())
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(report)
	} else {
		_, err = io.WriteString(stdout, triage.Text(report))
	}
	if err != nil {
		return diagnostic(stderr, 3, "cannot write report")
	}
	if report.Skipped.Priority+report.Skipped.Message > 0 {
		return 1
	}
	return 0
}
