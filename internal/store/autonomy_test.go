package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAutonomyGoalsSchedulesCircuitBreakerAndEpisodicMemory(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Governor default state and update
	gov, err := db.Governor(ctx)
	if err != nil {
		t.Fatalf("Governor: %v", err)
	}
	if gov.Paused || gov.MaxConsecutiveFailures != 3 {
		t.Fatalf("unexpected initial governor state: %+v", gov)
	}

	gov, err = db.UpdateGovernor(ctx, 8, 2, false, "")
	if err != nil {
		t.Fatalf("UpdateGovernor: %v", err)
	}
	if gov.MaxTasksPerHour != 8 || gov.MaxConsecutiveFailures != 2 {
		t.Fatalf("unexpected updated governor: %+v", gov)
	}

	// 2. Goal Backlog creation, priority ordering, claiming, and circuit-breaker tripping
	_, err = db.CreateGoal(ctx, AutonomyGoal{
		ID:          "goal-low",
		Owner:       "local",
		Title:       "Low priority goal",
		Instruction: "refactor comments",
		Priority:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateGoal(ctx, AutonomyGoal{
		ID:          "goal-high",
		Owner:       "local",
		Title:       "High priority goal",
		Instruction: "add store tests",
		Priority:    10,
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := db.ClaimNextGoal(ctx, "local")
	if err != nil {
		t.Fatalf("ClaimNextGoal: %v", err)
	}
	if claimed.ID != "goal-high" || claimed.Status != "running" {
		t.Fatalf("expected goal-high to be claimed first, got %+v", claimed)
	}
	if err := db.SetGoalTask(ctx, claimed.ID, "running", "task-auton-1"); err != nil {
		t.Fatal(err)
	}

	// Simulate first failure
	if err := db.RecordAutonomousOutcome(ctx, "task-auton-1", false); err != nil {
		t.Fatal(err)
	}
	gov, _ = db.Governor(ctx)
	if gov.ConsecutiveFailures != 1 || gov.Paused {
		t.Fatalf("expected 1 failure and not yet paused, got %+v", gov)
	}

	// Claim second goal and fail -> should trip circuit breaker (MaxConsecutiveFailures == 2)
	claimed2, err := db.ClaimNextGoal(ctx, "local")
	if err != nil {
		t.Fatalf("ClaimNextGoal 2: %v", err)
	}
	if err := db.SetGoalTask(ctx, claimed2.ID, "running", "task-auton-2"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordAutonomousOutcome(ctx, "task-auton-2", false); err != nil {
		t.Fatal(err)
	}
	gov, _ = db.Governor(ctx)
	if !gov.Paused || !strings.Contains(gov.PauseReason, "Circuit Breaker tripped") {
		t.Fatalf("expected circuit breaker to trip after 2 failures, got %+v", gov)
	}

	// 3. Recurring Schedules
	past := time.Now().Add(-10 * time.Second)
	_, err = db.CreateSchedule(ctx, AutonomySchedule{
		ID:          "sched-1",
		Owner:       "local",
		Name:        "Hourly Verify",
		Instruction: "run verification suite",
		Mode:        "verify",
		IntervalSec: 60,
		Enabled:     true,
		NextRunAt:   past,
	})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	due, err := db.ClaimDueSchedule(ctx, time.Now())
	if err != nil {
		t.Fatalf("ClaimDueSchedule: %v", err)
	}
	if due.ID != "sched-1" || due.RunCount != 1 {
		t.Fatalf("unexpected claimed schedule: %+v", due)
	}

	// 4. Episodic Memory & Recall
	if err := db.RecordEpisode(ctx, Episode{
		TaskID:      "task-ep-1",
		Owner:       "local",
		Instruction: "Fix SQLite watermark bug in store/jobs.go",
		Mode:        "execute",
		Status:      "succeeded",
		Files:       []string{"internal/store/jobs.go"},
		Lesson:      "Ensure updated_at advances monotonically using max(now, prev+1).",
	}); err != nil {
		t.Fatalf("RecordEpisode: %v", err)
	}

	recalled := db.RecallEpisodicContext(ctx, "local", "update store/jobs.go watermark", 5)
	if !strings.Contains(recalled, "Episodic Memory") || !strings.Contains(recalled, "monotonically") {
		t.Fatalf("expected recalled episodic memory to include lesson, got:\n%s", recalled)
	}

	// Unrelated query must not inject irrelevant episodes
	if unrelated := db.RecallEpisodicContext(ctx, "local", "configure kubernetes ingress", 5); unrelated != "" {
		t.Fatalf("expected unrelated query to recall nothing, got:\n%s", unrelated)
	}

	// Unspaced Thai episodic recall via character n-gram scoring
	if err := db.RecordEpisode(ctx, Episode{
		TaskID:      "task-ep-thai",
		Owner:       "local",
		Instruction: "แก้ไขบั๊กหน้าตั้งค่าโมเดลในระบบเดสก์ท็อป",
		Mode:        "execute",
		Status:      "succeeded",
		Files:       []string{"internal/desktop/settings.go"},
		Lesson:      "ตรวจสอบการบันทึกค่าโมเดลแบบอะตอมมิกเสมอ",
	}); err != nil {
		t.Fatalf("RecordEpisode Thai: %v", err)
	}
	thaiRecalled := db.RecallEpisodicContext(ctx, "local", "ปรับปรุงหน้าตั้งค่าโมเดล", 5)
	if !strings.Contains(thaiRecalled, "อะตอมมิก") {
		t.Fatalf("expected unspaced Thai query to recall Thai episode, got:\n%s", thaiRecalled)
	}

	// 5. Procedural Skill extraction (.apw/skills/<slug>/SKILL.md)
	relSkill, err := SaveProceduralSkill(dir, "SQLite Watermark Fix!", "SQLite Watermark Fix", "Fix monotonic timestamp in store", "Advance updated_at by at least 1ns.", []string{"internal/store/jobs.go"})
	if err != nil {
		t.Fatalf("SaveProceduralSkill: %v", err)
	}
	if relSkill != ".apw/skills/sqlite-watermark-fix/SKILL.md" {
		t.Fatalf("unexpected skill path: %s", relSkill)
	}
	rawSkill, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relSkill)))
	if err != nil {
		t.Fatalf("ReadFile skill: %v", err)
	}
	if !strings.Contains(string(rawSkill), "name: sqlite-watermark-fix") || !strings.Contains(string(rawSkill), "internal/store/jobs.go") {
		t.Fatalf("unexpected SKILL.md content:\n%s", string(rawSkill))
	}
}

func TestSaveProceduralSkillRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveProceduralSkill(dir, "dup", "Dup", "first", "first", nil); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := SaveProceduralSkill(dir, "dup", "Dup", "second", "second", nil); !errors.Is(err, ErrSkillExists) {
		t.Fatalf("expected ErrSkillExists, got %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".apw", "skills", "dup", "SKILL.md"))
	if err != nil || !strings.Contains(string(raw), "first") || strings.Contains(string(raw), "second") {
		t.Fatalf("existing skill changed: %q %v", raw, err)
	}
}

// Pausing or resuming from the Desktop sends no limits; the configured limits must survive.
func TestUpdateGovernorPauseKeepsConfiguredLimits(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.UpdateGovernor(ctx, 25, 7, false, ""); err != nil {
		t.Fatal(err)
	}
	gov, err := db.UpdateGovernor(ctx, 0, 0, true, "maintenance")
	if err != nil {
		t.Fatal(err)
	}
	if !gov.Paused || gov.PauseReason != "maintenance" || gov.MaxTasksPerHour != 25 || gov.MaxConsecutiveFailures != 7 {
		t.Fatalf("pause reset limits: %+v", gov)
	}
	if gov, err = db.UpdateGovernor(ctx, 0, 0, false, ""); err != nil {
		t.Fatal(err)
	}
	if gov.Paused || gov.PauseReason != "" || gov.MaxTasksPerHour != 25 || gov.MaxConsecutiveFailures != 7 {
		t.Fatalf("resume reset limits: %+v", gov)
	}
}

// Auto-derived titles from Thai instructions must stay valid UTF-8 when shortened.
func TestAutonomyDerivedTitlesStayValidUTF8(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// One ASCII byte shifts the 3-byte Thai runes so byte cuts at 60/72 land mid-rune.
	thai := "x" + strings.Repeat("ปรับปรุงหน้าตั้งค่า", 10)
	g, err := db.CreateGoal(ctx, AutonomyGoal{ID: "goal-th", Instruction: thai})
	if err != nil {
		t.Fatal(err)
	}
	sch, err := db.CreateSchedule(ctx, AutonomySchedule{ID: "sched-th", Instruction: thai, IntervalSec: 60, Enabled: true, NextRunAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"goal title": g.Title, "schedule name": sch.Name} {
		if !utf8.ValidString(v) || !strings.HasSuffix(v, "...") {
			t.Fatalf("%s is not a valid shortened string: %q", name, v)
		}
	}
	goals, err := db.Goals(ctx, "local")
	if err != nil || len(goals) != 1 || !utf8.ValidString(goals[0].Title) {
		t.Fatalf("stored goal title invalid: %+v %v", goals, err)
	}
}
