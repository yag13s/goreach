package main

import (
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("default version = %q, want %q", version, "dev")
	}
}
