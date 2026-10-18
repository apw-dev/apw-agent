package mathx

// Clamp limits v to [lo, hi], swapping the bounds when lo > hi.
func Clamp(v, lo, hi int) int {
	if lo > hi {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
