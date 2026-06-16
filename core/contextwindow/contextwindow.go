package contextwindow

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/shxntanu/epsilon/core/types"
)

const (
	DefaultMaxTokens             = 200000
	DefaultCompactionThreshold   = 0.85
	defaultMessageOverheadTokens = 4
	defaultToolCallOverhead      = 6
)

type Category string

const (
	CategorySystem       Category = "system"
	CategoryUser         Category = "user_messages"
	CategoryAssistant    Category = "model_responses"
	CategoryToolCall     Category = "tool_calls"
	CategoryToolResult   Category = "tool_results"
	CategoryConversation Category = "conversation"
)

type Config struct {
	MaxTokens           int
	CompactionThreshold float64
	Estimator           TokenEstimator
}

type TokenEstimator interface {
	EstimateText(text string) int
}

type EstimatorFunc func(text string) int

func (fn EstimatorFunc) EstimateText(text string) int {
	return fn(text)
}

type Tracker struct {
	config Config
}

type Summary struct {
	MaxTokens           int
	UsedTokens          int
	RemainingTokens     int
	PercentUsed         float64
	CompactionThreshold float64
	CompactionTokens    int
	TokensUntilCompact  int
	NeedsCompaction     bool
	Buckets             []Bucket
	Segments            []Segment
}

type Bucket struct {
	Category    Category
	Label       string
	Count       int
	Tokens      int
	PercentUsed float64
	ShareOfUsed float64
}

type Segment struct {
	Index    int
	Category Category
	Label    string
	Tokens   int
}

func NewTracker(config Config) *Tracker {
	return &Tracker{
		config: normalizeConfig(config),
	}
}

func DefaultTracker() *Tracker {
	return NewTracker(Config{})
}

func ConfigFromModelInfo(model types.ModelInfo, base Config) Config {
	if model.MaxInputTokens > 0 {
		base.MaxTokens = model.MaxInputTokens
	} else if model.MaxTokens > 0 {
		base.MaxTokens = model.MaxTokens
	}
	return normalizeConfig(base)
}

func (t *Tracker) Analyze(messages []types.Message) Summary {
	return Analyze(messages, t.config)
}

func Analyze(messages []types.Message, config Config) Summary {
	config = normalizeConfig(config)
	summary := Summary{
		MaxTokens:           config.MaxTokens,
		CompactionThreshold: config.CompactionThreshold,
		Buckets:             defaultBuckets(),
	}

	for i, message := range messages {
		addMessage(&summary, config.Estimator, i, message)
	}

	for i := range summary.Buckets {
		summary.Buckets[i].PercentUsed = percent(summary.Buckets[i].Tokens, summary.MaxTokens)
		summary.Buckets[i].ShareOfUsed = percent(summary.Buckets[i].Tokens, summary.UsedTokens)
	}
	summary.RemainingTokens = max(0, summary.MaxTokens-summary.UsedTokens)
	summary.PercentUsed = percent(summary.UsedTokens, summary.MaxTokens)
	summary.CompactionTokens = int(math.Ceil(float64(summary.MaxTokens) * summary.CompactionThreshold))
	summary.TokensUntilCompact = max(0, summary.CompactionTokens-summary.UsedTokens)
	summary.NeedsCompaction = summary.PercentUsed >= summary.CompactionThreshold
	return summary
}

func normalizeConfig(config Config) Config {
	if config.MaxTokens <= 0 {
		config.MaxTokens = DefaultMaxTokens
	}
	if config.CompactionThreshold <= 0 || config.CompactionThreshold > 1 {
		config.CompactionThreshold = DefaultCompactionThreshold
	}
	if config.Estimator == nil {
		config.Estimator = EstimatorFunc(EstimateText)
	}
	return config
}

func defaultBuckets() []Bucket {
	return []Bucket{
		{Category: CategorySystem, Label: "System"},
		{Category: CategoryUser, Label: "User messages"},
		{Category: CategoryAssistant, Label: "Model responses"},
		{Category: CategoryToolCall, Label: "Tool calls"},
		{Category: CategoryToolResult, Label: "Tool results"},
	}
}

func addMessage(summary *Summary, estimator TokenEstimator, index int, message types.Message) {
	switch message.Role {
	case types.RoleSystem:
		addTextSegment(summary, estimator, index, CategorySystem, "system", messageText(message))
	case types.RoleUser:
		addTextSegment(summary, estimator, index, CategoryUser, "user", messageText(message))
	case types.RoleAssistant:
		addTextSegment(summary, estimator, index, CategoryAssistant, "assistant", messageText(message))
		for _, call := range message.ToolCalls {
			addToolCallSegment(summary, estimator, index, call)
		}
	case types.RoleTool:
		label := "tool result"
		if message.ToolName != "" {
			label = "tool result: " + message.ToolName
		}
		addTextSegment(summary, estimator, index, CategoryToolResult, label, messageText(message))
	default:
		addTextSegment(summary, estimator, index, CategoryConversation, string(message.Role),
			messageText(message))
	}
}

func addTextSegment(summary *Summary, estimator TokenEstimator, index int,
	category Category, label string, text string) {
	tokens := defaultMessageOverheadTokens + estimator.EstimateText(text)
	if strings.TrimSpace(text) == "" {
		tokens = defaultMessageOverheadTokens
	}
	addSegment(summary, Segment{
		Index:    index,
		Category: category,
		Label:    label,
		Tokens:   tokens,
	})
}

func addToolCallSegment(summary *Summary, estimator TokenEstimator, index int, call types.ToolCall) {
	var payload strings.Builder
	payload.WriteString(call.Name)
	if len(call.Input) > 0 {
		payload.WriteString(" ")
		payload.Write(call.Input)
	}

	addSegment(summary, Segment{
		Index:    index,
		Category: CategoryToolCall,
		Label:    "tool call: " + call.Name,
		Tokens:   defaultToolCallOverhead + estimator.EstimateText(payload.String()),
	})
}

func addSegment(summary *Summary, segment Segment) {
	if segment.Tokens <= 0 {
		return
	}

	summary.Segments = append(summary.Segments, segment)
	summary.UsedTokens += segment.Tokens
	for i := range summary.Buckets {
		if summary.Buckets[i].Category == segment.Category {
			summary.Buckets[i].Count++
			summary.Buckets[i].Tokens += segment.Tokens
			return
		}
	}
	summary.Buckets = append(summary.Buckets, Bucket{
		Category: segment.Category,
		Label:    string(segment.Category),
		Count:    1,
		Tokens:   segment.Tokens,
	})
}

func messageText(message types.Message) string {
	var b strings.Builder
	for _, part := range message.Content {
		if part.Kind != types.ContentText {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}

	if message.Role == types.RoleTool && b.Len() == 0 {
		data, err := json.Marshal(message)
		if err == nil {
			return string(data)
		}
	}
	return b.String()
}

func EstimateText(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	runes := len([]rune(text))
	return max(1, int(math.Ceil(float64(runes)/4)))
}

func percent(tokens int, maxTokens int) float64 {
	if maxTokens <= 0 {
		return 0
	}
	return float64(tokens) / float64(maxTokens)
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
