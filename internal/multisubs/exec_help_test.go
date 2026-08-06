package multisubs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdExecHelpClearsStaleProfileEnv(t *testing.T) {
	app, logPath := newExecTestApp(t)
	t.Setenv("CODEX_HOME", "/tmp/stale-codex")
	t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")

	if err := app.Run([]string{"codex", "exec", "--help"}); err != nil {
		t.Fatalf("exec help failed: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "args=exec --help") {
		t.Fatalf("expected help passthrough args, got %q", log)
	}
	if strings.Contains(log, managedCodexAuthConfig) {
		t.Fatalf("exact help delegation received managed auth override: %q", log)
	}
	if !strings.Contains(log, "profile=\n") {
		t.Fatalf("expected active profile to be cleared, got %q", log)
	}
	if !strings.Contains(log, "codex_home=\n") {
		t.Fatalf("expected codex home to be cleared, got %q", log)
	}
}

func TestCmdExecHelpFailureDoesNotRepeatArguments(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 23\n"), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", binDir)

	app := newTestAppForCLI(t)
	const privateArgument = "synthetic-private-argument"
	stderr, err := captureStderr(t, func() error {
		return app.cmdExec([]string{"--help", privateArgument})
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 23 {
		t.Fatalf("exec help failure = %T (%v), want exit code 23", err, err)
	}
	if exitErr.Message != "Codex exec help command failed" {
		t.Fatalf("unexpected exec help failure: %q", exitErr.Message)
	}
	if strings.Contains(err.Error(), privateArgument) || strings.Contains(stderr, privateArgument) {
		t.Fatalf("exec help failure repeated a private argument: error=%q stderr=%q", err, stderr)
	}
}
