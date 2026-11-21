package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/yag13s/goreach/internal/report"
)

// calledFunc is a function with a known call count and where it lives.
type calledFunc struct {
	file  string
	fn    report.FuncReport
	calls int
}

// funcsWithCalls returns the report's functions that carry a call count, in
// report order, limited to packages matching one of prefixes (all if empty).
func funcsWithCalls(rpt *report.Report, prefixes []string) []calledFunc {
	var funcs []calledFunc
	for _, pkg := range rpt.Packages {
		if len(prefixes) > 0 && !slices.ContainsFunc(prefixes, func(p string) bool {
			return strings.HasPrefix(pkg.ImportPath, p)
		}) {
			continue
		}
		for _, file := range pkg.Files {
			for _, fn := range file.Functions {
				if fn.Calls != nil {
					funcs = append(funcs, calledFunc{file: file.FileName, fn: fn, calls: *fn.Calls})
				}
			}
		}
	}
	return funcs
}

// printCalls prints the most called functions (if top > 0) and the functions
// never called (if cold) of a report.
func printCalls(w io.Writer, rpt *report.Report, prefixes []string, top int, cold bool) error {
	funcs := funcsWithCalls(rpt, prefixes)
	if len(funcs) == 0 {
		if len(prefixes) > 0 && len(funcsWithCalls(rpt, nil)) > 0 {
			return fmt.Errorf("no functions in packages matching -pkg %s", strings.Join(prefixes, ","))
		}
		return fmt.Errorf("the report has no call counts: they need coverage data from a " +
			"-covermode=atomic (or count) build and a report written by goreach v0.13.0 or later")
	}

	var b strings.Builder
	if top > 0 {
		called := slices.DeleteFunc(slices.Clone(funcs), func(f calledFunc) bool { return f.calls == 0 })
		slices.SortStableFunc(called, func(a, b calledFunc) int { return cmp.Compare(b.calls, a.calls) })
		shown := called[:min(top, len(called))]

		heading(&b, fmt.Sprintf("Most Called Functions (top %d of %d called)", len(shown), len(called)))
		width := 5
		if len(shown) > 0 {
			width = max(width, len(groupDigits(shown[0].calls)))
		}
		fmt.Fprintf(&b, "  %*s  function\n", width, "calls")
		for _, f := range shown {
			fmt.Fprintf(&b, "  %*s  %s:%d  %s\n", width, groupDigits(f.calls), f.file, f.fn.Line, f.fn.Name)
		}
	}

	if cold {
		if top > 0 {
			b.WriteString("\n")
		}
		var never []calledFunc
		stmts := 0
		for _, f := range funcs {
			if f.calls == 0 {
				never = append(never, f)
				stmts += f.fn.TotalStatements
			}
		}
		heading(&b, fmt.Sprintf("Never Called Functions (%d of %d, %s statements)", len(never), len(funcs), groupDigits(stmts)))
		for _, f := range never {
			fmt.Fprintf(&b, "  %s:%d  %s  (%d stmts)\n", f.file, f.fn.Line, f.fn.Name, f.fn.TotalStatements)
		}
		if len(never) == 0 {
			b.WriteString("  (none)\n")
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// heading writes title underlined, followed by a blank line.
func heading(b *strings.Builder, title string) {
	fmt.Fprintf(b, "%s\n%s\n\n", title, strings.Repeat("=", len(title)))
}

// groupDigits formats n with thousands separators: 1234567 → "1,234,567".
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
