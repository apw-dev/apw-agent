package workspace

import "testing"

func TestExclusiveLockAndRelease(t *testing.T) {
	root := t.TempDir()
	first, err := Acquire(root, "write")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Acquire(root, "write"); err == nil {
		other.Close()
		t.Fatal("two writers acquired one workspace")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(root, "write")
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
