package age

import "testing"

func TestParse(t *testing.T) {
	if n, err := Parse(" 42 "); err != nil || n != 42 {
		t.Fatalf("Parse = %d, %v", n, err)
	}
	if _, err := Parse("x"); err == nil {
		t.Fatal("no error for x")
	}
}
