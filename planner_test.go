package main

import (
	"strings"
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

func TestRailpackVersionIsResolved(t *testing.T) {
	if v := railpackVersion(); !strings.HasPrefix(v, "v") {
		t.Errorf("railpackVersion() = %q, want a module version", v)
	}
}
