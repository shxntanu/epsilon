package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace bounds file tools to a single root directory.
type Workspace struct {
	root string
}

// NewWorkspace returns a workspace rooted at root.
func NewWorkspace(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("workspace root is empty")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", absRoot)
	}

	evaluatedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("evaluate workspace root: %w", err)
	}

	return &Workspace{root: evaluatedRoot}, nil
}

// Root returns the absolute, symlink-evaluated workspace root.
func (w *Workspace) Root() string {
	return w.root
}

func (w *Workspace) resolveReadPath(path string) (string, error) {
	target, err := w.cleanPath(path)
	if err != nil {
		return "", err
	}

	evaluatedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("evaluate path: %w", err)
	}
	if !w.contains(evaluatedTarget) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return evaluatedTarget, nil
}

func (w *Workspace) resolveWritePath(path string) (string, error) {
	target, err := w.cleanPath(path)
	if err != nil {
		return "", err
	}

	parent := filepath.Dir(target)
	ancestor, err := nearestExistingAncestor(parent)
	if err != nil {
		return "", err
	}
	evaluatedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("evaluate parent ancestor: %w", err)
	}
	if !w.contains(evaluatedAncestor) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create parent directories: %w", err)
	}

	if _, err := os.Lstat(target); err == nil {
		evaluatedTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			return "", fmt.Errorf("evaluate existing path: %w", err)
		}
		if !w.contains(evaluatedTarget) {
			return "", fmt.Errorf("path escapes workspace: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat target path: %w", err)
	}

	return target, nil
}

func (w *Workspace) cleanPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative to workspace")
	}

	clean := filepath.Clean(path)
	if clean == "." {
		return "", fmt.Errorf("path is empty")
	}

	target := filepath.Join(w.root, clean)
	if !w.contains(target) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return target, nil
}

func (w *Workspace) contains(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func nearestExistingAncestor(path string) (string, error) {
	current := path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return "", fmt.Errorf("parent path is not a directory: %s", current)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat parent path: %w", err)
		}

		next := filepath.Dir(current)
		if next == current {
			return "", fmt.Errorf("no existing parent path for: %s", path)
		}
		current = next
	}
}
