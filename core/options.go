package core

import (
	"fmt"

	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

type Option func(*Harness) error

func WithEventBufferSize(size int) Option {
	return func(h *Harness) error {
		h.eventBufferSize = size
		return nil
	}
}

func WithRunIDGenerator(fn func() (string, error)) Option {
	return func(h *Harness) error {
		h.newRunID = fn
		return nil
	}
}

func WithProvider(provider types.Provider) Option {
	return func(h *Harness) error {
		h.provider = provider
		return nil
	}
}

func WithPermissionBroker(broker permissions.Broker) Option {
	return func(h *Harness) error {
		h.permissionBroker = broker
		return nil
	}
}

func WithEventStore(store events.Store) Option {
	return func(h *Harness) error {
		h.eventStore = store
		return nil
	}
}

func WithSessionDir(dir string) Option {
	return func(h *Harness) error {
		store, err := events.NewJSONLStore(dir)
		if err != nil {
			return fmt.Errorf("create JSONL event store: %w", err)
		}

		h.eventStore = store
		return nil
	}
}

func WithTool(tool tools.Tool) Option {
	return func(h *Harness) error {
		return h.toolRegistry.Register(tool)
	}
}

func WithTools(toolList ...tools.Tool) Option {
	return func(h *Harness) error {
		for _, tool := range toolList {
			if err := h.toolRegistry.Register(tool); err != nil {
				return err
			}
		}

		return nil
	}
}

func WithDefaultTools(workspaceRoot string) Option {
	return func(h *Harness) error {
		echoTool := tools.NewEchoTool()

		readFileTool, err := tools.NewReadFileTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create read_file tool: %w", err)
		}

		writeFileTool, err := tools.NewWriteFileTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create write_file tool: %w", err)
		}

		return WithTools(echoTool, readFileTool, writeFileTool)(h)
	}
}
