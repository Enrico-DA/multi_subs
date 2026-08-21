package multisubs

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

func TestMonitorHelpIncludesDoctorAndSnapshotPointer(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"codex", "monitor", "help"})
	})
	if err != nil {
		t.Fatalf("monitor help failed: %v", err)
	}
	if !strings.Contains(out, "multisubs usage") {
		t.Fatalf("expected a pointer to the usage snapshot, got:\n%s", out)
	}
	if !strings.Contains(out, "multisubs codex monitor doctor") {
		t.Fatalf("expected doctor usage in monitor help, got:\n%s", out)
	}
	for _, want := range []string{"--timeout 60s", "--json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in monitor help, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "--include-default Include the global Codex home (default true)") {
		t.Fatalf("expected global Codex home default in monitor help, got:\n%s", out)
	}
}

func TestHelpMonitorTopic(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"help", "codex", "monitor"})
	})
	if err != nil {
		t.Fatalf("help monitor failed: %v", err)
	}
	if !strings.Contains(out, "multisubs codex monitor") {
		t.Fatalf("expected monitor help topic output, got:\n%s", out)
	}
	if strings.Contains(out, "tui") {
		t.Fatalf("monitor help topic should not mention a terminal interface, got:\n%s", out)
	}
}

func TestMonitorUnknownSubcommand(t *testing.T) {
	app := newTestAppForCLI(t)
	_, err := captureStdout(t, func() error {
		return app.Run([]string{"codex", "monitor", "snapshot"})
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T (%v)", err, err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(exitErr.Message, "unknown monitor command") {
		t.Fatalf("unexpected message: %s", exitErr.Message)
	}
}

func TestMonitorDoctorHelpFlagSucceeds(t *testing.T) {
	app := newTestAppForCLI(t)
	if err := app.Run([]string{"codex", "monitor", "doctor", "--help"}); err != nil {
		t.Fatalf("monitor doctor --help failed: %v", err)
	}
}

func TestMonitorCompletionDefaultsToBash(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"codex", "monitor", "completion"})
	})
	if err != nil {
		t.Fatalf("monitor completion failed: %v", err)
	}
	if !strings.Contains(out, "complete -F _multisubs_complete multisubs") {
		t.Fatalf("expected bash completion registration, got:\n%s", out)
	}
}

func TestHelpMonitorCompletionTopic(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"help", "codex", "monitor", "completion"})
	})
	if err != nil {
		t.Fatalf("help monitor completion failed: %v", err)
	}
	if !strings.Contains(out, "multisubs codex monitor completion") {
		t.Fatalf("expected monitor completion help topic output, got:\n%s", out)
	}
}

func TestMonitorDoctorMixedChecksFailSummaryAndExit(t *testing.T) {
	t.Parallel()

	report := usage.DoctorReport{Checks: []usage.DoctorCheck{
		{Name: "codex binary", OK: false, Details: "missing"},
		{Name: "oauth fetch: personal", OK: true, Details: "ok"},
	}}
	var buf bytes.Buffer
	printMonitorDoctorHumanTo(&buf, report)

	out := buf.String()
	if !strings.Contains(out, "monitor doctor result: FAIL (degraded: at least one check failed)") {
		t.Fatalf("expected failed degraded result, got %q", out)
	}
	if strings.Contains(out, "monitor doctor result: PASS") {
		t.Fatalf("failed monitor check printed PASS: %q", out)
	}
	err := monitorDoctorResult(report)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("monitor doctor result = %T (%v), want exit code 1", err, err)
	}
}

func TestBareMonitorPrintsUsage(t *testing.T) {
	app := newTestAppForCLI(t)
	out, err := captureStdout(t, func() error {
		return app.Run([]string{"codex", "monitor"})
	})
	if err != nil {
		t.Fatalf("bare monitor failed: %v", err)
	}
	if !strings.Contains(out, "multisubs codex monitor doctor") {
		t.Fatalf("expected monitor usage, got:\n%s", out)
	}
}
