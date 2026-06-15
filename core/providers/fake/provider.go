package fake

import (
	"context"

	"github.com/shxntanu/epsilon/core/types"
)

type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Respond(ctx context.Context, req types.ModelRequest) (*types.ModelResponse, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role == types.RoleUser && len(msg.Content) > 0 {
			return &types.ModelResponse{
				Message: types.AssistantMessage("Echo: " + msg.Content[0].Text),
			}, nil
		}
	}

	return &types.ModelResponse{
		Message: types.AssistantMessage("Echo:"),
	}, nil
}
