package calc

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHiddenRenamed(t *testing.T) {
	if Sum(4, 5) != 9 || Report(2, 2) != "2 + 2 = 4" {
		t.Fatal("Sum or Report wrong")
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_hidden_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if file.Scope.Lookup("Calc") != nil || strings.Contains(string(src), "Calc(") {
			t.Fatalf("%s still uses the old name Calc", f)
		}
	}
}
