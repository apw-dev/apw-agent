package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

// Real calls are deliberately opt-in and use exactly the explicitly selected model.
func TestLiveSelectedCLI(t *testing.T) {
	if os.Getenv("APW_LIVE_TEST") != "1" {
		t.Skip("set APW_LIVE_TEST=1 and explicitly select backend/model")
	}
	backend, model := BackendType(os.Getenv("APW_LIVE_BACKEND")), os.Getenv("APW_LIVE_MODEL")
	if model == "" || (backend != BackendAGY && backend != BackendCodex && backend != BackendClaude) {
		t.Fatal("explicit supported APW_LIVE_BACKEND and APW_LIVE_MODEL are required")
	}
	nonce := fmt.Sprintf("APW_ACCEPT_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	response, err := New(backend, model, 85*time.Second).Ask(ctx, core.ModelRequest{RequestID: nonce, Model: model, Prompt: "Do not use tools or access files. Return exactly this text, with no explanation: " + nonce})
	passed := err == nil && response != nil && response.Completed && strings.TrimSpace(response.Content) == nonce
	report := map[string]any{"backend": backend, "requested_model": model, "passed": passed, "duration_ms": time.Since(start).Milliseconds()}
	if err != nil {
		report["error"] = err.Error()
	}
	if response != nil {
		report["adapter_observed_model"] = response.ModelObserved
		report["model_evidence"] = response.ModelEvidence
		report["response_matches_nonce"] = strings.TrimSpace(response.Content) == nonce
		report["raw_response"] = response.RawResponse
	}
	if path := os.Getenv("APW_LIVE_REPORT"); path != "" {
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if !passed {
		t.Fatalf("selected CLI acceptance failed: %v", err)
	}
}
