package desktop

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/transport"
)

func (s *Server) withAutonomyStore(w http.ResponseWriter, fn func(*config.Config, *store.Store)) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	if svc, ok := s.Backend.(*app.Service); ok && svc != nil && svc.Store != nil {
		fn(cfg, svc.Store)
		return
	}
	apwDir := filepath.Join(cfg.Workspace, ".apw")
	if err := os.MkdirAll(apwDir, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	db, err := store.Open(filepath.Join(apwDir, "tasks.db"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	defer db.Close()
	fn(cfg, db)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	graph, err := surgeon.InspectWorkspaceGraph(cfg.Workspace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "graph_error", err.Error())
		return
	}
	writeJSON(w, graph)
}

func (s *Server) handleGetAutonomy(w http.ResponseWriter, r *http.Request) {
	s.withAutonomyStore(w, func(cfg *config.Config, db *store.Store) {
		ctx := r.Context()
		gov, err := db.Governor(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		goals, err := db.Goals(ctx, "local")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		schedules, err := db.Schedules(ctx, "local")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		episodes, err := db.Episodes(ctx, "local", 20)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		wsSkills := daemon.DiscoverSkills(cfg.Workspace)["workspace"]
		if wsSkills == nil {
			wsSkills = []string{}
		}
		writeJSON(w, map[string]any{
			"governor":      gov,
			"goals":         goals,
			"schedules":     schedules,
			"episodes":      episodes,
			"skills":        wsSkills,
			"skills_loaded": daemon.CountSkills(cfg.Workspace),
		})
	})
}

func (s *Server) handleUpdateGovernor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MaxTasksPerHour        int    `json:"max_tasks_per_hour"`
		MaxConsecutiveFailures int    `json:"max_consecutive_failures"`
		Paused                 bool   `json:"paused"`
		PauseReason            string `json:"pause_reason,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	s.withAutonomyStore(w, func(_ *config.Config, db *store.Store) {
		gov, err := db.UpdateGovernor(r.Context(), body.MaxTasksPerHour, body.MaxConsecutiveFailures, body.Paused, body.PauseReason)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		writeJSON(w, gov)
	})
}

func (s *Server) handleManageGoals(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action      string `json:"action,omitempty"`
		ID          string `json:"id,omitempty"`
		Title       string `json:"title,omitempty"`
		Instruction string `json:"instruction,omitempty"`
		Mode        string `json:"mode,omitempty"`
		Priority    int    `json:"priority,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	s.withAutonomyStore(w, func(_ *config.Config, db *store.Store) {
		ctx := r.Context()
		switch strings.ToLower(strings.TrimSpace(body.Action)) {
		case "delete":
			if strings.TrimSpace(body.ID) == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "id required for delete")
				return
			}
			if err := db.DeleteGoal(ctx, body.ID, "local"); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error())
				return
			}
			writeJSON(w, map[string]any{"deleted": body.ID})
		default:
			if strings.TrimSpace(body.Instruction) == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "instruction is required")
				return
			}
			id := strings.TrimSpace(body.ID)
			if id == "" {
				id = strings.Replace(app.NewID(), "task-", "goal-", 1)
			}
			goal, err := db.CreateGoal(ctx, store.AutonomyGoal{
				ID:          id,
				Owner:       "local",
				Title:       strings.TrimSpace(body.Title),
				Instruction: strings.TrimSpace(body.Instruction),
				Mode:        strings.TrimSpace(body.Mode),
				Priority:    body.Priority,
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if svc, ok := s.Backend.(*app.Service); ok && svc != nil {
				svc.StepAutonomy(ctx)
			}
			writeJSON(w, goal)
		}
	})
}

