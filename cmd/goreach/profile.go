package main

import (
	"fmt"
	"os"

	"golang.org/x/tools/cover"
)

// parseProfileText writes a text coverage profile to a temporary file and
// parses it using cover.ParseProfiles.
func parseProfileText(text string) ([]*cover.Profile, error) {
	tmpFile, err := os.CreateTemp("", "goreach-profile-*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(text); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	profiles, err := cover.ParseProfiles(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	return profiles, nil
}
