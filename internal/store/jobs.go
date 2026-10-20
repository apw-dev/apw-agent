package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

type Job struct {
	SessionID string `json:"session_id,omitempty"`
	core.Task
	Owner           string          `json:"owner"`
	RequestID       string          `json:"request_id"`
	Action          string          `json:"action"`
	Report          json.RawMessage `json:"report,omitempty"`
	Error           string          `json:"error,omitempty"`
	CancelRequested bool            `json:"cancel_requested"`
}
type Event struct {
	Seq     int64           `json:"seq"`
	TaskID  string          `json:"task_id"`
	Stage   string          `json:"stage"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}
type Delivery struct {
	ID       int64
	Account  string
	ChatID   int64
	Text     string
	Attempts int
}

func (s *Store) JobSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS task_runs (
task_id TEXT PRIMARY KEY, owner TEXT NOT NULL, request_id TEXT NOT NULL, action TEXT NOT NULL DEFAULT 'run',
report BLOB, error TEXT NOT NULL DEFAULT '', cancel_requested INTEGER NOT NULL DEFAULT 0,
UNIQUE(owner,request_id));
CREATE TABLE IF NOT EXISTS task_context (task_id TEXT PRIMARY KEY,session_id TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS task_events (seq INTEGER PRIMARY KEY AUTOINCREMENT,task_id TEXT NOT NULL,stage TEXT NOT NULL,message TEXT NOT NULL,data BLOB);
CREATE INDEX IF NOT EXISTS task_events_task ON task_events(task_id,seq);
CREATE INDEX IF NOT EXISTS tasks_status_created ON tasks(status,created_at);
CREATE INDEX IF NOT EXISTS tasks_updated ON tasks(updated_at);
CREATE TABLE IF NOT EXISTS inbox (account TEXT NOT NULL,update_id INTEGER NOT NULL,PRIMARY KEY(account,update_id));
CREATE TABLE IF NOT EXISTS outbox (id INTEGER PRIMARY KEY AUTOINCREMENT,account TEXT NOT NULL,dedupe_key TEXT NOT NULL,chat_id INTEGER NOT NULL,text TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_at INTEGER NOT NULL DEFAULT 0,sent INTEGER NOT NULL DEFAULT 0,UNIQUE(account,dedupe_key));
CREATE INDEX IF NOT EXISTS outbox_due ON outbox(account,sent,next_at);`)
	if err != nil {
		return err
	}
	return s.AutonomySchema()
}

const jobSelect = `SELECT t.id,t.workspace,t.instruction,t.mode,t.status,t.created_at,t.updated_at,r.owner,r.request_id,r.action,r.report,r.error,r.cancel_requested,COALESCE(c.session_id,'') FROM tasks t JOIN task_runs r ON r.task_id=t.id LEFT JOIN task_context c ON c.task_id=t.id `

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (*Job, error) {
	var j Job
	var report []byte
	var created, updated int64
	err := row.Scan(&j.ID, &j.Workspace, &j.Instruction, &j.Mode, &j.Status, &created, &updated, &j.Owner, &j.RequestID, &j.Action, &report, &j.Error, &j.CancelRequested, &j.SessionID)
	j.Report = report
	j.CreatedAt = time.Unix(0, created)
	j.UpdatedAt = time.Unix(0, updated)
	return &j, err
}

