package multisubs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/processprobe"
)

func TestCodexLoginStatusTimeout(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}

	codexPath := filepath.Join(fakeBin, "codex")
	script := `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "login" ] && [ "${2:-}" = "status" ]; then
  sleep 3
  echo "Logged in using ChatGPT"
  exit 0
fi
if [ "${1:-}" = "--version" ]; then
  echo "codex-cli fake-timeout"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(codexPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	prev := codexLoginStatusTimeout
	codexLoginStatusTimeout = 150 * time.Millisecond
	defer func() { codexLoginStatusTimeout = prev }()

	start := time.Now()
	state, account, detail := codexLoginStatus(filepath.Join(root, "codex-home"))
	elapsed := time.Since(start)
	if state != "error" {
		t.Fatalf("expected error state, got %q", state)
	}
	if account != "-" {
		t.Fatalf("expected account '-', got %q", account)
	}
	if !strings.Contains(detail, "timed out") {
		t.Fatalf("expected timeout detail, got %q", detail)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected fast timeout handling, elapsed=%s", elapsed)
	}
}

func TestCodexLoginStatusDoesNotClassifyTruncatedOutput(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	script := "#!/bin/sh\nprintf 'Logged in using ChatGPT account@example.test\\n'\nhead -c " + strconv.Itoa(processprobe.OutputLimit*2) + " /dev/zero\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+":/usr/bin:/bin")

	state, account, detail := probeCodexLoginStatusWithTimeout(filepath.Join(root, "codex-home"), 2*time.Second, false)
	if state != "error" || account != "" {
		t.Fatalf("truncated output was classified: state=%q account=%q detail=%q", state, account, detail)
	}
	if detail != "codex login status output exceeded safe limit" {
		t.Fatalf("unexpected truncated-output detail: %q", detail)
	}
}
