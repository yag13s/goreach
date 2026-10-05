package main

import (
	"flag"
	"fmt"

	"github.com/yag13s/goreach/internal/report"
)

// filterFlags holds the flags that choose which functions a report lists.
// They never affect the report's totals.
type filterFlags struct {
	threshold *float64
	minStmts  *int
}

func addFilterFlags(fs *flag.FlagSet) filterFlags {
	return filterFlags{
		threshold: fs.Float64("threshold", 100, "list functions with coverage at or below this percentage"),
		minStmts:  fs.Int("min-statements", 0, "list functions with at least N unreached statements"),
	}
}

// filter validates the flag values and returns the filter they describe.
// Call it after fs.Parse.
func (f filterFlags) filter() (report.FuncFilter, error) {
	if *f.threshold < 0 || *f.threshold > 100 {
		return report.FuncFilter{}, fmt.Errorf("-threshold must be between 0 and 100")
	}
	if *f.minStmts < 0 {
		return report.FuncFilter{}, fmt.Errorf("-min-statements must be non-negative")
	}
	return report.FuncFilter{MaxCoverage: *f.threshold, MinUnreached: *f.minStmts}, nil
}
