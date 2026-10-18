package conf

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, body string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestHiddenDefaultsAndValidation(t *testing.T) {
	if c, err := load(t, `{}`); err != nil || c.Host != "127.0.0.1" || c.Port != 8080 {
		t.Fatalf("defaults = %+v, %v", c, err)
	}
	if c, err := load(t, `{"host":"","port":0}`); err != nil || c.Host != "127.0.0.1" || c.Port != 8080 {
		t.Fatalf("zero values = %+v, %v", c, err)
	}
	for _, body := range []string{`{"port":70000}`, `{"port":-1}`, `{"port":65536}`} {
		if _, err := load(t, body); !errors.Is(err, ErrInvalidPort) {
			t.Fatalf("%s: err = %v, want ErrInvalidPort", body, err)
		}
	}
	if c, err := load(t, `{"port":65535}`); err != nil || c.Port != 65535 {
		t.Fatalf("max port = %+v, %v", c, err)
	}
}
