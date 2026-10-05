package covparse

import (
	"os"
	"path/filepath"
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

	profiles, err := ParseProfileFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("got %d profiles, want 1", len(profiles))
	}
	p := profiles[0]
	if p.FileName != "example.com/pkg/foo.go" || p.Mode != "set" {
		t.Errorf("got file %q mode %q, want example.com/pkg/foo.go / set", p.FileName, p.Mode)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].NumStmt != 2 || p.Blocks[0].Count != 1 {
		t.Errorf("unexpected blocks: %+v", p.Blocks)
	}
}

func TestParseProfileFile_Malformed(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "coverage.txt")
	if err := os.WriteFile(profilePath, []byte("not a coverage profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseProfileFile(profilePath)
	if err == nil {
		t.Fatal("expected error for malformed profile")
	}
	if !strings.Contains(err.Error(), "covparse") {
		t.Errorf("error should mention covparse, got: %v", err)
	}
}

func TestParseProfileFile_NotFound(t *testing.T) {
	_, err := ParseProfileFile("/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
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
