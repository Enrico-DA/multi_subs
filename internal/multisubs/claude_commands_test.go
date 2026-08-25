package multisubs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeLoginUsesOfficialClaudeAIFlowForManagedProfile(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "work")
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			t.Fatalf("unexpected capture args: %#v", args)
		}
		if claudeConfigDirFromEnv(env) == "" {
			t.Fatal("managed login checked the shared default identity")
		}
		return fakeClaudeAuthJSONWithOrg(true, "work@example.com", "work-org"), nil, nil
	}
	runner.run = func(_ context.Context, args, env []string) error {
		if !reflect.DeepEqual(args, []string{"auth", "login", "--claudeai", "--email", "work@example.com"}) {
			t.Fatalf("login args: %#v", args)
		}
		if got := claudeConfigDirFromEnv(env); got != profiles["work"].ConfigDir {
			t.Fatalf("login config dir: got %q want %q", got, profiles["work"].ConfigDir)
		}
		return nil
	}
	if _, err := captureStdout(t, func() error {
		return app.cmdClaudeLogin([]string{"work", "--email", "work@example.com"})
	}); err != nil {
		t.Fatalf("Claude login: %v", err)
	}
}

func TestClaudeLoginIgnoresDefaultOrganizationDuplicate(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profile := createClaudeProfiles(t, app, "work")["work"]
	runner.run = func(context.Context, []string, []string) error { return nil }
	runner.capture = func(_ context.Context, args []string, env []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			t.Fatalf("unexpected login capture: %#v", args)
		}
		if got := claudeConfigDirFromEnv(env); got != profile.ConfigDir {
			t.Fatalf("managed login checked a non-target identity: %q", got)
		}
		return fakeClaudeAuthJSONWithOrg(true, "work@example.com", "shared-org"), nil, nil
	}
	if _, err := captureStdout(t, func() error { return app.cmdClaudeLogin([]string{"work"}) }); err != nil {
		t.Fatalf("default organization must not block managed login: %v", err)
	}
}

func TestClaudeLoginRejectsManagedOrganizationDuplicate(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "other", "work")
	runner.run = func(context.Context, []string, []string) error { return nil }
	runner.capture = func(_ context.Context, args []string, env []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			t.Fatalf("unexpected login capture: %#v", args)
		}
		switch claudeConfigDirFromEnv(env) {
		case profiles["work"].ConfigDir:
			return fakeClaudeAuthJSONWithOrg(true, "work@example.com", "shared-org"), nil, nil
		case profiles["other"].ConfigDir:
			return fakeClaudeAuthJSONWithOrg(true, "other@example.com", "shared-org"), nil, nil
		case "":
			t.Fatal("managed duplicate check probed the shared default identity")
		default:
			t.Fatalf("unexpected managed config dir: %q", claudeConfigDirFromEnv(env))
		}
		return nil, nil, nil
	}
	_, err := captureStdout(t, func() error { return app.cmdClaudeLogin([]string{"work"}) })
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(exitErr.Message, "same organization") {
		t.Fatalf("expected managed duplicate organization error, got %T %v", err, err)
	}
}

func TestClaudeCLIRunsDefaultWithoutConfigAndManagedWithDerivedConfig(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "work")
	var invocations int
	runner.runInteractive = func(args, env []string) error {
		invocations++
		switch invocations {
		case 1:
			if !reflect.DeepEqual(args, []string{"--model", "sonnet", "hello"}) {
				t.Fatalf("default CLI args: %#v", args)
			}
			if envContainsKey(env, "CLAUDE_CONFIG_DIR") {
				t.Fatalf("default CLI must not set CLAUDE_CONFIG_DIR: %q", env)
			}
		case 2:
			if !reflect.DeepEqual(args, []string{"--continue"}) {
				t.Fatalf("managed CLI args: %#v", args)
			}
			if got := claudeConfigDirFromEnv(env); got != profiles["work"].ConfigDir {
				t.Fatalf("managed CLI config: got %q want %q", got, profiles["work"].ConfigDir)
			}
		default:
			t.Fatalf("unexpected invocation %d", invocations)
		}
		return nil
	}
	if err := app.cmdClaudeCLI([]string{"default", "--model", "sonnet", "hello"}); err != nil {
		t.Fatalf("default CLI: %v", err)
	}
	if err := app.cmdClaudeCLI([]string{"work", "--continue"}); err != nil {
		t.Fatalf("managed CLI: %v", err)
	}
}

