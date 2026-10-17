package executor

import (
	"fmt"
	"github.com/apiwat/apw-agent/internal/reflex"
	"path/filepath"
	"strings"
)

type DiagnosticComparison struct {
	ObservedBefore      []reflex.Diagnostic `json:"observed_before,omitempty"`
	NotObservedBefore   []reflex.Diagnostic `json:"not_observed_before,omitempty"`
	NoLongerObserved    []reflex.Diagnostic `json:"no_longer_observed,omitempty"`
	BaselinePassed      bool                `json:"baseline_passed"`
	BaselineFailedStage string              `json:"baseline_failed_stage,omitempty"`
}

func compareDiagnostics(baseline, current *reflex.VerificationResult) *DiagnosticComparison {
	if baseline == nil || current == nil {
		return nil
	}
	comparison := &DiagnosticComparison{BaselinePassed: baseline.Passed, BaselineFailedStage: baseline.FailedStage}
	key := func(d reflex.Diagnostic) string {
		return fmt.Sprintf("%s:%s:%s", d.Stage, filepath.ToSlash(filepath.Clean(d.File)), strings.TrimSpace(d.Message))
	}
	before := map[string]bool{}
	after := map[string]bool{}
	for _, d := range baseline.Diagnostics {
		before[key(d)] = true
	}
	for _, d := range current.Diagnostics {
		after[key(d)] = true
		if before[key(d)] {
			comparison.ObservedBefore = append(comparison.ObservedBefore, d)
		} else {
			comparison.NotObservedBefore = append(comparison.NotObservedBefore, d)
		}
	}
	for _, d := range baseline.Diagnostics {
		if !after[key(d)] {
			comparison.NoLongerObserved = append(comparison.NoLongerObserved, d)
		}
	}
	return comparison
}
