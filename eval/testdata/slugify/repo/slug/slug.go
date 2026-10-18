package slug

import "strings"

// Make turns a title into a URL slug.
func Make(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "-"))
}
