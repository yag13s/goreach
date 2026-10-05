// Package covparse converts GOCOVERDIR binary coverage data into parsed
// coverage profiles by driving `go tool covdata`.
package covparse

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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

// mergeAndParse merges a set of coverage directories and returns their profiles.
// If only one directory is provided, it parses directly without merging.
func mergeAndParse(dirs []string) ([]*cover.Profile, error) {
	if len(dirs) == 1 {
		return ParseDir(dirs[0])
	}

	mergeDir, err := os.MkdirTemp("", "goreach-merge-*")
	if err != nil {
		return nil, fmt.Errorf("covparse: create merge dir: %w", err)
	}
	defer os.RemoveAll(mergeDir)

	joined := strings.Join(dirs, ",")
	cmd := exec.Command("go", "tool", "covdata", "merge", "-i="+joined, "-o="+mergeDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("covparse: go tool covdata merge: %w\n%s", err, out)
	}

	return ParseDir(mergeDir)
}

// groupByMetaHash groups coverage directories by their covmeta hash set.
// Each directory's identity is the sorted set of covmeta.<hash> filenames it
// contains. Directories sharing the same hash set belong to the same build.
func groupByMetaHash(dirs []string) (map[string][]string, error) {
	groups := make(map[string][]string)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("covparse: read dir %s: %w", dir, err)
		}
		var hashes []string
		for _, e := range entries {
			if hash, ok := strings.CutPrefix(e.Name(), "covmeta."); ok {
				hashes = append(hashes, hash)
			}
		}
		sort.Strings(hashes)
		key := strings.Join(hashes, ",")
		groups[key] = append(groups[key], dir)
	}
	return groups, nil
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
	sort.Strings(dirs)
	return dirs, nil
}
