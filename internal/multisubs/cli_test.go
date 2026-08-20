package multisubs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

func TestCmdCLIRunsInteractiveCodexWithProfileEnv(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary")

	if err := app.Run([]string{"codex", "cli", "primary", "check this repo"}); err != nil {
		t.Fatalf("cli failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "profile=primary") {
		t.Fatalf("expected primary profile in log, got %q", log)
	}
	wantCodexHome := filepath.Join(app.store.paths.ProfilesDir, "primary", "codex-home")
	if !strings.Contains(log, "codex_home="+wantCodexHome) {
		t.Fatalf("expected primary CODEX_HOME in log, got %q", log)
	}
	wantArgs := "check this repo -c " + managedCodexAuthConfig
	if !strings.Contains(log, "args="+wantArgs) {
		t.Fatalf("expected cli args %q in log, got %q", wantArgs, log)
	}
}

func TestCmdCLIFailsWhenSharedConfigDoesNotUseFileStore(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary")
	writeDefaultConfig(t, app, "model = \"global\"\n")

	err := app.Run([]string{"codex", "cli", "primary"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T (%v)", err, err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(exitErr.Message, "requires file-backed auth") {
		t.Fatalf("unexpected error message: %s", exitErr.Message)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected codex to not be invoked, stat err=%v", err)
	}
}

func TestCmdCLIHelpWorksWithoutProfiles(t *testing.T) {
	app := newTestAppForCLI(t)

	out, err := captureStdout(t, func() error {
		return app.Run([]string{"codex", "cli", "--help"})
	})
	if err != nil {
		t.Fatalf("cli --help failed: %v", err)
	}
	if !strings.Contains(out, "multisubs codex cli [<name>") {
		t.Fatalf("expected cli help, got %q", out)
	}
}

func TestCodexCLIProfileHelpUsesNeutralProviderPathThroughAppDispatch(t *testing.T) {
	dispatches := []struct {
		name string
		run  func(*App, string) error
	}{
		{
			name: "App.Run",
			run: func(app *App, helpFlag string) error {
				return app.Run([]string{"codex", "cli", "missing-profile", helpFlag})
			},
		},
		{
			name: "cmdCodex",
			run: func(app *App, helpFlag string) error {
				return app.cmdCodex([]string{"cli", "missing-profile", helpFlag})
			},
		},
	}

	for _, dispatch := range dispatches {
		dispatch := dispatch
		for _, helpFlag := range []string{"--help", "-h"} {
			helpFlag := helpFlag
			t.Run(dispatch.name+"_"+strings.TrimLeft(helpFlag, "-"), func(t *testing.T) {
				root := t.TempDir()
				multisubsHome := filepath.Join(root, "missing-multisubs-home")
				t.Setenv("MULTISUBS_HOME", multisubsHome)
				t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "stale-default-codex"))
				t.Setenv("CODEX_HOME", filepath.Join(root, "stale-codex"))
				t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")
				t.Setenv("MULTISUBS_SELECTED_PROFILE_PATH", filepath.Join(root, "stale-selection"))
				t.Setenv("MULTISUBS_HEARTBEAT_PROMPT", "stale-prompt")
				t.Setenv("MULTISUBS_FUTURE_CONTROL", "stale")
				t.Setenv("OPENAI_API_KEY", "stale-secret")
				t.Setenv("CODEX_AUTH_TOKEN", "stale-secret")
				t.Setenv("MULTICODEX_HOME", filepath.Join(root, "legacy-product-state"))
				t.Setenv("MULTICODEX_ACTIVE_PROFILE", "legacy")
				t.Setenv("MULTICODEX_UNKNOWN_CONTROL", "legacy")
				t.Setenv("WORKER_TOOL_SETTING", "keep")
				logPath := installCodexHelpRecorder(t, root)

				app, err := NewApp()
				if err != nil {
					t.Fatalf("NewApp: %v", err)
				}
				if err := dispatch.run(app, helpFlag); err != nil {
					t.Fatalf("%s target-scoped CLI help: %v", dispatch.name, err)
				}
				assertNeutralCodexHelpInvocation(t, logPath, helpFlag, true)
				if _, err := os.Stat(multisubsHome); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("target-scoped CLI help created product state: %v", err)
				}
			})
		}
	}
}

