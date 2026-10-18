package clock

import "time"

// Format renders d for display.
func Format(d time.Duration) string {
	return d.String()
}
