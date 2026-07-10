package googlechat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shxntanu/epsilon/multiplayer/chat"
)

const (
	// DefaultMaxBodyBytes is the maximum request body size accepted by DecodeEvent.
	DefaultMaxBodyBytes int64 = 1 << 20

	epsilonMention = "@epsilon"
)

var (
	// ErrBodyTooLarge is returned when DecodeEvent receives more than DefaultMaxBodyBytes.
	ErrBodyTooLarge = errors.New("googlechat: event body too large")
	// ErrNilReader is returned when DecodeEvent is called with a nil reader.
	ErrNilReader = errors.New("googlechat: nil reader")
)

// InteractionEvent is the subset of a Google Chat interaction event used by epsilond.
type InteractionEvent struct {
	Type      string          `json:"type"`
	EventTime string          `json:"eventTime,omitempty"`
	Message   ChatMessage     `json:"message,omitempty"`
	Space     Space           `json:"space,omitempty"`
	Thread    Thread          `json:"thread,omitempty"`
	User      User            `json:"user,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

// ChatMessage is the subset of a Google Chat message carried by interaction events.
type ChatMessage struct {
	Name        string       `json:"name,omitempty"`
	Text        string       `json:"text,omitempty"`
	CreateTime  string       `json:"createTime,omitempty"`
	Sender      User         `json:"sender,omitempty"`
	Space       Space        `json:"space,omitempty"`
	Thread      Thread       `json:"thread,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
	Attachments []Attachment `json:"attachment,omitempty"`
}

// UnmarshalJSON accepts both Google Chat's attachment field and pluralized test fixtures.
func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	type chatMessage ChatMessage
	var raw struct {
		chatMessage
		Attachment  []Attachment `json:"attachment,omitempty"`
		Attachments []Attachment `json:"attachments,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*m = ChatMessage(raw.chatMessage)
	m.Attachments = raw.Attachment
	if len(m.Attachments) == 0 {
		m.Attachments = raw.Attachments
	}
	return nil
}

// Space identifies the Google Chat space where an event occurred.
type Space struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// Thread identifies a Google Chat message thread.
type Thread struct {
	Name string `json:"name,omitempty"`
}

// User identifies a Google Chat user.
type User struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Type        string `json:"type,omitempty"`
	IsAnonymous bool   `json:"isAnonymous,omitempty"`
}

// Annotation is the subset of Google Chat message annotations used for mention detection.
type Annotation struct {
	Type        string      `json:"type,omitempty"`
	StartIndex  int         `json:"startIndex,omitempty"`
	Length      int         `json:"length,omitempty"`
	UserMention UserMention `json:"userMention,omitempty"`
}

// UserMention describes an annotated user mention in a Google Chat message.
type UserMention struct {
	User User   `json:"user,omitempty"`
	Type string `json:"type,omitempty"`
}

// Attachment is the subset of Google Chat attachment metadata preserved by normalization.
type Attachment struct {
	Name         string `json:"name,omitempty"`
	ContentName  string `json:"contentName,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	ThumbnailURI string `json:"thumbnailUri,omitempty"`
	DownloadURI  string `json:"downloadUri,omitempty"`
	Source       string `json:"source,omitempty"`
}

// Event is an internal normalized representation of a Google Chat interaction event.
type Event = chat.Event

// AttachmentMeta contains stable attachment metadata copied from Google Chat.
type AttachmentMeta = chat.Attachment

// ReplyPayload is a Google Chat message payload for replying in an existing thread.
type ReplyPayload struct {
	Text   string      `json:"text"`
	Thread ReplyThread `json:"thread,omitempty"`
}

// ReplyThread identifies the thread for a Google Chat reply payload.
type ReplyThread struct {
	Name string `json:"name,omitempty"`
}

// DecodeEvent decodes and normalizes a Google Chat interaction event.
func DecodeEvent(r io.Reader) (*chat.Event, error) {
	if r == nil {
		return nil, ErrNilReader
	}

	body, err := io.ReadAll(io.LimitReader(r, DefaultMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("googlechat: read event: %w", err)
	}
	if int64(len(body)) > DefaultMaxBodyBytes {
		return nil, ErrBodyTooLarge
	}

	var event InteractionEvent
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&event); err != nil {
		return nil, fmt.Errorf("googlechat: decode event: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("googlechat: invalid trailing JSON")
	}

	event.Raw = append(json.RawMessage(nil), body...)
	return Normalize(event)
}

// Normalize converts a decoded Google Chat interaction event into epsilond's stable event shape.
func Normalize(input InteractionEvent) (*chat.Event, error) {
	createTime, err := parseTime(firstNonEmpty(input.Message.CreateTime, input.EventTime))
	if err != nil {
		return nil, err
	}

	sender := input.Message.Sender
	if sender.Name == "" && sender.DisplayName == "" {
		sender = input.User
	}

	attachments := make([]chat.Attachment, 0, len(input.Message.Attachments))
	for _, attachment := range input.Message.Attachments {
		attachments = append(attachments, chat.Attachment{
			Name:         attachment.Name,
			ContentName:  attachment.ContentName,
			ContentType:  attachment.ContentType,
			ThumbnailURI: attachment.ThumbnailURI,
			DownloadURI:  attachment.DownloadURI,
			Source:       attachment.Source,
		})
	}

	return &chat.Event{
		Platform:          chat.PlatformGoogleChat,
		EventType:         input.Type,
		SpaceName:         firstNonEmpty(input.Message.Space.Name, input.Space.Name),
		SpaceDisplayName:  firstNonEmpty(input.Message.Space.DisplayName, input.Space.DisplayName),
		ThreadName:        firstNonEmpty(input.Message.Thread.Name, input.Thread.Name),
		MessageName:       input.Message.Name,
		SenderName:        sender.Name,
		SenderDisplayName: sender.DisplayName,
		Text:              input.Message.Text,
		CreateTime:        createTime,
		Attachments:       attachments,
		IsMention:         IsEpsilonMention(input),
		Raw:               cloneRaw(input.Raw),
	}, nil
}

// IsEpsilonMention reports whether the event appears to mention @Epsilon.
func IsEpsilonMention(event InteractionEvent) bool {
	mentionAnnotations := 0
	for _, annotation := range event.Message.Annotations {
		if !strings.EqualFold(annotation.Type, "USER_MENTION") {
			continue
		}
		mentionAnnotations++

		user := annotation.UserMention.User
		if strings.EqualFold(user.DisplayName, "Epsilon") || strings.Contains(strings.ToLower(user.Name), "/epsilon") {
			return true
		}
	}
	if mentionAnnotations > 0 {
		return false
	}

	return containsEpsilonMention(event.Message.Text)
}

// NewReplyPayload returns a same-thread Google Chat reply payload.
func NewReplyPayload(threadName, text string) ReplyPayload {
	return ReplyPayload{
		Text: text,
		Thread: ReplyThread{
			Name: threadName,
		},
	}
}

// NewReplyPayloadJSON returns JSON for a same-thread Google Chat reply payload.
func NewReplyPayloadJSON(threadName, text string) ([]byte, error) {
	return json.Marshal(NewReplyPayload(threadName, text))
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("googlechat: parse create time: %w", err)
	}
	return parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func containsEpsilonMention(text string) bool {
	return strings.Contains(strings.ToLower(text), epsilonMention)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
