package core

type Option func(*Harness)

func WithEventBufferSize(size int) Option {
	return func(h *Harness) {
		h.eventBufferSize = size
	}
}

func WithRunIDGenerator(fn func() (string, error)) Option {
	return func(h *Harness) {
		h.newRunID = fn
	}
}

func WithProvider(provider Provider) Option {
	return func(h *Harness) {
		h.provider = provider
	}
}
