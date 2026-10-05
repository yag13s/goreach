package covparse

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/cover"
)

// BuildGroup represents a set of coverage directories that share the same
// covmeta hash set (i.e. they were produced by the same build).
type BuildGroup struct {
	Dirs            []string
	NewestTimestamp time.Time // newest covcounters file ModTime in the group
}

// Profiles merges the group's coverage directories and returns their profiles.
func (g BuildGroup) Profiles() ([]*cover.Profile, error) {
	return mergeAndParse(g.Dirs)
}

// ParseDirRecursiveGrouped walks dir recursively, groups coverage directories
// by covmeta hash, and returns BuildGroups sorted by newest covcounters
// timestamp ascending (last element = newest build).
func ParseDirRecursiveGrouped(dir string) ([]BuildGroup, error) {
	covDirs, err := findCoverageDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(covDirs) == 0 {
		return nil, fmt.Errorf("covparse: no coverage data found under %s", dir)
	}

	hashGroups, err := groupByMetaHash(covDirs)
	if err != nil {
		return nil, err
	}

	groups := make([]BuildGroup, 0, len(hashGroups))
	for _, dirs := range hashGroups {
		ts, tsErr := newestCounterTime(dirs)
		if tsErr != nil {
			return nil, tsErr
		}
		groups = append(groups, BuildGroup{Dirs: dirs, NewestTimestamp: ts})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].NewestTimestamp.Before(groups[j].NewestTimestamp)
	})

	return groups, nil
}

// newestCounterTime returns the most recent ModTime of covcounters.* files
// across the given directories.
func newestCounterTime(dirs []string) (time.Time, error) {
	var newest time.Time
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return time.Time{}, fmt.Errorf("covparse: read dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "covcounters.") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return time.Time{}, fmt.Errorf("covparse: stat %s/%s: %w", dir, e.Name(), err)
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
	}
	return newest, nil
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
			seen[filepath.Dir(path)] = true
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
