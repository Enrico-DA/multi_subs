package multicodex

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestWithManagedCodexAuthOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no delimiter",
			args: []string{"--model", "gpt-5", "prompt"},
			want: []string{"--model", "gpt-5", "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "before delimiter",
			args: []string{"--sandbox", "read-only", "--", "-c", "prompt text"},
			want: []string{"--sandbox", "read-only", "-c", managedCodexAuthConfig, "--", "-c", "prompt text"},
		},
		{
			name: "short config",
			args: []string{"-c", `cli_auth_credentials_store="keyring"`, "prompt"},
			want: []string{"-c", `cli_auth_credentials_store="keyring"`, "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "long config",
			args: []string{"--config", `cli_auth_credentials_store="keyring"`, "prompt"},
			want: []string{"--config", `cli_auth_credentials_store="keyring"`, "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "long config equals",
			args: []string{`--config=cli_auth_credentials_store="keyring"`, "prompt"},
			want: []string{`--config=cli_auth_credentials_store="keyring"`, "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "attached short config",
			args: []string{`-ccli_auth_credentials_store="keyring"`, "prompt"},
			want: []string{`-ccli_auth_credentials_store="keyring"`, "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "repeated config overrides",
			args: []string{"-c", "one=1", "--config=two=2", "-cthree=3", "prompt"},
			want: []string{"-c", "one=1", "--config=two=2", "-cthree=3", "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "short profile",
			args: []string{"-p", "unsafe", "prompt"},
			want: []string{"-p", "unsafe", "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "long profile",
			args: []string{"--profile", "unsafe", "prompt"},
			want: []string{"--profile", "unsafe", "prompt", "-c", managedCodexAuthConfig},
		},
		{
			name: "harmless arguments keep order",
			args: []string{"--model=gpt-5", "--sandbox", "workspace-write", "review this repo"},
			want: []string{"--model=gpt-5", "--sandbox", "workspace-write", "review this repo", "-c", managedCodexAuthConfig},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			original := append([]string(nil), test.args...)
			got := withManagedCodexAuthOverride(test.args)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("managed args: got %#v want %#v", got, test.want)
			}
			if !reflect.DeepEqual(test.args, original) {
				t.Fatalf("input args changed: got %#v want %#v", test.args, original)
			}
		})
	}
}

func TestProfileCodexEnvSetsProfileAndStripsAccountOverrides(t *testing.T) {
	t.Parallel()

	env := profileCodexEnv([]string{
		"PATH=/bin",
		"CODEX_HOME=/tmp/stale",
		"MULTICODEX_ACTIVE_PROFILE=stale",
		"MULTICODEX_SELECTED_PROFILE_PATH=/tmp/out",
		"OPENAI_API_KEY=secret",
		"OPENAI_BASE_URL=https://example.invalid",
		"CODEX_AUTH_TOKEN=secret",
	}, "/tmp/codex-home", "work")

	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{
		"CODEX_HOME=/tmp/stale",
		"MULTICODEX_ACTIVE_PROFILE=stale",
		"MULTICODEX_SELECTED_PROFILE_PATH=",
		"OPENAI_API_KEY=",
		"OPENAI_BASE_URL=",
		"CODEX_AUTH_TOKEN=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("expected %s to be stripped from %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/bin") {
		t.Fatalf("expected PATH to remain, got %q", joined)
	}
	if !strings.Contains(joined, "CODEX_HOME=/tmp/codex-home") {
		t.Fatalf("expected CODEX_HOME to be set, got %q", joined)
	}
	if !strings.Contains(joined, "MULTICODEX_ACTIVE_PROFILE=work") {
		t.Fatalf("expected profile env to be set, got %q", joined)
	}
}

func TestNeutralCodexEnvStripsProfileState(t *testing.T) {
	env := neutralCodexEnv([]string{
		"PATH=/bin",
		"CODEX_HOME=/tmp/stale",
		"MULTICODEX_ACTIVE_PROFILE=stale",
		"OPENAI_API_KEY=secret",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CODEX_HOME=") || strings.Contains(joined, "MULTICODEX_ACTIVE_PROFILE=") || strings.Contains(joined, "OPENAI_API_KEY=") {
		t.Fatalf("expected neutral env to strip Codex profile and account overrides, got %q", joined)
	}
	if !strings.Contains(joined, "PATH=/bin") {
		t.Fatalf("expected PATH to remain, got %q", joined)
	}
}

func TestRunInteractiveCodexWithProfileExecsWhenTerminalAttached(t *testing.T) {
	oldLookPath := execLookPath
	oldSyscallExec := syscallExec
	oldInteractive := isInteractiveTerminalAttached
	t.Cleanup(func() {
		execLookPath = oldLookPath
		syscallExec = oldSyscallExec
		isInteractiveTerminalAttached = oldInteractive
	})

	isInteractiveTerminalAttached = func() bool { return true }
	execLookPath = func(bin string) (string, error) {
		if bin != "codex" {
			t.Fatalf("unexpected bin lookup: %s", bin)
		}
		return "/tmp/fake-codex", nil
	}

	var gotPath string
	var gotArgs []string
	var gotEnv []string
	sentinel := errors.New("exec called")
	syscallExec = func(path string, args []string, env []string) error {
		gotPath = path
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), env...)
		return sentinel
	}

	err := RunInteractiveCodexWithProfile("/tmp/codex-home", "work", []string{"--version"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if gotPath != "/tmp/fake-codex" {
		t.Fatalf("unexpected exec path: %q", gotPath)
	}
	if want := []string{"codex", "--version", "-c", managedCodexAuthConfig}; strings.Join(gotArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected exec args: got=%q want=%q", gotArgs, want)
	}
	env := strings.Join(gotEnv, "\n")
	if !strings.Contains(env, "CODEX_HOME=/tmp/codex-home") {
		t.Fatalf("expected CODEX_HOME in env, got %q", env)
	}
	if !strings.Contains(env, "MULTICODEX_ACTIVE_PROFILE=work") {
		t.Fatalf("expected profile env in env, got %q", env)
	}
}
