package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

const validPlanJSON = `{"objective":"change greeting","affected_files":["main.go"],"subtasks":[{"id":"one","action":"patch","target_file":"main.go","instruction":"return hello"}],"acceptance_criteria":["updated"],"checks":[{"criterion":"updated","kind":"contains","file":"main.go","value":"hello"}]}`

func planSuccess(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
	return &core.ModelResponse{Completed: true, Content: validPlanJSON}, nil
}

func architectExecutor(t *testing.T, primary, backup *routingProvider) *Executor {
	t.Helper()
	ex := New(t.TempDir(), primary, primary, "primary-model", "primary-model", 1)
	ex.ArchitectPool = []FailoverCandidate{
		{Provider: primary, Model: "primary-model", ProviderKey: "primary"},
		{Provider: backup, Model: "backup-model", ProviderKey: "backup"},
	}
	return ex
}

func TestPlanTaskFailsOverOnQuotaAndCoolsDownPrimary(t *testing.T) {
	primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return nil, errors.New("429 quota exceeded; resets in 2m")
	}}
	backup := &routingProvider{name: "codex-cli", ask: planSuccess}
	ex := architectExecutor(t, primary, backup)
	var fallbacks int
	ex.OnProgress = func(ev ProgressEvent) {
		if ev.Stage == "planning_fallback" {
			fallbacks++
		}
	}
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "greet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan.Objective != "change greeting" || backup.calls != 1 || primary.calls != 1 {
		t.Fatalf("plan=%+v primary=%d backup=%d", plan, primary.calls, backup.calls)
	}
	if fallbacks != 1 || len(ex.Failover.Groups) != 1 {
		t.Fatalf("expected one fallback event and a quota cooldown, got events=%d groups=%v", fallbacks, ex.Failover.Groups)
	}
	// A second plan skips the cooled-down primary entirely.
	if _, err := ex.PlanTask(context.Background(), core.Task{ID: "task-2", Instruction: "greet"}, ""); err != nil {
		t.Fatal(err)
	}
	if primary.calls != 1 || backup.calls != 2 {
		t.Fatalf("cooled-down primary was retried: primary=%d backup=%d", primary.calls, backup.calls)
	}
}

func TestPlanTaskStopsOnFatalErrorAndCancellation(t *testing.T) {
	fatal := errors.New("401 unauthorized")
	primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) { return nil, fatal }}
	backup := &routingProvider{name: "codex-cli", ask: planSuccess}
	if _, err := architectExecutor(t, primary, backup).PlanTask(context.Background(), core.Task{ID: "t"}, ""); !errors.Is(err, fatal) || backup.calls != 0 {
		t.Fatalf("fatal error fell through: err=%v backup=%d", err, backup.calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	primary = &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		cancel()
		return nil, context.DeadlineExceeded
	}}
	backup = &routingProvider{name: "codex-cli", ask: planSuccess}
	if _, err := architectExecutor(t, primary, backup).PlanTask(ctx, core.Task{ID: "t"}, ""); !errors.Is(err, context.Canceled) || backup.calls != 0 {
		t.Fatalf("parent cancellation did not stop failover: err=%v backup=%d", err, backup.calls)
	}
}

func TestReviewFailsOverOnIncompleteResponse(t *testing.T) {
	primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return &core.ModelResponse{Completed: false, Content: "partial"}, nil
	}}
	backup := &routingProvider{name: "codex-cli", ask: func(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
		if req.Model != "backup-model" {
			t.Errorf("backup asked with model %q", req.Model)
		}
		return &core.ModelResponse{Completed: true, Content: "reviewed"}, nil
	}}
	ex := architectExecutor(t, primary, backup)
	ex.Mode = core.ModeReview
	report, err := ex.Execute(context.Background(), "inspect", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary != "reviewed" || primary.calls != 1 || backup.calls != 1 {
		t.Fatalf("summary=%q primary=%d backup=%d", report.Summary, primary.calls, backup.calls)
	}
}

func TestSingleArchitectKeepsOriginalReviewError(t *testing.T) {
	only := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return &core.ModelResponse{Completed: true, Content: "  "}, nil
	}}
	ex := New(t.TempDir(), only, only, "m", "m", 1)
	ex.Mode = core.ModeReview
	_, err := ex.Execute(context.Background(), "inspect", "")
	if err == nil || err.Error() != "review response is incomplete" {
		t.Fatalf("single-provider error changed: %v", err)
	}
}

func TestAddArchitectFallbacksSkipsUnusableProviders(t *testing.T) {
	primary := &routingProvider{name: "agy", ask: planSuccess}
	ex := New(t.TempDir(), primary, primary, "m", "m", 1)
	ex.ArchitectPool = nil
	cfg := &config.Config{Workspace: t.TempDir(), Providers: map[string]config.ProviderConfig{
		"main":   {Type: config.ProviderMock, Model: "m"},
		"broken": {Type: "bogus", Model: "x"},
		"spare":  {Type: config.ProviderMock, Model: "spare-model"},
	}}
	cfg.Roles.Architect = "main"
	cfg.Failover.Architect = []string{"main", "broken", "missing", "spare", "spare"}
	closeExtras, skipped := ex.AddArchitectFallbacks(cfg)
	defer closeExtras()
	if len(skipped) != 2 {
		t.Fatalf("expected broken and missing to be skipped, got %v", skipped)
	}
	got := ex.architectCandidates()
	if len(got) != 2 || got[0].ProviderKey != "main" || got[1].ProviderKey != "spare" || got[1].Model != "spare-model" {
		t.Fatalf("unexpected architect pool: %+v", got)
	}
}

func TestRememberReadKeepsValidUTF8(t *testing.T) {
	ex := New(t.TempDir(), nil, nil, "", "", 1)
	thai := strings.Repeat("สวัสดี", 4000) // 3 bytes per rune, well over the entry limit
	for i := 0; i < 6; i++ {
		ex.rememberRead("file.go", thai[:len(thai)-i]) // shift byte alignment each round
		if !utf8.ValidString(ex.ReadContext) {
			t.Fatalf("round %d produced invalid UTF-8", i)
		}
		if len(ex.ReadContext) > maxReadContext {
			t.Fatalf("round %d exceeded context budget: %d", i, len(ex.ReadContext))
		}
	}
	if !strings.Contains(ex.ReadContext, "[read truncated]") {
		t.Fatal("oversized read was not marked as truncated")
	}
	for _, n := range []int{0, 1, 2, 3, 4, 5, 100} {
		if h := headUTF8("กขค", n); !utf8.ValidString(h) || len(h) > n {
			t.Fatalf("headUTF8(%d)=%q", n, h)
		}
		if tl := tailUTF8("กขค", n); !utf8.ValidString(tl) || len(tl) > n {
			t.Fatalf("tailUTF8(%d)=%q", n, tl)
		}
	}
}
