package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentsMDLoadsWorkspaceAndParents(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	child := filepath.Join(project, "pkg")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("parent instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("project instructions"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadAgentsMD(child)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Project Instructions (AGENTS.md)",
		"project instructions",
		"parent instructions",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("LoadAgentsMD missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "project instructions") > strings.Index(got, "parent instructions") {
		t.Fatalf("more specific AGENTS.md should appear first:\n%s", got)
	}
}

func TestLoadAgentsMDMissingReturnsEmpty(t *testing.T) {
	got, err := LoadAgentsMD(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("LoadAgentsMD = %q, want empty", got)
	}
}
