package clock

import (
	"fmt"
	"time"
)

// Format renders d as HH:MM:SS.
func Format(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign = "-"
		d = -d
	}
	total := int64(d / time.Second)
	return fmt.Sprintf("%s%02d:%02d:%02d", sign, total/3600, total/60%60, total%60)
}
