package planner

import (
	"runtime/debug"
	"testing"
)

func TestProviderSummary(t *testing.T) {
	tests := []struct {
		name      string
		providers []string
		want      string
	}{
		{name: "none", providers: nil, want: "none detected"},
		{name: "empty entry", providers: []string{""}, want: "none detected"},
		{name: "one", providers: []string{"node"}, want: "node"},
		{name: "several", providers: []string{"node", "python"}, want: "node, python"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerSummary(tt.providers); got != tt.want {
				t.Errorf("providerSummary(%v) = %q, want %q", tt.providers, got, tt.want)
			}
		})
	}
}

func TestRailpackVersionFrom(t *testing.T) {
	deps := []*debug.Module{
		{Path: "github.com/moby/buildkit", Version: "v0.32.2"},
		{Path: railpackModulePath, Version: "v0.36.4"},
	}

	if got := railpackVersionFrom(deps); got != "v0.36.4" {
		t.Errorf("railpackVersionFrom(deps) = %q, want %q", got, "v0.36.4")
	}
	if got := railpackVersionFrom(nil); got != "unknown" {
		t.Errorf("railpackVersionFrom(nil) = %q, want %q", got, "unknown")
	}
}
