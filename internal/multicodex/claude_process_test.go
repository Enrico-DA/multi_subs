package multicodex

import (
	"strings"
	"testing"
)

func TestClaudeDefaultEnvRemovesConfigAndCredentialOverrides(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"WORKER_TOOL_SETTING=keep",
		"CLAUDE_CONFIG_DIR=/tmp/stale",
		"CLAUDE_CODE_OAUTH_TOKEN=secret",
		"ANTHROPIC_API_KEY=secret",
		"ANTHROPIC_AUTH_TOKEN=secret",
		"ANTHROPIC_BASE_URL=https://example.invalid",
		"CLAUDE_CODE_USE_BEDROCK=1",
		"CLAUDE_CODE_USE_VERTEX=1",
		"CLAUDE_CODE_USE_FOUNDRY=1",
		"MULTICODEX_CLAUDE_PROFILE=stale",
		"MULTICODEX_CLAUDE_CONFIG_DIR=/tmp/out",
		"MULTICODEX_CLAUDE_TARGET=stale",
		"MULTICODEX_ACTIVE_CLAUDE_PROFILE=stale",
		"MULTICODEX_SELECTED_CLAUDE_PROFILE=stale",
		"MULTICODEX_ACTIVE_PROVIDER=claude",
		"MULTICODEX_SELECTED_PROVIDER=claude",
	}
	env := claudeEnv(base, "")
	joined := strings.Join(env, "\n")
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"MULTICODEX_CLAUDE_PROFILE",
		"MULTICODEX_CLAUDE_CONFIG_DIR",
		"MULTICODEX_CLAUDE_TARGET",
		"MULTICODEX_ACTIVE_CLAUDE_PROFILE",
		"MULTICODEX_SELECTED_CLAUDE_PROFILE",
		"MULTICODEX_ACTIVE_PROVIDER",
		"MULTICODEX_SELECTED_PROVIDER",
	} {
		if envContainsKey(env, key) {
			t.Fatalf("default Claude env retained %s: %q", key, joined)
		}
	}
	for _, want := range []string{"PATH=/bin", "WORKER_TOOL_SETTING=keep"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("default Claude env dropped %q: %q", want, joined)
		}
	}
}

func TestClaudeManagedEnvSetsExactlyOneDerivedConfigDir(t *testing.T) {
	env := claudeEnv([]string{
		"PATH=/bin",
		"CLAUDE_CONFIG_DIR=/tmp/stale-one",
		"CLAUDE_CONFIG_DIR=/tmp/stale-two",
		"ANTHROPIC_API_KEY=secret",
	}, "/private/multicodex/providers/claude/profiles/work/config")
	count := 0
	for _, item := range env {
		if strings.HasPrefix(item, "CLAUDE_CONFIG_DIR=") {
			count++
			if item != "CLAUDE_CONFIG_DIR=/private/multicodex/providers/claude/profiles/work/config" {
				t.Fatalf("unexpected managed config entry: %q", item)
			}
		}
	}
	if count != 1 {
		t.Fatalf("CLAUDE_CONFIG_DIR entry count: got %d want 1; env=%q", count, env)
	}
	if envContainsKey(env, "ANTHROPIC_API_KEY") {
		t.Fatalf("managed Claude env retained API key: %q", env)
	}
}

func TestClaudeArgsRequestFableParsesModelWithoutChangingArgs(t *testing.T) {
	if !claudeArgsRequestFable([]string{"--model", "claude-fable-latest", "prompt"}) {
		t.Fatal("expected --model Fable to be detected")
	}
	if !claudeArgsRequestFable([]string{"--model=FABLE", "prompt"}) {
		t.Fatal("expected --model=FABLE to be detected")
	}
	if claudeArgsRequestFable([]string{"--", "--model=fable"}) {
		t.Fatal("model-looking prompt text after -- must not affect routing")
	}
	if claudeArgsRequestFable([]string{"prompt"}) {
		t.Fatal("omitted model must use non-Fable routing without model injection")
	}
}
