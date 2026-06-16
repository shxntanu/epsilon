package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func plainANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func TestMarkdownRendererRendersCommonBlocks(t *testing.T) {
	rendered := plainANSI(newMarkdownRenderer(60).Render(strings.Join([]string{
		"# Summary",
		"",
		"- **Ship** markdown",
		"- Render `code`",
		"",
		"> useful quote",
	}, "\n")))

	for _, want := range []string{"# Summary", "• Ship markdown", "• Render code", "useful quote"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "**Ship**") || strings.Contains(rendered, "`code`") {
		t.Fatalf("rendered markdown kept inline markers:\n%s", rendered)
	}
}

func TestMarkdownRendererRendersFencedCode(t *testing.T) {
	rendered := plainANSI(newMarkdownRenderer(60).Render("```go\nfmt.Println(\"hi\")\n```"))

	if !strings.Contains(rendered, "go") || !strings.Contains(rendered, "fmt.Println(\"hi\")") {
		t.Fatalf("rendered code block missing language or body:\n%s", rendered)
	}
	if strings.Contains(rendered, "```") {
		t.Fatalf("rendered code block kept fences:\n%s", rendered)
	}
}

func TestHarnessMessageRendersAgentMarkdown(t *testing.T) {
	message := newHarnessMessage(80, densityComfortable,
		newMarkdownRenderer(80).heading, newMarkdownRenderer(80).quote)

	rendered := plainANSI(message.RenderAgent("## Done\n\nUse `epsilon`."))
	if !strings.Contains(rendered, "## Done") || !strings.Contains(rendered, "Use epsilon.") {
		t.Fatalf("agent markdown was not rendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "`epsilon`") {
		t.Fatalf("agent markdown kept inline code markers:\n%s", rendered)
	}
}
