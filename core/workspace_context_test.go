package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shxntanu/epsilon/core/prompts"
)

func TestWorkspaceContextPromptOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("agents instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, ".agents", "skills", "design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Design interfaces.
---

skill instructions`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills-lock.json"), []byte(`{
  "skills": {
    "design": {
      "source": "local/design",
      "skillPath": ".agents/skills/design/SKILL.md"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := prompts.NewCatalog(prompts.Definition{
		ID: prompts.Agent,
		Sections: []prompts.Section{{
			Name: "base",
			Text: "base prompt",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := New(
		WithPromptCatalog(catalog),
		WithWorkspaceContext(root),
		WithSystemPrompt("extra prompt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetActiveSkill("design"); err != nil {
		t.Fatal(err)
	}

	got := harness.CurrentSystemPrompt()
	wantOrder := []string{"base prompt", "agents instructions", "skill instructions", "extra prompt"}
	previous := -1
	for _, want := range wantOrder {
		index := strings.Index(got, want)
		if index < 0 {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
		if index <= previous {
			t.Fatalf("prompt order wrong for %q:\n%s", want, got)
		}
		previous = index
	}
}
