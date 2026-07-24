package multisubs

import (
	"errors"
	"strings"
	"testing"
)

func TestProfileCodexEnvSetsProfileAndStripsAccountOverrides(t *testing.T) {
	t.Parallel()

	env := profileCodexEnv([]string{
		"PATH=/bin",
		"CODEX_HOME=/tmp/stale",
		"MULTISUBS_HOME=/tmp/product",
		"MULTISUBS_ACTIVE_PROFILE=stale",
		"MULTISUBS_SELECTED_PROFILE_PATH=/tmp/out",
		"MULTISUBS_FUTURE_CONTROL=stale",
		"MULTICODEX_HOME=/tmp/legacy",
		"MULTICODEX_ACTIVE_PROFILE=legacy",
		"OPENAI_API_KEY=secret",
		"OPENAI_BASE_URL=https://example.invalid",
		"CODEX_AUTH_TOKEN=secret",
	}, "/tmp/codex-home", "work")

	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{
		"CODEX_HOME=/tmp/stale",
		"MULTISUBS_ACTIVE_PROFILE=stale",
		"MULTISUBS_SELECTED_PROFILE_PATH=",
		"MULTISUBS_HOME=",
		"MULTISUBS_FUTURE_CONTROL=",
		"MULTICODEX_HOME=",
		"MULTICODEX_ACTIVE_PROFILE=",
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
	if !strings.Contains(joined, "MULTISUBS_ACTIVE_PROFILE=work") {
		t.Fatalf("expected profile env to be set, got %q", joined)
	}
	productVariables := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, "MULTISUBS_") {
			productVariables++
			if entry != "MULTISUBS_ACTIVE_PROFILE=work" {
				t.Fatalf("managed Codex env retained unexpected product variable %q", entry)
			}
		}
	}
	if productVariables != 1 {
		t.Fatalf("managed Codex env product variable count: got %d want 1; env=%q", productVariables, env)
	}
}

func TestNeutralCodexEnvStripsProfileState(t *testing.T) {
	env := neutralCodexEnv([]string{
		"PATH=/bin",
		"CODEX_HOME=/tmp/stale",
		"MULTISUBS_ACTIVE_PROFILE=stale",
		"MULTISUBS_FUTURE_CONTROL=stale",
		"OPENAI_API_KEY=secret",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CODEX_HOME=") || strings.Contains(joined, "MULTISUBS_") || strings.Contains(joined, "MULTI"+"CODEX_") || strings.Contains(joined, "OPENAI_API_KEY=") {
		t.Fatalf("expected neutral env to strip Codex profile and account overrides, got %q", joined)
	}
	if !strings.Contains(joined, "PATH=/bin") {
		t.Fatalf("expected PATH to remain, got %q", joined)
	}
}

func TestDefaultCodexEnvReceivesNoProductVariables(t *testing.T) {
	env := profileCodexEnv([]string{
		"PATH=/bin",
		"MULTISUBS_ACTIVE_PROFILE=stale",
		"MULTISUBS_FUTURE_CONTROL=stale",
	}, "/tmp/default-codex-home", "")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "MULTISUBS_") || strings.Contains(joined, "MULTI"+"CODEX_") {
		t.Fatalf("default Codex env retained product variables: %q", joined)
	}
	if !strings.Contains(joined, "CODEX_HOME=/tmp/default-codex-home") {
		t.Fatalf("default Codex env dropped selected CODEX_HOME: %q", joined)
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
	if !strings.Contains(env, "MULTISUBS_ACTIVE_PROFILE=work") {
		t.Fatalf("expected profile env in env, got %q", env)
	}
}
