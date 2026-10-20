package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// GovernorState holds the 24/7 autonomous safety and rate-limit circuit breaker state.
type GovernorState struct {
	MaxTasksPerHour        int       `json:"max_tasks_per_hour"`
	MaxConsecutiveFailures int       `json:"max_consecutive_failures"`
	ConsecutiveFailures    int       `json:"consecutive_failures"`
	TasksLastHour          int       `json:"tasks_last_hour"`
	Paused                 bool      `json:"paused"`
	PauseReason            string    `json:"pause_reason,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// AutonomyGoal is a persistent long-horizon backlog item executed sequentially by the 24/7 worker.
type AutonomyGoal struct {
	ID          string    `json:"id"`
	Owner       string    `json:"owner"`
	Title       string    `json:"title"`
	Instruction string    `json:"instruction"`
	Mode        string    `json:"mode"`
	Priority    int       `json:"priority"`
	Status      string    `json:"status"` // pending, running, completed, failed, paused
	TaskID      string    `json:"task_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AutonomySchedule is a recurring 24/7 cron/interval job definition.
type AutonomySchedule struct {
	ID          string    `json:"id"`
	Owner       string    `json:"owner"`
	Name        string    `json:"name"`
	Instruction string    `json:"instruction"`
	Mode        string    `json:"mode"`
	IntervalSec int       `json:"interval_sec"`
	Enabled     bool      `json:"enabled"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	NextRunAt   time.Time `json:"next_run_at"`
	LastTaskID  string    `json:"last_task_id,omitempty"`
	RunCount    int       `json:"run_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// Episode is a self-evolving episodic memory entry distilled from a completed task.
type Episode struct {
	ID          int64     `json:"id"`
	TaskID      string    `json:"task_id"`
	Owner       string    `json:"owner"`
	Instruction string    `json:"instruction"`
	Mode        string    `json:"mode"`
	Status      string    `json:"status"`
	Files       []string  `json:"files,omitempty"`
	Lesson      string    `json:"lesson"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) AutonomySchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS autonomy_governor (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	max_tasks_per_hour INTEGER NOT NULL DEFAULT 10,
	max_consecutive_failures INTEGER NOT NULL DEFAULT 3,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	paused INTEGER NOT NULL DEFAULT 0,
	pause_reason TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO autonomy_governor (id, max_tasks_per_hour, max_consecutive_failures, consecutive_failures, paused, pause_reason, updated_at)
VALUES (1, 10, 3, 0, 0, '', 0);

CREATE TABLE IF NOT EXISTS autonomy_goals (
	id TEXT PRIMARY KEY,
	owner TEXT NOT NULL,
	title TEXT NOT NULL,
	instruction TEXT NOT NULL,
	mode TEXT NOT NULL DEFAULT 'execute',
	priority INTEGER NOT NULL DEFAULT 1,
	status TEXT NOT NULL DEFAULT 'pending',
	task_id TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_autonomy_goals_status ON autonomy_goals(owner, status, priority DESC, created_at ASC);

CREATE TABLE IF NOT EXISTS autonomy_schedules (
	id TEXT PRIMARY KEY,
	owner TEXT NOT NULL,
	name TEXT NOT NULL,
	instruction TEXT NOT NULL,
	mode TEXT NOT NULL DEFAULT 'verify',
	interval_sec INTEGER NOT NULL DEFAULT 3600,
	enabled INTEGER NOT NULL DEFAULT 1,
	last_run_at INTEGER NOT NULL DEFAULT 0,
	next_run_at INTEGER NOT NULL DEFAULT 0,
	last_task_id TEXT NOT NULL DEFAULT '',
	run_count INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_autonomy_schedules_due ON autonomy_schedules(enabled, next_run_at);

CREATE TABLE IF NOT EXISTS episodic_memory (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL UNIQUE,
	owner TEXT NOT NULL,
	instruction TEXT NOT NULL,
	mode TEXT NOT NULL,
	status TEXT NOT NULL,
	files TEXT NOT NULL DEFAULT '',
	lesson TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_episodic_memory_owner ON episodic_memory(owner, created_at DESC);
`)
	return err
}

func (s *Store) Governor(ctx context.Context) (GovernorState, error) {
	var g GovernorState
	var paused int
	var updated int64
	err := s.ro.QueryRowContext(ctx, `SELECT max_tasks_per_hour, max_consecutive_failures, consecutive_failures, paused, pause_reason, updated_at FROM autonomy_governor WHERE id = 1`).
		Scan(&g.MaxTasksPerHour, &g.MaxConsecutiveFailures, &g.ConsecutiveFailures, &paused, &g.PauseReason, &updated)
	if err != nil {
		return g, err
	}
	g.Paused = paused != 0
	if updated > 0 {
		g.UpdatedAt = time.Unix(0, updated)
	}
	oneHourAgo := time.Now().Add(-1 * time.Hour).UnixNano()
	_ = s.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE created_at >= ?`, oneHourAgo).Scan(&g.TasksLastHour)
	return g, nil
}

// UpdateGovernor sets the pause state. A limit of zero or less keeps the stored value, so
// pausing or resuming never resets limits the user configured.
func (s *Store) UpdateGovernor(ctx context.Context, maxPerHour, maxFailures int, paused bool, reason string) (GovernorState, error) {
	pausedInt := 0
	if paused {
		pausedInt = 1
	} else {
		reason = ""
	}
	now := time.Now().UnixNano()
	// Resuming also clears the failure streak that may have tripped the breaker.
	_, err := s.db.ExecContext(ctx, `UPDATE autonomy_governor SET
		max_tasks_per_hour = CASE WHEN ? > 0 THEN ? ELSE max_tasks_per_hour END,
		max_consecutive_failures = CASE WHEN ? > 0 THEN ? ELSE max_consecutive_failures END,
		consecutive_failures = CASE WHEN ? = 0 THEN 0 ELSE consecutive_failures END,
		paused = ?, pause_reason = ?, updated_at = ? WHERE id = 1`,
		maxPerHour, maxPerHour, maxFailures, maxFailures, pausedInt, pausedInt, reason, now)
	if err != nil {
		return GovernorState{}, err
	}
	return s.Governor(ctx)
}

func (s *Store) CreateGoal(ctx context.Context, g AutonomyGoal) (*AutonomyGoal, error) {
	if strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.Instruction) == "" {
		return nil, errors.New("goal id and instruction are required")
	}
	if strings.TrimSpace(g.Owner) == "" {
		g.Owner = "local"
	}
	if strings.TrimSpace(g.Title) == "" {
		g.Title = g.Instruction
		g.Title = clipRunes(g.Title, 72)
	}
	if g.Mode == "" {
		g.Mode = "execute"
	}
	if g.Priority <= 0 {
		g.Priority = 1
	}
	if g.Status == "" {
		g.Status = "pending"
	}
	now := time.Now()
	g.CreatedAt = now
	g.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO autonomy_goals (id, owner, title, instruction, mode, priority, status, task_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.Owner, g.Title, g.Instruction, g.Mode, g.Priority, g.Status, g.TaskID, now.UnixNano(), now.UnixNano())
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) Goals(ctx context.Context, owner string) ([]AutonomyGoal, error) {
	rows, err := s.ro.QueryContext(ctx,
		`SELECT id, owner, title, instruction, mode, priority, status, task_id, created_at, updated_at FROM autonomy_goals WHERE owner = ? ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END, priority DESC, created_at DESC LIMIT 100`,
		owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AutonomyGoal, 0)
	for rows.Next() {
		var g AutonomyGoal
		var cNs, uNs int64
		if err := rows.Scan(&g.ID, &g.Owner, &g.Title, &g.Instruction, &g.Mode, &g.Priority, &g.Status, &g.TaskID, &cNs, &uNs); err != nil {
			return nil, err
		}
		g.CreatedAt = time.Unix(0, cNs)
		g.UpdatedAt = time.Unix(0, uNs)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) DeleteGoal(ctx context.Context, id, owner string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM autonomy_goals WHERE id = ? AND owner = ?`, id, owner)
	return err
}

