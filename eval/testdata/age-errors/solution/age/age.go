package age

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidAge reports input that is not an age from 0 to 150.
var ErrInvalidAge = errors.New("invalid age")

// Parse reads an age in years.
func Parse(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > 150 {
		return 0, fmt.Errorf("%w %q", ErrInvalidAge, s)
	}
	return n, nil
}
