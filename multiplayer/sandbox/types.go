package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// NetworkMode controls network access for a sandboxed task.
type NetworkMode string

const (
	// NetworkModeAnthropicSandboxRuntime uses the srt allowlist model.
	NetworkModeAnthropicSandboxRuntime NetworkMode = "anthropic_sandbox_runtime"
	NetworkModeDisabled                NetworkMode = "disabled"
	NetworkModeLoopback                NetworkMode = "loopback"
	NetworkModeDefault                 NetworkMode = "default"
)

// Status is the normalized terminal or current state of a sandbox run.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusSkipped   Status = "skipped"
)

// TaskSpec describes one isolated worker execution request.
type TaskSpec struct {
	TaskID       string
	ThreadID     string
	Instruction  string
	Command      []string
	Model        string
	Timeout      time.Duration
	Mounts       []Mount
	EnvAllowlist []string
	Env          map[string]string
	Limits       ResourceLimits
	NetworkMode  NetworkMode
	AllowedHosts []string
}

// Mount maps a host path into the sandbox filesystem.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ResourceLimits describes resource ceilings for a sandboxed process.
type ResourceLimits struct {
	MemoryBytes    int64
	NanoCPUs       int64
	PIDs           int64
	MaxOutputBytes int64
}

// Usage describes model token and estimated cost usage observed during a run.
type Usage struct {
	InputTokens   int64
	OutputTokens  int64
	EstimatedCost string
}

// WorkerResultFile is the JSON-friendly result shape workers should emit.
type WorkerResultFile struct {
	Status        Status
	Summary       string
	EvidenceRefs  []string
	Confidence    float64
	OpenQuestions []string
	Usage         Usage
	ExitCode      *int
	Error         string
}

// Result is the runner-returned result shape for a sandbox execution.
type Result struct {
	Status        Status
	Summary       string
	EvidenceRefs  []string
	Confidence    float64
	OpenQuestions []string
	Usage         Usage
	ExitCode      *int
	Error         string
}

// Runner executes a task inside a sandbox.
type Runner interface {
	Run(ctx context.Context, spec TaskSpec) (Result, error)
}

// NoopRunner validates requests and returns a skipped result.
type NoopRunner struct{}

// Run validates spec and returns a skipped/not implemented result.
func (NoopRunner) Run(ctx context.Context, spec TaskSpec) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("sandbox: nil context")
	}
	if err := ValidateTaskSpec(spec); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{
			Status: StatusCancelled,
			Error:  err.Error(),
		}, err
	}

	return Result{
		Status:     StatusSkipped,
		Summary:    "sandbox runner not implemented",
		Confidence: 0,
		Error:      "not implemented",
	}, nil
}

// ValidateTaskSpec checks the basic structural requirements for a task spec.
func ValidateTaskSpec(spec TaskSpec) error {
	if spec.TaskID == "" {
		return errors.New("sandbox: task ID is required")
	}
	if spec.ThreadID == "" {
		return errors.New("sandbox: thread ID is required")
	}
	if spec.Instruction == "" {
		return errors.New("sandbox: instruction is required")
	}
	if spec.Timeout < 0 {
		return errors.New("sandbox: timeout must be non-negative")
	}
	if err := validateLimits(spec.Limits); err != nil {
		return err
	}
	for i, mount := range spec.Mounts {
		if mount.Source == "" {
			return fmt.Errorf("sandbox: mount %d source is required", i)
		}
		if mount.Target == "" {
			return fmt.Errorf("sandbox: mount %d target is required", i)
		}
	}
	return nil
}

func validateLimits(limits ResourceLimits) error {
	if limits.MemoryBytes < 0 {
		return errors.New("sandbox: memory bytes must be non-negative")
	}
	if limits.NanoCPUs < 0 {
		return errors.New("sandbox: nano cpus must be non-negative")
	}
	if limits.PIDs < 0 {
		return errors.New("sandbox: pids must be non-negative")
	}
	if limits.MaxOutputBytes < 0 {
		return errors.New("sandbox: max output bytes must be non-negative")
	}
	return nil
}
