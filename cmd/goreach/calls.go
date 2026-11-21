package main

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

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
		if !matchesAny(pkg.ImportPath, prefixes) {
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

// callsView selects the sections printCalls prints.
type callsView struct {
	top   int  // most called functions, if > 0
	loops int  // calls inside loops that ran most per call of their caller, if > 0
	cold  bool // functions never called
}

// printCalls prints the sections of v for a report: its most called
// functions, calls made inside loops, and functions never called.
func printCalls(w io.Writer, rpt *report.Report, prefixes []string, v callsView) error {
	funcs := funcsWithCalls(rpt, prefixes)
	if len(funcs) == 0 {
		if len(prefixes) > 0 && len(funcsWithCalls(rpt, nil)) > 0 {
			return fmt.Errorf("no functions in packages matching -pkg %s", strings.Join(prefixes, ","))
		}
		return fmt.Errorf("the report has no call counts: they need coverage data from a " +
			"-covermode=atomic (or count) build and a report written by goreach v0.13.0 or later")
	}

	var b strings.Builder
	span := collectionSpan(rpt.Collection)
	if c := rpt.Collection; c != nil {
		if first, last := c.FirstWrite.UTC().Format("2006-01-02 15:04"), c.LastWrite.UTC().Format("2006-01-02 15:04"); first == last {
			fmt.Fprintf(&b, "Collected %s UTC", first)
		} else {
			fmt.Fprintf(&b, "Collected %s – %s UTC", first, last)
		}
		if span > 0 {
			fmt.Fprintf(&b, " (%s)", span.Round(time.Minute))
		}
		if c.Builds > 1 {
			fmt.Fprintf(&b, ", %d builds", c.Builds)
		}
		b.WriteString("\n\n")
	}

	sep := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
	}

	if v.top > 0 {
		called := slices.DeleteFunc(slices.Clone(funcs), func(f calledFunc) bool { return f.calls == 0 })
		slices.SortStableFunc(called, func(a, b calledFunc) int { return cmp.Compare(b.calls, a.calls) })
		shown := called[:min(v.top, len(called))]

		heading(&b, fmt.Sprintf("Most Called Functions (top %d of %d called)", len(shown), len(called)))
		width := 5
		if len(shown) > 0 {
			width = max(width, len(groupDigits(shown[0].calls)))
		}
		if span > 0 {
			fmt.Fprintf(&b, "  %*s  %8s  function\n", width, "calls", "per hour")
		} else {
			fmt.Fprintf(&b, "  %*s  function\n", width, "calls")
		}
		for _, f := range shown {
			if span > 0 {
				fmt.Fprintf(&b, "  %*s  %8s  %s:%d  %s\n", width, groupDigits(f.calls), perHour(f.calls, span), f.file, f.fn.Line, f.fn.Name)
			} else {
				fmt.Fprintf(&b, "  %*s  %s:%d  %s\n", width, groupDigits(f.calls), f.file, f.fn.Line, f.fn.Name)
			}
		}
	}

	if v.loops > 0 {
		sep()
		writeLoopCalls(&b, loopCalls(rpt, prefixes), v.loops)
	}

	if v.cold {
		sep()
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

// collectionSpan returns how long the data was collected over, or 0 if that
// is unknown or under a minute (too short for a meaningful rate).
func collectionSpan(c *report.Collection) time.Duration {
	if c == nil {
		return 0
	}
	if d := c.LastWrite.Sub(c.FirstWrite); d >= time.Minute {
		return d
	}
	return 0
}

// perHour formats n events over span as a rate per hour.
func perHour(n int, span time.Duration) string {
	r := float64(n) / span.Hours()
	if r >= 10 {
		return groupDigits(int(math.Round(r)))
	}
	return strconv.FormatFloat(r, 'f', 1, 64)
}

// loopCall is a call made inside a loop and the function making it.
type loopCall struct {
	file   string
	caller string
	call   report.LoopCall
}

// loopCalls returns the report's calls made inside loops that repeat (run
// more than once per call of their caller), the most executed first, limited
// to packages matching one of prefixes (all if empty).
func loopCalls(rpt *report.Report, prefixes []string) []loopCall {
	var calls []loopCall
	for _, pkg := range rpt.Packages {
		if !matchesAny(pkg.ImportPath, prefixes) {
			continue
		}
		for _, file := range pkg.Files {
			for _, fn := range file.Functions {
				for _, c := range fn.LoopCalls {
					if c.PerCall() > 1 {
						calls = append(calls, loopCall{file: file.FileName, caller: fn.Name, call: c})
					}
				}
			}
		}
	}
	slices.SortStableFunc(calls, func(a, b loopCall) int {
		return cmp.Or(cmp.Compare(b.call.Executions, a.call.Executions), cmp.Compare(b.call.PerCall(), a.call.PerCall()))
	})
	return calls
}

// writeLoopCalls writes the first n of calls.
func writeLoopCalls(b *strings.Builder, calls []loopCall, n int) {
	shown := calls[:min(n, len(calls))]
	heading(b, fmt.Sprintf("Calls Inside Loops (top %d of %d repeating)", len(shown), len(calls)))
	if len(shown) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	width := max(len("executions"), len(groupDigits(shown[0].call.Executions)))
	fmt.Fprintf(b, "  %8s  %*s  call\n", "per call", width, "executions")
	for _, c := range shown {
		fmt.Fprintf(b, "  %8s  %*s  %s:%d  %s → %s", strconv.FormatFloat(c.call.PerCall(), 'f', 1, 64),
			width, groupDigits(c.call.Executions), c.file, c.call.Line, c.caller, c.call.Expr)
		if c.call.Callee != "" && !strings.HasSuffix(c.call.Callee, "."+c.call.Expr) && c.call.Callee != c.call.Expr {
			fmt.Fprintf(b, "  [%s]", c.call.Callee)
		}
		if c.call.TakesContext {
			b.WriteString("  (ctx)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n  per call: how many times the call ran each time its caller ran.\n" +
		"  (ctx): the call passes a context.Context, so it likely does I/O.\n" +
		"  A call that does I/O (a database read, an HTTP request) many times per call is a likely N+1.\n")
}

// matchesAny reports whether importPath has one of prefixes, or prefixes is
// empty.
func matchesAny(importPath string, prefixes []string) bool {
	return len(prefixes) == 0 || slices.ContainsFunc(prefixes, func(p string) bool {
		return strings.HasPrefix(importPath, p)
	})
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
