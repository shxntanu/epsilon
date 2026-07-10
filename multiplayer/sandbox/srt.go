package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const defaultSRTBinary = "srt"

// SRTRunner executes commands through Anthropic Sandbox Runtime's srt CLI.
type SRTRunner struct {
	Binary      string
	SettingsDir string
}

// Run executes spec.Command through srt with generated filesystem/network settings.
func (r SRTRunner) Run(ctx context.Context, spec TaskSpec) (Result, error) {
	if err := ValidateTaskSpec(spec); err != nil {
		return Result{}, err
	}
	if len(spec.Command) == 0 {
		return Result{}, errors.New("sandbox: command is required for srt runner")
	}
	if err := ctx.Err(); err != nil {
		return Result{Status: StatusCancelled, Error: err.Error()}, err
	}
	binary := r.Binary
	if binary == "" {
		binary = defaultSRTBinary
	}
	settingsPath, cleanup, err := r.writeSettings(spec)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	runCtx := ctx
	cancel := func() {}
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
	}
	defer cancel()

	args := append([]string{"--settings", settingsPath}, spec.Command...)
	cmd := exec.CommandContext(runCtx, binary, args...)
	cmd.Env = envForSpec(spec)
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)
	if runCtx.Err() != nil {
		return Result{
			Status: StatusCancelled,
			Error:  runCtx.Err().Error(),
		}, runCtx.Err()
	}
	result := Result{
		Status:     StatusSucceeded,
		Summary:    truncateOutput(string(output), spec.Limits.MaxOutputBytes),
		Confidence: 1,
	}
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			result.ExitCode = &code
		}
		return result, nil
	}
	_ = duration
	return result, nil
}

func (r SRTRunner) writeSettings(spec TaskSpec) (string, func(), error) {
	dir := r.SettingsDir
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", func() {}, fmt.Errorf("create srt settings dir: %w", err)
	}
	settings := srtSettings{
		Network: srtNetwork{
			AllowedDomains: spec.AllowedHosts,
			DeniedDomains:  []string{},
		},
		Filesystem: srtFilesystem{
			DenyRead:   []string{"~/.ssh", "~/.config", "~/.aws", "~/.gnupg"},
			AllowRead:  readMounts(spec.Mounts),
			AllowWrite: writeMounts(spec.Mounts),
			DenyWrite:  []string{".env", ".npmrc", ".pypirc"},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", func() {}, fmt.Errorf("encode srt settings: %w", err)
	}
	path := filepath.Join(dir, "epsilond-srt-"+spec.TaskID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", func() {}, fmt.Errorf("write srt settings: %w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

type srtSettings struct {
	Network    srtNetwork    `json:"network"`
	Filesystem srtFilesystem `json:"filesystem"`
}

type srtNetwork struct {
	AllowedDomains []string `json:"allowedDomains"`
	DeniedDomains  []string `json:"deniedDomains"`
}

type srtFilesystem struct {
	DenyRead   []string `json:"denyRead"`
	AllowRead  []string `json:"allowRead"`
	AllowWrite []string `json:"allowWrite"`
	DenyWrite  []string `json:"denyWrite"`
}

func readMounts(mounts []Mount) []string {
	paths := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		paths = append(paths, mount.Source)
	}
	return paths
}

func writeMounts(mounts []Mount) []string {
	paths := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if !mount.ReadOnly {
			paths = append(paths, mount.Source)
		}
	}
	return paths
}

func envForSpec(spec TaskSpec) []string {
	if len(spec.EnvAllowlist) == 0 && len(spec.Env) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(spec.EnvAllowlist))
	for _, name := range spec.EnvAllowlist {
		allowed[name] = true
	}
	env := make([]string, 0, len(spec.Env))
	for key, value := range spec.Env {
		if len(allowed) == 0 || allowed[key] {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func truncateOutput(output string, maxBytes int64) string {
	if maxBytes <= 0 || int64(len(output)) <= maxBytes {
		return output
	}
	if maxBytes < 64 {
		return output[:maxBytes]
	}
	half := int(maxBytes / 2)
	return output[:half] + "\n[output truncated]\n" + output[len(output)-half:]
}
