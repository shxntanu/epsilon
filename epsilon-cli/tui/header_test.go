package tui

import (
	"strings"
	"testing"
)

func TestHeaderAnimationState(t *testing.T) {
	idle := model{activityState: activityState{status: "ready"}}
	if idle.shouldAnimateHeader() {
		t.Fatal("idle ready header should not animate")
	}

	active := model{activityState: activityState{status: "thinking", busy: true}}
	if !active.shouldAnimateHeader() {
		t.Fatal("thinking header should animate")
	}
}

func TestRenderHeaderTextTruncatesLongSession(t *testing.T) {
	m := model{
		activityState: activityState{
			status:       "thinking",
			sessionTitle: "trace a very long multi repository refactor with streaming output",
		},
	}

	header := plainANSI(m.renderHeaderText(54))
	if !strings.Contains(header, "Epsilon") {
		t.Fatalf("header missing logo: %q", header)
	}
	if !strings.Contains(header, "thinking") {
		t.Fatalf("header missing status: %q", header)
	}
	if strings.Contains(header, "streaming output") {
		t.Fatalf("header did not truncate long session title: %q", header)
	}
}

func TestStatusBadgeUsesAnimatedThinkingFrame(t *testing.T) {
	first := plainANSI(model{activityState: activityState{
		status:      "thinking",
		headerFrame: 0,
	}}.renderStatusBadge())
	second := plainANSI(model{activityState: activityState{
		status:      "thinking",
		headerFrame: 1,
	}}.renderStatusBadge())

	if first == second {
		t.Fatalf("thinking badge did not change across frames: %q", first)
	}
}
