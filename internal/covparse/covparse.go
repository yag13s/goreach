// Package covparse converts GOCOVERDIR binary coverage data into parsed
// coverage profiles by driving `go tool covdata`.
package covparse

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/tools/cover"
)

// ParseDir converts a single GOCOVERDIR directory into coverage profiles.
// It invokes `go tool covdata textfmt` under the hood.
func ParseDir(dir string) ([]*cover.Profile, error) {
	tmpFile, err := os.CreateTemp("", "goreach-profile-*.txt")
	if err != nil {
		return nil, fmt.Errorf("covparse: create temp file: %w", err)
	}
	_ = tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+dir, "-o="+tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("covparse: go tool covdata textfmt: %w\n%s", err, out)
	}

	return ParseProfileFile(tmpFile.Name())
}

// ParseProfileFile parses a text coverage profile file (the format written by
// `go test -coverprofile` and `go tool covdata textfmt`).
func ParseProfileFile(path string) ([]*cover.Profile, error) {
	profiles, err := cover.ParseProfiles(path)
	if err != nil {
		return nil, fmt.Errorf("covparse: parse profile: %w", err)
	}
	return profiles, nil
}
