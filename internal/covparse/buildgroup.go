package covparse

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/cover"
)

// BuildGroup represents a set of coverage directories that share the same
// covmeta hash set (i.e. they were produced by the same build).
type BuildGroup struct {
	Dirs            []string
	NewestTimestamp time.Time // when the group's newest covcounters file was written
}

// Profiles merges the group's coverage directories and returns their profiles.
func (g BuildGroup) Profiles() ([]*cover.Profile, error) {
	return mergeAndParse(g.Dirs)
}

// FindBuildGroups walks dir recursively, groups coverage directories by
// covmeta hash, and returns BuildGroups sorted by newest covcounters timestamp
// ascending (last element = newest build). Groups with equal timestamps are
// ordered by directory so the choice of newest build is deterministic.
func FindBuildGroups(dir string) ([]BuildGroup, error) {
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

	slices.SortFunc(groups, func(a, b BuildGroup) int {
		return cmp.Or(
			a.NewestTimestamp.Compare(b.NewestTimestamp),
			slices.Compare(a.Dirs, b.Dirs),
		)
	})

	return groups, nil
}

// newestCounterTime returns when the most recent covcounters.* file across
// the given directories was written.
func newestCounterTime(dirs []string) (time.Time, error) {
	_, last, err := CounterTimes(dirs)
	return last, err
}

// CounterTimes returns when the earliest and the latest covcounters.* file
// across the given directories was written. Both are zero if there are no
// counter files.
//
// The runtime names counter files covcounters.<hash>.<pid>.<unixnano>, so the
// write time is read from the name; copying the data around (cp, aws s3 sync)
// changes ModTime but not the name. ModTime is the fallback for names that do
// not follow that pattern.
func CounterTimes(dirs []string) (first, last time.Time, err error) {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("covparse: read dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "covcounters.") {
				continue
			}
			t, ok := counterFileTime(e.Name())
			if !ok {
				info, err := e.Info()
				if err != nil {
					return time.Time{}, time.Time{}, fmt.Errorf("covparse: stat %s/%s: %w", dir, e.Name(), err)
				}
				t = info.ModTime()
			}
			if first.IsZero() || t.Before(first) {
				first = t
			}
			if t.After(last) {
				last = t
			}
		}
	}
	return first, last, nil
}

// counterFileTime extracts the write time from a counter file name of the
// form covcounters.<hash>.<pid>.<unixnano>.
func counterFileTime(name string) (time.Time, bool) {
	parts := strings.Split(name, ".")
	if len(parts) != 4 || parts[0] != "covcounters" {
		return time.Time{}, false
	}
	ns, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || ns <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ns), true
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

	return slices.Sorted(maps.Keys(seen)), nil
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
		slices.Sort(hashes)
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
