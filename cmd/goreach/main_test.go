package main

import (
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("default version = %q, want %q", version, "dev")
	}
}

func TestSortStrings(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "already sorted",
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "reverse order",
			input: []string{"c", "b", "a"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single element",
			input: []string{"x"},
			want:  []string{"x"},
		},
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "nil slice",
			input: nil,
			want:  nil,
		},
		{
			name:  "duplicates",
			input: []string{"b", "a", "b", "a"},
			want:  []string{"a", "a", "b", "b"},
		},
		{
			name:  "package paths",
			input: []string{"myapp/internal/db", "myapp/cmd", "myapp/internal/auth"},
			want:  []string{"myapp/cmd", "myapp/internal/auth", "myapp/internal/db"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			var input []string
			if tt.input != nil {
				input = make([]string, len(tt.input))
				copy(input, tt.input)
			}

			sortStrings(input)

			if len(input) != len(tt.want) {
				t.Fatalf("sortStrings() len = %d, want %d", len(input), len(tt.want))
			}
			for i := range input {
				if input[i] != tt.want[i] {
					t.Errorf("sortStrings()[%d] = %q, want %q", i, input[i], tt.want[i])
				}
			}
		})
	}
}
