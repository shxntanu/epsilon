package contextwindow

import (
	"encoding/json"
	"testing"

	"github.com/shxntanu/epsilon/core/types"
)

func TestAnalyzeSplitsContextByCategory(t *testing.T) {
	toolInput := json.RawMessage(`{"path":"README.md"}`)
	messages := []types.Message{
		types.SystemMessage("system instructions"),
		types.UserMessage("please read the readme"),
		{
			Role:    types.RoleAssistant,
			Content: []types.ContentPart{types.TextPart("I will inspect it.")},
			ToolCalls: []types.ToolCall{
				{ID: "call_1", Name: "read_file", Input: toolInput},
			},
		},
		types.ToolMessage("call_1", "read_file", types.TextToolResult("hello from the readme")),
		types.AssistantMessage("The readme says hello."),
	}

	summary := Analyze(messages, Config{
		MaxTokens:           100,
		CompactionThreshold: 0.5,
		Estimator: EstimatorFunc(func(text string) int {
			return 10
		}),
	})

	if summary.UsedTokens == 0 {
		t.Fatalf("expected context tokens")
	}
	assertBucket(t, summary, CategorySystem, 1)
	assertBucket(t, summary, CategoryUser, 1)
	assertBucket(t, summary, CategoryAssistant, 2)
	assertBucket(t, summary, CategoryToolCall, 1)
	assertBucket(t, summary, CategoryToolResult, 1)

	if len(summary.Segments) != 6 {
		t.Fatalf("segments = %d, want 6", len(summary.Segments))
	}
	if summary.Segments[3].Category != CategoryToolCall {
		t.Fatalf("segment[3] category = %s, want tool call", summary.Segments[3].Category)
	}
	if !summary.NeedsCompaction {
		t.Fatalf("expected compaction threshold to be exceeded")
	}
	if summary.TokensUntilCompact != 0 {
		t.Fatalf("tokens until compaction = %d, want 0", summary.TokensUntilCompact)
	}
	if bucket := bucketFor(summary, CategoryToolCall); bucket.ShareOfUsed == 0 {
		t.Fatalf("expected tool call share of used context")
	}
}

func TestAnalyzeDefaultsConfig(t *testing.T) {
	summary := Analyze([]types.Message{types.UserMessage("hello")}, Config{})
	if summary.MaxTokens != DefaultMaxTokens {
		t.Fatalf("max tokens = %d, want default", summary.MaxTokens)
	}
	if summary.CompactionThreshold != DefaultCompactionThreshold {
		t.Fatalf("threshold = %f, want default", summary.CompactionThreshold)
	}
	if summary.RemainingTokens <= 0 {
		t.Fatalf("expected remaining tokens")
	}
}

func TestConfigFromModelInfo(t *testing.T) {
	config := ConfigFromModelInfo(types.ModelInfo{
		MaxInputTokens: 128000,
	}, Config{
		CompactionThreshold: 0.75,
	})
	if config.MaxTokens != 128000 {
		t.Fatalf("max tokens = %d, want 128000", config.MaxTokens)
	}
	if config.CompactionThreshold != 0.75 {
		t.Fatalf("threshold = %f, want 0.75", config.CompactionThreshold)
	}
}

func TestEstimateText(t *testing.T) {
	if got := EstimateText(""); got != 0 {
		t.Fatalf("empty estimate = %d, want 0", got)
	}
	if got := EstimateText("abcd"); got != 1 {
		t.Fatalf("four char estimate = %d, want 1", got)
	}
	if got := EstimateText("abcde"); got != 2 {
		t.Fatalf("five char estimate = %d, want 2", got)
	}
}

func assertBucket(t *testing.T, summary Summary, category Category, wantCount int) {
	t.Helper()
	bucket := bucketFor(summary, category)
	if bucket.Category == "" {
		t.Fatalf("missing bucket %s", category)
	}
	if bucket.Count != wantCount {
		t.Fatalf("%s count = %d, want %d", category, bucket.Count, wantCount)
	}
	if bucket.Tokens == 0 {
		t.Fatalf("%s tokens = 0, want non-zero", category)
	}
}

func bucketFor(summary Summary, category Category) Bucket {
	for _, bucket := range summary.Buckets {
		if bucket.Category != category {
			continue
		}
		return bucket
	}
	return Bucket{}
}
