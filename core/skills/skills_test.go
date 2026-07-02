package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistryLoadsSkillsFromLockfile(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "impeccable")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: impeccable
description: Build polished frontend interfaces.
---

Use visual craft.`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills-lock.json"), []byte(`{
  "version": 1,
  "skills": {
    "impeccable": {
      "source": "pbakaus/impeccable",
      "skillPath": ".agents/skills/impeccable/SKILL.md"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", registry.Len())
	}
	skill, ok := registry.Get("impeccable")
	if !ok {
		t.Fatal("skill not loaded")
	}
	if skill.Description != "Build polished frontend interfaces." {
		t.Fatalf("Description = %q", skill.Description)
	}
	if !strings.Contains(skill.Content, "Use visual craft.") {
		t.Fatalf("Content missing SKILL.md body: %q", skill.Content)
	}
}

func TestLoadRegistryMissingLockfileReturnsEmptyRegistry(t *testing.T) {
	registry, err := LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if registry.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", registry.Len())
	}
}
