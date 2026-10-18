package mathx

import "testing"

func TestHiddenClamp(t *testing.T) {
	cases := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5}, {-5, 0, 10, 0}, {50, 0, 10, 10}, {0, 0, 0, 0}, {7, 10, 0, 7}, {-1, 10, 0, 0}, {99, 10, 0, 10},
	}
	for _, c := range cases {
		if got := Clamp(c.v, c.lo, c.hi); got != c.want {
			t.Fatalf("Clamp(%d, %d, %d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}
