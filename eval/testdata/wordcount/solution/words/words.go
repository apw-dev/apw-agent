package words

import "strings"

// Count returns how often each word occurs in s.
func Count(s string) map[string]int {
	counts := map[string]int{}
	for _, w := range strings.Fields(s) {
		w = strings.ToLower(strings.Trim(w, ".,!?;:"))
		if w != "" {
			counts[w]++
		}
	}
	return counts
}
