package tui

import "charm.land/lipgloss/v2"

var (
	tuiInk        = lipgloss.Color("252")
	tuiInkStrong  = lipgloss.Color("255")
	tuiMuted      = lipgloss.Color("246")
	tuiSubtle     = lipgloss.Color("244")
	tuiFaint      = lipgloss.Color("240")
	tuiLine       = lipgloss.Color("238")
	tuiLineStrong = lipgloss.Color("241")
	tuiSurface    = lipgloss.Color("234")
	tuiSurface2   = lipgloss.Color("235")
	tuiSurface3   = lipgloss.Color("237")
	tuiSelected   = lipgloss.Color("238")

	tuiAccentAgent  = lipgloss.Color("146")
	tuiAccentUser   = lipgloss.Color("109")
	tuiAccentInfo   = lipgloss.Color("117")
	tuiAccentModel  = lipgloss.Color("183")
	tuiAccentTool   = lipgloss.Color("178")
	tuiAccentOK     = lipgloss.Color("120")
	tuiAccentWarn   = lipgloss.Color("220")
	tuiAccentDanger = lipgloss.Color("203")

	tuiStatusReady    = lipgloss.Color("35")
	tuiStatusThinking = lipgloss.Color("57")
	tuiStatusWorking  = lipgloss.Color("24")
	tuiStatusWarn     = lipgloss.Color("58")
	tuiStatusDanger   = lipgloss.Color("52")
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
		status == "resuming":
		bg = tuiStatusWorking
		fg = tuiInkStrong
	case hasStatusPrefix(status, "model:") || hasStatusPrefix(status, "effort:"):
		bg = tuiAccentModel
		fg = lipgloss.Color("234")
		bold = true
	case hasStatusPrefix(status, "details:") || hasStatusPrefix(status, "density:"):
		bg = tuiSelected
	case hasStatusPrefix(status, "mouse:"):
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
