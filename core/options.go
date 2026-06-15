package core

import "github.com/shxntanu/epsilon/core/tools"

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

func WithProvider(provider Provider) Option {
	return func(h *Harness) error {
		h.provider = provider
		return nil
	}
}

func WithTool(tool tools.Tool) Option {
	return func(h *Harness) error {
		return h.toolRegistry.Register(tool)
	}
}
