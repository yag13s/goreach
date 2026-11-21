package covparse

import (
	"cmp"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/cover"
)

// FuncCoverage holds per-function coverage data extracted from `go tool covdata func`.
type FuncCoverage struct {
	FileName        string // e.g. "github.com/user/pkg/file.go"
	FuncName        string // goreach (astmap) normalized: "(*Type).Method"
	Line            int    // line covdata func reports: where the function's first block starts
	CoveragePercent float64

	// Calls is how many times the function was entered, or nil if unknown.
	// covdata func does not report it; [AttachCalls] fills it in.
	Calls *int
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
		if line == "" {
			continue
		}
		// Skip only the summary line itself: a module path may start with
		// "total" too (totally.dev/x/file.go:...).
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "total" && f[1] == "(statements)" {
			continue
		}

		// Format: "github.com/user/pkg/file.go:42:\t\tFuncName\t\t75.0%"
		fileName, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// The line number, then the function name and pct%.
		lineStr, rest, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		lineNum, _ := strconv.Atoi(lineStr)

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
			Line:            lineNum,
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

// AttachCalls sets Calls on each of funcs from the profiles of the same
// coverage data. It needs no source code: a function's first block is the
// first block of its file at or after the line covdata func reports for the
// function and before the next function's, and that block runs once per call.
//
// Calls stays nil for functions whose file has no profile, whose line is
// unknown or shared with another function, or when the profiles carry no
// execution counts (set mode).
func AttachCalls(funcs []FuncCoverage, profiles []*cover.Profile) {
	byFile := make(map[string]*cover.Profile, len(profiles))
	for _, p := range profiles {
		if p.Mode != "set" {
			byFile[p.FileName] = p
		}
	}

	// Indexes of funcs per file, in declaration order.
	perFile := make(map[string][]int)
	for i, fc := range funcs {
		if fc.Line > 0 {
			perFile[fc.FileName] = append(perFile[fc.FileName], i)
		}
	}

	for file, idxs := range perFile {
		prof, ok := byFile[file]
		if !ok {
			continue
		}
		slices.SortFunc(idxs, func(a, b int) int { return cmp.Compare(funcs[a].Line, funcs[b].Line) })
		blocks := slices.Clone(prof.Blocks)
		slices.SortFunc(blocks, func(a, b cover.ProfileBlock) int {
			return cmp.Or(cmp.Compare(a.StartLine, b.StartLine), cmp.Compare(a.StartCol, b.StartCol))
		})

		for n, i := range idxs {
			start := funcs[i].Line
			// Several functions reported on one line cannot be told apart.
			if (n > 0 && funcs[idxs[n-1]].Line == start) || (n+1 < len(idxs) && funcs[idxs[n+1]].Line == start) {
				continue
			}
			at, _ := slices.BinarySearchFunc(blocks, start, func(b cover.ProfileBlock, line int) int {
				return cmp.Compare(b.StartLine, line)
			})
			if at == len(blocks) || (n+1 < len(idxs) && blocks[at].StartLine >= funcs[idxs[n+1]].Line) {
				continue // no block of its own
			}
			calls := blocks[at].Count
			funcs[i].Calls = &calls
		}
	}
}
