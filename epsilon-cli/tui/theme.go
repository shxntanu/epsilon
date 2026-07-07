package tui

import "charm.land/lipgloss/v2"

var (
	tuiInk        = lipgloss.Color("252")
	tuiInkStrong  = lipgloss.Color("255")
	tuiMuted      = lipgloss.Color("246")
	tuiSubtle     = lipgloss.Color("244")
	tuiFaint      = lipgloss.Color("240")
	tuiLine       = lipgloss.Color("#1597BB")
	tuiLineStrong = lipgloss.Color("#8FD6E1")
	tuiSurface    = lipgloss.Color("#07041A")
	tuiSurface2   = lipgloss.Color("#0A1026")
	tuiSurface3   = lipgloss.Color("#081D24")
	tuiSelected   = lipgloss.Color("#150E56")

	tuiAccentAgent   = lipgloss.Color("#1597BB")
	tuiAccentUser    = lipgloss.Color("#E66C98")
	tuiAccentInfo    = lipgloss.Color("#8FD6E1")
	tuiAccentModel   = lipgloss.Color("#8FD6E1")
	tuiAccentCommand = lipgloss.Color("#8FD6E1")
	tuiAccentTool    = lipgloss.Color("#1597BB")
	tuiAccentOK      = lipgloss.Color("#8FD6E1")
	tuiAccentWarn    = lipgloss.Color("#E66C98")
	tuiAccentDanger  = lipgloss.Color("#E66C98")

	tuiStatusReady    = lipgloss.Color("#1597BB")
	tuiStatusThinking = lipgloss.Color("#150E56")
	tuiStatusWorking  = lipgloss.Color("#1597BB")
	tuiStatusWarn     = lipgloss.Color("#7B113A")
	tuiStatusDanger   = lipgloss.Color("#7B113A")
)

func statusBadgeStyle(status string) lipgloss.Style {
	bg := tuiSurface3
	fg := tuiInk
	bold := false

	switch {
	case status == "ready":
		bg = tuiStatusReady
		fg = tuiInkStrong
		bold = true
	case status == "thinking":
		bg = tuiStatusThinking
		fg = tuiInkStrong
		bold = true
	case hasStatusPrefix(status, "error") || hasStatusPrefix(status, "models:error") ||
		hasStatusPrefix(status, "sessions:error"):
		bg = tuiStatusDanger
		fg = tuiInkStrong
		bold = true
	case hasStatusPrefix(status, "models") || hasStatusPrefix(status, "sessions") ||
		hasStatusPrefix(status, "skills") || status == "resuming":
		bg = tuiStatusWorking
		fg = tuiInkStrong
	case hasStatusPrefix(status, "model:") || hasStatusPrefix(status, "effort:"):
		bg = tuiAccentModel
		fg = tuiInkStrong
		bold = true
	case hasStatusPrefix(status, "skill:"):
		bg = tuiAccentTool
		fg = tuiInkStrong
		bold = true
	case hasStatusPrefix(status, "details:") || hasStatusPrefix(status, "density:") ||
		hasStatusPrefix(status, "background:") || hasStatusPrefix(status, "context:"):
		bg = tuiSelected
	case hasStatusPrefix(status, "mouse:") || hasStatusPrefix(status, "view:"):
		bg = tuiStatusWarn
		fg = tuiInkStrong
	}

	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Bold(bold).
		Padding(0, 1)
}

func hasStatusPrefix(status string, prefix string) bool {
	if len(status) < len(prefix) {
		return false
	}
	return status[:len(prefix)] == prefix
}
