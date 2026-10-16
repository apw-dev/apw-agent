package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/provider/api"
	"github.com/apiwat/apw-agent/internal/provider/local"
)

// New instantiates a provider.Provider from a ProviderConfig.
func New(cfg config.ProviderConfig) (provider.Provider, error) {
	switch cfg.Type {
	case config.ProviderAPI:
		return api.New(cfg)
	case config.ProviderLocal:
		return local.New(cfg)
	case config.ProviderMock:
		return &builtinEngineProvider{model: cfg.Model}, nil
	default:
		return nil, fmt.Errorf("factory: unsupported provider type %q", cfg.Type)
	}
}

// builtinEngineProvider provides an intelligent built-in local heuristic planner when no external LLM API is configured.
type builtinEngineProvider struct {
	model string
}

func (b *builtinEngineProvider) Name() string {
	return "apw-builtin"
}

func (b *builtinEngineProvider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	instruction := extractLineValue(req.Prompt, "Instruction:")
	if instruction == "" {
		instruction = "ตรวจสอบและวิเคราะห์โปรเจกต์"
	}

	files := extractWorkspaceFiles(req.Prompt)
	var subtasks []architect.Subtask
	if len(files) > 0 {
		subtasks = append(subtasks, architect.Subtask{
			ID:          "step-1",
			Description: fmt.Sprintf("อ่านและตรวจสอบโครงสร้างไฟล์หลัก (%s)", files[0]),
			Action:      architect.ActionRead,
			TargetFile:  files[0],
			Instruction: "Read and snapshot file integrity before verification",
		})
	}
	subtasks = append(subtasks, architect.Subtask{
		ID:          fmt.Sprintf("step-%d", len(subtasks)+1),
		Description: "รันวงจรตรวจสอบคุณภาพโค้ด (gofmt -> go vet -> go build -> go test)",
		Action:      architect.ActionVerify,
		Instruction: "Run closed-loop Reflex verification",
	})

	affected := files
	if len(affected) > 4 {
		affected = affected[:4]
	}

	planObj := map[string]any{
		"objective":           fmt.Sprintf("ดำเนินการตามคำสั่ง: %s", instruction),
		"affected_files":      affected,
		"subtasks":            subtasks,
		"acceptance_criteria": []string{"gofmt, go vet, go build และ go test ผ่านทั้งหมด"},
	}
	planBytes, _ := json.MarshalIndent(planObj, "", "  ")
	content := "```json\n" + string(planBytes) + "\n```"

	modelName := b.model
	if modelName == "" {
		modelName = "APW Engine (Built-in)"
	}

	return &core.ModelResponse{
		RequestID:     req.RequestID,
		Provider:      b.Name(),
		ModelObserved: modelName,
		Content:       content,
		Completed:     true,
		Timestamp:     time.Now(),
	}, nil
}

func (b *builtinEngineProvider) Close() error {
	return nil
}

func extractLineValue(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func extractWorkspaceFiles(text string) []string {
	var files []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "File:") {
			f := strings.TrimSpace(strings.TrimPrefix(line, "File:"))
			if f != "" && !strings.HasPrefix(f, ".") && !strings.Contains(f, "..") {
				files = append(files, f)
			}
		}
	}
	return files
}
