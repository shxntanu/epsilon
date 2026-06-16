package tui

import (
	"testing"

	"github.com/shxntanu/epsilon/core/slash"
)

func TestSlashSelectorQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQuery string
		wantOK    bool
	}{
		{
			name:      "slash prefix",
			input:     "/sta",
			wantQuery: "sta",
			wantOK:    true,
		},
		{
			name:      "bare slash",
			input:     "/",
			wantQuery: "",
			wantOK:    true,
		},
		{
			name:  "escaped slash",
			input: "//help",
		},
		{
			name:  "command with args",
			input: "/events off",
		},
		{
			name:  "plain message",
			input: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotOK := slashSelectorQuery(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotQuery != tt.wantQuery {
				t.Fatalf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestDensityModeFromSlash(t *testing.T) {
	if got := densityModeFromSlash(slash.DensityCompact); got != densityCompact {
		t.Fatalf("compact density = %v, want compact", got)
	}
	if got := slashDensity(densityComfortable); got != slash.DensityComfortable {
		t.Fatalf("comfortable density = %v, want comfortable", got)
	}
}
