package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadAgentsMD(workspaceRoot string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	var contents []string
	for dir := root; ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
		if err == nil {
			if text := strings.TrimSpace(string(data)); text != "" {
				contents = append(contents, text)
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read AGENTS.md at %s: %w", dir, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	if len(contents) == 0 {
		return "", nil
	}

	return "## Project Instructions (AGENTS.md)\n" + strings.Join(contents, "\n\n"), nil
}
