package main

import (
	"encoding/json"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	input := `{
		"repo": "https://github.com/user/repo.git",
		"ref": "abc123def",
		"context": "apps/web",
		"authSecret": "GIT_AUTH_TOKEN",
		"railpack": {
			"buildCmd": "npm run build",
			"startCmd": "npm start",
			"envs": {"NODE_ENV": "production"}
		}
	}`

	var config Config
	if err := json.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Repo != "https://github.com/user/repo.git" {
		t.Errorf("repo = %q, want %q", config.Repo, "https://github.com/user/repo.git")
	}
	if config.Ref != "abc123def" {
		t.Errorf("ref = %q, want %q", config.Ref, "abc123def")
	}
	if config.Context != "apps/web" {
		t.Errorf("context = %q, want %q", config.Context, "apps/web")
	}
	if config.AuthSecret != "GIT_AUTH_TOKEN" {
		t.Errorf("authSecret = %q, want %q", config.AuthSecret, "GIT_AUTH_TOKEN")
	}
	if config.Railpack.BuildCmd != "npm run build" {
		t.Errorf("railpack.buildCmd = %q, want %q", config.Railpack.BuildCmd, "npm run build")
	}
	if config.Railpack.StartCmd != "npm start" {
		t.Errorf("railpack.startCmd = %q, want %q", config.Railpack.StartCmd, "npm start")
	}
	if config.Railpack.Envs["NODE_ENV"] != "production" {
		t.Errorf("railpack.envs[NODE_ENV] = %q, want %q", config.Railpack.Envs["NODE_ENV"], "production")
	}
}

func TestConfigMinimal(t *testing.T) {
	input := `{"repo": "https://github.com/user/repo.git"}`

	var config Config
	if err := json.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Repo != "https://github.com/user/repo.git" {
		t.Errorf("repo = %q, want %q", config.Repo, "https://github.com/user/repo.git")
	}
	if config.Ref != "" {
		t.Errorf("ref = %q, want empty", config.Ref)
	}
	if config.Context != "" {
		t.Errorf("context = %q, want empty", config.Context)
	}
	if config.AuthSecret != "" {
		t.Errorf("authSecret = %q, want empty", config.AuthSecret)
	}
}
