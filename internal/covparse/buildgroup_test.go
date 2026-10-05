package covparse

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestFindCoverageDirs(t *testing.T) {
	root := t.TempDir()

	// Create a directory with coverage files
	covDir := filepath.Join(root, "pod-abc")
	if err := os.MkdirAll(covDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(covDir, "covmeta.xyz"), []byte("meta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(covDir, "covcounters.xyz"), []byte("counters"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create another directory without coverage files
	otherDir := filepath.Join(root, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "readme.txt"), []byte("not coverage"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := findCoverageDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 coverage dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != covDir {
		t.Errorf("expected %s, got %s", covDir, dirs[0])
	}
}

func TestFindCoverageDirs_Empty(t *testing.T) {
	root := t.TempDir()
	dirs, err := findCoverageDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestFindCoverageDirs_Nested(t *testing.T) {
	root := t.TempDir()

	// Create nested directory structure
	dir1 := filepath.Join(root, "service-a", "pod-1")
	dir2 := filepath.Join(root, "service-a", "pod-2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "covmeta.abc"), []byte("m"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "covcounters.abc"), []byte("c"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := findCoverageDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Errorf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestGroupByMetaHash(t *testing.T) {
	root := t.TempDir()

	// Build A: two pods with the same covmeta hash
	podA1 := filepath.Join(root, "build-a", "pod-1")
	podA2 := filepath.Join(root, "build-a", "pod-2")
	// Build B: one pod with a different covmeta hash
	podB1 := filepath.Join(root, "build-b", "pod-1")

	for _, d := range []string{podA1, podA2, podB1} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Same hash for build A pods
	for _, d := range []string{podA1, podA2} {
		if err := os.WriteFile(filepath.Join(d, "covmeta.aaaa1111"), []byte("m"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Different hash for build B
	if err := os.WriteFile(filepath.Join(podB1, "covmeta.bbbb2222"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs := []string{podA1, podA2, podB1}
	groups, err := groupByMetaHash(dirs)
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}

	// Check build A group
	groupA, ok := groups["aaaa1111"]
	if !ok {
		t.Fatal("expected group with key 'aaaa1111'")
	}
	sort.Strings(groupA)
	if len(groupA) != 2 {
		t.Fatalf("expected 2 dirs in group A, got %d", len(groupA))
	}

	// Check build B group
	groupB, ok := groups["bbbb2222"]
	if !ok {
		t.Fatal("expected group with key 'bbbb2222'")
	}
	if len(groupB) != 1 {
		t.Fatalf("expected 1 dir in group B, got %d", len(groupB))
	}
	if groupB[0] != podB1 {
		t.Errorf("expected %s, got %s", podB1, groupB[0])
	}
}

func TestGroupByMetaHash_MultipleHashes(t *testing.T) {
	root := t.TempDir()

	// A directory with two covmeta hashes (e.g. multi-package binary)
	dir1 := filepath.Join(root, "pod-1")
	dir2 := filepath.Join(root, "pod-2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		// Both dirs have the same two hashes
		for _, h := range []string{"covmeta.hash1", "covmeta.hash2"} {
			if err := os.WriteFile(filepath.Join(d, h), []byte("m"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	groups, err := groupByMetaHash([]string{dir1, dir2})
	if err != nil {
		t.Fatal(err)
	}

	// Both dirs share the same hash set, so one group
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %v", len(groups), groups)
	}

	// The key should be the sorted join of hashes
	groupDirs, ok := groups["hash1,hash2"]
	if !ok {
		t.Fatal("expected group with key 'hash1,hash2'")
	}
	if len(groupDirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(groupDirs))
	}
}

func TestNewestCounterTime(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "d1")
	dir2 := filepath.Join(root, "d2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Write covcounters files with different timestamps
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	f1 := filepath.Join(dir1, "covcounters.abc")
	if err := os.WriteFile(f1, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f1, older, older); err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(dir2, "covcounters.def")
	if err := os.WriteFile(f2, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f2, newer, newer); err != nil {
		t.Fatal(err)
	}

	ts, err := newestCounterTime([]string{dir1, dir2})
	if err != nil {
		t.Fatal(err)
	}

	// Should match the newer file's time (within 1 second tolerance)
	diff := ts.Sub(newer)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("newest time %v differs from expected %v by %v", ts, newer, diff)
	}
}

func TestNewestCounterTime_NoCovCounters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "covmeta.abc"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := newestCounterTime([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time for dir with no covcounters, got %v", ts)
	}
}

func TestFindBuildGroups_Ordering(t *testing.T) {
	root := t.TempDir()

	// Build A (older) and Build B (newer) with different covmeta hashes
	dirA := filepath.Join(root, "build-a")
	dirB := filepath.Join(root, "build-b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Build A: older timestamp
	if err := os.WriteFile(filepath.Join(dirA, "covmeta.aaa"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	counterA := filepath.Join(dirA, "covcounters.aaa")
	if err := os.WriteFile(counterA, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	olderTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(counterA, olderTime, olderTime); err != nil {
		t.Fatal(err)
	}

	// Build B: newer timestamp
	if err := os.WriteFile(filepath.Join(dirB, "covmeta.bbb"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	counterB := filepath.Join(dirB, "covcounters.bbb")
	if err := os.WriteFile(counterB, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	newerTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(counterB, newerTime, newerTime); err != nil {
		t.Fatal(err)
	}

	groups, err := FindBuildGroups(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group should be older (build A), last should be newer (build B)
	if !groups[0].NewestTimestamp.Before(groups[1].NewestTimestamp) {
		t.Errorf("groups not sorted by timestamp: [0]=%v [1]=%v",
			groups[0].NewestTimestamp, groups[1].NewestTimestamp)
	}

	// Verify build A is first (has dirA)
	if groups[0].Dirs[0] != dirA {
		t.Errorf("expected first group to contain %s, got %v", dirA, groups[0].Dirs)
	}
	// Verify build B is last (has dirB)
	if groups[1].Dirs[0] != dirB {
		t.Errorf("expected second group to contain %s, got %v", dirB, groups[1].Dirs)
	}
}