func (s *Store) CreateJob(ctx context.Context, j Job) (*Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	existing, err := scanJob(tx.QueryRowContext(ctx, jobSelect+`WHERE r.owner=? AND r.request_id=?`, j.Owner, j.RequestID))
	if err == nil {
		if existing.Instruction != j.Instruction || existing.Mode != j.Mode || existing.SessionID != j.SessionID {
			return nil, false, errors.New("idempotency key reused with a different task")
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	now := time.Now()
	j.CreatedAt = now
	j.UpdatedAt = now
	j.Status = core.StatusQueued
	j.Action = "run"
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,workspace,instruction,mode,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, j.ID, j.Workspace, j.Instruction, j.Mode, j.Status, now.UnixNano(), now.UnixNano())
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_runs(task_id,owner,request_id,action) VALUES(?,?,?,?)`, j.ID, j.Owner, j.RequestID, j.Action)
	if err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_context(task_id,session_id) VALUES(?,?)", j.ID, j.SessionID); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return &j, true, nil
}
func (s *Store) Job(ctx context.Context, id, owner string) (*Job, error) {
	return scanJob(s.ro.QueryRowContext(ctx, jobSelect+`WHERE t.id=? AND r.owner=?`, id, owner))
}
func (s *Store) Jobs(ctx context.Context, owner string) ([]Job, error) {
	rows, err := s.ro.QueryContext(ctx, jobSelect+`WHERE r.owner=? ORDER BY t.created_at DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}
func (s *Store) Pending(ctx context.Context) (*Job, error) {
	return scanJob(s.ro.QueryRowContext(ctx, jobSelect+`WHERE t.status='queued' ORDER BY t.created_at LIMIT 1`))
}
func (s *Store) Claim(ctx context.Context, id string) (bool, error) {
	r, err := s.db.ExecContext(ctx, `UPDATE tasks SET status='planning',updated_at=? WHERE id=? AND status='queued'`, time.Now().UnixNano(), id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}
func (s *Store) SetJob(ctx context.Context, id string, status core.TaskStatus, report json.RawMessage, cause string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?,updated_at=MAX(COALESCE((SELECT MAX(updated_at) FROM tasks),0)+1,?) WHERE id=?`, status, time.Now().UnixNano(), id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE task_runs SET report=COALESCE(?,report),error=? WHERE task_id=?`, []byte(report), cause, id)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"status": string(status)})
	_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,stage,message,data) VALUES(?,'task_status',?,?)`, id, status, data)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) AddEvent(ctx context.Context, id, stage, message string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO task_events(task_id,stage,message,data) VALUES(?,?,?,?)`, id, stage, message, b)
	return err
}

