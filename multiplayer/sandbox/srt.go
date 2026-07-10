package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultSRTBinary         = "srt"
	defaultSRTTimeout        = 10 * time.Minute
	defaultSRTMaxOutputBytes = int64(1 << 20)
)

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

	spec = withSRTDefaults(spec)
	if err := prepareResultPath(spec.ResultPath); err != nil {
		return Result{}, err
	}

	runCtx := ctx
	cancel := func() {}
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
	}
	defer cancel()

	args := append([]string{"--settings", settingsPath}, spec.Command...)
	cmd := exec.CommandContext(runCtx, binary, args...)
	cmd.Dir = workingDir(spec)
	cmd.Env = envForSpec(spec)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := newBoundedBuffer(spec.Limits.MaxOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start srt command: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	err = nil
	select {
	case err = <-waitCh:
	case <-runCtx.Done():
		killProcessGroup(cmd.Process)
		err = <-waitCh
	}
	_ = time.Since(start)
	if runCtx.Err() != nil {
		result := Result{
			Status: StatusCancelled,
			Error:  runCtx.Err().Error(),
		}
		result.ResultPointer = spec.ResultPath
		_ = writeNormalizedResult(spec.ResultPath, result)
		return result, runCtx.Err()
	}

	result := resultFromCommand(spec, output.String(), err)
	if fileResult, fileErr := readWorkerResult(spec.ResultPath); fileErr == nil {
		result = mergeWorkerResult(result, fileResult)
	} else if !errors.Is(fileErr, os.ErrNotExist) && err == nil {
		result.Status = StatusFailed
		result.Error = fileErr.Error()
	}
	if err != nil && result.Status == StatusSucceeded {
		result.Status = StatusFailed
		result.Error = err.Error()
	}
	result.ResultPointer = spec.ResultPath
	if writeErr := writeNormalizedResult(spec.ResultPath, result); writeErr != nil && err == nil {
		return Result{}, writeErr
	}
	return result, nil
}

func withSRTDefaults(spec TaskSpec) TaskSpec {
	if spec.Timeout == 0 {
		spec.Timeout = defaultSRTTimeout
	}
	if spec.Limits.MaxOutputBytes == 0 {
		spec.Limits.MaxOutputBytes = defaultSRTMaxOutputBytes
	}
	return spec
}

func prepareResultPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale result file: %w", err)
	}
	return nil
}

func workingDir(spec TaskSpec) string {
	if strings.TrimSpace(spec.WorkingDir) != "" {
		return spec.WorkingDir
	}
	for _, mount := range spec.Mounts {
		if !mount.ReadOnly && strings.TrimSpace(mount.Source) != "" {
			return mount.Source
		}
	}
	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.Source) != "" {
			return mount.Source
		}
	}
	return ""
}

func resultFromCommand(spec TaskSpec, output string, err error) Result {
	result := Result{
		Status:     StatusSucceeded,
		Summary:    truncateOutput(output, spec.Limits.MaxOutputBytes),
		Confidence: 1,
	}
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			result.ExitCode = &code
		}
	}
	return result
}

func readWorkerResult(path string) (Result, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, os.ErrNotExist
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	var file WorkerResultFile
	if err := json.Unmarshal(body, &file); err != nil {
		return Result{}, fmt.Errorf("decode result.json: %w", err)
	}
	return Result{
		Status:        file.Status,
		Summary:       file.Summary,
		EvidenceRefs:  file.EvidenceRefs,
		Confidence:    file.Confidence,
		OpenQuestions: file.OpenQuestions,
		Usage:         file.Usage,
		ExitCode:      file.ExitCode,
		Error:         file.Error,
	}, nil
}

func mergeWorkerResult(base Result, worker Result) Result {
	if worker.Status != "" {
		base.Status = worker.Status
	}
	if worker.Summary != "" {
		base.Summary = worker.Summary
	}
	if worker.EvidenceRefs != nil {
		base.EvidenceRefs = worker.EvidenceRefs
	}
	if worker.Confidence != 0 {
		base.Confidence = worker.Confidence
	}
	if worker.OpenQuestions != nil {
		base.OpenQuestions = worker.OpenQuestions
	}
	if worker.Usage != (Usage{}) {
		base.Usage = worker.Usage
	}
	if worker.ExitCode != nil {
		base.ExitCode = worker.ExitCode
	}
	if worker.Error != "" {
		base.Error = worker.Error
	}
	return base
}

