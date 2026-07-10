package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Platform identifies a chat provider.
type Platform string

const (
	// PlatformGoogleChat identifies Google Chat events and clients.
	PlatformGoogleChat Platform = "google_chat"
	// PlatformSlack identifies Slack events and clients.
	PlatformSlack Platform = "slack"
)

// RawEvent is the bounded HTTP request payload received from a chat platform.
type RawEvent struct {
	Platform   Platform
	Body       []byte
	Headers    http.Header
	ReceivedAt time.Time
}

// Event is epsilond's platform-neutral chat event.
type Event struct {
	Platform          Platform
	EventType         string
	SpaceName         string
	SpaceDisplayName  string
	ThreadName        string
	MessageName       string
	SenderName        string
	SenderDisplayName string
	Text              string
	CreateTime        time.Time
	ReceivedAt        time.Time
	Attachments       []Attachment
	IsMention         bool
	Raw               json.RawMessage
}

// Attachment contains stable platform-neutral attachment metadata.
type Attachment struct {
	Name         string
	ContentName  string
	ContentType  string
	ThumbnailURI string
	DownloadURI  string
	Source       string
	Metadata     map[string]string
}

// Reply identifies a platform-neutral message reply.
type Reply struct {
	Platform   Platform
	SpaceName  string
	ThreadName string
	Text       string
	Metadata   map[string]string
}

// Parser normalizes a provider-specific payload into an Event.
type Parser interface {
	Parse(ctx context.Context, event RawEvent) (*Event, error)
}

// Client sends platform-specific replies.
type Client interface {
	Reply(ctx context.Context, reply Reply) error
}

// AttachmentDownloader downloads platform-specific attachment bodies.
type AttachmentDownloader interface {
	DownloadAttachment(ctx context.Context, attachment Attachment) ([]byte, error)
}

// RequestVerifier verifies platform-specific HTTP requests before ingestion.
type RequestVerifier interface {
	Verify(ctx context.Context, r *http.Request) error
}

// Adapter combines parsing and reply behavior for one platform.
type Adapter interface {
	Parser
	Client
}

// ParserFunc adapts a function into a Parser.
type ParserFunc func(ctx context.Context, event RawEvent) (*Event, error)

// Parse calls f(ctx, event).
func (f ParserFunc) Parse(ctx context.Context, event RawEvent) (*Event, error) {
	return f(ctx, event)
}

// DecodeReaderParser adapts existing decoder functions that operate on readers.
func DecodeReaderParser(platform Platform, decode func(io.Reader) (*Event, error)) Parser {
	return ParserFunc(func(ctx context.Context, event RawEvent) (*Event, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		normalized, err := decode(bytesReader(event.Body))
		if err != nil {
			return nil, err
		}
		normalized.Platform = platform
		normalized.ReceivedAt = event.ReceivedAt
		return normalized, nil
	})
}

func bytesReader(body []byte) io.Reader {
	return bytes.NewReader(body)
}
