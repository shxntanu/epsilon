package terminal

import "github.com/shxntanu/epsilon/epsilon-cli/tui/render"

// ScrollbackEmitter tracks which stable transcript blocks have already been
// printed into native terminal scrollback.
type ScrollbackEmitter struct {
	printed map[string]bool
	cursor  int
}

// NewScrollbackEmitter returns an initialized native scrollback emitter.
func NewScrollbackEmitter() ScrollbackEmitter {
	return ScrollbackEmitter{
		printed: make(map[string]bool),
	}
}

// Pending returns stable, not-yet-printed blocks in transcript order. It stops
// at the first unstable block so later output cannot overtake live content.
func (e *ScrollbackEmitter) Pending(blocks []render.Block) []render.Block {
	if e.printed == nil {
		e.printed = make(map[string]bool)
	}
	pending := make([]render.Block, 0)
	for e.cursor < len(blocks) {
		block := blocks[e.cursor]
		if e.printed[block.ID] {
			e.cursor++
			continue
		}
		if !block.Stable {
			break
		}
		pending = append(pending, block)
		e.cursor++
	}
	return pending
}

// MarkPrinted records the blocks that have been emitted to terminal scrollback.
func (e *ScrollbackEmitter) MarkPrinted(blocks []render.Block) {
	if e.printed == nil {
		e.printed = make(map[string]bool)
	}
	for _, block := range blocks {
		e.printed[block.ID] = true
	}
}

// Reset clears all printed state. Use when the transcript is replaced.
func (e *ScrollbackEmitter) Reset() {
	e.printed = make(map[string]bool)
	e.cursor = 0
}
