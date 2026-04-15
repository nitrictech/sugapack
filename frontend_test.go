package main

import (
	"testing"

	"github.com/moby/buildkit/client/llb"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestFetchGitSource(t *testing.T) {
	config := Config{
		Repo: "https://github.com/user/repo.git",
		Ref:  "abc123",
	}

	state := fetchGitSource(config)

	// Verify state can be marshaled (basic sanity check)
	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state: %v", err)
	}
}

func TestFetchGitSourceWithContext(t *testing.T) {
	config := Config{
		Repo:    "https://github.com/user/repo.git",
		Ref:     "abc123",
		Context: "apps/web",
	}

	state := fetchGitSource(config)

	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state with context: %v", err)
	}
}

func TestFetchGitSourceWithAuth(t *testing.T) {
	config := Config{
		Repo:       "https://github.com/user/private-repo.git",
		Ref:        "main",
		AuthSecret: "GIT_AUTH_TOKEN",
	}

	state := fetchGitSource(config)

	_, err := state.Marshal(t.Context(), llb.Platform(specs.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatalf("failed to marshal git source state with auth: %v", err)
	}
}

func TestBuildPlanArgs(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   []string
	}{
		{
			name:   "basic",
			config: Config{},
			want:   []string{"sugapack", "plan", "/src", "--out", "/out/plan.json"},
		},
		{
			name: "with build cmd",
			config: Config{
				Railpack: RailpackConfig{BuildCmd: "npm run build"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--build-cmd", "npm run build"},
		},
		{
			name: "with start cmd",
			config: Config{
				Railpack: RailpackConfig{StartCmd: "npm start"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--start-cmd", "npm start"},
		},
		{
			name: "with both",
			config: Config{
				Railpack: RailpackConfig{BuildCmd: "make", StartCmd: "./server"},
			},
			want: []string{"sugapack", "plan", "/src", "--out", "/out/plan.json", "--build-cmd", "make", "--start-cmd", "./server"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPlanArgs(tt.config)
			if len(got) != len(tt.want) {
				t.Fatalf("buildPlanArgs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildPlanArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]string
		want specs.Platform
	}{
		{
			name: "default",
			opts: map[string]string{},
			want: specs.Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			name: "arm64",
			opts: map[string]string{"platform": "linux/arm64"},
			want: specs.Platform{OS: "linux", Architecture: "arm64"},
		},
		{
			name: "arm64 v8",
			opts: map[string]string{"platform": "linux/arm64/v8"},
			want: specs.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePlatform(tt.opts)
			if got.OS != tt.want.OS || got.Architecture != tt.want.Architecture || got.Variant != tt.want.Variant {
				t.Errorf("parsePlatform() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
