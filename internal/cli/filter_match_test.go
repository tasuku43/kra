package cli

import "testing"

func TestFuzzyFilterMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{
			name:     "empty needle matches",
			haystack: "example-org/helmfiles",
			needle:   "",
			want:     true,
		},
		{
			name:     "ordered subsequence matches",
			haystack: "example-org/helmfiles",
			needle:   "cs",
			want:     true,
		},
		{
			name:     "whitespace in needle is ignored",
			haystack: "example-org/helmfiles",
			needle:   "c s",
			want:     true,
		},
		{
			name:     "case insensitive",
			haystack: "tasuku43/GIONX",
			needle:   "gx",
			want:     true,
		},
		{
			name:     "order mismatch does not match",
			haystack: "example-org/helmfiles",
			needle:   "sc",
			want:     false,
		},
		{
			name:     "non-empty needle does not match empty haystack",
			haystack: "",
			needle:   "a",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyFilterMatch(tt.haystack, tt.needle)
			if got != tt.want {
				t.Fatalf("fuzzyFilterMatch(%q, %q) = %t, want %t", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}
