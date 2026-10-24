package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/apiwat/apw-agent/internal/session"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/provider/factory"
	"github.com/apiwat/apw-agent/internal/research"
	"github.com/apiwat/apw-agent/internal/sandbox"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/workspace"
)

type Request struct {
	SessionID   string        `json:"session_id,omitempty"`
	RequestID   string        `json:"request_id"`
	Instruction string        `json:"instruction"`
	Mode        core.TaskMode `json:"mode"`
}
type API interface {
	Plan(context.Context, string, string) (*architect.ExecutionPlan, error)
	Submit(context.Context, string, Request) (*store.Job, error)
	Get(context.Context, string, string) (*store.Job, error)
	List(context.Context, string) ([]store.Job, error)
	Events(context.Context, string, string, int64) ([]store.Event, error)
	Cancel(context.Context, string, string) error
	Resume(context.Context, string, string) (*store.Job, error)
	Apply(context.Context, string, string) (*store.Job, error)
}
type Factory func(*config.Config) (*executor.Executor, func(), error)
type Service struct {
	Config       *config.Config
	ConfigPath   string
	Store        *store.Store
	ctx          context.Context
	cancel       context.CancelFunc
	lease        *workspace.Lock
	factory      Factory
	wake         chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
	activeID     string
	activeCancel context.CancelFunc
	failure      error
	cfgMu        sync.Mutex
	cfgCache     *config.Config
	cfgMod       time.Time
	cfgSize      int64
}

// configurePlanning applies apw.json research and planning settings to an executor.
func configurePlanning(e *executor.Executor, cfg *config.Config) {
	e.Research = research.New(cfg.Research)
	e.Proposals = cfg.Planning.Proposals
	e.JudgeProposals = cfg.Planning.Judge
	e.Lookup = cfg.Planning.Lookup
	e.Audit = cfg.Planning.Audit
	if e.Audit == "" {
		e.Audit = executor.AuditModel
	}
	if e.SubagentConcurrency == 0 {
		e.SubagentConcurrency = executor.DefaultSubagentConcurrency
	}
}

// SessionFileID maps a job owner and conversation id to the id of the session file that holds
// the conversation (.apw/sessions/<id>.json).
func SessionFileID(owner, conversation string) string {
	key := owner
	if conversation != "" {
		key += string(rune(0)) + conversation
	}
	sum := sha256.Sum256([]byte(key))
	return "owner-" + hex.EncodeToString(sum[:16])
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "task-" + hex.EncodeToString(b[:])
}