func (s *Store) ClaimNextGoal(ctx context.Context, owner string) (*AutonomyGoal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var g AutonomyGoal
	var cNs, uNs int64
	err = tx.QueryRowContext(ctx,
		`SELECT id, owner, title, instruction, mode, priority, status, task_id, created_at, updated_at FROM autonomy_goals WHERE owner = ? AND status = 'pending' ORDER BY priority DESC, created_at ASC LIMIT 1`,
		owner).Scan(&g.ID, &g.Owner, &g.Title, &g.Instruction, &g.Mode, &g.Priority, &g.Status, &g.TaskID, &cNs, &uNs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_, err = tx.ExecContext(ctx, `UPDATE autonomy_goals SET status = 'running', updated_at = ? WHERE id = ?`, now.UnixNano(), g.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	g.Status = "running"
	g.CreatedAt = time.Unix(0, cNs)
	g.UpdatedAt = now
	return &g, nil
}

func (s *Store) SetGoalTask(ctx context.Context, id, status, taskID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE autonomy_goals SET status = ?, task_id = ?, updated_at = ? WHERE id = ?`,
		status, taskID, time.Now().UnixNano(), id)
	return err
}

func (s *Store) CreateSchedule(ctx context.Context, sch AutonomySchedule) (*AutonomySchedule, error) {
	if strings.TrimSpace(sch.ID) == "" || strings.TrimSpace(sch.Instruction) == "" {
		return nil, errors.New("schedule id and instruction are required")
	}
	if strings.TrimSpace(sch.Owner) == "" {
		sch.Owner = "local"
	}
	if strings.TrimSpace(sch.Name) == "" {
		sch.Name = sch.Instruction
		sch.Name = clipRunes(sch.Name, 60)
	}
	if sch.Mode == "" {
		sch.Mode = "verify"
	}
	if sch.IntervalSec < 30 {
		sch.IntervalSec = 30
	}
	now := time.Now()
	sch.CreatedAt = now
	if sch.NextRunAt.IsZero() {
		sch.NextRunAt = now.Add(time.Duration(sch.IntervalSec) * time.Second)
	}
	en := 0
	if sch.Enabled {
		en = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO autonomy_schedules (id, owner, name, instruction, mode, interval_sec, enabled, last_run_at, next_run_at, last_task_id, run_count, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, '', 0, ?)`,
		sch.ID, sch.Owner, sch.Name, sch.Instruction, sch.Mode, sch.IntervalSec, en, sch.NextRunAt.UnixNano(), now.UnixNano())
	if err != nil {
		return nil, err
	}
	return &sch, nil
}

func (s *Store) Schedules(ctx context.Context, owner string) ([]AutonomySchedule, error) {
	rows, err := s.ro.QueryContext(ctx,
		`SELECT id, owner, name, instruction, mode, interval_sec, enabled, last_run_at, next_run_at, last_task_id, run_count, created_at FROM autonomy_schedules WHERE owner = ? ORDER BY created_at DESC LIMIT 100`,
		owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AutonomySchedule, 0)
	for rows.Next() {
		var sch AutonomySchedule
		var en int
		var lastNs, nextNs, cNs int64
		if err := rows.Scan(&sch.ID, &sch.Owner, &sch.Name, &sch.Instruction, &sch.Mode, &sch.IntervalSec, &en, &lastNs, &nextNs, &sch.LastTaskID, &sch.RunCount, &cNs); err != nil {
			return nil, err
		}
		sch.Enabled = en != 0
		if lastNs > 0 {
			sch.LastRunAt = time.Unix(0, lastNs)
		}
		if nextNs > 0 {
			sch.NextRunAt = time.Unix(0, nextNs)
		}
		sch.CreatedAt = time.Unix(0, cNs)
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *Store) ToggleSchedule(ctx context.Context, id, owner string, enabled bool) error {
	en := 0
	nextNs := int64(0)
	if enabled {
		en = 1
		nextNs = time.Now().Add(5 * time.Second).UnixNano()
	}
	if enabled {
		_, err := s.db.ExecContext(ctx, `UPDATE autonomy_schedules SET enabled = ?, next_run_at = ? WHERE id = ? AND owner = ?`, en, nextNs, id, owner)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE autonomy_schedules SET enabled = 0 WHERE id = ? AND owner = ?`, id, owner)
	return err
}

func (s *Store) DeleteSchedule(ctx context.Context, id, owner string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM autonomy_schedules WHERE id = ? AND owner = ?`, id, owner)
	return err
}

func (s *Store) ClaimDueSchedule(ctx context.Context, now time.Time) (*AutonomySchedule, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var sch AutonomySchedule
	var en int
	var lastNs, nextNs, cNs int64
	err = tx.QueryRowContext(ctx,
		`SELECT id, owner, name, instruction, mode, interval_sec, enabled, last_run_at, next_run_at, last_task_id, run_count, created_at FROM autonomy_schedules WHERE enabled = 1 AND next_run_at <= ? ORDER BY next_run_at ASC LIMIT 1`,
		now.UnixNano()).Scan(&sch.ID, &sch.Owner, &sch.Name, &sch.Instruction, &sch.Mode, &sch.IntervalSec, &en, &lastNs, &nextNs, &sch.LastTaskID, &sch.RunCount, &cNs)
	if err != nil {
		return nil, err
	}
	nextRun := now.Add(time.Duration(sch.IntervalSec) * time.Second)
	_, err = tx.ExecContext(ctx,
		`UPDATE autonomy_schedules SET last_run_at = ?, next_run_at = ?, run_count = run_count + 1 WHERE id = ?`,
		now.UnixNano(), nextRun.UnixNano(), sch.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	sch.Enabled = true
	sch.LastRunAt = now
	sch.NextRunAt = nextRun
	sch.RunCount++
	sch.CreatedAt = time.Unix(0, cNs)
	return &sch, nil
}

func (s *Store) SetScheduleTask(ctx context.Context, id, taskID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE autonomy_schedules SET last_task_id = ? WHERE id = ?`, taskID, id)
	return err
}

// RecordAutonomousOutcome updates linked goals and the circuit-breaker governor when a task finishes.
func (s *Store) RecordAutonomousOutcome(ctx context.Context, taskID string, succeeded bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	goalStatus := "failed"
	if succeeded {
		goalStatus = "completed"
	}
	res, err := tx.ExecContext(ctx, `UPDATE autonomy_goals SET status = ?, updated_at = ? WHERE task_id = ?`,
		goalStatus, time.Now().UnixNano(), taskID)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()

	var schedCount int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM autonomy_schedules WHERE last_task_id = ?`, taskID).Scan(&schedCount)

	// Only adjust circuit-breaker failure streak for autonomous tasks (linked to a goal or schedule)
	if rowsAffected > 0 || schedCount > 0 {
		if succeeded {
			_, _ = tx.ExecContext(ctx, `UPDATE autonomy_governor SET consecutive_failures = 0, updated_at = ? WHERE id = 1`, time.Now().UnixNano())
		} else {
			_, _ = tx.ExecContext(ctx, `UPDATE autonomy_governor SET consecutive_failures = consecutive_failures + 1, updated_at = ? WHERE id = 1`, time.Now().UnixNano())
			var consec, maxFail int
			if err := tx.QueryRowContext(ctx, `SELECT consecutive_failures, max_consecutive_failures FROM autonomy_governor WHERE id = 1`).Scan(&consec, &maxFail); err == nil && maxFail > 0 && consec >= maxFail {
				_, _ = tx.ExecContext(ctx, `UPDATE autonomy_governor SET paused = 1, pause_reason = ? WHERE id = 1`,
					fmt.Sprintf("Circuit Breaker tripped: %d consecutive autonomous tasks failed", consec))
			}
		}
	}
	return tx.Commit()
}

func (s *Store) RecordEpisode(ctx context.Context, ep Episode) error {
	if ep.TaskID == "" || ep.Lesson == "" {
		return nil
	}
	if ep.Owner == "" {
		ep.Owner = "local"
	}
	filesStr := strings.Join(ep.Files, ",")
	now := time.Now().UnixNano()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO episodic_memory (task_id, owner, instruction, mode, status, files, lesson, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(task_id) DO UPDATE SET status = excluded.status, files = excluded.files, lesson = excluded.lesson`,
		ep.TaskID, ep.Owner, ep.Instruction, ep.Mode, ep.Status, filesStr, ep.Lesson, now)
	return err
}

func (s *Store) Episodes(ctx context.Context, owner string, limit int) ([]Episode, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.ro.QueryContext(ctx,
		`SELECT id, task_id, owner, instruction, mode, status, files, lesson, created_at FROM episodic_memory WHERE owner = ? ORDER BY created_at DESC LIMIT ?`,
		owner, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Episode, 0)
	for rows.Next() {
		var ep Episode
		var filesStr string
		var cNs int64
		if err := rows.Scan(&ep.ID, &ep.TaskID, &ep.Owner, &ep.Instruction, &ep.Mode, &ep.Status, &filesStr, &ep.Lesson, &cNs); err != nil {
			return nil, err
		}
		if filesStr != "" {
			ep.Files = strings.Split(filesStr, ",")
		}
		ep.CreatedAt = time.Unix(0, cNs)
		out = append(out, ep)
	}
	return out, rows.Err()
}

// tokenizeEpisodic breaks English/code identifiers and unspaced Thai phrases into search tokens
// and Thai 2/3-rune character n-grams so episodic recall works across both English and Thai.
func tokenizeEpisodic(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(tok string) {
		if len(tok) < 2 || seen[tok] {
			return
		}
		seen[tok] = true
		out = append(out, tok)
	}
	var asciiBuf, thaiBuf []rune
	flushASCII := func() {
		if len(asciiBuf) >= 3 {
			w := string(asciiBuf)
			add(w)
			for _, sub := range strings.FieldsFunc(w, func(r rune) bool { return r == '_' || r == '-' || r == '/' || r == '.' }) {
				if len(sub) >= 3 {
					add(sub)
				}
			}
		}
		asciiBuf = asciiBuf[:0]
	}
	flushThai := func() {
		if len(thaiBuf) >= 2 {
			add(string(thaiBuf))
			for i := 0; i+3 <= len(thaiBuf); i++ {
				add(string(thaiBuf[i : i+3]))
			}
			for i := 0; i+2 <= len(thaiBuf); i++ {
				add(string(thaiBuf[i : i+2]))
			}
		}
		thaiBuf = thaiBuf[:0]
	}
	for _, r := range text {
		switch {
		case r >= 0x0E00 && r <= 0x0E7F:
			flushASCII()
			thaiBuf = append(thaiBuf, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/' || r == '.':
			flushThai()
			asciiBuf = append(asciiBuf, r)
		default:
			flushASCII()
			flushThai()
		}
	}
	flushASCII()
	flushThai()
	return out
}

func scoreEpisode(ep Episode, queryLower string, tokens []string) int {
	score := 0
	instrLower := strings.ToLower(ep.Instruction)
	lessonLower := strings.ToLower(ep.Lesson)
	filesLower := strings.ToLower(strings.Join(ep.Files, " "))
	for _, f := range ep.Files {
		fl := strings.ToLower(strings.TrimSpace(f))
		base := strings.ToLower(filepath.Base(fl))
		if fl != "" && (strings.Contains(queryLower, fl) || (len(base) >= 4 && strings.Contains(queryLower, base))) {
			score += 20
		}
	}
	for _, tok := range tokens {
		if strings.Contains(filesLower, tok) {
			score += 6
		}
		if strings.Contains(instrLower, tok) {
			score += 4
		}
		if strings.Contains(lessonLower, tok) {
			score += 3
		}
	}
	return score
}

// SearchEpisodes searches episodic memory for lessons matching query (supporting English, file paths, and Thai).
func (s *Store) SearchEpisodes(ctx context.Context, owner, query string, limit int) ([]Episode, error) {
	episodes, err := s.Episodes(ctx, owner, 100)
	if err != nil || len(episodes) == 0 {
		return episodes, err
	}
	if limit <= 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		if len(episodes) > limit {
			return episodes[:limit], nil
		}
		return episodes, nil
	}
	tokens := tokenizeEpisodic(query)
	queryLower := strings.ToLower(query)
	type scoredEp struct {
		ep    Episode
		score int
	}
	var scored []scoredEp
	for _, ep := range episodes {
		if sc := scoreEpisode(ep, queryLower, tokens); sc > 0 {
			scored = append(scored, scoredEp{ep: ep, score: sc})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].ep.CreatedAt.After(scored[j].ep.CreatedAt)
	})
	var out []Episode
	for i := 0; i < len(scored) && i < limit; i++ {
		out = append(out, scored[i].ep)
	}
	return out, nil
}

// RecallEpisodicContext formats relevant past episodic lessons for the Executor's planning prompt.
func (s *Store) RecallEpisodicContext(ctx context.Context, owner, instruction string, limit int) string {
	matched, err := s.SearchEpisodes(ctx, owner, instruction, limit)
	if err != nil || len(matched) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Episodic Memory (Lessons from Past Tasks in this Workspace)\n")
	for _, ep := range matched {
		filesNote := ""
		if len(ep.Files) > 0 {
			filesNote = " [" + strings.Join(ep.Files, ", ") + "]"
		}
		fmt.Fprintf(&sb, "- [%s]%s %s -> %s\n", strings.ToUpper(ep.Status), filesNote, ep.Instruction, ep.Lesson)
	}
	return sb.String()
}

var skillSlugInvalid = regexp.MustCompile(`[^a-z0-9_-]+`)

// ErrSkillExists reports that SaveProceduralSkill refused to overwrite an existing skill.
var ErrSkillExists = errors.New("skill already exists")

// SaveProceduralSkill crystallizes a verified task pattern into .apw/skills/<slug>/SKILL.md.
func SaveProceduralSkill(workspace, slug, title, instruction, summary string, files []string) (string, error) {
	cleanSlug := strings.ToLower(strings.TrimSpace(slug))
	cleanSlug = skillSlugInvalid.ReplaceAllString(cleanSlug, "-")
	cleanSlug = strings.Trim(cleanSlug, "-_")
	if cleanSlug == "" {
		cleanSlug = fmt.Sprintf("skill-%d", time.Now().Unix())
	}
	if len(cleanSlug) > 48 {
		cleanSlug = cleanSlug[:48]
	}
	if title == "" {
		title = cleanSlug
	}
	rel := filepath.Join(".apw", "skills", cleanSlug, "SKILL.md")
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + cleanSlug + "\n")
	sb.WriteString("description: " + strings.ReplaceAll(title, "\n", " ") + "\n")
	sb.WriteString("---\n\n")
	sb.WriteString("# " + title + "\n\n")
	sb.WriteString("## Trigger / Task Pattern\n")
	sb.WriteString(instruction + "\n\n")
	if len(files) > 0 {
		sb.WriteString("## Key Touched Files\n")
		for _, f := range files {
			sb.WriteString("- `" + f + "`\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Verified Procedure & Summary\n")
	sb.WriteString(summary + "\n")

	if err := surgeon.CreateAPWCustomization(workspace, rel, []byte(sb.String())); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: skill %q already exists", ErrSkillExists, cleanSlug)
		}
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// IsNoRows reports whether err is sql.ErrNoRows.
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
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
