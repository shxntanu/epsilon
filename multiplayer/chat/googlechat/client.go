package googlechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (c *Client) configured() error {
	if c == nil {
		return fmt.Errorf("googlechat: client is nil")
	}
	return nil
}

// GetAttachment retrieves Google Chat attachment metadata by resource name.
func (c *Client) GetAttachment(ctx context.Context, name string) (Attachment, error) {
	if err := c.configured(); err != nil {
		return Attachment{}, err
	}
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return Attachment{}, fmt.Errorf("googlechat: attachment name is empty")
	}
	body, err := c.get(ctx, c.baseURL()+"/"+name)
	if err != nil {
		return Attachment{}, err
	}
	var attachment Attachment
	if err := json.Unmarshal(body, &attachment); err != nil {
		return Attachment{}, fmt.Errorf("googlechat: decode attachment metadata: %w", err)
	}
	return attachment, nil
}

// DownloadMedia downloads uploaded media bytes by attachmentDataRef.resourceName.
func (c *Client) DownloadMedia(ctx context.Context, resourceName string) ([]byte, error) {
	if err := c.configured(); err != nil {
		return nil, err
	}
	resourceName = strings.Trim(strings.TrimSpace(resourceName), "/")
	if resourceName == "" {
		return nil, fmt.Errorf("googlechat: media resource name is empty")
	}
	return c.get(ctx, c.baseURL()+"/media/"+escapeResourceName(resourceName)+"?alt=media")
}

// DownloadAttachment downloads a normalized Google Chat attachment body.
func (c *Client) DownloadAttachment(ctx context.Context, attachment chat.Attachment) ([]byte, error) {
	if err := c.configured(); err != nil {
		return nil, err
	}
	if resourceName := strings.TrimSpace(attachment.Metadata["attachment_data_resource_name"]); resourceName != "" {
		return c.DownloadMedia(ctx, resourceName)
	}
	downloadURI := strings.TrimSpace(attachment.DownloadURI)
	if downloadURI == "" {
		return nil, fmt.Errorf("googlechat: attachment %q has no downloadable media reference", attachment.Name)
	}
	return c.get(ctx, downloadURI)
}

// Reply posts text into an existing Google Chat thread.
func (c *Client) Reply(ctx context.Context, reply chat.Reply) error {
	if err := c.configured(); err != nil {
		return err
	}
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
	token, err := c.bearerToken(ctx)
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

func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	token, err := c.bearerToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("googlechat: create get request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googlechat: send get request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("googlechat: read get response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("googlechat: get failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) bearerToken(ctx context.Context) (string, error) {
	tokenProvider := c.TokenProvider
	if tokenProvider == nil {
		return "", fmt.Errorf("googlechat: token provider is not configured")
	}
	return tokenProvider.BearerToken(ctx)
}

func (c *Client) baseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return defaultAPIBaseURL
	}
	return baseURL
}

func escapeResourceName(resourceName string) string {
	return strings.ReplaceAll(url.PathEscape(resourceName), "%2F", "/")
}