func TestClaudeStatusLeavesDefaultUnverifiedAndChecksEveryManagedProfile(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	wantEmail := map[string]string{
		"":                          "cached-default@example.com",
		profiles["alpha"].ConfigDir: "alpha@example.com",
		profiles["beta"].ConfigDir:  "beta@example.com",
	}
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			t.Fatalf("status args: %#v", args)
		}
		dir := claudeConfigDirFromEnv(env)
		email, ok := wantEmail[dir]
		if !ok {
			t.Fatalf("unexpected status target config: %q", dir)
		}
		return fakeClaudeAuthJSON(true, email), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeStatus(nil) })
	if err != nil {
		t.Fatalf("Claude status: %v", err)
	}
	for _, want := range []string{"default", "identity unavailable", claudeDefaultIdentityDetail, "alpha", "beta", "alpha@example.com", "beta@example.com", "logged-in"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nNext:\n") {
		t.Fatalf("logged-in unverified default added a recovery step:\n%s", out)
	}
	for _, forbidden := range []string{"cached-default@example.com", "org-cached-default@example.com"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("status exposed cached default identity %q:\n%s", forbidden, out)
		}
	}
	if calls := runner.Calls(); len(calls) != 3 {
		t.Fatalf("status call count: got %d want 3 target probes (%+v)", len(calls), calls)
	}
}

func TestClaudeAuthStatusAcceptsOfficialLoggedOutExitOne(t *testing.T) {
	runner := &fakeClaudeRunner{}
	exitOne := exec.Command("sh", "-c", "exit 1").Run()
	runner.capture = func(context.Context, []string, []string) ([]byte, []byte, error) {
		return fakeClaudeAuthJSON(false, ""), nil, exitOne
	}
	status, err := fetchClaudeAuthStatus(context.Background(), runner, "")
	if err != nil {
		t.Fatalf("logged-out status: %v", err)
	}
	if status.LoggedIn {
		t.Fatal("expected logged-out status")
	}
}

func TestClaudeAuthAndUsageProbeFailuresHideCapturedDiagnostics(t *testing.T) {
	const marker = "synthetic-secret-marker"
	runner := &fakeClaudeRunner{
		capture: func(context.Context, []string, []string) ([]byte, []byte, error) {
			return nil, []byte(marker), errors.New("transport failed: " + marker)
		},
	}
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "auth",
			run: func() error {
				_, err := fetchClaudeAuthStatus(context.Background(), runner, "")
				return err
			},
			want: "Claude auth status failed: unknown failure",
		},
		{
			name: "usage",
			run: func() error {
				_, err := fetchClaudeUsage(context.Background(), runner, "")
				return err
			},
			want: "Claude usage command failed: unknown failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected fixed probe failure %q, got %v", test.want, err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("probe failure exposed synthetic secret: %v", err)
			}
		})
	}
}

func TestClaudeUsageReportsAllWindowsAndMissingFable(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "work")
	fable := 30.0
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			if claudeConfigDirFromEnv(env) == "" {
				t.Fatal("usage queried cached default identity")
			}
			return fakeClaudeAuthJSONWithOrg(true, "work@example.com", "work-org"), nil, nil
		}
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("usage args: %#v", args)
		}
		if claudeConfigDirFromEnv(env) == "" {
			return fakeClaudeUsageEnvelope(10, 20, &fable), nil, nil
		}
		return fakeClaudeUsageEnvelope(40, 50, nil), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeUsage(nil) })
	requireExitCode(t, err, 1)
	for _, want := range []string{
		"work",
		"default · identity unavailable",
		"Session (~5h)",
		"10% used · Resets in 2 hours",
		"Weekly all models",
		"20% used · Resets Monday at 09:00",
		"Fable weekly",
		"30% used · Resets Tuesday at 10:00",
		"not reported",
		"partial · identity unavailable",
		"Result: partial · 1 of 2 accounts available",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "default@example.com") || strings.Contains(out, "default-org") {
		t.Fatalf("usage exposed cached default identity:\n%s", out)
	}
}

