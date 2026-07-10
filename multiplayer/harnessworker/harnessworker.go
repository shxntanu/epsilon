// Package harnessworker adapts the existing core Harness for epsilond workers.
package harnessworker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shxntanu/epsilon/core"
	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/types"
	"github.com/shxntanu/epsilon/multiplayer/sandbox"
)

const defaultWorkerSystemPrompt = `You are an Epsilon repo-inspection worker.

You are running in read-only launch mode. You may inspect the workspace with available tools, but you must not modify files, run shell commands, apply patches, install packages, commit changes, or ask for interactive approval.

Return concise findings only. Prefer evidence refs that name files, paths, commands, or tool calls instead of pasting large logs. If you cannot complete the task with read-only inspection, say what is blocked and what additional execution would be required.`

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
	workspaceRoot := r.workspaceRootForSpec(spec)
	h, err := r.newHarness(workspaceRoot)
	if err != nil {
		return sandbox.Result{}, err
	}
	sess, err := h.StartSession(ctx)
	if err != nil {
		return sandbox.Result{}, fmt.Errorf("start harness session: %w", err)
	}
	defer func() { _ = h.CloseSession(context.WithoutCancel(ctx), sess.ID()) }()

	if err := sess.Send(ctx, types.UserMessage(workerPrompt(spec))); err != nil {
		return sandbox.Result{}, fmt.Errorf("send worker instruction: %w", err)
	}
	if err := sess.Step(ctx); err != nil {
		return sandbox.Result{
			Status: statusFromContext(ctx),
			Error:  err.Error(),
		}, fmt.Errorf("run harness step: %w", err)
	}

	result := resultFromMessages(sess.Messages())
	result.ResultPointer = spec.ResultPath
	if err := writeResultFile(spec.ResultPath, result); err != nil {
		return sandbox.Result{}, err
	}
	return result, nil
}

func (r *Runner) workspaceRootForSpec(spec sandbox.TaskSpec) string {
	if strings.TrimSpace(spec.WorkspaceRoot) != "" {
		if info, err := os.Stat(spec.WorkspaceRoot); err == nil && info.IsDir() {
			return spec.WorkspaceRoot
		}
	}
	if strings.TrimSpace(spec.WorkingDir) != "" {
		if info, err := os.Stat(spec.WorkingDir); err == nil && info.IsDir() {
			return spec.WorkingDir
		}
	}
	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.Source) == "" {
			continue
		}
		if info, err := os.Stat(mount.Source); err == nil && info.IsDir() {
			return mount.Source
		}
	}
	return r.cfg.WorkspaceRoot
}

