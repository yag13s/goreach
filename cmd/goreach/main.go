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

// command is a goreach subcommand.
type command struct {
	name    string
	summary string
	run     func(args []string) error
}

var commands = []command{
	{"analyze", "Analyze coverage data and output JSON report", runAnalyze},
	{"merge", "Merge multiple report.json files (max coverage per function)", runMerge},
	{"summary", "Print coverage summary as text", runSummary},
	{"view", "Open report.json in browser UI", runView},
	{"version", "Print version information", runVersion},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	name := os.Args[1]
	switch name {
	case "-h", "-help", "--help", "help":
		usage()
		return
	}

	for _, cmd := range commands {
		if cmd.name != name {
			continue
		}
		if err := cmd.run(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "goreach %s: %v\n", name, err)
			os.Exit(1)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "goreach: unknown command %q\n", name)
	usage()
	os.Exit(1)
}

func runVersion([]string) error {
	fmt.Printf("goreach %s\n", version)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: goreach <command> [flags]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	for _, cmd := range commands {
		fmt.Fprintf(os.Stderr, "  %-9s %s\n", cmd.name, cmd.summary)
	}
}
