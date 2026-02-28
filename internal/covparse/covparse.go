// Package covparse converts GOCOVERDIR binary coverage data into text profiles.
package covparse

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ParseDir converts a single GOCOVERDIR directory to a text coverage profile.
// It invokes `go tool covdata textfmt` under the hood.
func ParseDir(dir string) (string, error) {
	tmpFile, err := os.CreateTemp("", "goreach-profile-*.txt")
	if err != nil {
		return "", fmt.Errorf("covparse: create temp file: %w", err)
	}
	_ = tmpFile.Close()

	cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+dir, "-o="+tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("covparse: go tool covdata textfmt: %w\n%s", err, out)
	}

	data, err := os.ReadFile(tmpFile.Name())
	_ = os.Remove(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("covparse: read profile: %w", err)
	}
	return string(data), nil
}

// ParseDirRecursive walks dir recursively to find directories containing
// coverage data files (covmeta.* / covcounters.*), groups them by build
// (covmeta hash), merges each group separately, and returns one text
// coverage profile per build group. This prevents cross-build contamination
// when source code changes between builds.
func ParseDirRecursive(dir string) ([]string, error) {
	covDirs, err := findCoverageDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(covDirs) == 0 {
		return nil, fmt.Errorf("covparse: no coverage data found under %s", dir)
	}

	groups, err := groupByMetaHash(covDirs)
	if err != nil {
		return nil, err
	}

	// Sort group keys for deterministic output
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var profiles []string
	for _, k := range keys {
		text, err := mergeAndParse(groups[k])
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, text)
	}
	return profiles, nil
}

// mergeAndParse merges a set of coverage directories and returns the text profile.
// If only one directory is provided, it parses directly without merging.
func mergeAndParse(dirs []string) (string, error) {
	if len(dirs) == 1 {
		return ParseDir(dirs[0])
	}

	mergeDir, err := os.MkdirTemp("", "goreach-merge-*")
	if err != nil {
		return "", fmt.Errorf("covparse: create merge dir: %w", err)
	}
	defer os.RemoveAll(mergeDir)

	joined := strings.Join(dirs, ",")
	cmd := exec.Command("go", "tool", "covdata", "merge", "-i="+joined, "-o="+mergeDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("covparse: go tool covdata merge: %w\n%s", err, out)
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
