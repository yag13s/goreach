// Package covparse converts GOCOVERDIR binary coverage data into text profiles.
package covparse

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ParseDir converts a single GOCOVERDIR directory to a text coverage profile.
// It invokes `go tool covdata textfmt` under the hood.
func ParseDir(dir string) (string, error) {
	tmpFile, err := os.CreateTemp("", "goreach-profile-*.txt")
	if err != nil {
		return "", fmt.Errorf("covparse: create temp file: %w", err)
	}
	tmpFile.Close()

	cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+dir, "-o="+tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("covparse: go tool covdata textfmt: %w\n%s", err, out)
	}

	data, err := os.ReadFile(tmpFile.Name())
	os.Remove(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("covparse: read profile: %w", err)
	}
	return string(data), nil
}

// ParseDirRecursive walks dir recursively to find directories containing
// coverage data files (covmeta.* / covcounters.*), merges them, and returns
// a single text coverage profile.
func ParseDirRecursive(dir string) (string, error) {
	covDirs, err := findCoverageDirs(dir)
	if err != nil {
		return "", err
	}
	if len(covDirs) == 0 {
		return "", fmt.Errorf("covparse: no coverage data found under %s", dir)
	}

	// If only one dir, parse directly
	if len(covDirs) == 1 {
		return ParseDir(covDirs[0])
	}

	// Merge multiple directories first
	mergeDir, err := os.MkdirTemp("", "goreach-merge-*")
	if err != nil {
		return "", fmt.Errorf("covparse: create merge dir: %w", err)
	}
	defer os.RemoveAll(mergeDir)

	dirs := strings.Join(covDirs, ",")
	cmd := exec.Command("go", "tool", "covdata", "merge", "-i="+dirs, "-o="+mergeDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("covparse: go tool covdata merge: %w\n%s", err, out)
	}

	return ParseDir(mergeDir)
}

// ParseProfileFile reads and returns the contents of a text coverage profile file.
func ParseProfileFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("covparse: read profile: %w", err)
	}
	return string(data), nil
}

// findCoverageDirs walks root and returns directories that contain coverage data files.
func findCoverageDirs(root string) ([]string, error) {
	seen := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "covmeta.") || strings.HasPrefix(name, "covcounters.") {
			dir := filepath.Dir(path)
			if !seen[dir] {
				seen[dir] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("covparse: walk %s: %w", root, err)
	}

	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	return dirs, nil
}
