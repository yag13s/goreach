# goreach CLI

Command-line tool for analyzing Go coverage data to identify unreached code paths in running services.

## Install

```bash
go install github.com/yag13s/goreach/cmd/goreach@latest
```

## Usage

```
goreach <command> [flags]
```

## Commands

### analyze

Analyze coverage data and output a JSON report of unreached functions and blocks.

```bash
# From GOCOVERDIR
goreach analyze -coverdir /tmp/coverage -pretty

# From text coverage profile
goreach analyze -profile coverage.out -pretty -o report.json

# Recursive (multiple build versions in subdirectories)
goreach analyze -coverdir /tmp/coverage -r -pretty -o report.json
```

| Flag | Description | Default |
|------|-------------|---------|
| `-coverdir <dir>` | GOCOVERDIR path | — |
| `-profile <file>` | Text coverage profile (`go test -coverprofile`) | — |
| `-r` | Recursively search `-coverdir` for coverage data | `false` |
| `-pkg <prefixes>` | Package filter (comma-separated import path prefixes) | all |
| `-threshold <float>` | Include functions with coverage ≤ X% | `100` |
| `-min-statements <n>` | Include functions with ≥ N unreached statements | `0` |
| `-o <file>` | Output file | stdout |
| `-pretty` | Pretty-print JSON | `false` |

`-coverdir` and `-profile` are mutually exclusive.

### merge

Merge multiple JSON reports into one, taking the maximum coverage per function across all inputs.

```bash
goreach merge -pretty -o merged.json reports/v1.json reports/v2.json
```

| Flag | Description | Default |
|------|-------------|---------|
| `-o <file>` | Output file | stdout |
| `-pretty` | Pretty-print JSON | `false` |

Uses the newest report as the structural base. Functions present only in older reports are excluded.
When an older build has higher coverage but no block detail, the latest build's blocks are preserved in `latest_unreached_blocks`.

### view

Launch an interactive web UI to browse the report with inline source preview.

```bash
goreach view report.json
goreach view -src . report.json
```

| Flag | Description | Default |
|------|-------------|---------|
| `-src <dir>` | Source root for inline code preview | — (disabled) |
| `-port <n>` | HTTP port | `0` (random) |
| `-no-open` | Do not auto-open browser | `false` |
| `-report <file>` | Path to report.json (alternative to positional argument) | — |

### summary

Print a text coverage summary per package to stdout.

```bash
goreach summary -coverdir /tmp/coverage
goreach summary -profile coverage.out
```

| Flag | Description | Default |
|------|-------------|---------|
| `-coverdir <dir>` | GOCOVERDIR path | — |
| `-profile <file>` | Text coverage profile | — |
| `-r` | Recursively search `-coverdir` | `false` |

### version

```bash
goreach version
```

## Workflow

### Single build

```bash
# 1. Build with coverage instrumentation
go build -cover -covermode=atomic -o myserver .

# 2. Run the service
GOCOVERDIR=/tmp/coverage ./myserver

# 3. Analyze (after process exits or after flushing via SDK)
goreach analyze -coverdir /tmp/coverage -pretty -o report.json

# 4. View in browser
goreach view -src . report.json
```

### Multiple build versions

```bash
# Analyze each build version separately, then merge
for dir in coverage-data/*/; do
    version=$(basename "$dir")
    goreach analyze -coverdir "$dir" -r -pretty -o "reports/$version.json"
done

goreach merge -pretty -o merged.json reports/*.json
goreach view -src . merged.json
```

## JSON Output Format

```json
{
  "version": 1,
  "generated_at": "2025-01-01T00:00:00Z",
  "mode": "atomic",
  "total": {
    "total_statements": 120,
    "covered_statements": 95,
    "coverage_percent": 79.16
  },
  "packages": [{
    "import_path": "myapp/handler",
    "files": [{
      "file_name": "myapp/handler/handler.go",
      "functions": [{
        "name": "HandleRequest",
        "line": 28,
        "total_statements": 12,
        "covered_statements": 9,
        "coverage_percent": 75.0,
        "unreached_blocks": [
          {"start_line": 42, "start_col": 1, "end_line": 45, "end_col": 1, "num_statements": 2}
        ]
      }]
    }]
  }]
}
```

## Requirements

- Go 1.26+
- `go tool covdata` (bundled with Go)

For the flush SDK (emitting coverage from a running process without stopping it), see the [root README](../../README.md).
