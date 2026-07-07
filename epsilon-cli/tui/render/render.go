package render

// BlockKind identifies the kind of transcript block being rendered.
type BlockKind string

const (
	BlockPlain     BlockKind = "plain"
	BlockUser      BlockKind = "user"
	BlockAssistant BlockKind = "assistant"
	BlockEvent     BlockKind = "event"
	BlockDiff      BlockKind = "diff"
	BlockStatus    BlockKind = "status"
	BlockTool      BlockKind = "tool"
	BlockToolGroup BlockKind = "tool_group"
)

// Block is the stable rendering contract for chat transcript items.
// Rendering owns presentation only; Bubble Tea state, viewport offsets, mouse
// handling, and scroll state stay outside this package.
type Block struct {
	ID       string
	Kind     BlockKind
	Stable   bool
	Content  string
	Metadata map[string]string
}

// Renderer renders one transcript block at the provided width.
type Renderer interface {
	RenderBlock(block Block, width int) RenderedBlock
}

// RenderedBlock is append-only once its source Block is stable.
type RenderedBlock struct {
	ID     string
	Lines  []string
	Height int
}
