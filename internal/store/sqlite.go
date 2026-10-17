package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection for persisting agent tasks (M0 spike).
type Store struct {
	db *sql.DB
	// ro is a read-only handle pool so polling reads never queue behind the single writer connection.
	ro *sql.DB
}

// Open opens or creates a SQLite database at path with WAL mode and busy_timeout enabled.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.JobSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// The ro handle is opened after migrations so the database file and WAL exist.
	roDSN := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	ro, err := sql.Open("sqlite", roDSN)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite read pool %q: %w", path, err)
	}
	ro.SetMaxOpenConns(4)
	if err := ro.Ping(); err != nil {
		_ = ro.Close()
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite read pool %q: %w", path, err)
	}
	s.ro = ro
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id          TEXT PRIMARY KEY,
	workspace   TEXT NOT NULL,
	instruction TEXT NOT NULL,
	mode        TEXT NOT NULL,
	status      TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate tasks table: %w", err)
	}
	return nil
}

// JournalMode returns the active SQLite journal_mode (e.g., "wal").
func (s *Store) JournalMode(ctx context.Context) (string, error) {
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&mode); err != nil {
		return "", err
	}
	return mode, nil
}

// SaveTask inserts or updates a Task in the tasks table.
func (s *Store) SaveTask(ctx context.Context, t core.Task) error {
	const q = `
INSERT INTO tasks (id, workspace, instruction, mode, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	workspace   = excluded.workspace,
	instruction = excluded.instruction,
	mode        = excluded.mode,
	status      = excluded.status,
	updated_at  = excluded.updated_at;`

	now := time.Now()
	createdAt := t.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := t.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err := s.db.ExecContext(ctx, q,
		t.ID,
		t.Workspace,
		t.Instruction,
		t.Mode,
		string(t.Status),
		createdAt.UnixNano(),
		updatedAt.UnixNano(),
	)
	return err
}

// GetTask retrieves a Task by ID.
func (s *Store) GetTask(ctx context.Context, id string) (*core.Task, error) {
	const q = `SELECT id, workspace, instruction, mode, status, created_at, updated_at FROM tasks WHERE id = ?;`
	var (
		t         core.Task
		status    string
		createdNs int64
		updatedNs int64
	)
	err := s.ro.QueryRowContext(ctx, q, id).Scan(
		&t.ID,
		&t.Workspace,
		&t.Instruction,
		&t.Mode,
		&status,
		&createdNs,
		&updatedNs,
	)
	if err != nil {
		return nil, err
	}
	t.Status = core.TaskStatus(status)
	t.CreatedAt = time.Unix(0, createdNs)
	t.UpdatedAt = time.Unix(0, updatedNs)
	return &t, nil
}

// CountTasks returns the total number of rows in the tasks table.
func (s *Store) CountTasks(ctx context.Context) (int, error) {
	var count int
	if err := s.ro.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks;").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Close closes the writer and read-only database handles.
func (s *Store) Close() error {
	var errs []error
	if s.ro != nil {
		errs = append(errs, s.ro.Close())
	}
	if s.db != nil {
		errs = append(errs, s.db.Close())
	}
	return errors.Join(errs...)
}
