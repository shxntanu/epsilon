package slash

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryRegisterAndExecute(t *testing.T) {
	registry := NewRegistry()
	called := false
	registry.Register(Command{
		Name:    "demo",
		Aliases: []string{"d"},
		Handler: func(_ context.Context, exec Execution) (Result, error) {
			called = exec.Args == "hello"
			return Result{Message: "ok"}, nil
		},
	})

	result, handled, err := registry.Execute(context.Background(), "/d hello", Execution{})
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if !handled {
		t.Fatalf("alias command was not handled")
	}
	if !called {
		t.Fatalf("registered command did not receive args")
	}
	if result.Message != "ok" {
		t.Fatalf("message = %q, want ok", result.Message)
	}
}

func TestParseInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOK      bool
		wantEscaped bool
		wantName    string
		wantArgs    string
	}{
		{
			name:     "command without args",
			input:    "/help",
			wantOK:   true,
			wantName: "help",
		},
		{
			name:     "command with args",
			input:    " /events off ",
			wantOK:   true,
			wantName: "events",
			wantArgs: "off",
		},
		{
			name:        "escaped slash",
			input:       "//help",
			wantOK:      true,
			wantEscaped: true,
		},
		{
			name:  "plain message",
			input: "hello",
		},
		{
			name:  "bare slash",
			input: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInput(tt.input)
			if got.OK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", got.OK, tt.wantOK)
			}
			if got.Escaped != tt.wantEscaped {
				t.Fatalf("escaped = %v, want %v", got.Escaped, tt.wantEscaped)
			}
			if got.Name != tt.wantName {
				t.Fatalf("name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Args != tt.wantArgs {
				t.Fatalf("args = %q, want %q", got.Args, tt.wantArgs)
			}
		})
	}
}

func TestDefaultRegistryActions(t *testing.T) {
	registry := NewDefaultRegistry()

	result, handled, err := registry.Execute(context.Background(), "/events off",
		Execution{State: State{EventsVisible: true}})
	if err != nil {
		t.Fatalf("execute events: %v", err)
	}
	if !handled || result.Action != ActionSetEvents || result.Bool {
		t.Fatalf("events result = %#v, handled %v", result, handled)
	}

	result, handled, err = registry.Execute(context.Background(), "/density",
		Execution{State: State{Density: DensityComfortable}})
	if err != nil {
		t.Fatalf("execute density: %v", err)
	}
	if !handled || result.Action != ActionSetDensity || result.Density != DensityCompact {
		t.Fatalf("density result = %#v, handled %v", result, handled)
	}

	result, handled, err = registry.Execute(context.Background(), "/model",
		Execution{State: State{Model: "gpt-5.4"}})
	if err != nil {
		t.Fatalf("execute model picker: %v", err)
	}
	if !handled || result.Action != ActionPickModel {
		t.Fatalf("model picker result = %#v, handled %v", result, handled)
	}

	result, handled, err = registry.Execute(context.Background(), "/model gpt-4o",
		Execution{State: State{Model: "gpt-5.4"}})
	if err != nil {
		t.Fatalf("execute model: %v", err)
	}
	if !handled || result.Action != ActionSetModel || result.Model != "gpt-4o" {
		t.Fatalf("model result = %#v, handled %v", result, handled)
	}

	result, handled, err = registry.Execute(context.Background(), "/effort high",
		Execution{State: State{Effort: "medium"}})
	if err != nil {
		t.Fatalf("execute effort: %v", err)
	}
	if !handled || result.Action != ActionSetEffort || result.Effort != "high" {
		t.Fatalf("effort result = %#v, handled %v", result, handled)
	}

	result, handled, err = registry.Execute(context.Background(), "/effort off",
		Execution{State: State{Effort: "high"}})
	if err != nil {
		t.Fatalf("execute effort off: %v", err)
	}
	if !handled || result.Action != ActionSetEffort || result.Effort != "" {
		t.Fatalf("effort off result = %#v, handled %v", result, handled)
	}
}

func TestUnknownCommand(t *testing.T) {
	registry := NewDefaultRegistry()
	_, handled, err := registry.Execute(context.Background(), "/nope", Execution{})
	if !handled {
		t.Fatalf("unknown slash command should be handled")
	}
	if !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("err = %v, want ErrUnknownCommand", err)
	}
}

func TestMatchFuzzyCommands(t *testing.T) {
	registry := NewDefaultRegistry()
	matches := registry.Match("den", 3)
	if len(matches) == 0 {
		t.Fatalf("expected fuzzy matches")
	}
	if matches[0].Command.Name != "density" {
		t.Fatalf("top match = %q, want density", matches[0].Command.Name)
	}

	matches = registry.Match("?", 3)
	if len(matches) == 0 || matches[0].Command.Name != "help" {
		t.Fatalf("alias match = %#v, want help", matches)
	}
}
