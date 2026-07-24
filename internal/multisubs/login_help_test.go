package multisubs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRunCLITargetScopedLoginHelpUsesNeutralProviderWithoutState(t *testing.T) {
	tests := []struct {
		provider string
		wantArgs []string
		install  func(*testing.T, string) string
	}{
		{provider: "codex", wantArgs: []string{"login"}, install: installCodexHelpRecorder},
		{provider: "claude", wantArgs: []string{"auth", "login", "--claudeai"}, install: installClaudeHelpRecorder},
	}
	for _, test := range tests {
		test := test
		for _, helpFlag := range []string{"--help", "-h"} {
			helpFlag := helpFlag
			t.Run(test.provider+"_"+strings.TrimLeft(helpFlag, "-"), func(t *testing.T) {
				root := t.TempDir()
				multisubsHome := filepath.Join(root, "missing-multisubs-home")
				t.Setenv("HOME", root)
				t.Setenv("MULTISUBS_HOME", multisubsHome)
				t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "missing-default-codex"))
				setStaleProviderEnvironment(t, root)
				logPath := test.install(t, root)

				if err := RunCLI([]string{test.provider, "login", "missing-profile", helpFlag}); err != nil {
					t.Fatalf("RunCLI target-scoped %s login help: %v", test.provider, err)
				}

				wantArgs := append(append([]string(nil), test.wantArgs...), helpFlag)
				assertRecordedNeutralProviderInvocation(t, logPath, wantArgs)
				if _, err := os.Stat(multisubsHome); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("target-scoped %s login help created product state: %v", test.provider, err)
				}
			})
		}
	}
}

func TestCodexTargetScopedLoginHelpWorksThroughEveryDispatch(t *testing.T) {
	dispatches := []struct {
		name string
		run  func(*App, string) error
	}{
		{name: "App.Run", run: func(app *App, flag string) error {
			return app.Run([]string{"codex", "login", "missing-profile", flag})
		}},
		{name: "cmdCodex", run: func(app *App, flag string) error {
			return app.cmdCodex([]string{"login", "missing-profile", flag})
		}},
		{name: "cmdLogin", run: func(app *App, flag string) error {
			return app.cmdLogin([]string{"missing-profile", flag})
		}},
	}
	for _, dispatch := range dispatches {
		dispatch := dispatch
		for _, helpFlag := range []string{"--help", "-h"} {
			helpFlag := helpFlag
			t.Run(dispatch.name+"_"+strings.TrimLeft(helpFlag, "-"), func(t *testing.T) {
				root := t.TempDir()
				multisubsHome := filepath.Join(root, "missing-multisubs-home")
				t.Setenv("MULTISUBS_HOME", multisubsHome)
				t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "missing-default-codex"))
				setStaleProviderEnvironment(t, root)
				t.Setenv("MULTI"+"CODEX_HOME", filepath.Join(root, "legacy-home"))
				logPath := installCodexHelpRecorder(t, root)
				app, err := NewApp()
				if err != nil {
					t.Fatalf("NewApp: %v", err)
				}

				if err := dispatch.run(app, helpFlag); err != nil {
					t.Fatalf("%s target-scoped Codex login help: %v", dispatch.name, err)
				}

				assertRecordedNeutralProviderInvocation(t, logPath, []string{"login", helpFlag})
				if _, err := os.Stat(multisubsHome); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("%s target-scoped Codex login help created product state: %v", dispatch.name, err)
				}
			})
		}
	}
}

func TestClaudeTargetScopedLoginHelpWorksThroughEveryDispatchWithoutProbes(t *testing.T) {
	dispatches := []struct {
		name string
		run  func(*App, string) error
	}{
		{name: "App.Run", run: func(app *App, flag string) error {
			return app.Run([]string{"claude", "login", "missing-profile", flag})
		}},
		{name: "cmdClaude", run: func(app *App, flag string) error {
			return app.cmdClaude([]string{"login", "missing-profile", flag})
		}},
		{name: "cmdClaudeLogin", run: func(app *App, flag string) error {
			return app.cmdClaudeLogin([]string{"missing-profile", flag})
		}},
	}
	for _, dispatch := range dispatches {
		dispatch := dispatch
		for _, helpFlag := range []string{"--help", "-h"} {
			helpFlag := helpFlag
			t.Run(dispatch.name+"_"+strings.TrimLeft(helpFlag, "-"), func(t *testing.T) {
				app, runner, root := newClaudeTestApp(t)
				setStaleProviderEnvironment(t, root)
				t.Setenv("MULTI"+"CODEX_HOME", filepath.Join(root, "legacy-home"))
				runner.run = func(_ context.Context, args, env []string) error {
					wantArgs := []string{"auth", "login", "--claudeai", helpFlag}
					if !reflect.DeepEqual(args, wantArgs) {
						t.Fatalf("Claude login help args: got=%q want=%q", args, wantArgs)
					}
					assertNeutralProviderEnvironment(t, env)
					return nil
				}

				if err := dispatch.run(app, helpFlag); err != nil {
					t.Fatalf("%s target-scoped Claude login help: %v", dispatch.name, err)
				}

				calls := runner.Calls()
				if len(calls) != 1 || calls[0].Kind != "run" {
					t.Fatalf("Claude login help made provider probes or extra calls: %+v", calls)
				}
				if _, err := os.Stat(app.store.paths.MultisubsHome); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("%s target-scoped Claude login help created product state: %v", dispatch.name, err)
				}
			})
		}
	}
}

