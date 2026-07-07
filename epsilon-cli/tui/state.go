package tui

import (
	"context"

	"charm.land/bubbles/v2/spinner"

	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/session"
	"github.com/shxntanu/epsilon/core/skills"
	"github.com/shxntanu/epsilon/core/slash"
	"github.com/shxntanu/epsilon/core/types"
	"github.com/shxntanu/epsilon/epsilon-cli/tui/terminal"
)

type appState struct {
	ctx          context.Context
	session      *session.Session
	subscription *events.Subscription
	events       <-chan types.Event
	broker       *PermissionBroker
}

type providerState struct {
	contextView     func() contextwindow.Summary
	modelInfo       func() (types.ModelInfo, bool)
	listModels      func(context.Context) ([]types.ModelInfo, error)
	listSessions    func(context.Context) ([]events.SessionInfo, error)
	resumeSession   func(context.Context, string) (*session.Session, error)
	currentModel    func() string
	currentEffort   func() string
	currentPlanMode func() bool
	setModel        func(context.Context, string) error
	setEffort       func(string) error
	setPlanMode     func(bool) error
	renameSession   func(context.Context, string, string) (bool, error)
	setActiveSkill  func(string) error
	clearSkill      func()
	suggestSkill    func(string) *skills.Skill
	currentSkill    func() *skills.Skill
	listSkills      func() []skills.Skill
	refreshSkills   func() (int, error)
}

type inputState struct {
	composer      composer
	slash         *slash.Registry
	slashCursor   int
	skillCursor   int
	fileCursor    int
	filePaths     []string
	promptHistory []string
	historyIndex  int
	historyDraft  string
	historyActive bool
}

type overlayState struct {
	modelPicker   *modelPicker
	sessionPicker *sessionPicker
	skillPicker   *skillPicker
	permission    *permissionPrompt
}

type transcriptState struct {
	entries      []transcriptEntry
	pendingUsers []string
	streaming    int
}

type layoutState struct {
	width          int
	height         int
	followOutput   bool
	density        densityMode
	showEvents     bool
	showContext    bool
	showBackground bool
}

type activityState struct {
	spinner         spinner.Model
	thinking        thinking
	busy            bool
	showSpinner     bool
	dirty           bool
	status          string
	sessionTitle    string
	headerFrame     int
	headerAnimating bool
	quitArmed       bool
}

type scrollbackState struct {
	scrollback     terminal.ScrollbackEmitter
	scrollPrinting bool
}

type stepState struct {
	stepCancel   context.CancelFunc
	stepSeq      uint64
	activeStepID uint64
}

type visualState struct {
	styles styles
}