func (s *Server) handleManageSchedules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action      string `json:"action,omitempty"`
		ID          string `json:"id,omitempty"`
		Name        string `json:"name,omitempty"`
		Instruction string `json:"instruction,omitempty"`
		Mode        string `json:"mode,omitempty"`
		IntervalSec int    `json:"interval_sec,omitempty"`
		Enabled     *bool  `json:"enabled,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	s.withAutonomyStore(w, func(_ *config.Config, db *store.Store) {
		ctx := r.Context()
		switch strings.ToLower(strings.TrimSpace(body.Action)) {
		case "delete":
			if strings.TrimSpace(body.ID) == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "id required for delete")
				return
			}
			if err := db.DeleteSchedule(ctx, body.ID, "local"); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error())
				return
			}
			writeJSON(w, map[string]any{"deleted": body.ID})
		case "toggle":
			if strings.TrimSpace(body.ID) == "" || body.Enabled == nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "id and enabled required for toggle")
				return
			}
			if err := db.ToggleSchedule(ctx, body.ID, "local", *body.Enabled); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error())
				return
			}
			writeJSON(w, map[string]any{"id": body.ID, "enabled": *body.Enabled})
		default:
			if strings.TrimSpace(body.Instruction) == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "instruction is required")
				return
			}
			id := strings.TrimSpace(body.ID)
			if id == "" {
				id = strings.Replace(app.NewID(), "task-", "sched-", 1)
			}
			enabled := true
			if body.Enabled != nil {
				enabled = *body.Enabled
			}
			interval := body.IntervalSec
			if interval <= 0 {
				interval = 3600
			}
			sch, err := db.CreateSchedule(ctx, store.AutonomySchedule{
				ID:          id,
				Owner:       "local",
				Name:        strings.TrimSpace(body.Name),
				Instruction: strings.TrimSpace(body.Instruction),
				Mode:        strings.TrimSpace(body.Mode),
				IntervalSec: interval,
				Enabled:     enabled,
				NextRunAt:   time.Now().Add(time.Duration(interval) * time.Second),
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			writeJSON(w, sch)
		}
	})
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug        string   `json:"slug,omitempty"`
		Title       string   `json:"title,omitempty"`
		Instruction string   `json:"instruction,omitempty"`
		Summary     string   `json:"summary,omitempty"`
		Files       []string `json:"files,omitempty"`
		TaskID      string   `json:"task_id,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}

	instr := strings.TrimSpace(body.Instruction)
	summary := strings.TrimSpace(body.Summary)
	files := body.Files
	title := strings.TrimSpace(body.Title)
	slug := strings.TrimSpace(body.Slug)

	if body.TaskID != "" {
		if api, bErr := s.backend(r.Context()); bErr == nil {
			if job, jErr := api.Get(r.Context(), body.TaskID, "local"); jErr == nil && job != nil {
				if instr == "" {
					instr = job.Instruction
				}
				if len(job.Report) > 0 {
					var rep struct {
						Summary       string   `json:"summary"`
						ModifiedFiles []string `json:"modified_files"`
						Plan          struct {
							Objective     string   `json:"objective"`
							AffectedFiles []string `json:"affected_files"`
						} `json:"plan"`
					}
					if json.Unmarshal(job.Report, &rep) == nil {
						if summary == "" {
							summary = rep.Summary
						}
						if title == "" {
							title = rep.Plan.Objective
						}
						if len(files) == 0 {
							files = append(files, rep.ModifiedFiles...)
							if len(files) == 0 {
								files = append(files, rep.Plan.AffectedFiles...)
							}
						}
					}
				}
				if slug == "" {
					slug = "learned-" + job.ID
				}
			}
		}
	}

	if instr == "" && summary == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "instruction or task_id is required to synthesize a skill")
		return
	}
	if title == "" {
		title = instr
		if r := []rune(title); len(r) > 64 {
			title = string(r[:64])
		}
	}
	if slug == "" {
		slug = title
	}
	if summary == "" {
		summary = instr
	}

	relPath, err := store.SaveProceduralSkill(cfg.Workspace, slug, title, instr, summary, files)
	if errors.Is(err, store.ErrSkillExists) {
		writeError(w, http.StatusConflict, "skill_exists", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "skill_error", err.Error())
		return
	}
	wsSkills := daemon.DiscoverSkills(cfg.Workspace)["workspace"]
	if wsSkills == nil {
		wsSkills = []string{}
	}
	writeJSON(w, map[string]any{
		"path":          relPath,
		"skill_path":    relPath,
		"skills_loaded": daemon.CountSkills(cfg.Workspace),
		"skills":        wsSkills,
	})
}