func TestRunCLIRejectsMixedTargetScopedLoginHelpBeforeState(t *testing.T) {
	for _, args := range [][]string{
		{"codex", "login", "missing-profile", "--help", "extra"},
		{"codex", "login", "missing-profile", "-h", "--help"},
		{"codex", "login", "missing-profile", "--", "--help"},
		{"claude", "login", "missing-profile", "--help", "extra"},
		{"claude", "login", "missing-profile", "-h", "--help"},
		{"claude", "login", "missing-profile", "--", "--help"},
	} {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := t.TempDir()
			multisubsHome := filepath.Join(root, "missing-multisubs-home")
			t.Setenv("HOME", root)
			t.Setenv("MULTISUBS_HOME", multisubsHome)
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "missing-default-codex"))
			setStaleProviderEnvironment(t, root)

			err := RunCLI(args)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("RunCLI(%q) = %T (%v), want exit code 2", args, err, err)
			}
			if _, err := os.Stat(multisubsHome); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("mixed login help created product state: %v", err)
			}
		})
	}
}

func setStaleProviderEnvironment(t *testing.T, root string) {
	t.Helper()
	t.Setenv("CODEX_HOME", filepath.Join(root, "stale-codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "stale-claude"))
	t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")
	t.Setenv("MULTISUBS_FUTURE_CONTROL", "stale")
	t.Setenv("OPENAI_API_KEY", "stale-secret")
	t.Setenv("CODEX_AUTH_TOKEN", "stale-secret")
	t.Setenv("ANTHROPIC_API_KEY", "stale-secret")
	t.Setenv("WORKER_TOOL_SETTING", "keep")
}

func installClaudeHelpRecorder(t *testing.T, root string) string {
	t.Helper()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "claude-help.log")
	script := `#!/bin/sh
{
  printf 'arg_count=%s\n' "$#"
  index=0
  for arg in "$@"; do
    printf 'arg_%s=%s\n' "$index" "$arg"
    index=$((index + 1))
  done
  env
} > ` + shellQuote(logPath) + "\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Claude: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func assertRecordedNeutralProviderInvocation(t *testing.T, logPath string, wantArgs []string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read provider help log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "arg_count="+strconv.Itoa(len(wantArgs))+"\n") {
		t.Fatalf("provider help argument count: got log %q want args %q", log, wantArgs)
	}
	for index, arg := range wantArgs {
		if !strings.Contains(log, "arg_"+strconv.Itoa(index)+"="+arg+"\n") {
			t.Fatalf("provider help arguments: got log %q want args %q", log, wantArgs)
		}
	}
	if strings.Contains(log, managedCodexAuthConfig) {
		t.Fatalf("neutral provider help received managed Codex auth override: %q", log)
	}
	if !strings.Contains(log, "\nWORKER_TOOL_SETTING=keep\n") {
		t.Fatalf("neutral provider help dropped unrelated environment: %q", log)
	}
	provider := "claude"
	if len(wantArgs) > 0 && wantArgs[0] == "login" {
		provider = "codex"
	}
	assertNeutralProviderLog(t, log, provider)
}

func assertNeutralProviderEnvironment(t *testing.T, env []string) {
	t.Helper()
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if key == "CLAUDE_CONFIG_DIR" ||
			strings.HasPrefix(key, "MULTISUBS_") || strings.HasPrefix(key, "MULTI"+"CODEX_") ||
			key == "ANTHROPIC_API_KEY" {
			t.Fatalf("neutral provider environment retained %s: %q", key, env)
		}
	}
}

func assertNeutralProviderLog(t *testing.T, log, provider string) {
	t.Helper()
	for _, line := range strings.Split(log, "\n") {
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		denied := strings.HasPrefix(key, "MULTISUBS_") || strings.HasPrefix(key, "MULTI"+"CODEX_")
		if provider == "codex" {
			denied = denied || key == "CODEX_HOME" || key == "OPENAI_API_KEY" || key == "CODEX_AUTH_TOKEN"
		} else {
			denied = denied || key == "CLAUDE_CONFIG_DIR" || key == "ANTHROPIC_API_KEY"
		}
		if denied {
			t.Fatalf("neutral provider log retained %s: %q", key, log)
		}
	}
}
