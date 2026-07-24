package multisubs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
	if !strings.Contains(out, "multisubs codex cli <name>") {
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
	t.Setenv("MULTISUBS_FAKE_CODEX_LOG_DIR", root)

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
: "${CODEX_HOME:?CODEX_HOME must be set}"
: "${MULTISUBS_ACTIVE_PROFILE:?MULTISUBS_ACTIVE_PROFILE must be set}"
: "${MULTISUBS_FAKE_CODEX_LOG_DIR:?MULTISUBS_FAKE_CODEX_LOG_DIR must be set}"
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
} > "$MULTISUBS_FAKE_CODEX_LOG_DIR/$MULTISUBS_ACTIVE_PROFILE.log"
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
}
