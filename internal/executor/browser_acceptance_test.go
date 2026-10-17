package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/browser"
)

func TestBrowserAcceptanceCheck(t *testing.T) {
	root := t.TempDir()
	plan := &architect.ExecutionPlan{AcceptanceCriteria: []string{"shows count"}, Checks: []architect.AcceptanceCheck{
		{Criterion: "shows count", Kind: "browser", Value: "Count  is 10"},
	}}
	var visited []string
	visit := func(text string, errs []string, err error) pageVisitor {
		return func(path string) (*browser.Page, error) {
			visited = append(visited, path)
			if err != nil {
				return nil, err
			}
			return &browser.Page{Text: text, Errors: errs}, nil
		}
	}
	if results, ok := acceptanceSnapshot(root, plan, true, nil, visit("Vite + React Count is 10", nil, nil), false); !ok || !strings.Contains(results[0].Evidence, "Count is 10") || visited[0] != "/" {
		t.Fatalf("visible text not accepted: %+v %v", results, visited)
	}
	for name, v := range map[string]pageVisitor{
		"missing text":   visit("Count is 9", nil, nil),
		"uncaught error": visit("Count is 10", []string{"Uncaught TypeError"}, nil),
		"render failure": visit("", nil, errors.New("no DOM")),
		"no browser":     nil,
	} {
		if results, ok := acceptanceSnapshot(root, plan, true, nil, v, false); ok {
			t.Fatalf("%s accepted: %+v", name, results)
		}
	}
	calls := len(visited)
	if _, ok := acceptanceSnapshot(root, plan, false, nil, visit("Count is 10", nil, nil), false); ok || len(visited) != calls {
		t.Fatal("browser check ran or passed although verification failed")
	}
}
