package multisubs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeDefaultEnvRemovesConfigAndCredentialOverrides(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"WORKER_TOOL_SETTING=keep",
		"CLAUDE_CONFIG_DIR=/tmp/stale",
		"CLAUDE_CODE_OAUTH_TOKEN=secret",
		"CLAUDE_CODE_OAUTH_REFRESH_TOKEN=secret",
		"CLAUDE_CODE_OAUTH_SCOPES=user:inference",
		"CLAUDE_CODE_OAUTH_CLIENT_ID=override",
		"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR=7",
		"ANTHROPIC_API_KEY=secret",
		"ANTHROPIC_AUTH_TOKEN=secret",
		"ANTHROPIC_BASE_URL=https://example.invalid",
		"ANTHROPIC_IDENTITY_TOKEN=secret",
		"ANTHROPIC_FOUNDRY_AUTH_TOKEN=secret",
		"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR=8",
		"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=1",
		"CLAUDE_CODE_USE_BEDROCK=1",
		"CLAUDE_CODE_USE_VERTEX=1",
		"CLAUDE_CODE_USE_FOUNDRY=1",
		"CLAUDE_CODE_USE_ANTHROPIC_AWS=1",
		"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD=1",
		"CLAUDE_CODE_USE_GATEWAY=1",
		"CLAUDE_CODE_USE_MANTLE=1",
		"MULTISUBS_CLAUDE_PROFILE=stale",
		"MULTISUBS_CLAUDE_CONFIG_DIR=/tmp/out",
		"MULTISUBS_CLAUDE_TARGET=stale",
		"MULTISUBS_ACTIVE_CLAUDE_PROFILE=stale",
		"MULTISUBS_SELECTED_CLAUDE_PROFILE=stale",
		"MULTISUBS_ACTIVE_PROVIDER=claude",
		"MULTISUBS_SELECTED_PROVIDER=claude",
		"MULTISUBS_FUTURE_CONTROL=stale",
		"MULTICODEX_HOME=/tmp/legacy",
		"MULTICODEX_CLAUDE_PROFILE=legacy",
		"CODEX_USAGE_MONITOR_ACCOUNTS_FILE=/tmp/legacy-accounts.json",
	}
	env := claudeEnv(base, "")
	joined := strings.Join(env, "\n")
	for _, key := range []string{
		"CLAUDE_CONFIG_DIR",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"CLAUDE_CODE_OAUTH_REFRESH_TOKEN",
		"CLAUDE_CODE_OAUTH_SCOPES",
		"CLAUDE_CODE_OAUTH_CLIENT_ID",
		"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_IDENTITY_TOKEN",
		"ANTHROPIC_FOUNDRY_AUTH_TOKEN",
		"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR",
		"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"CLAUDE_CODE_USE_ANTHROPIC_AWS",
		"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD",
		"CLAUDE_CODE_USE_GATEWAY",
		"CLAUDE_CODE_USE_MANTLE",
		"MULTISUBS_CLAUDE_PROFILE",
		"MULTISUBS_CLAUDE_CONFIG_DIR",
		"MULTISUBS_CLAUDE_TARGET",
		"MULTISUBS_ACTIVE_CLAUDE_PROFILE",
		"MULTISUBS_SELECTED_CLAUDE_PROFILE",
		"MULTISUBS_ACTIVE_PROVIDER",
		"MULTISUBS_SELECTED_PROVIDER",
		"MULTISUBS_FUTURE_CONTROL",
		"MULTICODEX_HOME",
		"MULTICODEX_CLAUDE_PROFILE",
		"CODEX_USAGE_MONITOR_ACCOUNTS_FILE",
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
		"MULTISUBS_FUTURE_CONTROL=stale",
	}, "/private/multisubs/providers/claude/profiles/work/config")
	count := 0
	for _, item := range env {
		if strings.HasPrefix(item, "CLAUDE_CONFIG_DIR=") {
			count++
			if item != "CLAUDE_CONFIG_DIR=/private/multisubs/providers/claude/profiles/work/config" {
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
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (strings.HasPrefix(key, "MULTISUBS_") || strings.HasPrefix(key, "MULTI"+"CODEX_")) {
			t.Fatalf("managed Claude env retained product variable %s: %q", key, env)
		}
	}
}

func TestClaudeEnvStripsEveryDeniedCredentialAndProviderSelector(t *testing.T) {
	const managedDir = "/private/multisubs/providers/claude/profiles/work/config"
	for key := range claudeDeniedEnvKeys {
		t.Run(key, func(t *testing.T) {
			defaultEnv := claudeEnv([]string{"PATH=/bin", key + "=untrusted"}, "")
			if envContainsKey(defaultEnv, key) {
				t.Fatalf("default environment retained denied key %s: %q", key, defaultEnv)
			}
			managedEnv := claudeEnv([]string{"PATH=/bin", key + "=untrusted"}, managedDir)
			if key == "CLAUDE_CONFIG_DIR" {
				if got := claudeConfigDirFromEnv(managedEnv); got != managedDir {
					t.Fatalf("managed config dir: got %q want %q", got, managedDir)
				}
				return
			}
			if envContainsKey(managedEnv, key) {
				t.Fatalf("managed environment retained denied key %s: %q", key, managedEnv)
			}
		})
	}
	for _, key := range []string{"CLAUDE_CODE_OAUTH_FUTURE_OVERRIDE", "CLAUDE_CODE_SKIP_FUTURE_AUTH"} {
		if envContainsKey(claudeEnv([]string{key + "=untrusted"}, ""), key) {
			t.Fatalf("default environment retained denied prefix key %s", key)
		}
	}
}

func TestClaudeProbeFailureUsesDeterministicCategories(t *testing.T) {
	const marker = "synthetic-secret-marker"
	expiredContext, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{name: "timeout", ctx: expiredContext, err: errors.New(marker), want: "timed out"},
		{
			name: "launch failure",
			ctx:  context.Background(),
			err:  &os.PathError{Op: "fork/exec", Path: marker, Err: errors.New(marker)},
			want: "launch failure",
		},
		{name: "unknown failure", ctx: context.Background(), err: errors.New(marker), want: "unknown failure"},
		{name: "nil error", ctx: context.Background(), want: "unknown failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := claudeProbeFailure(test.ctx, test.err)
			if got != test.want {
				t.Fatalf("failure category: got %q want %q", got, test.want)
			}
			if strings.Contains(got, marker) {
				t.Fatalf("failure category exposed synthetic secret: %q", got)
			}
		})
	}
}

func TestClaudeRunWithReservationPassesLockDescriptorToChild(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	output := filepath.Join(root, "result")
	script := "#!/bin/sh\nset -eu\n[ -e /dev/fd/3 ]\nprintf inherited > " + shellQuote(output) + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake Claude: %v", err)
	}
	t.Setenv("PATH", binDir+":/usr/bin:/bin")
	lockFile, err := os.OpenFile(filepath.Join(root, "reservation.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open reservation: %v", err)
	}
	defer lockFile.Close()
	env := []string{"PATH=" + os.Getenv("PATH")}
	if err := (osClaudeCommandRunner{}).RunWithReservation(context.Background(), []string{"probe"}, env, lockFile); err != nil {
		t.Fatalf("run with reservation: %v", err)
	}
	if data, err := os.ReadFile(output); err != nil || string(data) != "inherited" {
		t.Fatalf("child did not inherit reservation descriptor: data=%q err=%v", data, err)
	}
}
