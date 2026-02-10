package cli

import "strings"

// fuzzyFilterMatch reports whether needle matches haystack as an ordered subsequence.
// This mirrors gion's fuzzy filter semantics.
func fuzzyFilterMatch(haystack, needle string) bool {
	q := strings.ToLower(strings.TrimSpace(needle))
	if q == "" {
		return true
	}
	q = strings.Join(strings.Fields(q), "")
	if q == "" {
		return true
	}

	h := strings.ToLower(haystack)
	if h == "" {
		return false
	}

	j := 0
	for i := 0; i < len(h) && j < len(q); i++ {
		if h[i] == q[j] {
			j++
		}
	}
	return j == len(q)
}