func Open(ctx context.Context, cfg *config.Config, makeExecutor Factory) (*Service, error) {
	copyCfg := *cfg
	root, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	copyCfg.Workspace = root
	lease, err := workspace.Acquire(root, "service")
	if err != nil {
		return nil, err
	}
	if err = surgeon.RecoverTransactions(root); err != nil {
		lease.Close()
		return nil, err
	}
	db, err := store.Open(filepath.Join(root, ".apw", "tasks.db"))
	if err != nil {
		lease.Close()
		return nil, err
	}
	if err = db.RecoverJobs(ctx, func(id string) bool { return surgeon.Applied(root, id) }); err != nil {
		db.Close()
		lease.Close()
		return nil, err
	}
	if makeExecutor == nil {
		makeExecutor = DefaultFactory
	}
	runCtx, cancel := context.WithCancel(ctx)
	s := &Service{Config: &copyCfg, Store: db, ctx: runCtx, cancel: cancel, lease: lease, factory: makeExecutor, wake: make(chan struct{}, 1)}
	s.wg.Add(2)
	go s.worker()
	go s.autonomyLoop()
	return s, nil
}
func DefaultFactory(cfg *config.Config) (*executor.Executor, func(), error) {
	copyCfg := *cfg
	copyCfg.Providers = map[string]config.ProviderConfig{}
	for key, value := range cfg.Providers {
		value.Workspace = cfg.Workspace
		copyCfg.Providers[key] = value
	}
	cfg = &copyCfg
	a := cfg.Providers[cfg.Roles.Architect]
	r := cfg.Providers[cfg.Roles.Reflex]
	arch, err := factory.New(a)
	if err != nil {
		return nil, nil, err
	}
	ref, err := factory.New(r)
	if err != nil {
		arch.Close()
		return nil, nil, err
	}
	e := executor.New(cfg.Workspace, arch, ref, a.Model, r.Model, cfg.Budget.MaxRepairs)
	e.Trusted = cfg.Policy.Trusted
	closeExtras, err := e.ConfigureFailover(cfg)
	if err != nil {
		arch.Close()
		ref.Close()
		return nil, nil, err
	}
	return e, func() { closeExtras(); arch.Close(); ref.Close() }, nil
}
func (s *Service) Close() error {
	s.cancel()
	s.wg.Wait()
	return errors.Join(s.Err(), s.Store.Close(), s.lease.Close())
}
func (s *Service) Done() <-chan struct{} { return s.ctx.Done() }
func (s *Service) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failure
}
func (s *Service) fail(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	if s.failure == nil {
		s.failure = err
	}
	s.mu.Unlock()
	s.cancel()
}
func (s *Service) persistJob(id string, status core.TaskStatus, report json.RawMessage, cause string) {
	if err := s.Store.SetJob(context.Background(), id, status, report, cause); err != nil {
		s.fail(err)
	}
}
func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) Submit(ctx context.Context, owner string, r Request) (*store.Job, error) {
	cfg, refreshErr := s.taskConfig()
	if refreshErr != nil {
		return nil, refreshErr
	}
	if s.ctx.Err() != nil {
		return nil, errors.New("service is shutting down")
	}
	if owner == "" || len(owner) > 128 || len(r.SessionID) > 128 || r.RequestID == "" || len(r.RequestID) > 256 || strings.TrimSpace(r.Instruction) == "" || len(r.Instruction) > 32*1024 || !r.Mode.Valid() {
		return nil, errors.New("owner, request ID, instruction, and valid mode are required")
	}
	if (r.Mode == core.ModeExecute || r.Mode == core.ModeVerify) && !cfg.Policy.Trusted {
		return nil, errors.New("workspace is not trusted for build/test; use review/preview or apw trust")
	}
	j, _, err := s.Store.CreateJob(ctx, store.Job{Task: core.Task{ID: NewID(), Workspace: s.Config.Workspace, Instruction: r.Instruction, Mode: string(r.Mode)}, Owner: owner, RequestID: r.RequestID, SessionID: r.SessionID})
	if err == nil {
		s.signal()
	}
	return j, err
}
func (s *Service) Get(ctx context.Context, id, owner string) (*store.Job, error) {
	return s.Store.Job(ctx, id, owner)
}
func (s *Service) List(ctx context.Context, owner string) ([]store.Job, error) {
	return s.Store.Jobs(ctx, owner)
}
func (s *Service) Events(ctx context.Context, id, owner string, after int64) ([]store.Event, error) {
	return s.Store.Events(ctx, id, owner, after)
}
func (s *Service) Cancel(ctx context.Context, id, owner string) error {
	if err := s.Store.RequestCancel(ctx, id, owner); err != nil {
		return err
	}
	job, err := s.Store.Job(ctx, id, owner)
	if err != nil {
		return err
	}
	switch job.Status {
	case core.StatusReady, core.StatusPreview, core.StatusFailed, core.StatusInterrupted:
		return s.Store.SetJob(ctx, id, core.StatusCancelled, nil, "cancelled by owner")
	}
	s.mu.Lock()
	if s.activeID == id && s.activeCancel != nil {
		s.activeCancel()
	}
	s.mu.Unlock()
	s.signal()
	return nil
}
func (s *Service) Resume(ctx context.Context, id, owner string) (*store.Job, error) {
	cfg, refreshErr := s.taskConfig()
	if refreshErr != nil {
		return nil, refreshErr
	}
	if !cfg.Policy.Trusted {
		return nil, errors.New("workspace is not trusted for build/test")
	}
	job, err := s.Store.Job(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	action := "resume"
	if _, err := executor.CheckpointForTask(s.Config.Workspace, id); err != nil {
		if job.Status == core.StatusFailed || job.Status == core.StatusInterrupted || job.Status == core.StatusCancelled {
			action = "run"
		} else {
			return nil, err
		}
	}
	j, err := s.Store.Requeue(ctx, id, owner, action, core.StatusInterrupted, core.StatusFailed, core.StatusCancelled, core.StatusPreview, core.StatusReady)
	if err == nil {
		s.signal()
	}
	return j, err
}
func (s *Service) Apply(ctx context.Context, id, owner string) (*store.Job, error) {
	job, err := s.Store.Job(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if job.Status == core.StatusSucceeded {
		if !surgeon.Applied(s.Config.Workspace, id) {
			return nil, errors.New("completed review/verify has no verified source changes to apply")
		}
		return job, nil
	}
	cp, err := executor.CheckpointForTask(s.Config.Workspace, id)
	if err != nil {
		return nil, err
	}
	if cp.Phase != "ready" && cp.Phase != "applied" {
		return nil, errors.New("task is not ready to apply")
	}
	j, err := s.Store.Requeue(ctx, id, owner, "apply", core.StatusReady, core.StatusFailed, core.StatusInterrupted)
	if err == nil {
		s.signal()
	}
	return j, err
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		}
		for s.ctx.Err() == nil {
			j, err := s.Store.Pending(s.ctx)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) && s.ctx.Err() == nil {
					s.fail(err)
				}
				break
			}
			claimed, err := s.Store.Claim(s.ctx, j.ID)
			if err != nil {
				if s.ctx.Err() == nil {
					s.fail(err)
				}
				break
			}
			if !claimed {
				break
			}
			s.run(j)
		}
	}
}
func (s *Service) run(j *store.Job) {
	cfg, cfgErr := s.taskConfig()
	if cfgErr != nil {
		s.persistJob(j.ID, core.StatusFailed, nil, cfgErr.Error())
		return
	}
	timeout := time.Duration(cfg.Budget.TaskTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	s.mu.Lock()
	s.activeID = j.ID
	s.activeCancel = cancel
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.activeID = ""; s.activeCancel = nil; s.mu.Unlock() }()
	current, err := s.Store.Job(ctx, j.ID, j.Owner)
	if err != nil {
		if ctx.Err() == nil {
			s.fail(err)
		}
		s.persistJob(j.ID, core.StatusInterrupted, nil, err.Error())
		return
	}
	if current.CancelRequested {
		s.persistJob(j.ID, core.StatusCancelled, nil, "cancelled before start")
		return
	}
	var e *executor.Executor
	closeProvider := func() {}
	if core.TaskMode(j.Mode) == core.ModeVerify || j.Action == "apply" {
		e = executor.New(cfg.Workspace, nil, nil, "", "", cfg.Budget.MaxRepairs)
	} else {
		providerCfg := cfg
		if core.TaskMode(j.Mode) == core.ModeReview {
			providerCfg = readOnlyConfig(cfg)
		}
		e, closeProvider, err = s.factory(providerCfg)
	}
	sessionID := SessionFileID(j.Owner, j.SessionID)
	sess, loadErr := session.Load(cfg.Workspace, sessionID)
	if loadErr != nil {
		if !errors.Is(loadErr, os.ErrNotExist) {
			s.persistJob(j.ID, core.StatusFailed, nil, "conversation cannot be loaded; saved file preserved: "+loadErr.Error())
			return
		}
		sess = session.New(cfg.Workspace)
		sess.ID = sessionID
	}
	if sess.Owner == "" {
		sess.Owner = j.Owner
	}
	if sess.Conversation == "" {
		sess.Conversation = j.SessionID
	}
	if j.Action == "run" && j.SessionID != "" {
		sess.RecordTurn(session.Turn{
			TaskID:      j.ID,
			Timestamp:   current.CreatedAt,
			Instruction: j.Instruction,
		})
		_ = sess.Save()
	}
	var report *executor.ExecutionReport
	if err == nil {
		defer closeProvider()
		e.Session = sess
		e.VerifyOverride = project.FromConfig(cfg.Verify)
		e.WorktreeRoot = cfg.WorktreeRoot
		e.Sandbox = sandbox.FromConfig(cfg.Sandbox)
		e.BrowserCheck = cfg.Verify.Browser
		e.EpisodicContext = s.Store.RecallEpisodicContext(ctx, j.Owner, j.Instruction, 5)
		configurePlanning(e, cfg)

		e.TaskID = j.ID
		e.Mode = core.TaskMode(j.Mode)
		e.Trusted = cfg.Policy.Trusted
		e.OnProgress = func(ev executor.ProgressEvent) {
			status := core.TaskStatus("")
			switch ev.Stage {
			case "analyzing", "reviewing":
				status = core.StatusAnalyzing
			case "planning", "replanning", "planning_attempt", "replanning_attempt", "research", "research_attempt", "research_source", "research_done", "proposals", "proposal", "proposal_chosen", "proposal_judge", "plan_ready", "planning_fallback", "research_fallback":
				status = core.StatusPlanning
			case "step_start", "step_done", "directive_attempt", "step_directive_fallback", "patch_retry", "subagent_wave", "subagent_start", "subagent_done", "worktree_start", "worktree_ready":
				status = core.StatusPatching
			case "verifying", "baseline":
				status = core.StatusVerifying
			case "applying":
				status = core.StatusApplying
			}
			evCtx, evCancel := context.WithTimeout(context.Background(), 5*time.Second)
			eventErr := s.Store.AddEventPhase(evCtx, j.ID, ev.Stage, ev.Message, ev, status)
			evCancel()
			if eventErr != nil {
				s.fail(eventErr)
				cancel()
			}
		}
		if j.Action != "apply" {
			e.OnProgress(executor.ProgressEvent{
				Stage:   "analyzing",
				Message: "เริ่มดำเนินการ: กำลังวิเคราะห์โปรเจกต์และเตรียมสภาพแวดล้อม...",
			})
		}
		if core.TaskMode(j.Mode) == core.ModeReview && j.Action != "apply" {
			closeFallbacks, skipped := e.AddArchitectFallbacks(cfg)
			defer closeFallbacks()
			for _, skipErr := range skipped {
				e.OnProgress(executor.ProgressEvent{Stage: "review_fallback", Message: "Unavailable fallback skipped: " + skipErr.Error()})
			}
		}
		if j.Action == "apply" || j.Action == "resume" {
			cp, loadErr := executor.CheckpointForTask(s.Config.Workspace, j.ID)
			if loadErr != nil {
				err = loadErr
			} else if j.Action == "apply" {
				s.persistJob(j.ID, core.StatusApplying, nil, "")
				report, err = e.Apply(ctx, *cp)
			} else {
				e.Mode = core.ModeExecute
				if cp.Mode == core.ModeUnverified {
					e.Mode = core.ModeUnverified
				}
				report, err = e.Resume(ctx, *cp)
			}
		} else {
			report, err = e.Execute(ctx, j.Instruction, "")
		}
	}
	status := core.StatusFailed
	if report != nil && err == nil {
		switch {
		case report.Applied:
			status = core.StatusSucceeded
		case (report.Mode == core.ModeReview || report.Mode == core.ModeVerify) && report.Passed:
			status = core.StatusSucceeded
		case report.Mode == core.ModePreview:
			status = core.StatusPreview
		case report.Passed:
			status = core.StatusReady
		}
	}
	latest, _ := s.Store.Job(context.Background(), j.ID, j.Owner)
	if latest != nil && latest.CancelRequested && !surgeon.Applied(s.Config.Workspace, j.ID) {
		status = core.StatusCancelled
	}
	if err != nil {
		if latest != nil && latest.CancelRequested {
			status = core.StatusCancelled
		} else if s.ctx.Err() != nil {
			status = core.StatusInterrupted
		}
	}
	if sess != nil && (report != nil || err != nil) {
		turn := session.Turn{TaskID: j.ID, Timestamp: current.CreatedAt, Instruction: j.Instruction}
		if report != nil {
			turn.ModifiedFiles = report.ModifiedFiles
			turn.Summary = report.Summary
			turn.Success = report.Passed
			turn.Applied = report.Applied
			if report.Plan != nil {
				turn.PlanID = report.Plan.PlanID
				turn.TotalSteps = len(report.Plan.Subtasks)
			}
			turn.CompletedSteps = len(report.Steps)
		}
		if err != nil {
			turn.Error = err.Error()
			if turn.Summary == "" {
				turn.Summary = err.Error()
			}
		}
		sess.RecordTurn(turn)
		if saveErr := sess.Save(); saveErr != nil {
			err = errors.Join(err, saveErr)
			status = core.StatusFailed
		}
	}
	var data []byte
	if report != nil {
		data, _ = json.Marshal(report)
	}
	cause := ""
	if status == core.StatusFailed && err == nil && report != nil {
		var unmet []string
		for _, check := range report.Acceptance {
			if !check.Passed {
				unmet = append(unmet, check.Criterion)
			}
		}
		if len(unmet) > 0 {
			cause = "acceptance not satisfied: " + strings.Join(unmet, "; ")
		} else if len(report.ReflexAttempts) > 0 {
			if last := report.ReflexAttempts[len(report.ReflexAttempts)-1].Result; last != nil && !last.Passed {
				cause = "verification failed at stage " + last.FailedStage
				if len(last.Diagnostics) > 0 && last.Diagnostics[0].Message != "" {
					cause += ": " + last.Diagnostics[0].Message
				}
			}
		}
	}
	if err != nil {
		cause = err.Error()
	}
	s.persistJob(j.ID, status, data, cause)
	s.recordAutonomyAndMemory(j, status, report, cause)
}

