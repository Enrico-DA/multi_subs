package multicodex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeExecRoutesNonFableByLowestWorstUsageAndPassesArgsExactly(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	usageByDir := map[string][]byte{
		profiles["alpha"].ConfigDir: fakeClaudeUsageEnvelope(20, 40, nil),
		profiles["beta"].ConfigDir:  fakeClaudeUsageEnvelope(10, 50, nil),
	}
	setFakeUsageCapture(t, runner, usageByDir)
	wantArgs := []string{"-p", "--model", "sonnet", "prompt text", "--allowedTools", "Read,Glob"}
	runner.run = func(_ context.Context, args, env []string) error {
		if !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("Claude args changed: got %#v want %#v", args, wantArgs)
		}
		if got := claudeConfigDirFromEnv(env); got != profiles["alpha"].ConfigDir {
			t.Fatalf("selected config dir: got %q want %q", got, profiles["alpha"].ConfigDir)
		}
		return nil
	}
	if err := app.cmdClaudeExec(wantArgs[1:]); err != nil {
		t.Fatalf("Claude exec: %v", err)
	}
	for _, call := range runner.Calls() {
		if call.Kind == "capture" && claudeConfigDirFromEnv(call.Env) == "" {
			t.Fatal("default reserve usage should not be queried while a managed profile is eligible")
		}
	}
}

func TestClaudeExecFableRoutingRequiresAllThreeWindows(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta", "missing")
	alphaFable := 90.0
	betaFable := 55.0
	usageByDir := map[string][]byte{
		profiles["alpha"].ConfigDir:   fakeClaudeUsageEnvelope(10, 20, &alphaFable),
		profiles["beta"].ConfigDir:    fakeClaudeUsageEnvelope(30, 40, &betaFable),
		profiles["missing"].ConfigDir: fakeClaudeUsageEnvelope(1, 2, nil),
	}
	setFakeUsageCapture(t, runner, usageByDir)
	runner.run = func(_ context.Context, args, env []string) error {
		if got := claudeConfigDirFromEnv(env); got != profiles["beta"].ConfigDir {
			t.Fatalf("selected Fable config dir: got %q want %q", got, profiles["beta"].ConfigDir)
		}
		if !reflect.DeepEqual(args, []string{"-p", "--model=fable", "hello"}) {
			t.Fatalf("unexpected args: %#v", args)
		}
		return nil
	}
	if err := app.cmdClaudeExec([]string{"--model=fable", "hello"}); err != nil {
		t.Fatalf("Fable exec: %v", err)
	}
}

func TestClaudeExecUsesDefaultOnlyWhenNoManagedProfileIsQuotaEligible(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "exhausted", "broken")
	usageByDir := map[string][]byte{
		profiles["exhausted"].ConfigDir: fakeClaudeUsageEnvelope(10, 100, nil),
		profiles["broken"].ConfigDir:    []byte(`{"is_error":true,"result":"not logged in"}`),
		"":                              fakeClaudeUsageEnvelope(2, 3, nil),
	}
	setFakeUsageCapture(t, runner, usageByDir)
	runner.run = func(_ context.Context, args, env []string) error {
		if claudeConfigDirFromEnv(env) != "" || envContainsKey(env, "CLAUDE_CONFIG_DIR") {
			t.Fatalf("default reserve must have CLAUDE_CONFIG_DIR absent: %q", env)
		}
		if !reflect.DeepEqual(args, []string{"-p", "hello"}) {
			t.Fatalf("unexpected default args: %#v", args)
		}
		return nil
	}
	if err := app.cmdClaudeExec([]string{"hello"}); err != nil {
		t.Fatalf("default reserve exec: %v", err)
	}
}

func TestClaudeExecSkipsBusyManagedProfileAndUsesAnotherEligibleManagedProfile(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	setFakeUsageCapture(t, runner, map[string][]byte{
		profiles["alpha"].ConfigDir: fakeClaudeUsageEnvelope(1, 2, nil),
		profiles["beta"].ConfigDir:  fakeClaudeUsageEnvelope(10, 20, nil),
	})
	store := newClaudeStore(app.store.paths)
	alphaReservation, acquired, err := store.acquireReservation("alpha")
	if err != nil || !acquired {
		t.Fatalf("reserve alpha: acquired=%v err=%v", acquired, err)
	}
	defer alphaReservation.Release()
	runner.run = func(_ context.Context, _ []string, env []string) error {
		if got := claudeConfigDirFromEnv(env); got != profiles["beta"].ConfigDir {
			t.Fatalf("expected beta after busy alpha, got %q", got)
		}
		return nil
	}
	if err := app.cmdClaudeExec([]string{"hello"}); err != nil {
		t.Fatalf("exec with one busy profile: %v", err)
	}
}