func writeNormalizedResult(path string, result Result) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	file := WorkerResultFile{
		Status:        result.Status,
		Summary:       result.Summary,
		EvidenceRefs:  result.EvidenceRefs,
		Confidence:    result.Confidence,
		OpenQuestions: result.OpenQuestions,
		Usage:         result.Usage,
		ExitCode:      result.ExitCode,
		Error:         result.Error,
	}
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode normalized result.json: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o640); err != nil {
		return fmt.Errorf("write normalized result.json: %w", err)
	}
	return nil
}

func killProcessGroup(process *os.Process) {
	if process == nil {
		return
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil {
		_ = process.Kill()
	}
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
			AllowedDomains:    allowedDomains(spec),
			DeniedDomains:     []string{},
			AllowLocalBinding: false,
		},
		Filesystem: srtFilesystem{
			DenyRead:   sensitiveReadDenies(),
			AllowRead:  readMounts(spec.Mounts),
			AllowWrite: writeMounts(spec.Mounts),
			DenyWrite:  denyWritePaths(spec.Mounts),
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", func() {}, fmt.Errorf("encode srt settings: %w", err)
	}
	path := filepath.Join(dir, "epsilond-srt-"+safeFilename(spec.TaskID)+".json")
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
	AllowedDomains    []string `json:"allowedDomains"`
	DeniedDomains     []string `json:"deniedDomains"`
	AllowLocalBinding bool     `json:"allowLocalBinding"`
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
		paths = append(paths, cleanPath(mount.Source))
	}
	return paths
}

func writeMounts(mounts []Mount) []string {
	paths := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if !mount.ReadOnly {
			paths = append(paths, cleanPath(mount.Source))
		}
	}
	return paths
}

func denyWritePaths(mounts []Mount) []string {
	paths := []string{
		".env",
		".npmrc",
		".pypirc",
		".netrc",
	}
	for _, mount := range mounts {
		if mount.ReadOnly {
			paths = append(paths, cleanPath(mount.Source))
		}
	}
	return paths
}

func sensitiveReadDenies() []string {
	denies := []string{
		"~/.ssh",
		"~/.config",
		"~/.aws",
		"~/.gnupg",
		"~/.docker",
		"~/.netrc",
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		denies = append(denies,
			filepath.Join(home, ".ssh"),
			filepath.Join(home, ".config"),
			filepath.Join(home, ".aws"),
			filepath.Join(home, ".gnupg"),
			filepath.Join(home, ".docker"),
			filepath.Join(home, ".netrc"),
			filepath.Join(home, ".gitconfig"),
			filepath.Join(home, ".git-credentials"),
		)
	}
	return denies
}

func allowedDomains(spec TaskSpec) []string {
	if spec.NetworkMode == NetworkModeDisabled || spec.NetworkMode == "" {
		return []string{}
	}
	return spec.AllowedHosts
}

func envForSpec(spec TaskSpec) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.TempDir(),
		"TMPDIR=" + os.TempDir(),
		"TEMP=" + os.TempDir(),
		"TMP=" + os.TempDir(),
	}
	allowed := make(map[string]bool, len(spec.EnvAllowlist))
	for _, name := range spec.EnvAllowlist {
		allowed[name] = true
	}
	for key, value := range spec.Env {
		if len(allowed) == 0 || allowed[key] {
			env = append(env, key+"="+value)
		}
	}
	if spec.ResultPath != "" {
		env = append(env, "EPSILOND_RESULT_PATH="+spec.ResultPath)
	}
	env = append(env,
		"EPSILOND_TASK_ID="+spec.TaskID,
		"EPSILOND_THREAD_ID="+spec.ThreadID,
	)
	if spec.RunID != "" {
		env = append(env, "EPSILOND_RUN_ID="+spec.RunID)
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

type boundedBuffer struct {
	limit int64
	buf   bytes.Buffer
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	if limit <= 0 {
		limit = defaultSRTMaxOutputBytes
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	available := b.limit - int64(b.buf.Len())
	if available > 0 {
		if int64(len(p)) > available {
			_, _ = b.buf.Write(p[:available])
		} else {
			_, _ = b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return b.buf.String()
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func safeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "task"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	safe := strings.Trim(b.String(), "._-")
	if safe == "" {
		return "task"
	}
	if len(safe) > 80 {
		return safe[:80]
	}
	return safe
}
