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

func TestFishCompletionUsesExactCommandPaths(t *testing.T) {
	output := renderFishCompletion()
	if strings.Contains(output, "__fish_seen_subcommand_from") {
		t.Fatalf("Fish completion still uses broad seen-subcommand conditions:\n%s", output)
	}
	for _, want := range []string{
		"__multisubs_path_is codex",
		"__multisubs_path_is claude",
		"__multisubs_path_starts_with doctor",
		"__multisubs_path_starts_with codex doctor",
		"__multisubs_path_is codex monitor",
		"__multisubs_path_starts_with codex monitor doctor",
		"__multisubs_path_is help codex monitor",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("Fish completion output missing condition %q", want)
		}
	}
	if strings.Contains(output, "__multisubs_path_starts_with claude doctor") ||
		strings.Contains(output, "__multisubs_path_is claude doctor") {
		t.Fatalf("Fish completion attaches flags or arguments to claude doctor:\n%s", output)
	}
}

func TestFishCompletionTokensFollowStrictCommandTree(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want []string
	}{
		{
			name: "top level",
			want: []string{"init", "doctor", "codex", "claude", "completion", "version", "help"},
		},
		{
			name: "Claude provider",
			path: []string{"claude"},
			want: []string{"add", "login", "cli", "exec", "status", "usage", "doctor", "help"},
		},
		{
			name: "Claude doctor has no flags",
			path: []string{"claude", "doctor"},
		},
		{
			name: "Codex status has no deeper commands",
			path: []string{"codex", "status"},
		},
		{
			name: "top-level doctor flags",
			path: []string{"doctor"},
			want: []string{"--json", "--timeout"},
		},
		{
			name: "top-level doctor flags after option",
			path: []string{"doctor", "--json"},
			want: []string{"--json", "--timeout"},
		},
		{
			name: "focused Codex doctor flags",
			path: []string{"codex", "doctor"},
			want: []string{"--json", "--timeout"},
		},
		{
			name: "focused Codex doctor flags after value",
			path: []string{"codex", "doctor", "--timeout", "2s"},
			want: []string{"--json", "--timeout"},
		},
		{
			name: "Codex monitor root",
			path: []string{"codex", "monitor"},
			want: []string{
				"doctor", "completion", "help", "tui",
				"--interval", "--timeout", "--no-color", "--no-alt-screen",
				"--include-default", "--include-active", "--discover",
			},
		},
		{
			name: "Codex monitor doctor",
			path: []string{"codex", "monitor", "doctor"},
			want: []string{"--json", "--timeout", "--include-default", "--include-active", "--discover", "--app-server"},
		},
		{
			name: "Codex monitor completion",
			path: []string{"codex", "monitor", "completion"},
			want: []string{"bash", "zsh", "fish"},
		},
		{
			name: "Codex monitor help",
			path: []string{"codex", "monitor", "help"},
		},
		{
			name: "Codex monitor TUI",
			path: []string{"codex", "monitor", "tui"},
			want: []string{
				"--interval", "--timeout", "--no-color", "--no-alt-screen",
				"--include-default", "--include-active", "--discover",
			},
		},
		{
			name: "nested global help",
			path: []string{"help", "codex", "monitor"},
			want: []string{"doctor", "completion", "help", "tui"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := fishCompletionTokens(test.path)
			if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("Fish tokens for %q: got=%q want=%q", strings.Join(test.path, " "), got, test.want)
			}
		})
	}
}

func TestCompletionStopsAtCodexMonitorHelp(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		forbidden string
	}{
		{
			name:      "bash",
			output:    renderBashCompletion(),
			forbidden: `help)` + "\n" + `                COMPREPLY=( $(compgen -W "doctor completion tui help"`,
		},
		{
			name:      "zsh",
			output:    renderZshCompletion(),
			forbidden: `help) compadd -- doctor completion tui help`,
		},
		{
			name:      "fish",
			output:    renderFishCompletion(),
			forbidden: "__multisubs_path_is codex monitor help",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if strings.Contains(test.output, test.forbidden) {
				t.Fatalf("%s completion advertises arguments after codex monitor help:\n%s", test.name, test.output)
			}
		})
	}
}

func TestProviderHelpCompletionStopsAfterOneTopic(t *testing.T) {
	tests := []struct {
		shell     string
		namespace string
		output    string
		guarded   string
		fishPath  []string
	}{
		{
			shell:     "bash",
			namespace: "codex",
			output:    renderBashCompletion(),
			guarded: "        help)\n" +
				"          if (( COMP_CWORD == 3 )); then\n" +
				"            COMPREPLY=( $(compgen -W \"init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help\" -- \"$cur\") )\n" +
				"          fi",
		},
		{
			shell:     "bash",
			namespace: "claude",
			output:    renderBashCompletion(),
			guarded: "        help)\n" +
				"          if (( COMP_CWORD == 3 )); then\n" +
				"            COMPREPLY=( $(compgen -W \"add login cli exec status usage doctor help\" -- \"$cur\") )\n" +
				"          fi",
		},
		{
			shell:     "zsh",
			namespace: "codex",
			output:    renderZshCompletion(),
			guarded: "        help)\n" +
				"          if (( CURRENT == 4 )); then\n" +
				"            compadd -- init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help\n" +
				"          fi",
		},
		{
			shell:     "zsh",
			namespace: "claude",
			output:    renderZshCompletion(),
			guarded: "        help)\n" +
				"          if (( CURRENT == 4 )); then\n" +
				"            compadd -- add login cli exec status usage doctor help\n" +
				"          fi",
		},
		{
			shell:     "fish",
			namespace: "codex",
			fishPath:  []string{"codex", "help", "login"},
		},
		{
			shell:     "fish",
			namespace: "claude",
			fishPath:  []string{"claude", "help", "login"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.shell+"_"+test.namespace, func(t *testing.T) {
			if test.shell == "fish" {
				if got := fishCompletionTokens(test.fishPath); len(got) != 0 {
					t.Fatalf("Fish completion continues after %s help topic: %q", test.namespace, got)
				}
				return
			}
			if !strings.Contains(test.output, test.guarded) {
				t.Fatalf("%s completion lacks a one-topic %s help guard:\n%s", test.shell, test.namespace, test.output)
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
