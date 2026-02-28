// Command goreach analyzes Go coverage data to identify unreached code paths.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

var version = "dev"

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("goreach %s\n", version)
	case "analyze":
		if err := runAnalyze(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach analyze: %v\n", err)
			os.Exit(1)
		}
	case "summary":
		if err := runSummary(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach summary: %v\n", err)
			os.Exit(1)
		}
	case "merge":
		if err := runMerge(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach merge: %v\n", err)
			os.Exit(1)
		}
	case "view":
		if err := runView(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach view: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "goreach: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: goreach <command> [flags]

Commands:
  analyze   Analyze coverage data and output JSON report
  merge     Merge multiple report.json files (max coverage per function)
  summary   Print coverage summary as text
  view      Open report.json in browser UI
  version   Print version information`)
}
