package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

// TaskSummary is the retention view of a task: identity, state and age, without the report blob.
type TaskSummary struct {
	ID        string
	Mode      string
	Action    string
	Status    core.TaskStatus
	UpdatedAt time.Time
}

// TaskSummaries returns every task across owners, oldest change first.
func (s *Store) TaskSummaries(ctx context.Context) ([]TaskSummary, error) {
	rows, err := s.ro.QueryContext(ctx, `SELECT t.id,t.mode,COALESCE(r.action,''),t.status,t.updated_at FROM tasks t LEFT JOIN task_runs r ON r.task_id=t.id ORDER BY t.updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TaskSummary
	for rows.Next() {
		var t TaskSummary
		var updated int64
		if err = rows.Scan(&t.ID, &t.Mode, &t.Action, &t.Status, &updated); err != nil {
			return nil, err
		}
		t.UpdatedAt = time.Unix(0, updated)
		result = append(result, t)
	}
	return result, rows.Err()
}

// PruneRequest selects retained rows to delete. A zero SentBefore or a non-positive InboxKeep disables that rule.
type PruneRequest struct {
	TaskIDs    []string
	SentBefore time.Time
	InboxKeep  int
	DryRun     bool
}

// PruneResult counts rows deleted (or, for a dry run, rows that would be deleted).
type PruneResult struct {
	Events  int64 `json:"events"`
	Reports int64 `json:"reports"`
	Outbox  int64 `json:"outbox"`
	Inbox   int64 `json:"inbox"`
}

// prunableTasks re-checks terminal state inside the statement so a task requeued after selection is never pruned.
const prunableTasks = `SELECT t.id FROM tasks t WHERE t.id IN (SELECT value FROM json_each(?)) AND t.status IN ('succeeded','failed','cancelled','provider_unavailable')`

const retentionStage = "retention"

const (
	pruneEventsWhere  = `FROM task_events WHERE stage<>'` + retentionStage + `' AND task_id IN (` + prunableTasks + `)`
	pruneReportsWhere = `FROM task_runs WHERE report IS NOT NULL AND task_id IN (` + prunableTasks + `)`
	pruneOutboxWhere  = `FROM outbox WHERE sent=1 AND next_at<?`
	pruneInboxWhere   = `FROM inbox WHERE rowid IN (SELECT rowid FROM (SELECT rowid,ROW_NUMBER() OVER (PARTITION BY account ORDER BY update_id DESC) AS rn FROM inbox) WHERE rn>?)`
)

// Prune deletes task events, clears task reports, removes delivered outbox rows and trims the inbox in one transaction.
// Task rows, statuses, errors and undelivered outbox rows are kept. A dry run only counts, through the read-only pool.
func (s *Store) Prune(ctx context.Context, req PruneRequest) (PruneResult, error) {
	ids, err := json.Marshal(append([]string{}, req.TaskIDs...))
	if err != nil {
		return PruneResult{}, err
	}
	if req.DryRun {
		return s.countPrunable(ctx, req, ids)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PruneResult{}, err
	}
	defer tx.Rollback()
	var result PruneResult
	exec := func(target *int64, query string, args ...any) error {
		r, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		*target, err = r.RowsAffected()
		return err
	}
	if len(req.TaskIDs) > 0 {
		if err = exec(&result.Events, `DELETE `+pruneEventsWhere, ids); err != nil {
			return PruneResult{}, err
		}
		if err = exec(&result.Reports, `UPDATE task_runs SET report=NULL WHERE report IS NOT NULL AND task_id IN (`+prunableTasks+`)`, ids); err != nil {
			return PruneResult{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,stage,message) SELECT id,'`+retentionStage+`','events and report pruned by retention policy' FROM (`+prunableTasks+`) p WHERE NOT EXISTS (SELECT 1 FROM task_events e WHERE e.task_id=p.id AND e.stage='`+retentionStage+`')`, ids); err != nil {
			return PruneResult{}, err
		}
	}
	if !req.SentBefore.IsZero() {
		if err = exec(&result.Outbox, `DELETE `+pruneOutboxWhere, req.SentBefore.UnixNano()); err != nil {
			return PruneResult{}, err
		}
	}
	if req.InboxKeep > 0 {
		if err = exec(&result.Inbox, `DELETE `+pruneInboxWhere, req.InboxKeep); err != nil {
			return PruneResult{}, err
		}
	}
	return result, tx.Commit()
}

func (s *Store) countPrunable(ctx context.Context, req PruneRequest, ids []byte) (PruneResult, error) {
	var result PruneResult
	count := func(target *int64, query string, args ...any) error {
		return s.ro.QueryRowContext(ctx, `SELECT COUNT(*) `+query, args...).Scan(target)
	}
	if len(req.TaskIDs) > 0 {
		if err := count(&result.Events, pruneEventsWhere, ids); err != nil {
			return PruneResult{}, err
		}
		if err := count(&result.Reports, pruneReportsWhere, ids); err != nil {
			return PruneResult{}, err
		}
	}
	if !req.SentBefore.IsZero() {
		if err := count(&result.Outbox, pruneOutboxWhere, req.SentBefore.UnixNano()); err != nil {
			return PruneResult{}, err
		}
	}
	if req.InboxKeep > 0 {
		if err := count(&result.Inbox, pruneInboxWhere, req.InboxKeep); err != nil {
			return PruneResult{}, err
		}
	}
	return result, nil
}

// Compact returns freed pages to the filesystem: incremental_vacuum when the database uses incremental
// auto-vacuum, a full VACUUM only when requested, then a WAL truncation checkpoint. It reports what ran.
func (s *Store) Compact(ctx context.Context, full bool) (string, error) {
	var mode int
	if err := s.db.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return "", err
	}
	action := "none"
	switch {
	case full:
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return "", err
		}
		action = "vacuum"
	case mode == 2:
		// incremental_vacuum returns no rows; drain it through Exec.
		if _, err := s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
			return "", err
		}
		action = "incremental_vacuum"
	}
	var busy, logFrames, checkpointed sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		return action, err
	}
	return action, nil
}
