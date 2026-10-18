package slug

import "testing"

func TestMake(t *testing.T) {
	if got := Make("hello world"); got != "hello-world" {
		t.Fatalf("Make = %q", got)
	}
}
