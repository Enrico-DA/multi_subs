package multisubs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIHelpNamespacesDoNotCreateState(t *testing.T) {
	for _, args := range [][]string{
		{"help"},
		{"codex", "help"},
		{"claude", "help"},
		{"help", "usage"},
		{"help", "codex", "usage"},
		{"help", "codex", "exec"},
		{"help", "claude", "usage"},
	} {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("MULTISUBS_HOME", "")
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

			if err := RunCLI(args); err != nil {
				t.Fatalf("RunCLI(%q): %v", args, err)
			}
			if _, err := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("help created multisubs state: %v", err)
			}
		})
	}
}

func TestRunCLICodexCLIProfileHelpUsesNeutralProviderPathWithoutState(t *testing.T) {
	for _, helpFlag := range []string{"--help", "-h"} {
		helpFlag := helpFlag
		t.Run(strings.TrimLeft(helpFlag, "-"), func(t *testing.T) {
			root := t.TempDir()
			multisubsHome := filepath.Join(root, "missing-multisubs-home")
			t.Setenv("HOME", root)
			t.Setenv("MULTISUBS_HOME", multisubsHome)
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "stale-default-codex"))
			t.Setenv("CODEX_HOME", filepath.Join(root, "stale-codex"))
			t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")
			t.Setenv("MULTISUBS_SELECTED_PROFILE_PATH", filepath.Join(root, "stale-selection"))
			t.Setenv("MULTISUBS_HEARTBEAT_PROMPT", "stale-prompt")
			t.Setenv("MULTISUBS_FUTURE_CONTROL", "stale")
			t.Setenv("OPENAI_API_KEY", "stale-secret")
			t.Setenv("CODEX_AUTH_TOKEN", "stale-secret")
			t.Setenv("WORKER_TOOL_SETTING", "keep")
			logPath := installCodexHelpRecorder(t, root)

			if err := RunCLI([]string{"codex", "cli", "missing-profile", helpFlag}); err != nil {
				t.Fatalf("RunCLI target-scoped CLI help: %v", err)
			}
			assertNeutralCodexHelpInvocation(t, logPath, helpFlag, false)
			if _, err := os.Stat(multisubsHome); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("target-scoped CLI help created product state: %v", err)
			}
		})
	}
}

func TestRunCLIRejectsBareCodexCommandsWithoutStateMutation(t *testing.T) {
	commands := []string{
		"add",
		"login",
		"login-all",
		"cli",
		"exec",
		"status",
		"reconcile",
		"heartbeat",
		"monitor",
		"dry-run",
	}
	for _, command := range commands {
		command := command
		t.Run(command, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("MULTISUBS_HOME", "")
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

			err := RunCLI([]string{command})
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("RunCLI(%q) = %T (%v), want exit code 2", command, err, err)
			}
			if !strings.Contains(exitErr.Message, "multisubs codex "+command) {
				t.Fatalf("missing namespaced guidance: %q", exitErr.Message)
			}
			if _, err := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("bare command created state: %v", err)
			}
		})
	}
}

func TestRunCLIRejectsLegacyEnvironmentBeforeStateAccess(t *testing.T) {
	home := t.TempDir()
	legacyHome := filepath.Join(home, "multicodex")
	t.Setenv("HOME", home)
	t.Setenv("MULTICODEX_HOME", legacyHome)
	t.Setenv("MULTISUBS_HOME", filepath.Join(home, "multisubs"))

	err := RunCLI([]string{"help"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected legacy environment rejection, got %T (%v)", err, err)
	}
	if !strings.Contains(exitErr.Message, "clear them before running multisubs") {
		t.Fatalf("unexpected legacy environment error: %q", exitErr.Message)
	}
	for _, path := range []string{legacyHome, filepath.Join(home, "multisubs")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy rejection accessed state path %s: %v", path, err)
		}
	}
}

func TestRunCLIRejectsUnknownCommandWithoutStateMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MULTISUBS_HOME", "")
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

	err := RunCLI([]string{"typo-command"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected usage error, got %T (%v)", err, err)
	}
	if _, err := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown command created state: %v", err)
	}
}

func TestRunCLIRejectsUndocumentedArgumentsBeforeCreatingState(t *testing.T) {
	commands := [][]string{
		{"init", "unexpected"},
		{"version", "unexpected"},
		{"__complete-codex-profiles", "unexpected"},
		{"__complete-claude-profiles", "unexpected"},
		{"codex", "init", "unexpected"},
		{"codex", "login-all", "unexpected"},
		{"codex", "status", "unexpected"},
		{"codex", "usage", "--json"},
		{"codex", "reconcile", "unexpected"},
		{"codex", "cli", "--help", "unexpected"},
		{"codex", "cli", "-h", "missing-profile"},
		{"codex", "cli", "missing-profile", "--help", "unexpected"},
		{"codex", "cli", "missing-profile", "unexpected", "-h"},
		{"codex", "monitor", "doctor", "unexpected"},
		{"codex", "monitor", "tui", "unexpected"},
		{"claude", "status", "unexpected"},
		{"claude", "usage", "--json"},
		{"claude", "doctor", "unexpected"},
		{"usage", "--json"},
	}

	for _, args := range commands {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("MULTISUBS_HOME", "")
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

			err := RunCLI(args)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("RunCLI(%q) = %T (%v), want exit code 2", args, err, err)
			}
			if _, err := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid command created state: %v", err)
			}
		})
	}
}

func TestRunCLICodexStatusDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MULTISUBS_HOME", "")
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

	if err := RunCLI([]string{"codex", "status"}); err != nil {
		t.Fatalf("RunCLI codex status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("codex status created state: %v", err)
	}
}

func TestRunCLIRejectsTopLevelUsageArgumentsWithoutStateMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MULTISUBS_HOME", "")
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", "")

	err := RunCLI([]string{"usage", "--json"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected usage error, got %T (%v)", err, err)
	}
	if exitErr.Message != "usage: multisubs usage" {
		t.Fatalf("unexpected usage error: %q", exitErr.Message)
	}
	if _, statErr := os.Stat(filepath.Join(home, "multisubs")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid top-level usage created state: %v", statErr)
	}
}