func (r *Runner) newHarness(workspaceRoot string) (*core.Harness, error) {
	systemPrompt := strings.TrimSpace(r.cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultWorkerSystemPrompt
	}
	opts := []core.Option{
		core.WithProvider(r.cfg.Provider),
		core.WithWorkspaceContext(workspaceRoot),
		core.WithPermissionBroker(permissions.NewDenyBroker()),
		core.WithReadOnlyDefaultTools(workspaceRoot),
		core.WithRuntimeSettings(harnessconfig.Settings{
			Model:        r.cfg.Model,
			Effort:       r.cfg.Effort,
			SystemPrompt: systemPrompt,
		}),
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

func workerPrompt(spec sandbox.TaskSpec) string {
	var b strings.Builder
	b.WriteString("Worker task:\n")
	b.WriteString(strings.TrimSpace(spec.Instruction))
	b.WriteString("\n\nConstraints:\n")
	b.WriteString("- Use only the available read-only tools.\n")
	b.WriteString("- Do not modify files, run shell commands, apply patches, install dependencies, or commit changes.\n")
	b.WriteString("- Keep the answer compact and cite files/tool evidence by reference.\n")
	b.WriteString("- If exact verification requires execution, report it as blocked instead of guessing.\n")
	b.WriteString("\nReturn your final answer as JSON with this shape:\n")
	b.WriteString(`{"status":"succeeded|failed|blocked","summary":"...","evidence_refs":["..."],"confidence":0.0,"open_questions":["..."]}`)
	b.WriteString("\n")
	if spec.TaskID != "" || spec.ThreadID != "" || spec.WorkerRole != "" {
		b.WriteString("\nTask metadata:\n")
		if spec.TaskID != "" {
			b.WriteString("- task_id: " + spec.TaskID + "\n")
		}
		if spec.ThreadID != "" {
			b.WriteString("- thread_id: " + spec.ThreadID + "\n")
		}
		if spec.WorkerRole != "" {
			b.WriteString("- worker_role: " + spec.WorkerRole + "\n")
		}
	}
	return b.String()
}

func resultFromMessages(messages []types.Message) sandbox.Result {
	text := lastAssistantText(messages)
	var file sandbox.WorkerResultFile
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &file); err != nil {
		summary := strings.TrimSpace(text)
		if summary == "" {
			summary = "Harness completed without assistant text."
		}
		return sandbox.Result{
			Status:       sandbox.StatusSucceeded,
			Summary:      summary,
			EvidenceRefs: evidenceRefs(messages),
			Confidence:   1,
		}
	}
	result := sandbox.Result{
		Status:        normalizeStatus(file.Status),
		Summary:       file.Summary,
		EvidenceRefs:  file.EvidenceRefs,
		Confidence:    file.Confidence,
		OpenQuestions: file.OpenQuestions,
		Usage:         file.Usage,
		ExitCode:      file.ExitCode,
		Error:         file.Error,
	}
	if result.Summary == "" {
		result.Summary = strings.TrimSpace(text)
	}
	if result.Confidence == 0 {
		result.Confidence = 1
	}
	if len(result.EvidenceRefs) == 0 {
		result.EvidenceRefs = evidenceRefs(messages)
	}
	return result
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end >= start {
		return text[start : end+1]
	}
	return text
}

func normalizeStatus(status sandbox.Status) sandbox.Status {
	switch status {
	case sandbox.StatusSucceeded, sandbox.StatusFailed, sandbox.StatusCancelled, sandbox.StatusSkipped:
		return status
	case sandbox.Status("blocked"):
		return sandbox.StatusFailed
	default:
		return sandbox.StatusSucceeded
	}
}

func evidenceRefs(messages []types.Message) []string {
	seen := map[string]bool{}
	refs := make([]string, 0)
	for _, message := range messages {
		if message.Role != types.RoleTool {
			continue
		}
		ref := evidenceRef(message)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs
}

func evidenceRef(message types.Message) string {
	parts := []string{message.ToolName}
	if path := strings.TrimSpace(message.ToolResultMetadata["path"]); path != "" {
		parts = append(parts, path)
	}
	if workspace := strings.TrimSpace(message.ToolResultMetadata["workspace"]); workspace != "" {
		parts = append(parts, "workspace="+workspace)
	}
	if truncated := strings.TrimSpace(message.ToolResultMetadata["truncated"]); truncated == "true" {
		parts = append(parts, "truncated=true")
	}
	if len(message.Content) > 0 {
		text := strings.TrimSpace(message.Content[0].Text)
		if text != "" && len(parts) == 1 {
			parts = append(parts, truncateEvidence(text, 160))
		}
	}
	return strings.TrimSpace(strings.Join(parts, ": "))
}

func truncateEvidence(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func writeResultFile(path string, result sandbox.Result) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	body, err := json.MarshalIndent(sandbox.WorkerResultFile{
		Status:        result.Status,
		Summary:       result.Summary,
		EvidenceRefs:  result.EvidenceRefs,
		Confidence:    result.Confidence,
		OpenQuestions: result.OpenQuestions,
		Usage:         result.Usage,
		ExitCode:      result.ExitCode,
		Error:         result.Error,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode harness result: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o640); err != nil {
		return fmt.Errorf("write harness result: %w", err)
	}
	return nil
}

func statusFromContext(ctx context.Context) sandbox.Status {
	if ctx.Err() != nil {
		return sandbox.StatusCancelled
	}
	return sandbox.StatusFailed
}

// StatusFromContext maps context cancellation into a sandbox status.
func StatusFromContext(ctx context.Context) sandbox.Status {
	return statusFromContext(ctx)
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
