package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
)

func (s *Service) isIdle() bool {
	s.mu.Lock()
	active := s.activeID
	s.mu.Unlock()
	if active != "" {
		return false
	}
	if _, err := s.Store.Pending(s.ctx); err == nil {
		return false
	}
	return true
}

func (s *Service) safeAutonomousMode(raw string) core.TaskMode {
	mode := core.TaskMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		mode = core.ModeExecute
	}
	cfg, err := s.taskConfig()
	if err != nil || !cfg.Policy.Trusted {
		if mode == core.ModeExecute || mode == core.ModeVerify {
			return core.ModePreview
		}
	}
	return mode
}

// autonomyLoop runs 24/7 in the background, dispatching due schedules and pending goals when the worker is idle,
// governed by the rate-limit and consecutive-failure circuit breaker.
func (s *Service) autonomyLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.StepAutonomy(s.ctx)
		}
	}
}

// StepAutonomy performs one non-blocking tick of the 24/7 autonomous scheduler and goal backlog dispatcher.
func (s *Service) StepAutonomy(ctx context.Context) bool {
	if ctx.Err() != nil || !s.isIdle() {
		return false
	}
	gov, err := s.Store.Governor(ctx)
	if err != nil || gov.Paused || gov.TasksLastHour >= gov.MaxTasksPerHour {
		return false
	}

	// 1. Check due recurring schedules first
	if sch, err := s.Store.ClaimDueSchedule(ctx, time.Now()); err == nil && sch != nil {
		mode := s.safeAutonomousMode(sch.Mode)
		taskID := NewID()
		reqID := fmt.Sprintf("sched-%s-%d", sch.ID, sch.RunCount)
		job, _, createErr := s.Store.CreateJob(ctx, store.Job{
			Task: core.Task{
				ID:          taskID,
				Workspace:   s.Config.Workspace,
				Instruction: sch.Instruction,
				Mode:        string(mode),
			},
			Owner:     sch.Owner,
			RequestID: reqID,
		})
		if createErr == nil && job != nil {
			_ = s.Store.SetScheduleTask(ctx, sch.ID, job.ID)
			s.signal()
			return true
		}
	}

	// 2. Otherwise pick the highest-priority pending goal from the 24/7 backlog
	if goal, err := s.Store.ClaimNextGoal(ctx, "local"); err == nil && goal != nil {
		mode := s.safeAutonomousMode(goal.Mode)
		taskID := NewID()
		reqID := fmt.Sprintf("goal-%s-%d", goal.ID, time.Now().UnixNano())
		job, _, createErr := s.Store.CreateJob(ctx, store.Job{
			Task: core.Task{
				ID:          taskID,
				Workspace:   s.Config.Workspace,
				Instruction: goal.Instruction,
				Mode:        string(mode),
			},
			Owner:     goal.Owner,
			RequestID: reqID,
		})
		if createErr == nil && job != nil {
			_ = s.Store.SetGoalTask(ctx, goal.ID, "running", job.ID)
			s.signal()
			return true
		}
		_ = s.Store.SetGoalTask(ctx, goal.ID, "failed", "")
	}
	return false
}

func (s *Service) recordAutonomyAndMemory(j *store.Job, status core.TaskStatus, report *executor.ExecutionReport, cause string) {
	bg := context.Background()
	succeeded := status == core.StatusSucceeded || status == core.StatusReady || status == core.StatusPreview
	_ = s.Store.RecordAutonomousOutcome(bg, j.ID, succeeded)

	var files []string
	lesson := cause
	if report != nil {
		files = append(files, report.ModifiedFiles...)
		if len(files) == 0 && report.Plan != nil {
			files = append(files, report.Plan.AffectedFiles...)
		}
		if succeeded {
			obj := ""
			if report.Plan != nil {
				obj = report.Plan.Objective
			}
			repairs := len(report.ReflexAttempts)
			if obj != "" {
				lesson = fmt.Sprintf("Verified (%s): %s (steps: %d, verification attempts: %d)", status, obj, len(report.Steps), repairs)
			} else {
				lesson = fmt.Sprintf("Completed in mode %s with status %s", j.Mode, status)
			}
		}
	}
	if lesson == "" {
		lesson = fmt.Sprintf("Task finished with status %s", status)
	}
	lesson = clipRunes(lesson, 400)
	_ = s.Store.RecordEpisode(bg, store.Episode{
		TaskID:      j.ID,
		Owner:       j.Owner,
		Instruction: j.Instruction,
		Mode:        j.Mode,
		Status:      string(status),
		Files:       files,
		Lesson:      lesson,
	})
}

// clipRunes shortens s to at most n runes plus "..." without splitting a multi-byte
// character (Thai is 3 bytes per character; a byte cut stores invalid UTF-8).
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
