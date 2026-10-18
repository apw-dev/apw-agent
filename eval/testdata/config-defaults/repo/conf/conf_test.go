package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(path, []byte(`{"host":"example.org","port":9000}`), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.Host != "example.org" || c.Port != 9000 {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}