func (s *Store) SetPhase(ctx context.Context, id string, status core.TaskStatus) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET status=?,updated_at=? WHERE id=?`, status, time.Now().UnixNano(), id)
	return err
}

// AddEventPhase records an event and, when phase is non-empty, advances the task status in one transaction.
func (s *Store) AddEventPhase(ctx context.Context, id, stage, message string, data any, phase core.TaskStatus) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,stage,message,data) VALUES(?,?,?,?)`, id, stage, message, b); err != nil {
		return err
	}
	if phase != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?,updated_at=? WHERE id=?`, phase, time.Now().UnixNano(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Events returns up to 200 events after seq for a task owned by owner, or sql.ErrNoRows if the task is missing or not owned.
func (s *Store) Events(ctx context.Context, id, owner string, after int64) ([]Event, error) {
	rows, err := s.ro.QueryContext(ctx, `SELECT e.seq,e.task_id,e.stage,e.message,e.data FROM task_runs r LEFT JOIN task_events e ON e.task_id=r.task_id AND e.seq>? WHERE r.task_id=? AND r.owner=? ORDER BY e.seq LIMIT 200`, after, id, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []Event
	owned := false
	for rows.Next() {
		owned = true
		var seq sql.NullInt64
		var taskID, stage, message sql.NullString
		var data []byte
		if err = rows.Scan(&seq, &taskID, &stage, &message, &data); err != nil {
			return nil, err
		}
		if !seq.Valid {
			continue
		}
		events = append(events, Event{Seq: seq.Int64, TaskID: taskID.String, Stage: stage.String, Message: message.String, Data: data})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if !owned {
		return nil, sql.ErrNoRows
	}
	return events, nil
}

// RequestCancel flags a non-terminal task for cancellation atomically.
// Missing or foreign tasks return sql.ErrNoRows; already terminal tasks return nil.
func (s *Store) RequestCancel(ctx context.Context, id, owner string) error {
	r, err := s.db.ExecContext(ctx, `UPDATE task_runs SET cancel_requested=1 WHERE task_id=? AND owner=? AND task_id IN (SELECT id FROM tasks WHERE status NOT IN ('succeeded','cancelled'))`, id, owner)
	if err != nil {
		return err
	}
	if n, err := r.RowsAffected(); err != nil || n > 0 {
		return err
	}
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM task_runs WHERE task_id=? AND owner=?`, id, owner).Scan(&one)
}
func (s *Store) Requeue(ctx context.Context, id, owner, action string, allowed ...core.TaskStatus) (*Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, jobSelect+`WHERE t.id=? AND r.owner=?`, id, owner))
	if err != nil {
		return nil, err
	}
	if (j.Status == core.StatusQueued || j.Status == core.StatusPlanning || j.Status == core.StatusApplying) && j.Action == action {
		return j, nil
	}
	if action == "apply" && j.Status == core.StatusSucceeded {
		return j, nil
	}
	ok := false
	for _, status := range allowed {
		ok = ok || j.Status == status
	}
	if !ok {
		return nil, fmt.Errorf("cannot %s task in state %s", action, j.Status)
	}
	_, err = tx.ExecContext(ctx, `UPDATE task_runs SET action=?,cancel_requested=0,error='' WHERE task_id=?`, action, id)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE tasks SET status='queued',updated_at=? WHERE id=?`, time.Now().UnixNano(), id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	j.Status = core.StatusQueued
	j.Action = action
	return j, nil
}
func (s *Store) RecoverJobs(ctx context.Context, applied func(string) bool) error {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id FROM tasks t JOIN task_runs r ON r.task_id=t.id WHERE t.status IN ('queued','planning','analyzing','patching','verifying','applying')`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		status := core.StatusInterrupted
		if applied(id) {
			status = core.StatusSucceeded
		}
		if err = s.SetJob(ctx, id, status, nil, "recovered after service restart"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) TelegramOffset(ctx context.Context, account string) (int64, error) {
	var n sql.NullInt64
	err := s.ro.QueryRowContext(ctx, `SELECT MAX(update_id) FROM inbox WHERE account=?`, account).Scan(&n)
	if n.Valid {
		return n.Int64 + 1, err
	}
	return 0, err
}
func (s *Store) InboxDone(ctx context.Context, account string, id int64) (bool, error) {
	var n int
	err := s.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox WHERE account=? AND update_id=?`, account, id).Scan(&n)
	return n > 0, err
}
func (s *Store) CompleteInbox(ctx context.Context, account string, id, chat int64, reply string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if reply != "" {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO outbox(account,dedupe_key,chat_id,text) VALUES(?,?,?,?)`, account, fmt.Sprintf("update:%d", id), chat, reply)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inbox(account,update_id) VALUES(?,?)`, account, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) QueueDelivery(ctx context.Context, account, key string, chat int64, text string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO outbox(account,dedupe_key,chat_id,text) VALUES(?,?,?,?)`, account, key, chat, text)
	return err
}
func (s *Store) Deliveries(ctx context.Context, account string) ([]Delivery, error) {
	rows, err := s.ro.QueryContext(ctx, `SELECT id,chat_id,text,attempts FROM outbox WHERE account=? AND sent=0 AND next_at<=? ORDER BY id LIMIT 10`, account, time.Now().UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Delivery
	for rows.Next() {
		d := Delivery{Account: account}
		if err = rows.Scan(&d.ID, &d.ChatID, &d.Text, &d.Attempts); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// QueueDeliveries inserts many outbox rows in one transaction; keys[i] is the dedupe key for items[i].
func (s *Store) QueueDeliveries(ctx context.Context, account string, items []Delivery, keys []string) error {
	if len(items) != len(keys) {
		return errors.New("QueueDeliveries: items and keys length mismatch")
	}
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO outbox(account,dedupe_key,chat_id,text) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, d := range items {
		if _, err = stmt.ExecContext(ctx, account, keys[i], d.ChatID, d.Text); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TerminalJobsSince returns up to 100 deliverable-state jobs for owner updated after since (UnixNano), oldest first.
func (s *Store) TerminalJobsSince(ctx context.Context, owner string, since int64) ([]Job, error) {
	rows, err := s.ro.QueryContext(ctx, jobSelect+`WHERE r.owner=? AND t.updated_at>? AND t.status IN ('ready_to_apply','preview_ready','ready','preview','succeeded','failed','cancelled','interrupted','provider_unavailable') ORDER BY t.updated_at LIMIT 100`, owner, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (s *Store) DeliveryResult(ctx context.Context, id int64, sent bool, delay time.Duration) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET sent=?,attempts=attempts+1,next_at=? WHERE id=?`, sent, time.Now().Add(delay).UnixNano(), id)
	return err
}
