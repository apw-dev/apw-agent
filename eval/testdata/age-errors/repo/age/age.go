package age

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse reads an age in years.
func Parse(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid age %q", s)
	}
	return n, nil
}
