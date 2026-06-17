package prompts

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogRendersPromptWithExtensions(t *testing.T) {
	catalog := DefaultCatalog()

	prompt, ok := catalog.Render(Agent, "extra policy")
	if !ok {
		t.Fatalf("expected agent prompt")
	}
	if !strings.HasPrefix(prompt, DefaultAgent().Render()) {
		t.Fatalf("prompt does not start with default agent prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "extra policy") {
		t.Fatalf("prompt does not include extension: %q", prompt)
	}
}

func TestDefaultAgentPromptComesFromEmbeddedTextFile(t *testing.T) {
	data, err := defaultPromptFiles.ReadFile("defaults/agent.txt")
	if err != nil {
		t.Fatalf("read embedded prompt: %v", err)
	}

	if DefaultAgent().Render() != strings.TrimSpace(string(data)) {
		t.Fatalf("default agent prompt does not match embedded text file")
	}
}

func TestCatalogIDsAreSorted(t *testing.T) {
	catalog := DefaultCatalog()
	got := catalog.IDs()
	want := []ID{Agent, Summary, Title}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %#v, want %#v", got, want)
	}
}

func TestRegisterRejectsEmptyPrompt(t *testing.T) {
	catalog := DefaultCatalog()
	err := catalog.Register(Definition{
		ID: "empty",
	})
	if err == nil {
		t.Fatalf("expected empty prompt to fail")
	}
}
