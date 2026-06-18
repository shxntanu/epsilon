package slash

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrUnknownCommand = errors.New("unknown slash command")

type Action string

const (
	ActionNone            Action = ""
	ActionClearTranscript Action = "clear_transcript"
	ActionSetEvents       Action = "set_events"
	ActionSetDensity      Action = "set_density"
	ActionPickModel       Action = "pick_model"
	ActionSetModel        Action = "set_model"
	ActionSetEffort       Action = "set_effort"
	ActionPickSession     Action = "pick_session"
	ActionRenameSession   Action = "rename_session"
	ActionResumeChat      Action = "resume_chat"
	ActionQuit            Action = "quit"
)

type Density string

const (
	DensityComfortable Density = "comfortable"
	DensityCompact     Density = "compact"
)

type Command struct {
	Name        string
	Aliases     []string
	Usage       string
	Description string
	Handler     Handler
}

type Handler func(context.Context, Execution) (Result, error)

type Execution struct {
	SessionID string
	Args      string
	State     State
	Registry  *Registry
}

type State struct {
	Busy             bool
	AwaitingApproval bool
	EventsVisible    bool
	Density          Density
	MessageCount     int
	Model            string
	Effort           string
}

type Result struct {
	Action  Action
	Message string
	Bool    bool
	Density Density
	Model   string
	Effort  string
	Session string
}

type Registry struct {
	commands map[string]Command
	order    []string
}

type Input struct {
	Name    string
	Args    string
	Escaped bool
	OK      bool
}

