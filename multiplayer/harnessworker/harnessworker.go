// Package harnessworker adapts the existing core Harness for epsilond workers.
package harnessworker

import (
	"context"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/core"
	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
	"github.com/shxntanu/epsilon/multiplayer/sandbox"
)

// Config controls one read-only harness worker invocation.
type Config struct {
	WorkspaceRoot string
	SessionDir    string
	Provider      types.Provider
	Model         string
	Effort        string
	SystemPrompt  string
}

// Runner executes worker tasks using the existing core Harness.
type Runner struct {
	cfg Config
}

// New returns a harness-backed worker runner.
func New(cfg Config) (*Runner, error) {
	if strings.TrimSpace(cfg.WorkspaceRoot) == "" {
		return nil, fmt.Errorf("workspace root is empty")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}
	return &Runner{cfg: cfg}, nil
}

// Run executes a task with a read-only Epsilon harness tool set.
func (r *Runner) Run(ctx context.Context, spec sandbox.TaskSpec) (sandbox.Result, error) {
	if err := sandbox.ValidateTaskSpec(spec); err != nil {
		return sandbox.Result{}, err
	}
	h, err := r.newHarness()
	if err != nil {
		return sandbox.Result{}, err
	}
	sess, err := h.StartSession(ctx)
	if err != nil {
		return sandbox.Result{}, fmt.Errorf("start harness session: %w", err)
	}
	defer func() { _ = h.CloseSession(context.WithoutCancel(ctx), sess.ID()) }()

	if err := sess.Send(ctx, types.UserMessage(spec.Instruction)); err != nil {
		return sandbox.Result{}, fmt.Errorf("send worker instruction: %w", err)
	}
	if err := sess.Step(ctx); err != nil {
		return sandbox.Result{
			Status: StatusFromContext(ctx),
			Error:  err.Error(),
		}, fmt.Errorf("run harness step: %w", err)
	}

	messages := sess.Messages()
	summary := lastAssistantText(messages)
	if summary == "" {
		summary = "Harness completed without assistant text."
	}
	return sandbox.Result{
		Status:     sandbox.StatusSucceeded,
		Summary:    summary,
		Confidence: 1,
	}, nil
}

func (r *Runner) newHarness() (*core.Harness, error) {
	opts := []core.Option{
		core.WithProvider(r.cfg.Provider),
		core.WithPermissionBroker(permissions.NewDenyBroker()),
		core.WithTools(readOnlyTools(r.cfg.WorkspaceRoot)...),
	}
	if r.cfg.Model != "" || r.cfg.Effort != "" || r.cfg.SystemPrompt != "" {
		opts = append(opts, core.WithRuntimeSettings(typesToSettings(r.cfg)))
	}
	if strings.TrimSpace(r.cfg.SessionDir) != "" {
		store, err := events.NewJSONLStore(r.cfg.SessionDir)
		if err != nil {
			return nil, fmt.Errorf("create worker event store: %w", err)
		}
		opts = append(opts, core.WithEventStore(store))
	}
	h, err := core.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create harness: %w", err)
	}
	return h, nil
}

func readOnlyTools(workspaceRoot string) []tools.Tool {
	var result []tools.Tool
	if tool, err := tools.NewReadFileTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewGrepTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewRipgrepTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewListDirTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewFileTreeTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewGitStatusTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	if tool, err := tools.NewGitDiffTool(workspaceRoot); err == nil {
		result = append(result, tool)
	}
	return result
}

func typesToSettings(cfg Config) harnessconfig.Settings {
	return harnessconfig.Settings{
		Model:        cfg.Model,
		Effort:       cfg.Effort,
		SystemPrompt: cfg.SystemPrompt,
	}
}

// StatusFromContext maps context cancellation into a sandbox status.
func StatusFromContext(ctx context.Context) sandbox.Status {
	if ctx.Err() != nil {
		return sandbox.StatusCancelled
	}
	return sandbox.StatusFailed
}

func lastAssistantText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != types.RoleAssistant {
			continue
		}
		var b strings.Builder
		for _, part := range messages[i].Content {
			if part.Text != "" {
				b.WriteString(part.Text)
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}
