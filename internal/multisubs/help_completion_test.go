package multisubs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalHelpShowsSymmetricProviderNamespaces(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"help"})
	})
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}
	for _, want := range []string{
		"multisubs",
		"codex <command>",
		"claude <command>",
		"doctor [flags]",
		"completion <shell>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("global help missing %q", want)
		}
	}
	if strings.Contains(out, "multicodex") {
		t.Fatalf("global help contains the old product command: %s", out)
	}
	if strings.Contains(out, "multisubs usage") || strings.Contains(out, "reserved") {
		t.Fatalf("global help advertises top-level usage: %s", out)
	}
}

func TestProviderAndNestedHelpTopics(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"codex", "help"}, want: "multisubs codex"},
		{args: []string{"claude", "help"}, want: "multisubs claude"},
		{args: []string{"help", "codex", "heartbeat"}, want: "multisubs codex heartbeat"},
		{args: []string{"help", "codex", "monitor", "doctor"}, want: "multisubs codex monitor doctor"},
		{args: []string{"help", "claude", "exec"}, want: "multisubs claude exec"},
	}
	app := newTestAppForCLI(t)
	for _, test := range tests {
		test := test
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return app.Run(test.args)
			})
			if err != nil {
				t.Fatalf("help topic failed: %v", err)
			}
			if !strings.Contains(out, test.want) {
				t.Fatalf("help output missing %q: %s", test.want, out)
			}
		})
	}
}

func TestHelpUnknownTopic(t *testing.T) {
	app := newTestAppForCLI(t)
	_, err := captureStdout(t, func() error {
		return app.Run([]string{"help", "does-not-exist"})
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T (%v)", err, err)
	}
	if !strings.Contains(exitErr.Message, "unknown help topic") {
		t.Fatalf("unexpected message: %s", exitErr.Message)
	}
}

func TestVersionUsesNewProductIdentity(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"version"})
	})
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if !strings.HasPrefix(out, "multisubs ") || strings.Contains(out, "multicodex") {
		t.Fatalf("unexpected version output: %q", out)
	}
}

func TestCompletionScriptsCoverSymmetricCommandTree(t *testing.T) {
	tests := []struct {
		name string
		out  string
	}{
		{name: "bash", out: renderBashCompletion()},
		{name: "zsh", out: renderZshCompletion()},
		{name: "fish", out: renderFishCompletion()},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for _, want := range []string{
				"multisubs",
				"codex",
				"claude",
				"login",
				"exec",
				"status",
				"usage",
				"monitor",
				"doctor",
				"dry-run",
				"__complete-codex-profiles",
				"__complete-claude-profiles",
			} {
				if !strings.Contains(test.out, want) {
					t.Errorf("completion output missing %q", want)
				}
			}
			if strings.Contains(test.out, "multicodex") || strings.Contains(test.out, "__complete-profiles") {
				t.Errorf("completion output contains a legacy command or helper")
			}
			if strings.Contains(test.out, "completion version usage help") {
				t.Errorf("completion output advertises top-level usage")
			}
		})
	}
}

func TestCompletionCommandRejectsUnsupportedShell(t *testing.T) {
	app := newTestAppForCLI(t)
	_, err := captureStdout(t, func() error {
		return app.Run([]string{"completion", "tcsh"})
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T (%v)", err, err)
	}
}

func TestDynamicCodexProfileCompletionIsSorted(t *testing.T) {
	app := newTestAppForCLI(t)
	cfg := DefaultConfig()
	cfg.Profiles["zeta"] = Profile{Name: "zeta", CodexHome: filepath.Join(app.store.paths.ProfilesDir, "zeta", "codex-home")}
	cfg.Profiles["alpha"] = Profile{Name: "alpha", CodexHome: filepath.Join(app.store.paths.ProfilesDir, "alpha", "codex-home")}
	if err := app.store.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return app.Run([]string{"__complete-codex-profiles"})
	})
	if err != nil {
		t.Fatalf("complete Codex profiles failed: %v", err)
	}
	if got := strings.Fields(out); len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("unexpected Codex profile list: %q", out)
	}
}

func TestDynamicClaudeProfileCompletionIsSorted(t *testing.T) {
	app := newTestAppForCLI(t)
	store := newClaudeStore(app.store.paths)
	cfg := defaultClaudeConfig()
	for _, name := range []string{"zeta", "alpha"} {
		profile, err := store.CreateProfile(name)
		if err != nil {
			t.Fatalf("create Claude profile %s: %v", name, err)
		}
		cfg.Profiles[name] = profile
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save Claude config: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return app.Run([]string{"__complete-claude-profiles"})
	})
	if err != nil {
		t.Fatalf("complete Claude profiles failed: %v", err)
	}
	if got := strings.Fields(out); len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("unexpected Claude profile list: %q", out)
	}
}

func newTestAppForCLI(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))

	app, err := NewApp()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = old
	}()

	runErr := fn()
	_ = w.Close()
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read output: %v", readErr)
	}
	_ = r.Close()
	return string(out), runErr
}
