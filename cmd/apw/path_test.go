package main

import "testing"

func TestPathWithEntryAndWithout(t *testing.T) {
	orig := `C:\a;%USERPROFILE%\AppData\Roaming\npm;C:\b\`
	added, changed := pathWithEntry(orig, `C:\APW\bin`)
	if !changed || added != orig+`;C:\APW\bin` {
		t.Fatalf("add = %q %v", added, changed)
	}
	if again, changed := pathWithEntry(added, `c:\apw\bin\`); changed || again != added {
		t.Fatalf("second add changed PATH: %q", again)
	}
	removed, changed := pathWithoutEntry(added, `C:\APW\bin`)
	if !changed || removed != orig {
		t.Fatalf("remove = %q %v", removed, changed)
	}
	if empty, changed := pathWithEntry("", `C:\APW\bin`); !changed || empty != `C:\APW\bin` {
		t.Fatalf("empty add = %q", empty)
	}
}