func TestClaudeUsageHidesMalformedProviderResultText(t *testing.T) {
	const marker = "synthetic-provider-result-marker"
	app, runner, _ := newClaudeTestApp(t)
	runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			t.Fatal("default usage queried cached identity")
		}
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("unexpected usage args: %#v", args)
		}
		return fakeMalformedClaudeUsageEnvelope(marker), nil, nil
	}

	out, err := captureStdout(t, func() error { return app.cmdClaudeUsage(nil) })
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Claude usage malformed-result exit: %T %v", err, err)
	}
	if strings.Contains(out, marker) {
		t.Fatalf("Claude usage exposed provider result text: %s", out)
	}
	if !strings.Contains(out, "unavailable · usage response malformed") {
		t.Fatalf("Claude usage omitted safe structural parse category: %s", out)
	}
}

func TestClaudeDoctorReportsBinarySidecarAndAuthBasics(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "work")
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		switch {
		case reflect.DeepEqual(args, []string{"--version"}):
			return []byte("claude 2.0.0\n"), nil, nil
		case reflect.DeepEqual(args, []string{"auth", "status", "--json"}):
			if claudeConfigDirFromEnv(env) == "" {
				return fakeClaudeAuthJSONWithOrg(true, "cached-default@example.com", "cached-default-org"), nil, nil
			}
			return fakeClaudeAuthJSONWithOrg(true, "person@example.com", "work-org"), nil, nil
		default:
			t.Fatalf("unexpected doctor args: %#v", args)
			return nil, nil, nil
		}
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeDoctor(nil) })
	if err != nil {
		t.Fatalf("Claude doctor: %v", err)
	}
	for _, want := range []string{"[ok] sidecar: version 1", "[ok] Claude binary: claude 2.0.0", "[warn] target default", claudeDefaultIdentityDetail, "[ok] target work", "doctor result: PASS (1 warn)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"cached-default@example.com", "cached-default-org"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("doctor exposed cached default identity %q:\n%s", forbidden, out)
		}
	}
}

func TestClaudeDoctorIgnoresDefaultForManagedDuplicateChecks(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "work")
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"--version"}) {
			return []byte("claude 2.0.0\n"), nil, nil
		}
		if claudeConfigDirFromEnv(env) == "" {
			return fakeClaudeAuthJSONWithOrg(true, "cached-default@example.com", "shared-org"), nil, nil
		}
		return fakeClaudeAuthJSONWithOrg(true, "person@example.com", "shared-org"), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeDoctor(nil) })
	if err != nil || strings.Contains(out, "duplicates Claude organization") {
		t.Fatalf("default identity must not participate in duplicate checks, err=%v output=%s", err, out)
	}
	if strings.Contains(out, "cached-default@example.com") {
		t.Fatalf("doctor exposed cached default identity: %s", out)
	}
}

func TestClaudeDoctorFailsManagedDuplicateOrganizations(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "alpha", "beta")
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"--version"}) {
			return []byte("claude 2.0.0\n"), nil, nil
		}
		if claudeConfigDirFromEnv(env) == "" {
			return fakeClaudeAuthJSONWithOrg(true, "cached-default@example.com", "shared-org"), nil, nil
		}
		return fakeClaudeAuthJSONWithOrg(true, "person@example.com", "shared-org"), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeDoctor(nil) })
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(out, "duplicates Claude organization") {
		t.Fatalf("expected managed duplicate organization doctor failure, err=%v output=%s", err, out)
	}
}

