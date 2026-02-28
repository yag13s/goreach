package covparse

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseProfileFile(t *testing.T) {
	// Create a temp profile
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "coverage.txt")
	content := "mode: set\nexample.com/pkg/foo.go:1.1,5.1 2 1\n"
	if err := os.WriteFile(profilePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseProfileFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if result != content {
		t.Errorf("got %q, want %q", result, content)
	}
}

func TestParseProfileFile_NotFound(t *testing.T) {
	_, err := ParseProfileFile("/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

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

// TestParseDir_NoCoverageData tests ParseDir with a directory that has
// files pretending to be coverage data but with invalid content.
// go tool covdata should fail to parse them.
func TestParseDir_NoCoverageData(t *testing.T) {
	dir := t.TempDir()
	// Create files that look like coverage data by name but have invalid content.
	// covmeta files have a specific binary format; random bytes should cause a parse error.
	if err := os.WriteFile(filepath.Join(dir, "covmeta.abc123"), []byte("invalid-coverage-metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "covcounters.abc123"), []byte("invalid-counters"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseDir(dir)
	if err == nil {
		t.Fatal("expected error for directory with invalid coverage data")
	}
	if !strings.Contains(err.Error(), "covparse") {
		t.Errorf("error should mention covparse, got: %v", err)
	}
}

// TestParseDir_NonexistentDir tests ParseDir with a directory that does not exist.
func TestParseDir_NonexistentDir(t *testing.T) {
	_, err := ParseDir("/nonexistent/coverage/dir")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// TestParseDirRecursive_EmptyDir tests ParseDirRecursive with an empty directory
// that has no coverage data files anywhere.
func TestParseDirRecursive_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	_, err := ParseDirRecursive(dir)
	if err == nil {
		t.Fatal("expected error for empty directory with no coverage data")
	}
	if !strings.Contains(err.Error(), "no coverage data found") {
		t.Errorf("error should mention 'no coverage data found', got: %v", err)
	}
}

// TestParseDirRecursive_NonexistentDir tests ParseDirRecursive with a
// directory that does not exist.
func TestParseDirRecursive_NonexistentDir(t *testing.T) {
	_, err := ParseDirRecursive("/nonexistent/dir/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// TestParseDirRecursive_NoCoverageFiles tests ParseDirRecursive with a
// directory tree that has files but none are coverage data.
func TestParseDirRecursive_NoCoverageFiles(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "readme.txt"), []byte("not coverage"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseDirRecursive(dir)
	if err == nil {
		t.Fatal("expected error when no coverage files found")
	}
	if !strings.Contains(err.Error(), "no coverage data found") {
		t.Errorf("error should mention 'no coverage data found', got: %v", err)
	}
}