func TestClaudeExecReturnsBusyWithoutUsingDefaultWhenAllEligibleManagedProfilesAreBusy(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	setFakeUsageCapture(t, runner, map[string][]byte{
		profiles["alpha"].ConfigDir: fakeClaudeUsageEnvelope(1, 2, nil),
		profiles["beta"].ConfigDir:  fakeClaudeUsageEnvelope(3, 4, nil),
	})
	store := newClaudeStore(app.store.paths)
	var held []*claudeReservation
	for _, name := range []string{"alpha", "beta"} {
		reservation, acquired, err := store.acquireReservation(name)
		if err != nil || !acquired {
			t.Fatalf("reserve %s: acquired=%v err=%v", name, acquired, err)
		}
		held = append(held, reservation)
	}
	defer func() {
		for _, reservation := range held {
			reservation.Release()
		}
	}()
	runner.run = func(context.Context, []string, []string) error {
		t.Fatal("child must not start when all managed profiles are busy")
		return nil
	}

	err := app.cmdClaudeExec([]string{"hello"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != claudeBusyExitCode {
		t.Fatalf("expected busy ExitError, got %T %v", err, err)
	}
	if !strings.Contains(exitErr.Message, "default reserve was not used") {
		t.Fatalf("unexpected busy message: %s", exitErr.Message)
	}
	for _, call := range runner.Calls() {
		if call.Kind == "capture" && claudeConfigDirFromEnv(call.Env) == "" {
			t.Fatal("default usage must not be queried when eligible managed profiles are merely busy")
		}
	}
}

func TestClaudeExecHoldsReservationUntilChildReturnsAndReleasesOnExitError(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha")
	setFakeUsageCapture(t, runner, map[string][]byte{
		profiles["alpha"].ConfigDir: fakeClaudeUsageEnvelope(1, 2, nil),
	})
	store := newClaudeStore(app.store.paths)
	runner.run = func(context.Context, []string, []string) error {
		reservation, acquired, err := store.acquireReservation("alpha")
		if err != nil {
			t.Fatalf("probe held reservation: %v", err)
		}
		if acquired {
			reservation.Release()
			t.Fatal("reservation was not held while the child was running")
		}
		return &ExitError{Code: 42}
	}
	err := app.cmdClaudeExec([]string{"hello"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 42 {
		t.Fatalf("expected child exit 42, got %T %v", err, err)
	}
	reservation, acquired, err := store.acquireReservation("alpha")
	if err != nil || !acquired {
		t.Fatalf("reservation was not released after child return: acquired=%v err=%v", acquired, err)
	}
	reservation.Release()
}

func TestClaudeReservationRejectsSymlinkAndHardlinkLockFiles(t *testing.T) {
	for _, test := range []struct {
		name       string
		makeUnsafe func(*testing.T, string, string)
		message    string
	}{
		{
			name: "symlink",
			makeUnsafe: func(t *testing.T, lockPath, root string) {
				if err := os.Remove(lockPath); err != nil {
					t.Fatalf("remove lock: %v", err)
				}
				target := filepath.Join(root, "outside.lock")
				if err := os.WriteFile(target, nil, 0o600); err != nil {
					t.Fatalf("write target: %v", err)
				}
				if err := os.Symlink(target, lockPath); err != nil {
					t.Fatalf("symlink lock: %v", err)
				}
			},
			message: "symlink",
		},
		{
			name: "hardlink",
			makeUnsafe: func(t *testing.T, lockPath, root string) {
				if err := os.Link(lockPath, filepath.Join(root, "outside.link")); err != nil {
					t.Fatalf("hardlink lock: %v", err)
				}
			},
			message: "multiple hard links",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _, root := newClaudeTestApp(t)
			createClaudeProfiles(t, app, "alpha")
			store := newClaudeStore(app.store.paths)
			reservation, acquired, err := store.acquireReservation("alpha")
			if err != nil || !acquired {
				t.Fatalf("create initial lock: acquired=%v err=%v", acquired, err)
			}
			reservation.Release()
			lockPath := filepath.Join(store.paths.ClaudeRunDir, "reservations", "claude-alpha.lock")
			test.makeUnsafe(t, lockPath, root)
			_, _, err = store.acquireReservation("alpha")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %s rejection, got %v", test.message, err)
			}
		})
	}
}

func TestClaudeExecHelpFastPathSkipsStateAndUsage(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	runner.run = func(_ context.Context, args, env []string) error {
		if !reflect.DeepEqual(args, []string{"-p", "--help"}) {
			t.Fatalf("official help args: %#v", args)
		}
		if envContainsKey(env, "CLAUDE_CONFIG_DIR") {
			t.Fatalf("official help inherited Claude config: %q", env)
		}
		return nil
	}
	if err := app.cmdClaude([]string{"exec", "--help"}); err != nil {
		t.Fatalf("Claude exec help: %v", err)
	}
	if _, err := os.Stat(app.store.paths.MulticodexHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("help fast path mutated state: %v", err)
	}
	for _, call := range runner.Calls() {
		if call.Kind == "capture" {
			t.Fatalf("help fast path queried usage: %+v", call)
		}
	}
}

func setFakeUsageCapture(t *testing.T, runner *fakeClaudeRunner, usageByDir map[string][]byte) {
	t.Helper()
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, []string{"-p", "--output-format", "json", "/usage"}) {
			t.Fatalf("unexpected capture args: %#v", args)
		}
		configDir := claudeConfigDirFromEnv(env)
		result, ok := usageByDir[configDir]
		if !ok {
			return nil, []byte("not logged in"), errors.New("usage unavailable")
		}
		return result, nil, nil
	}
}
