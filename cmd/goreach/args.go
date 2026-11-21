package main

import (
	"flag"
	"fmt"
	"strings"
)

// positionalArgs returns fs's positional arguments. The flag package stops
// parsing at the first positional argument, so `view report.json -src .`
// would silently drop -src; such misplaced flags are reported instead.
func positionalArgs(fs *flag.FlagSet) ([]string, error) {
	for _, a := range fs.Args() {
		if strings.HasPrefix(a, "-") && a != "-" {
			return nil, fmt.Errorf("flag %s must come before positional arguments", a)
		}
	}
	return fs.Args(), nil
}

// noPositionalArgs rejects any positional argument.
func noPositionalArgs(fs *flag.FlagSet) error {
	args, err := positionalArgs(fs)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument %q", args[0])
	}
	return nil
}

// reportPathArg returns the report path given either by the -report flag
// (flagValue) or as the single positional argument. It returns "" when
// neither was given.
func reportPathArg(fs *flag.FlagSet, flagValue string) (string, error) {
	args, err := positionalArgs(fs)
	if err != nil {
		return "", err
	}
	switch {
	case len(args) > 1:
		return "", fmt.Errorf("expected one report path, got %d", len(args))
	case len(args) == 1 && flagValue != "":
		return "", fmt.Errorf("give the report path either with -report or as an argument, not both")
	case len(args) == 1:
		return args[0], nil
	}
	return flagValue, nil
}
