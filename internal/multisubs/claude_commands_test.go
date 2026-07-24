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
			return fakeClaudeAuthJSONWithOrg(true, "default@example.com", "default-org"), nil, nil
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

func TestClaudeLoginRejectsDefaultOrganizationDuplicate(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "work")
	runner.run = func(context.Context, []string, []string) error { return nil }
	runner.capture = func(_ context.Context, _ []string, env []string) ([]byte, []byte, error) {
		email := "work@example.com"
		if claudeConfigDirFromEnv(env) == "" {
			email = "default@example.com"
		}
		return fakeClaudeAuthJSONWithOrg(true, email, "shared-org"), nil, nil
	}
	_, err := captureStdout(t, func() error { return app.cmdClaudeLogin([]string{"work"}) })
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(exitErr.Message, "same organization") {
		t.Fatalf("expected duplicate organization error, got %T %v", err, err)
	}
}

func TestClaudeLoginFailsClosedWhenDefaultIdentityIsUnknown(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profile := createClaudeProfiles(t, app, "work")["work"]
	runner.run = func(context.Context, []string, []string) error { return nil }
	runner.capture = func(_ context.Context, _ []string, env []string) ([]byte, []byte, error) {
		if claudeConfigDirFromEnv(env) == profile.ConfigDir {
			return fakeClaudeAuthJSONWithOrg(true, "work@example.com", "work-org"), nil, nil
		}
		return nil, nil, errors.New("default status transport failed")
	}
	_, err := captureStdout(t, func() error { return app.cmdClaudeLogin([]string{"work"}) })
	if err == nil || !strings.Contains(err.Error(), "cannot verify Claude organization for default account") {
		t.Fatalf("expected fail-closed default identity error, got %v", err)
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

func TestClaudeStatusCallsOfficialJSONStatusForDefaultAndEveryManagedProfile(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	wantEmail := map[string]string{
		"":                          "default@example.com",
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
	for _, want := range []string{"default", "alpha", "beta", "default@example.com", "alpha@example.com", "beta@example.com", "logged-in"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
	if calls := runner.Calls(); len(calls) != 3 {
		t.Fatalf("status call count: got %d want 3 (%+v)", len(calls), calls)
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
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("usage args: %#v", args)
		}
		if claudeConfigDirFromEnv(env) == "" {
			return fakeClaudeUsageEnvelope(10, 20, &fable), nil, nil
		}
		return fakeClaudeUsageEnvelope(40, 50, nil), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeUsage(nil) })
	if err != nil {
		t.Fatalf("Claude usage: %v", err)
	}
	for _, want := range []string{
		"default (default)",
		"work (managed)",
		"session: 10% used; Resets in 2 hours",
		"weekly all models: 20% used; Resets Monday at 09:00",
		"Fable: 30% used; Resets Tuesday at 10:00",
		"Fable: unavailable",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage output missing %q:\n%s", want, out)
		}
	}
}

func TestClaudeUsageHidesMalformedProviderResultText(t *testing.T) {
	const marker = "synthetic-provider-result-marker"
	app, runner, _ := newClaudeTestApp(t)
	runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("unexpected usage args: %#v", args)
		}
		return fakeMalformedClaudeUsageEnvelope(marker), nil, nil
	}

	out, err := captureStdout(t, func() error { return app.cmdClaudeUsage(nil) })
	if err != nil {
		t.Fatalf("Claude usage: %v", err)
	}
	if strings.Contains(out, marker) {
		t.Fatalf("Claude usage exposed provider result text: %s", out)
	}
	if !strings.Contains(out, "parse Claude usage result: multiple percentages in one line") {
		t.Fatalf("Claude usage omitted structural parse category: %s", out)
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
				return fakeClaudeAuthJSONWithOrg(true, "default@example.com", "default-org"), nil, nil
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
	for _, want := range []string{"[ok] sidecar: version 1", "[ok] Claude binary: claude 2.0.0", "[ok] target default", "[ok] target work", "doctor result: PASS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestClaudeDoctorFailsDuplicateOrganizations(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	createClaudeProfiles(t, app, "work")
	runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"--version"}) {
			return []byte("claude 2.0.0\n"), nil, nil
		}
		return fakeClaudeAuthJSONWithOrg(true, "person@example.com", "shared-org"), nil, nil
	}
	out, err := captureStdout(t, func() error { return app.cmdClaudeDoctor(nil) })
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(out, "duplicates Claude organization") {
		t.Fatalf("expected duplicate organization doctor failure, err=%v output=%s", err, out)
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
		{name: "status", wantCategory: "unknown failure", run: func(app *App) error { return app.cmdClaudeStatus(nil) }},
		{name: "usage", wantCategory: "unknown failure", run: func(app *App) error { return app.cmdClaudeUsage(nil) }},
		{name: "doctor", wantCategory: "unknown failure", run: func(app *App) error { return app.cmdClaudeDoctor(nil) }},
		{name: "exec", wantCategory: "no usable Claude account", run: func(app *App) error { return app.cmdClaudeExec([]string{"hello"}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, runner, _ := newClaudeTestApp(t)
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
		name string
		run  func(*App) error
	}{
		{"status", func(app *App) error { return app.cmdClaudeStatus(nil) }},
		{"usage", func(app *App) error { return app.cmdClaudeUsage(nil) }},
		{"doctor", func(app *App) error { return app.cmdClaudeDoctor(nil) }},
		{"help", func(app *App) error { return app.cmdClaude([]string{"help"}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, runner, _ := newClaudeTestApp(t)
			runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
				switch {
				case reflect.DeepEqual(args, []string{"auth", "status", "--json"}):
					return fakeClaudeAuthJSON(false, ""), nil, nil
				case reflect.DeepEqual(args, claudeUsageProbeArgs()):
					return fakeClaudeUsageEnvelope(1, 2, nil), nil, nil
				case reflect.DeepEqual(args, []string{"--version"}):
					return []byte("claude test\n"), nil, nil
				default:
					return nil, nil, errors.New("unexpected capture")
				}
			}
			if _, err := captureStdout(t, func() error { return test.run(app) }); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			if _, err := os.Stat(app.store.paths.MultisubsHome); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s created provider state: %v", test.name, err)
			}
		})
	}
}

func TestClaudeCLIHelpFastPathRunsOfficialHelpWithoutLoadingSidecar(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	runner.run = func(_ context.Context, args, env []string) error {
		if !reflect.DeepEqual(args, []string{"--help"}) {
			t.Fatalf("CLI help args: %#v", args)
		}
		if envContainsKey(env, "CLAUDE_CONFIG_DIR") {
			t.Fatalf("CLI help should use neutral Claude env: %q", env)
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
