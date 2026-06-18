package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

func renderBashToolTitle(entry transcriptEntry) string {
	command := strings.TrimSpace(entry.toolMeta["command"])
	if command == "" {
		command = bashCommandFromInput(entry.toolInput)
	}
	if command == "" {
		return "Ran command"
	}
	return "Ran " + command
}

func bashCommandFromInput(input string) string {
	var data struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Command)
}

func renderExplorationTitle(entry transcriptEntry) string {
	if len(entry.tools) == 0 {
		return "Explored"
	}
	if entry.toolActive {
		return "Exploring"
	}
	if entry.toolError {
		return "Explored with errors"
	}
	return "Explored"
}

func renderExplorationToolTitle(entry transcriptEntry) string {
	name := strings.TrimSpace(entry.toolName)
	if name == "" {
		return strings.TrimSpace(entry.text)
	}
	if display := explorationDisplayName(entry); display != "" {
		name = display
	}
	if entry.toolError {
		return "Failed " + name
	}
	if entry.toolActive {
		return "Reading " + name
	}
	return "Read " + name
}

func explorationDisplayName(entry transcriptEntry) string {
	if path := strings.TrimSpace(entry.toolMeta["path"]); path != "" && path != "." {
		return path
	}
	var data struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(entry.toolInput), &data); err != nil {
		return ""
	}
	path := strings.TrimSpace(data.Path)
	if path == "" || path == "." {
		return ""
	}
	return path
}

func indentLines(text string, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func isExplorationTool(name string) bool {
	switch name {
	case "read_file", "list_dir", "file_tree", "grep", "ripgrep", "git_status", "git_diff":
		return true
	default:
		return false
	}
}

func toolGroupText(tools []transcriptEntry, active bool, failed int) string {
	count := len(tools)
	if count == 0 {
		return "Exploring workspace"
	}
	action := "Explored"
	if active {
		action = "Exploring"
	}
	if failed > 0 {
		action = "Explored with errors"
	}
	return fmt.Sprintf("%s %d %s: %s", action, count, pluralize(count, "item", "items"),
		compactToolNames(tools))
}

func compactToolNames(tools []transcriptEntry) string {
	const maxNames = 4
	names := make([]string, 0, min(len(tools), maxNames))
	for i, tool := range tools {
		if i >= maxNames {
			break
		}
		names = append(names, tool.toolName)
	}
	if len(tools) > maxNames {
		names = append(names, fmt.Sprintf("+%d more", len(tools)-maxNames))
	}
	return strings.Join(names, ", ")
}

func pluralize(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func defaultSessionTitle(prompt string, maxRunes int) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if maxRunes <= 0 {
		return title
	}

	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func toolRequestedText(name string) string {
	return "Preparing to use " + name
}

func toolRunningText(name string) string {
	return "Using " + name
}

func toolCompletedText(name string) string {
	return "Used " + name
}

func textFromContent(content []types.ContentPart) string {
	var b strings.Builder
	for _, part := range content {
		if part.Kind != types.ContentText {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}

	return b.String()
}

func onOff(value bool) string {
	if value {
		return "on"
	}

	return "off"
}