func TestCmdCLIKeepsGoalStateProfileLocalAcrossConcurrentTerminals(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))
	t.Setenv("TEST_FAKE_CODEX_LOG_DIR", root)

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
: "${CODEX_HOME:?CODEX_HOME must be set}"
: "${MULTISUBS_ACTIVE_PROFILE:?MULTISUBS_ACTIVE_PROFILE must be set}"
: "${TEST_FAKE_CODEX_LOG_DIR:?TEST_FAKE_CODEX_LOG_DIR must be set}"
mkdir -p "$CODEX_HOME"
goal_enabled=false
if [[ -f "$CODEX_HOME/config.toml" ]] && grep -Eq '^[[:space:]]*goals[[:space:]]*=[[:space:]]*true[[:space:]]*$' "$CODEX_HOME/config.toml"; then
  goal_enabled=true
fi
printf 'goal-state-for=%s\n' "$MULTISUBS_ACTIVE_PROFILE" > "$CODEX_HOME/state_5.sqlite"
{
  printf 'profile=%s\n' "$MULTISUBS_ACTIVE_PROFILE"
  printf 'codex_home=%s\n' "$CODEX_HOME"
  printf 'goal_enabled=%s\n' "$goal_enabled"
  printf 'args=%s\n' "$*"
} > "$TEST_FAKE_CODEX_LOG_DIR/$MULTISUBS_ACTIVE_PROFILE.log"
sleep 0.1
`
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	writeDefaultConfig(t, app, "model = \"global\"\ncli_auth_credentials_store = \"file\"\n\n[features]\ngoals = true\n")
	createExecProfiles(t, app, "alpha", "beta")

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, profileName := range []string{"alpha", "beta"} {
		profileName := profileName
		wg.Add(1)
		go func() {
			defer wg.Done()
			runApp, runErr := NewApp()
			if runErr != nil {
				errs <- runErr
				return
			}
			errs <- runApp.Run([]string{"codex", "cli", profileName, "check goal state"})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("cli failed: %v", err)
		}
	}

	for _, profileName := range []string{"alpha", "beta"} {
		logPath := filepath.Join(root, profileName+".log")
		logData, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read %s log: %v", profileName, err)
		}
		log := string(logData)
		wantHome := filepath.Join(root, "multi", "profiles", profileName, "codex-home")
		if !strings.Contains(log, "profile="+profileName) {
			t.Fatalf("expected profile %s in log, got %q", profileName, log)
		}
		if !strings.Contains(log, "codex_home="+wantHome) {
			t.Fatalf("expected CODEX_HOME %s in log, got %q", wantHome, log)
		}
		if !strings.Contains(log, "goal_enabled=true") {
			t.Fatalf("expected goals enabled through profile config, got %q", log)
		}

		statePath := filepath.Join(wantHome, "state_5.sqlite")
		stateData, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s goal state: %v", profileName, err)
		}
		if got, want := string(stateData), "goal-state-for="+profileName+"\n"; got != want {
			t.Fatalf("unexpected %s goal state: got=%q want=%q", profileName, got, want)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "default-codex", "state_5.sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected default Codex state to stay untouched, stat err=%v", err)
	}
}

func installCodexHelpRecorder(t *testing.T, root string) string {
	t.Helper()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex-help.log")
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
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func assertNeutralCodexHelpInvocation(t *testing.T, logPath, helpFlag string, includeLegacy bool) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read Codex help log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "arg_count=1\narg_0="+helpFlag+"\n") {
		t.Fatalf("unexpected forwarded Codex help arguments: %q", log)
	}
	if strings.Contains(log, managedCodexAuthConfig) {
		t.Fatalf("neutral Codex help received managed auth override: %q", log)
	}
	if !strings.Contains(log, "\nWORKER_TOOL_SETTING=keep\n") {
		t.Fatalf("neutral Codex help dropped an unrelated environment value: %q", log)
	}
	forbidden := []string{
		"CODEX_HOME",
		"MULTISUBS_HOME",
		"MULTISUBS_DEFAULT_CODEX_HOME",
		"MULTISUBS_ACTIVE_PROFILE",
		"MULTISUBS_SELECTED_PROFILE_PATH",
		"MULTISUBS_HEARTBEAT_PROMPT",
		"OPENAI_API_KEY",
		"CODEX_AUTH_TOKEN",
	}
	if includeLegacy {
		forbidden = append(forbidden, "MULTICODEX_HOME", "MULTICODEX_ACTIVE_PROFILE", "MULTICODEX_UNKNOWN_CONTROL")
	}
	for _, key := range forbidden {
		if strings.Contains(log, "\n"+key+"=") {
			t.Fatalf("neutral Codex help retained %s: %q", key, log)
		}
	}
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "MULTISUBS_") {
			t.Fatalf("neutral Codex help retained product variable %s: %q", line, log)
		}
	}
}

func TestRejectedProfileNameCreatesNoProductState(t *testing.T) {
	dispatches := []struct {
		name string
		run  func(*App) error
	}{
		{name: "codex cli", run: func(app *App) error { return app.cmdCLI([]string{"typo"}) }},
		{name: "codex login", run: func(app *App) error { return app.cmdLogin([]string{"typo"}) }},
	}

	for _, dispatch := range dispatches {
		t.Run(dispatch.name, func(t *testing.T) {
			root := t.TempDir()
			multisubsHome := filepath.Join(root, "multisubs")
			t.Setenv("MULTISUBS_HOME", multisubsHome)
			t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))

			app, err := NewApp()
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}

			err = dispatch.run(app)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("expected exit code 2 for an unknown profile, got %T (%v)", err, err)
			}
			if exitErr.Message != "unknown profile: typo" {
				t.Fatalf("unexpected rejection message: %q", exitErr.Message)
			}
			if _, statErr := os.Stat(multisubsHome); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("rejected profile name created product state: %v", statErr)
			}
		})
	}
}

func TestCmdCLIAutomaticallySelectsAccountAndRunsInteractiveCodex(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary")

	originalSelector := defaultExecAccountSelector
	var selectedModel string
	defaultExecAccountSelector = func(_ context.Context, _ []usage.MonitorAccount, model string) (usage.SelectedAccount, error) {
		selectedModel = model
		return usage.SelectedAccount{Account: usage.MonitorAccount{Label: "primary"}, WeeklyUsedPercent: 12}, nil
	}
	defer func() { defaultExecAccountSelector = originalSelector }()

	if err := app.Run([]string{"codex", "cli", "-m=gpt-5-codex-spark", "check this repo"}); err != nil {
		t.Fatalf("cli failed: %v", err)
	}
	if selectedModel != "gpt-5-codex-spark" {
		t.Fatalf("expected explicit model to reach selector, got %q", selectedModel)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "profile=primary") {
		t.Fatalf("expected primary profile in log, got %q", log)
	}
	wantCodexHome := filepath.Join(app.store.paths.ProfilesDir, "primary", "codex-home")
	if !strings.Contains(log, "codex_home="+wantCodexHome) {
		t.Fatalf("expected primary CODEX_HOME in log, got %q", log)
	}
	wantArgs := "-m=gpt-5-codex-spark check this repo -c " + managedCodexAuthConfig
	if !strings.Contains(log, "args="+wantArgs) {
		t.Fatalf("expected cli args %q in log, got %q", wantArgs, log)
	}
}

func TestCmdCLIManualAccountBypassesRoutingAndStripsSeparator(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary")

	originalSelector := defaultExecAccountSelector
	selectorCalled := false
	defaultExecAccountSelector = func(context.Context, []usage.MonitorAccount, string) (usage.SelectedAccount, error) {
		selectorCalled = true
		return usage.SelectedAccount{}, errors.New("selector must not run")
	}
	defer func() { defaultExecAccountSelector = originalSelector }()

	if err := app.Run([]string{"codex", "cli", "--account", "primary", "--", "check this repo"}); err != nil {
		t.Fatalf("cli failed: %v", err)
	}
	if selectorCalled {
		t.Fatal("manual account selection unexpectedly ran usage routing")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "profile=primary") || !strings.Contains(log, "args=check this repo") {
		t.Fatalf("unexpected manual cli launch: %q", log)
	}
	if strings.Contains(log, "args=-- ") {
		t.Fatalf("manual separator reached Codex: %q", log)
	}
}

func TestCmdCLIAutomaticRoutingCanUseDefaultAccount(t *testing.T) {
	app, logPath := newExecTestApp(t)

	originalSelector := defaultExecAccountSelector
	defaultExecAccountSelector = selectDefaultExecAccountForTest(t)
	defer func() { defaultExecAccountSelector = originalSelector }()

	if err := app.Run([]string{"codex", "cli"}); err != nil {
		t.Fatalf("cli failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "profile=\n") {
		t.Fatalf("expected no active profile for default account, got %q", log)
	}
	if !strings.Contains(log, "codex_home="+normalizeExecCodexHome(app.store.paths.DefaultCodexHome)) {
		t.Fatalf("expected default account CODEX_HOME, got %q", log)
	}
	if strings.Contains(log, managedCodexAuthConfig) {
		t.Fatalf("default interactive launch received managed auth override: %q", log)
	}
}

func TestCmdCLIAutomaticRoutingFailsClosedForAnyInvalidProfile(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary", "broken")
	makeStoredProfilePathInvalid(t, app, "broken")

	originalSelector := defaultExecAccountSelector
	selectorCalled := false
	defaultExecAccountSelector = func(context.Context, []usage.MonitorAccount, string) (usage.SelectedAccount, error) {
		selectorCalled = true
		return usage.SelectedAccount{Account: usage.MonitorAccount{Label: "primary"}}, nil
	}
	defer func() { defaultExecAccountSelector = originalSelector }()

	err := app.Run([]string{"codex", "cli"})
	if err == nil || !strings.Contains(err.Error(), "profile-local path") {
		t.Fatalf("expected unsafe profile to stop automatic routing, got %v", err)
	}
	if selectorCalled {
		t.Fatal("usage selector ran before every profile passed validation")
	}
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected Codex not to launch, stat err=%v", statErr)
	}
}

func TestCmdCLIManualAccountIgnoresUnrelatedInvalidProfile(t *testing.T) {
	app, logPath := newExecTestApp(t)
	createExecProfiles(t, app, "primary", "broken")
	makeStoredProfilePathInvalid(t, app, "broken")

	if err := app.Run([]string{"codex", "cli", "primary"}); err != nil {
		t.Fatalf("manual cli failed because of unrelated profile: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "profile=primary") {
		t.Fatalf("expected primary manual profile, got %q", data)
	}
}

func TestCmdCLIAutomaticDefaultAccountMustBeLoggedIn(t *testing.T) {
	app, logPath := newExecTestApp(t)
	t.Setenv("FAKE_CODEX_LOGIN_STATE", "logged-out")
	skipDefaultExecLoginRetryDelay(t)

	originalSelector := defaultExecAccountSelector
	defaultExecAccountSelector = selectDefaultExecAccountForTest(t)
	defer func() { defaultExecAccountSelector = originalSelector }()

	err := app.Run([]string{"codex", "cli"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected logged-out default error, got %T (%v)", err, err)
	}
	if !strings.Contains(exitErr.Message, "could not confirm the default Codex account login") {
		t.Fatalf("unexpected default error: %s", exitErr.Message)
	}
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected Codex not to launch, stat err=%v", statErr)
	}
}

func TestCmdCLIManualAccountRequiresName(t *testing.T) {
	app := newTestAppForCLI(t)
	err := app.Run([]string{"codex", "cli", "--account"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected usage ExitError, got %T (%v)", err, err)
	}
}

func makeStoredProfilePathInvalid(t *testing.T, app *App, name string) {
	t.Helper()
	cfg, err := app.loadOrInitConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	profile := cfg.Profiles[name]
	profile.CodexHome = filepath.Join(t.TempDir(), "outside-profile-home")
	cfg.Profiles[name] = profile
	if err := app.store.Save(cfg); err != nil {
		t.Fatalf("save invalid profile fixture: %v", err)
	}
}
