package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func TestIsQuestion(t *testing.T) {
	for text, want := range map[string]bool{
		"CROWDSEC_BOUNCER_KEY, CROWDSEC_MACHINE_ID, and CROWDSEC_MACHINE_PASSWORD ได้มายังไง": true,
		"How does the cache expire?":                true,
		"what port does the api listen on":          true,
		"ระบบนี้ใช้ฐานข้อมูลอะไร":                   true,
		"where is the address book stored?":         true, // "address" is not "add"
		"เพิ่ม NewWithTTL ใน store package":         false,
		"แก้ bug login ทำไมถึงพัง":                  false, // asks for a fix
		"Fix the login bug?":                        false,
		"Add docs that explain how to get the keys": false,
		"Make the cache TTL configurable":           false,
		"README":                                    false,
		"":                                          false,
	} {
		if got := isQuestion(text); got != want {
			t.Errorf("isQuestion(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestQuestionIsAnsweredWithoutPlanning(t *testing.T) {
	ws := t.TempDir() // not a Git repository: planning a change would fail
	mp := mock.New("Run `cscli bouncers add web-ui` and copy the key (docs/setup.md:12).")
	e := New(ws, mp, mp, "arch", "reflex", 1)
	e.Route = RouteAuto
	e.Audit = AuditModel // auditors must not run for an answer
	e.Mode = core.ModeExecute
	report, err := e.Execute(context.Background(), "CROWDSEC_BOUNCER_KEY ได้มายังไง", "")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Answered || report.Mode != core.ModeReview || !strings.Contains(report.Summary, "cscli bouncers add") {
		t.Fatalf("report = %+v", report)
	}
	if len(mp.Requests) != 1 || !strings.Contains(mp.Requests[0].Prompt, "Answer the user's question") {
		t.Fatalf("expected one answer request, got %d", len(mp.Requests))
	}
}

func TestRouteOffKeepsRequestedMode(t *testing.T) {
	ws := t.TempDir()
	mp := mock.New("unused")
	e := New(ws, mp, mp, "arch", "reflex", 1)
	e.Mode = core.ModePreview
	_, err := e.Execute(context.Background(), "CROWDSEC_BOUNCER_KEY ได้มายังไง", "")
	if err == nil || !strings.Contains(err.Error(), "Git repository") {
		t.Fatalf("expected the change pipeline (Git required), got %v", err)
	}
	if len(mp.Requests) != 0 {
		t.Fatalf("route off asked the model %d times", len(mp.Requests))
	}
}
