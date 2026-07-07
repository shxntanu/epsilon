package tui

import (
	"encoding/json"

	"github.com/shxntanu/epsilon/epsilon-cli/tui/render"
)

type transcriptToolBlock struct {
	Kind       transcriptEntryKind `json:"kind"`
	Text       string              `json:"text"`
	ToolCallID string              `json:"tool_call_id"`
	ToolName   string              `json:"tool_name"`
	ToolInput  string              `json:"tool_input"`
	ToolResult string              `json:"tool_result"`
	ToolMeta   map[string]string   `json:"tool_meta"`
	ToolError  bool                `json:"tool_error"`
	ToolActive bool                `json:"tool_active"`
}

func transcriptEntryToBlock(entry transcriptEntry) render.Block {
	block := render.Block{
		Kind:    transcriptKindToBlockKind(entry.kind),
		Content: entry.text,
		Metadata: map[string]string{
			"tool_call_id": entry.toolCallID,
			"tool_name":    entry.toolName,
			"tool_input":   entry.toolInput,
			"tool_result":  entry.toolResult,
		},
	}
	if entry.toolError {
		block.Metadata["tool_error"] = "true"
	}
	if entry.toolActive {
		block.Metadata["tool_active"] = "true"
	}
	if entry.toolCallID != "" {
		block.ID = "tool:" + entry.toolCallID
	}
	if len(entry.toolMeta) > 0 {
		if data, err := json.Marshal(entry.toolMeta); err == nil {
			block.Metadata["tool_meta"] = string(data)
		}
	}
	if len(entry.tools) > 0 {
		tools := make([]transcriptToolBlock, 0, len(entry.tools))
		for _, tool := range entry.tools {
			tools = append(tools, transcriptToolBlock{
				Kind:       tool.kind,
				Text:       tool.text,
				ToolCallID: tool.toolCallID,
				ToolName:   tool.toolName,
				ToolInput:  tool.toolInput,
				ToolResult: tool.toolResult,
				ToolMeta:   tool.toolMeta,
				ToolError:  tool.toolError,
				ToolActive: tool.toolActive,
			})
		}
		if data, err := json.Marshal(tools); err == nil {
			block.Metadata["tools"] = string(data)
		}
	}
	return block
}

func blockToTranscriptEntry(block render.Block) transcriptEntry {
	entry := transcriptEntry{
		kind:       blockKindToTranscriptKind(block.Kind),
		text:       block.Content,
		toolCallID: block.Metadata["tool_call_id"],
		toolName:   block.Metadata["tool_name"],
		toolInput:  block.Metadata["tool_input"],
		toolResult: block.Metadata["tool_result"],
		toolError:  block.Metadata["tool_error"] == "true",
		toolActive: block.Metadata["tool_active"] == "true",
	}
	if raw := block.Metadata["tool_meta"]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &entry.toolMeta)
	}
	if raw := block.Metadata["tools"]; raw != "" {
		var tools []transcriptToolBlock
		if err := json.Unmarshal([]byte(raw), &tools); err == nil {
			entry.tools = make([]transcriptEntry, 0, len(tools))
			for _, tool := range tools {
				entry.tools = append(entry.tools, transcriptEntry{
					kind:       tool.Kind,
					text:       tool.Text,
					toolCallID: tool.ToolCallID,
					toolName:   tool.ToolName,
					toolInput:  tool.ToolInput,
					toolResult: tool.ToolResult,
					toolMeta:   tool.ToolMeta,
					toolError:  tool.ToolError,
					toolActive: tool.ToolActive,
				})
			}
		}
	}
	return entry
}

func transcriptKindToBlockKind(kind transcriptEntryKind) render.BlockKind {
	switch kind {
	case transcriptUser:
		return render.BlockUser
	case transcriptAgent:
		return render.BlockAssistant
	case transcriptEvent:
		return render.BlockEvent
	case transcriptDiff:
		return render.BlockDiff
	case transcriptStatus:
		return render.BlockStatus
	case transcriptTool:
		return render.BlockTool
	case transcriptToolGroup:
		return render.BlockToolGroup
	default:
		return render.BlockPlain
	}
}

func blockKindToTranscriptKind(kind render.BlockKind) transcriptEntryKind {
	switch kind {
	case render.BlockUser:
		return transcriptUser
	case render.BlockAssistant:
		return transcriptAgent
	case render.BlockEvent:
		return transcriptEvent
	case render.BlockDiff:
		return transcriptDiff
	case render.BlockStatus:
		return transcriptStatus
	case render.BlockTool:
		return transcriptTool
	case render.BlockToolGroup:
		return transcriptToolGroup
	default:
		return transcriptPlain
	}
}
