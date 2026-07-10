package epsilond

import (
	"strings"

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/orchestrator"
)

type messageKind string

const (
	messageKindNewTask   messageKind = "new_task"
	messageKindFollowup  messageKind = "followup"
	messageKindInterrupt messageKind = "interrupt"
	messageKindStatus    messageKind = "status"
)

type messageClassification struct {
	Kind      messageKind
	Interrupt orchestrator.InterruptKind
	Reason    string
}

func classifyMessage(event *chat.Event) messageClassification {
	text := normalizedCommandText(event.Text)
	switch {
	case hasAnyPhrase(text, "status", "what are you doing", "what is your status", "progress", "current findings", "summarize current findings"):
		return messageClassification{Kind: messageKindStatus, Reason: event.Text}
	case hasAnyPhrase(text, "stop", "cancel", "abort", "nevermind", "never mind"):
		return messageClassification{Kind: messageKindInterrupt, Interrupt: orchestrator.InterruptKindCancel, Reason: event.Text}
	case hasAnyPhrase(text, "pause", "hold on", "wait"):
		return messageClassification{Kind: messageKindInterrupt, Interrupt: orchestrator.InterruptKindPause, Reason: event.Text}
	case hasAnyPhrase(text, "ignore that", "instead", "check this instead", "look at this instead", "actually"):
		return messageClassification{Kind: messageKindInterrupt, Interrupt: orchestrator.InterruptKindReplan, Reason: event.Text}
	case hasAnyPhrase(text, "also", "additionally", "one more thing", "follow up"):
		return messageClassification{Kind: messageKindFollowup, Reason: event.Text}
	default:
		return messageClassification{Kind: messageKindNewTask, Reason: event.Text}
	}
}

func normalizedCommandText(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "@epsilon", "")
	return strings.Join(strings.Fields(text), " ")
}

func hasAnyPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