func TestClaudeCommandProbeFailuresDoNotExposeCapturedDiagnostics(t *testing.T) {
	const marker = "synthetic-secret-marker"
	const providerDiagnostic = "transport failed: " + marker
	tests := []struct {
		name         string
		wantCategory string
		run          func(*App) error
	}{
		{name: "status", wantCategory: "status check failed", run: func(app *App) error { return app.cmdClaudeStatus(nil) }},
		{name: "usage", wantCategory: "usage probe failed", run: func(app *App) error { return app.cmdClaudeUsage(nil) }},
		{name: "doctor", wantCategory: "unknown failure", run: func(app *App) error { return app.cmdClaudeDoctor(nil) }},
		{name: "exec", wantCategory: "no usable Claude account", run: func(app *App) error { return app.cmdClaudeExec([]string{"hello"}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, runner, _ := newClaudeTestApp(t)
			if test.name == "status" || test.name == "exec" {
				createClaudeProfiles(t, app, "managed")
			}
			runner.capture = func(context.Context, []string, []string) ([]byte, []byte, error) {
				return nil, []byte(marker), errors.New(providerDiagnostic)
			}
			output, err := captureStdout(t, func() error { return test.run(app) })
			diagnostic := output
			if err != nil {
				diagnostic += err.Error()
			}
			if test.name == "exec" && strings.Contains(diagnostic, providerDiagnostic) {
				t.Fatalf("%s exposed raw per-account provider diagnostic: %s", test.name, diagnostic)
			}
			if strings.Contains(diagnostic, marker) {
				t.Fatalf("%s exposed synthetic secret: %s", test.name, diagnostic)
			}
			if !strings.Contains(diagnostic, test.wantCategory) {
				t.Fatalf("%s missing deterministic failure category: %s", test.name, diagnostic)
			}
		})
	}
}

func TestClaudeReadOnlyCommandsAndNamespaceHelpDoNotCreateState(t *testing.T) {
	tests := []struct {
		name     string
		wantCode int
		run      func(*App) error
	}{
		{name: "status", run: func(app *App) error { return app.cmdClaudeStatus(nil) }},
		{name: "usage", wantCode: 1, run: func(app *App) error { return app.cmdClaudeUsage(nil) }},
		{name: "doctor", run: func(app *App) error { return app.cmdClaudeDoctor(nil) }},
		{name: "help", run: func(app *App) error { return app.cmdClaude([]string{"help"}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, runner, _ := newClaudeTestApp(t)
			runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
				switch {
				case reflect.DeepEqual(args, []string{"auth", "status", "--json"}):
					if test.name == "usage" {
						t.Fatal("default usage queried cached identity")
					}
					return fakeClaudeAuthJSONWithOrg(true, "cached-default@example.com", "cached-default-org"), nil, nil
				case reflect.DeepEqual(args, claudeUsageProbeArgs()):
					return fakeClaudeUsageEnvelope(1, 2, nil), nil, nil
				case reflect.DeepEqual(args, []string{"--version"}):
					return []byte("claude test\n"), nil, nil
				default:
					return nil, nil, errors.New("unexpected capture")
				}
			}
			if _, err := captureStdout(t, func() error { return test.run(app) }); test.wantCode == 0 && err != nil {
				t.Fatalf("%s: %v", test.name, err)
			} else if test.wantCode != 0 {
				requireExitCode(t, err, test.wantCode)
			}
			if _, err := os.Stat(app.store.paths.MultisubsHome); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s created provider state: %v", test.name, err)
			}
		})
	}
}

func TestClaudeCLIHelpFastPathRunsOfficialHelpWithoutLoadingSidecar(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	t.Setenv("MULTISUBS_FUTURE_CONTROL", "stale")
	runner.run = func(_ context.Context, args, env []string) error {
		if !reflect.DeepEqual(args, []string{"--help"}) {
			t.Fatalf("CLI help args: %#v", args)
		}
		if envContainsKey(env, "CLAUDE_CONFIG_DIR") {
			t.Fatalf("CLI help should use neutral Claude env: %q", env)
		}
		for _, entry := range env {
			if strings.HasPrefix(entry, "MULTISUBS_") {
				t.Fatalf("CLI help retained product variable %q: %q", entry, env)
			}
		}
		return nil
	}
	if err := app.cmdClaude([]string{"cli", "missing-profile", "--help"}); err != nil {
		t.Fatalf("CLI help fast path: %v", err)
	}
	if _, err := os.Stat(app.store.paths.MultisubsHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CLI help mutated state: %v", err)
	}
}
