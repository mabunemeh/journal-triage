// Command journal-triage groups repeated systemd journal errors.
package main

import (
	"os"

	"github.com/mabunemeh/journal-triage/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
