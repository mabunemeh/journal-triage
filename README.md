# journal-triage

[![CI](https://github.com/mabunemeh/journal-triage/actions/workflows/ci.yml/badge.svg)](https://github.com/mabunemeh/journal-triage/actions/workflows/ci.yml)

Turn repeated Linux journal errors into a short, ranked summary. Group exact
matches by service, priority and message; show the most severe groups first,
then the most frequent. Written in Go, with no third-party dependencies.

Reads saved `journalctl -o json` output or stdin. It never runs commands,
connects to a host, or changes services. Saved logs work on Linux, macOS and
Windows; collecting the journal requires a systemd host.

## Install and try

Requires Go 1.25+ to build. The resulting binary needs no Go installation.

```sh
git clone https://github.com/mabunemeh/journal-triage.git
cd journal-triage
go build -o dist/journal-triage ./cmd/journal-triage
./dist/journal-triage examples/demo.jsonl
```

The demo contains synthetic records, including one informational message that
the default warning-and-higher filter excludes:

```text
Journal triage: 5 matched / 6 records; 1 filtered; 0 skipped
Groups: 3 total; showing 3
CRIT        1x "kernel" "Synthetic storage failure"
ERR         3x "api-demo.service" "Database connection timed out"
WARNING     1x "worker-demo.service" "Job queue is growing"
```

On Windows, build with `-o dist/journal-triage.exe` and run
`.\dist\journal-triage.exe examples/demo.jsonl`.

## Use with journalctl

After putting the executable on your PATH:

```sh
# This boot's warnings and errors; --all prevents large fields becoming null.
set -o pipefail
journalctl -b -o json --all --no-pager | journal-triage -

# Limit the time window and service at collection time.
journalctl --since '1 hour ago' -u nova-compute.service -o json --all --no-pager \
  | journal-triage --limit 10 -

# Save logs locally, then produce a JSON report.
mkdir -p logs
journalctl -b -o json --all --no-pager > logs/boot.jsonl
journal-triage --format json logs/boot.jsonl
```

`set -o pipefail` is a Bash option: it makes a collection failure fail the
pipeline even if the summarizer successfully reads an empty stream. Collection
only sees journal entries your account can access. `logs/` is ignored by Git;
review logs and reports before sharing them, as they can contain sensitive data.

## Options and behavior

Put options **before** the filename or `-`.

| Option | Default | Meaning |
| --- | --- | --- |
| `--format text\|json` | `text` | Human-readable summary or structured report |
| `--max-priority 0..7` | `4` | Include priorities 0 through this value |
| `--limit N` | `20` | Show at most N groups, with omitted count in JSON |
| `--version` | | Print version |

Priorities: `0 emerg`, `1 alert`, `2 crit`, `3 err`, `4 warning`, `5 notice`,
`6 info`, `7 debug`. Use `--max-priority 7` to include everything.

- Group labels use `_SYSTEMD_UNIT`, then `SYSLOG_IDENTIFIER`, then `(unknown)`.
- Messages match exactly, including whitespace. Different IDs, timestamps or
  priorities in otherwise similar records create separate groups.
- Ordering is priority ascending, count descending, then unit and message.
- Text output quotes log fields and escapes control characters and Unicode,
  so log content cannot inject terminal escape sequences into the report.
- JSON has `schema_version: 1`, record accounting (including
  `binary_messages`), omitted-group count, and a `groups` array. Even an empty
  report uses `groups: []`.

## Input limits and exit codes

Input must be UTF-8 JSON Lines: one object per line, as emitted by
`journalctl -o json`. Blank lines and an initial UTF-8 BOM are accepted. Pretty
JSON, journal export format, and binary `.journal` files are not supported.

[journalctl's JSON format](https://www.freedesktop.org/software/systemd/man/latest/journalctl.html)
can represent truncated fields as null, binary values as byte arrays, and
repeated fields as arrays. journalctl writes any message containing a tab, ANSI
colour code or other control character as a byte array; these are decoded, with
invalid UTF-8 replaced by U+FFFD, and counted as `binary_messages` in JSON and
a `Byte-array messages decoded:` line in text. Missing or unsupported
priorities are counted as skipped; matching records with missing, null, empty
or multiple messages are also counted as skipped. Priority filtering happens
before message validation. Skipped records are visible in both formats and
cause exit 1; they never become invented messages. Field names match exactly,
as journald defines them.

| Exit | Meaning |
| --- | --- |
| `0` | Complete summary, even if it contains errors from the journal |
| `1` | Summary produced, but some records had unsupported/missing fields |
| `2` | Invalid command-line arguments |
| `3` | Input/output failure or malformed input; input failures emit no report |

A record must fit within 8 MiB including its newline. The tool reads one line
at a time but retains all unique groups in memory. `--limit` limits display,
not memory: choose a bounded time window for large journals. This is an offline
summary, not a live follower or a root-cause diagnosis tool. Keep multi-host
exports separate because grouping does not include hostname or machine ID.

## Develop

```sh
go test -count=1 -cover ./...
go vet ./...
gofmt -w cmd internal
go test -run '^$' -bench . ./internal/triage
go test -run '^$' -fuzz FuzzParseRecord -fuzztime 1m ./internal/triage
```

`FuzzParseRecord` checks the single-pass field parser against a plain
`encoding/json` map decoder; `FuzzSummarize` checks that every record is
counted exactly once. CI checks Linux and Windows, runs the race detector and
both fuzzers briefly on Linux, lints the Go code, and scans for secrets. To enable local hooks, install `pre-commit` and
run `pre-commit install`; Go and Git Bash must be on PATH.

Licensed under [Apache-2.0](LICENSE).
