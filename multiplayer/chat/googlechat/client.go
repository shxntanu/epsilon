package googlechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shxntanu/epsilon/multiplayer/chat"
)

const defaultAPIBaseURL = "https://chat.googleapis.com/v1"

// TokenProvider returns an OAuth bearer token for Google Chat API calls.
type TokenProvider interface {
	BearerToken(ctx context.Context) (string, error)
}

// StaticTokenProvider returns a fixed bearer token.
type StaticTokenProvider struct {
	Token string
}

// BearerToken returns the configured static token.
func (p StaticTokenProvider) BearerToken(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	token := strings.TrimSpace(p.Token)
	if token == "" {
		return "", fmt.Errorf("googlechat: bearer token is empty")
	}
	return token, nil
}

// Client posts replies through the Google Chat API.
type Client struct {
	BaseURL       string
	TokenProvider TokenProvider
	HTTPClient    *http.Client
}

// Reply posts text into an existing Google Chat thread.
func (c Client) Reply(ctx context.Context, reply chat.Reply) error {
	if reply.Platform != "" && reply.Platform != chat.PlatformGoogleChat {
		return fmt.Errorf("googlechat: unsupported reply platform %q", reply.Platform)
	}
	spaceName := strings.Trim(strings.TrimSpace(reply.SpaceName), "/")
	if spaceName == "" {
		return fmt.Errorf("googlechat: reply space name is empty")
	}
	if strings.TrimSpace(reply.Text) == "" {
		return fmt.Errorf("googlechat: reply text is empty")
	}
	tokenProvider := c.TokenProvider
	if tokenProvider == nil {
		return fmt.Errorf("googlechat: token provider is not configured")
	}
	token, err := tokenProvider.BearerToken(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(NewReplyPayload(reply.ThreadName, reply.Text))
	if err != nil {
		return fmt.Errorf("googlechat: encode reply payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL()+"/"+spaceName+"/messages?messageReplyOption=REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD",
		bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("googlechat: create reply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("googlechat: send reply: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		return fmt.Errorf("googlechat: reply failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c Client) baseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return defaultAPIBaseURL
	}
	return baseURL
}