type Match struct {
	Command Command
	Score   int
	Alias   string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(Command{
		Name:        "help",
		Aliases:     []string{"?"},
		Usage:       "/help",
		Description: "show available slash commands",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			return Result{Message: exec.Registry.HelpText()}, nil
		},
	})
	registry.Register(Command{
		Name:        "clear",
		Aliases:     []string{"cls"},
		Usage:       "/clear",
		Description: "clear the visible transcript",
		Handler: func(context.Context, Execution) (Result, error) {
			return Result{
				Action:  ActionClearTranscript,
				Message: "cleared transcript",
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "details",
		Aliases:     []string{"events"},
		Usage:       "/details [on|off|toggle]",
		Description: "show or hide tool and event details",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			value, err := parseToggle(exec.Args, exec.State.EventsVisible, "details")
			if err != nil {
				return Result{}, err
			}
			return Result{
				Action:  ActionSetEvents,
				Bool:    value,
				Message: "details " + onOff(value),
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "density",
		Usage:       "/density [comfortable|compact|toggle]",
		Description: "switch transcript density",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			value, err := parseDensity(exec.Args, exec.State.Density)
			if err != nil {
				return Result{}, err
			}
			return Result{
				Action:  ActionSetDensity,
				Density: value,
				Message: "density " + string(value),
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "model",
		Aliases:     []string{"models"},
		Usage:       "/model [name]",
		Description: "show or change the model",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			model := strings.TrimSpace(exec.Args)
			if model == "" {
				return Result{Action: ActionPickModel}, nil
			}
			return Result{
				Action:  ActionSetModel,
				Model:   model,
				Message: "model " + model,
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "effort",
		Usage:       "/effort [minimal|low|medium|high|off]",
		Description: "show or change model effort",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			effort := strings.ToLower(strings.TrimSpace(exec.Args))
			if effort == "" {
				current := strings.TrimSpace(exec.State.Effort)
				if current == "" {
					current = "default"
				}
				return Result{Message: "effort " + current}, nil
			}
			if effort == "default" || effort == "none" || effort == "off" {
				return Result{
					Action:  ActionSetEffort,
					Message: "effort default",
				}, nil
			}
			if !validEffort(effort) {
				return Result{}, fmt.Errorf("effort must be minimal, low, medium, high, or off")
			}
			return Result{
				Action:  ActionSetEffort,
				Effort:  effort,
				Message: "effort " + effort,
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "resume",
		Usage:       "/resume [session-id]",
		Description: "resume a persisted chat",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			sessionID := strings.TrimSpace(exec.Args)
			if sessionID == "" {
				return Result{Action: ActionPickSession}, nil
			}
			return Result{
				Action:  ActionResumeChat,
				Session: sessionID,
				Message: "resuming " + sessionID,
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "rename",
		Aliases:     []string{"title"},
		Usage:       "/rename <title...>",
		Description: "rename the current session",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			title := strings.TrimSpace(exec.Args)
			if strings.TrimSpace(exec.SessionID) == "" {
				return Result{}, fmt.Errorf("cannot rename session without a current session")
			}
			if title == "" {
				return Result{}, fmt.Errorf("usage: /rename <title...>")
			}

			return Result{
				Action:  ActionRenameSession,
				Session: exec.SessionID,
				Message: "renaming current session",
				Model:   title, // reuse existing field for the title payload
			}, nil
		},
	})
	registry.Register(Command{
		Name:        "status",
		Usage:       "/status",
		Description: "show session and runtime state",
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			return Result{Message: renderStatus(exec)}, nil
		},
	})
	registry.Register(Command{
		Name:        "exit",
		Aliases:     []string{"quit"},
		Usage:       "/exit",
		Description: "quit epsilon",
		Handler: func(context.Context, Execution) (Result, error) {
			return Result{Action: ActionQuit}, nil
		},
	})

	return registry
}

func validEffort(effort string) bool {
	switch effort {
	case "minimal", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func (r *Registry) Register(command Command) {
	if r.commands == nil {
		r.commands = make(map[string]Command)
	}

	name := NormalizeName(command.Name)
	if name == "" || command.Handler == nil {
		return
	}

	command.Name = name
	if command.Usage == "" {
		command.Usage = "/" + name
	}

	if _, exists := r.commands[name]; !exists {
		r.order = append(r.order, name)
	}
	r.commands[name] = command

	for _, alias := range command.Aliases {
		alias = NormalizeName(alias)
		if alias == "" {
			continue
		}
		r.commands[alias] = command
	}
}

func (r *Registry) Execute(ctx context.Context, input string,
	exec Execution) (Result, bool, error) {
	parsed := ParseInput(input)
	if !parsed.OK || parsed.Escaped {
		return Result{}, false, nil
	}

	command, ok := r.Get(parsed.Name)
	if !ok {
		return Result{}, true, fmt.Errorf("%w: /%s", ErrUnknownCommand, parsed.Name)
	}

	exec.Args = parsed.Args
	exec.Registry = r
	result, err := command.Handler(ctx, exec)
	return result, true, err
}

func (r *Registry) Get(name string) (Command, bool) {
	command, ok := r.commands[NormalizeName(name)]
	return command, ok
}

func (r *Registry) Commands() []Command {
	commands := make([]Command, 0, len(r.order))
	for _, name := range r.order {
		commands = append(commands, r.commands[name])
	}
	sort.SliceStable(commands, func(i int, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	return commands
}

func (r *Registry) HelpText() string {
	var b strings.Builder
	b.WriteString("Slash commands")
	for _, command := range r.Commands() {
		b.WriteString("\n")
		b.WriteString(command.Usage)
		b.WriteString(" - ")
		b.WriteString(command.Description)
	}
	b.WriteString("\n")
	b.WriteString("Use // at the start to send a literal slash message.")
	return b.String()
}

func (r *Registry) Match(query string, limit int) []Match {
	query = NormalizeName(query)
	matches := make([]Match, 0, len(r.order))
	for _, command := range r.Commands() {
		score, alias := commandScore(command, query)
		if score <= 0 {
			continue
		}
		matches = append(matches, Match{
			Command: command,
			Score:   score,
			Alias:   alias,
		})
	}

	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Command.Name < matches[j].Command.Name
	})

	if limit > 0 && len(matches) > limit {
		return matches[:limit]
	}
	return matches
}

func ParseInput(input string) Input {
	text := strings.TrimSpace(input)
	if !strings.HasPrefix(text, "/") {
		return Input{}
	}
	if strings.HasPrefix(text, "//") {
		return Input{
			Escaped: true,
			OK:      true,
		}
	}

	body := strings.TrimSpace(strings.TrimPrefix(text, "/"))
	if body == "" {
		return Input{}
	}

	name, args, _ := strings.Cut(body, " ")
	return Input{
		Name: NormalizeName(name),
		Args: strings.TrimSpace(args),
		OK:   true,
	}
}

func UnescapeInput(input string) string {
	text := strings.TrimSpace(input)
	if strings.HasPrefix(text, "//") {
		return text[1:]
	}

	return input
}

func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
}

func commandScore(command Command, query string) (int, string) {
	candidates := append([]string{command.Name}, command.Aliases...)
	bestScore := 0
	bestAlias := ""
	for _, candidate := range candidates {
		candidate = NormalizeName(candidate)
		score := fuzzyScore(candidate, query)
		if score > bestScore {
			bestScore = score
			bestAlias = candidate
		}
	}

	return bestScore, bestAlias
}

func fuzzyScore(candidate string, query string) int {
	if query == "" {
		return 1
	}
	if candidate == query {
		return 1000 + len(candidate)
	}
	if strings.HasPrefix(candidate, query) {
		return 800 - (len(candidate) - len(query))
	}
	if strings.Contains(candidate, query) {
		return 600 - strings.Index(candidate, query)
	}

	score := 400
	last := -1
	for _, ch := range query {
		idx := strings.IndexRune(candidate[last+1:], ch)
		if idx < 0 {
			return 0
		}
		next := last + 1 + idx
		score -= idx
		last = next
	}
	return score - len(candidate)
}

func parseToggle(args string, current bool, name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "", "toggle":
		return !current, nil
	case "on", "true", "yes":
		return true, nil
	case "off", "false", "no":
		return false, nil
	default:
		return current, fmt.Errorf("usage: /%s [on|off|toggle]", name)
	}
}

func parseDensity(args string, current Density) (Density, error) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "", "toggle":
		if current == DensityCompact {
			return DensityComfortable, nil
		}
		return DensityCompact, nil
	case string(DensityComfortable):
		return DensityComfortable, nil
	case string(DensityCompact):
		return DensityCompact, nil
	default:
		return current, fmt.Errorf("usage: /density [comfortable|compact|toggle]")
	}
}

func renderStatus(exec Execution) string {
	state := "ready"
	if exec.State.Busy {
		state = "thinking"
	}
	if exec.State.AwaitingApproval {
		state = "approval"
	}

	model := exec.State.Model
	if strings.TrimSpace(model) == "" {
		model = "default"
	}
	effort := exec.State.Effort
	if strings.TrimSpace(effort) == "" {
		effort = "default"
	}

	return strings.Join([]string{
		"Status",
		"session: " + exec.SessionID,
		"state: " + state,
		"details: " + onOff(exec.State.EventsVisible),
		"density: " + string(exec.State.Density),
		"model: " + model,
		"effort: " + effort,
		fmt.Sprintf("messages: %d", exec.State.MessageCount),
	}, "\n")
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
