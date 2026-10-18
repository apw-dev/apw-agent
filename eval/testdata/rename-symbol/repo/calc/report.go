package calc

import "fmt"

// Report formats the sum of a and b.
func Report(a, b int) string {
	return fmt.Sprintf("%d + %d = %d", a, b, Calc(a, b))
}