var _ API = (*Service)(nil)

func (s *Service) Plan(ctx context.Context, owner, instruction string) (*architect.ExecutionPlan, error) {
	if owner == "" || strings.TrimSpace(instruction) == "" || len(instruction) > 32*1024 {
		return nil, errors.New("instruction required")
	}
	cfg, err := s.taskConfig()
	if err != nil {
		return nil, err
	}
	e, closeProvider, err := s.factory(readOnlyConfig(cfg))
	if err != nil {
		return nil, err
	}
	defer closeProvider()
	closeFallbacks, _ := e.AddArchitectFallbacks(cfg) // unusable fallbacks must not block planning
	defer closeFallbacks()
	e.VerifyOverride = project.FromConfig(cfg.Verify)
	e.BrowserCheck = cfg.Verify.Browser
	configurePlanning(e, cfg)
	return e.PlanTask(ctx, core.Task{ID: NewID(), Workspace: s.Config.Workspace, Instruction: instruction, Mode: "preview"}, executor.ProjectOverview(cfg.Workspace, executor.ConfiguredProfile(cfg.Workspace, cfg.Verify)))
}

// taskConfig returns the effective per-task config. The parsed file and the
// workspace identity check are cached keyed on the config file's ModTime+Size;
// callers always receive their own shallow copy.
func (s *Service) taskConfig() (*config.Config, error) {
	if s.ConfigPath == "" {
		return s.Config, nil
	}
	info, statErr := os.Stat(s.ConfigPath)
	if statErr == nil {
		s.cfgMu.Lock()
		if s.cfgCache != nil && s.cfgMod.Equal(info.ModTime()) && s.cfgSize == info.Size() {
			cp := *s.cfgCache
			s.cfgMu.Unlock()
			return &cp, nil
		}
		s.cfgMu.Unlock()
	}
	cfg, err := s.loadTaskConfig()
	if err != nil {
		return nil, err
	}
	if statErr == nil {
		cached := *cfg
		s.cfgMu.Lock()
		s.cfgCache, s.cfgMod, s.cfgSize = &cached, info.ModTime(), info.Size()
		s.cfgMu.Unlock()
	}
	return cfg, nil
}

func (s *Service) loadTaskConfig() (*config.Config, error) {
	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(cfg.Workspace) {
		cfg.Workspace = filepath.Join(filepath.Dir(s.ConfigPath), cfg.Workspace)
	}
	cfg.Workspace, err = filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	a, e1 := os.Stat(cfg.Workspace)
	b, e2 := os.Stat(s.Config.Workspace)
	if e1 != nil || e2 != nil || !os.SameFile(a, b) {
		return nil, errors.New("workspace changed; restart daemon for the new workspace")
	}
	return cfg, nil
}

// readOnlyConfig only initializes the selected architect for review/planning.
// Unused repair providers or their missing credentials must not block read-only work.
// Architect fallbacks are added afterwards by Executor.AddArchitectFallbacks, which
// skips any fallback that cannot be constructed.
func readOnlyConfig(cfg *config.Config) *config.Config {
	copyCfg := *cfg
	copyCfg.Roles.Reflex = cfg.Roles.Architect
	copyCfg.Failover.Reflex = nil
	copyCfg.Failover.Architect = nil
	return &copyCfg
}
