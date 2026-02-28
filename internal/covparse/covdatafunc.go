package covparse

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BuildGroup represents a set of coverage directories that share the same
// covmeta hash set (i.e. they were produced by the same build).
type BuildGroup struct {
	Dirs            []string
	NewestTimestamp time.Time // newest covcounters file ModTime in the group
}

// ParseProfile merges the group's coverage directories and returns a text profile.
func (g BuildGroup) ParseProfile() (string, error) {
	return mergeAndParse(g.Dirs)
}

// ParseDirRecursiveGrouped walks dir recursively, groups coverage directories
// by covmeta hash, and returns BuildGroups sorted by newest covcounters
// timestamp ascending (last element = newest build).
func ParseDirRecursiveGrouped(dir string) ([]BuildGroup, error) {
	covDirs, err := findCoverageDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(covDirs) == 0 {
		return nil, fmt.Errorf("covparse: no coverage data found under %s", dir)
	}

	hashGroups, err := groupByMetaHash(covDirs)
	if err != nil {
		return nil, err
	}

	groups := make([]BuildGroup, 0, len(hashGroups))
	for _, dirs := range hashGroups {
		ts, tsErr := newestCounterTime(dirs)
		if tsErr != nil {
			return nil, tsErr
		}
		groups = append(groups, BuildGroup{Dirs: dirs, NewestTimestamp: ts})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].NewestTimestamp.Before(groups[j].NewestTimestamp)
	})

	return groups, nil
}

// newestCounterTime returns the most recent ModTime of covcounters.* files
// across the given directories.
func newestCounterTime(dirs []string) (time.Time, error) {
	var newest time.Time
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return time.Time{}, fmt.Errorf("covparse: read dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "covcounters.") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return time.Time{}, fmt.Errorf("covparse: stat %s/%s: %w", dir, e.Name(), err)
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
	}
	return newest, nil
}

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
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		// Format: "github.com/user/pkg/file.go:42:\t\tFuncName\t\t75.0%"
		// Split on tab to get fields
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		fileName := line[:colonIdx]

		// Find the function name and percentage
		// After "file:line:", the rest is tab-separated: funcname and pct%
		rest := line[colonIdx+1:]
		// Skip the line number part (next colon)
		colonIdx2 := strings.Index(rest, ":")
		if colonIdx2 < 0 {
			continue
		}
		rest = rest[colonIdx2+1:]

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
	dotIdx := strings.LastIndex(name, ".")
	if dotIdx < 0 {
		// plain function, no receiver
		return name
	}

	typePart := name[:dotIdx]
	method := name[dotIdx+1:]

	// If typePart is empty (shouldn't happen), return as-is
	if typePart == "" {
		return name
	}

	// Check for pointer receiver
	if strings.HasPrefix(typePart, "*") {
		return "(" + typePart + ")." + method
	}

	// Value receiver — but only if typePart looks like a type name (starts uppercase)
	// Plain package-level functions don't have a dot in covdata func output,
	// but methods always have Type.Method format.
	return "(" + typePart + ")." + method
}
