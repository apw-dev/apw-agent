package devfixture

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the frontend fixtures from the Go types")

// The frontend unit tests read these fixtures instead of hand-written JSON, so a renamed or
// removed Go field fails here (and then in the frontend tests) instead of silently drifting.
// Regenerate with: go test ./internal/desktop/devfixture -update
func TestFrontendFixturesMatchGoTypes(t *testing.T) {
	dir := filepath.Join("..", "web", "src", "fixtures")
	for name, value := range FrontendFixtures() {
		want, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		path := filepath.Join(dir, name+".json")
		if *update {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("fixture %s missing (run with -update): %v", name, err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("fixture %s is stale; regenerate with: go test ./internal/desktop/devfixture -update", name)
		}
	}
}

func TestFixtureReportsUseRealFieldNames(t *testing.T) {
	raw, err := json.Marshal(FailedJob())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"reflex_attempts"`, `"raw_output"`, `"failed_stage"`, `"acceptance"`, `"criterion"`, `"evidence"`, `"modified_files"`, `"diff"`} {
		if !bytes.Contains(raw, []byte(field)) {
			t.Errorf("failed-job fixture lacks %s: %s", field, raw)
		}
	}
}
