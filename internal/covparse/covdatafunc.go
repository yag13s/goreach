package covparse

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FuncCoverage holds per-function coverage data extracted from `go tool covdata func`.
type FuncCoverage struct {
	FileName        string // e.g. "github.com/user/pkg/file.go"
	FuncName        string // goreach (astmap) normalized: "(*Type).Method"
	CoveragePercent float64
}

// RunCovdataFunc executes `go tool covdata func` on the given directories
// and returns per-function coverage data with goreach-normalized function names.
func RunCovdataFunc(dirs []string) ([]FuncCoverage, error) {
	joined := strings.Join(dirs, ",")
	cmd := exec.Command("go", "tool", "covdata", "func", "-i="+joined)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("covparse: go tool covdata func: %w\n%s", err, out)
	}
	return parseCovdataFuncOutput(string(out)), nil
}

// parseCovdataFuncOutput parses the output of `go tool covdata func`.
// Each line has the format: <file>:<line>: <funcname> <pct>%
// The last line is a total line: "total (statements) <pct>%" which is skipped.
func parseCovdataFuncOutput(output string) []FuncCoverage {
	var result []FuncCoverage
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		// Format: "github.com/user/pkg/file.go:42:\t\tFuncName\t\t75.0%"
		fileName, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// Skip the line number; what follows is the function name and pct%.
		_, rest, ok = strings.Cut(rest, ":")
		if !ok {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		funcName := fields[0]
		pctStr := strings.TrimSuffix(fields[1], "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			continue
		}

		result = append(result, FuncCoverage{
			FileName:        fileName,
			FuncName:        NormalizeCovdataFuncName(funcName),
			CoveragePercent: pct,
		})
	}
	return result
}

// NormalizeCovdataFuncName converts `go tool covdata func` function name format
// to the goreach (astmap) format:
//
//	FuncName       → FuncName
//	*Type.Method   → (*Type).Method
//	Type.Method    → (Type).Method
func NormalizeCovdataFuncName(name string) string {
	recv, method, ok := strings.CutLast(name, ".")
	if !ok || recv == "" {
		// plain function, no receiver
		return name
	}
	return "(" + recv + ")." + method
}
