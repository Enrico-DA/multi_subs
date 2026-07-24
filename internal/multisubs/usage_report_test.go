package multisubs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	monitorusage "github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

type fakeCodexUsageSource struct {
	summary    *monitorusage.Summary
	err        error
	fetch      func(context.Context) (*monitorusage.Summary, error)
	closeErr   error
	closeCalls int
}

func (source *fakeCodexUsageSource) Name() string { return "fake" }

func (source *fakeCodexUsageSource) Fetch(ctx context.Context) (*monitorusage.Summary, error) {
	if source.fetch != nil {
		return source.fetch(ctx)
	}
	return source.summary, source.err
}

func (source *fakeCodexUsageSource) Close() error {
	source.closeCalls++
	return source.closeErr
}

func TestUsageAccountScopeAndOrderIsManagedThenDefault(t *testing.T) {
	codexConfig := DefaultConfig()
	codexConfig.Profiles["zeta"] = Profile{Name: "zeta", CodexHome: "/profiles/zeta/codex-home"}
	codexConfig.Profiles["alpha"] = Profile{Name: "alpha", CodexHome: "/profiles/alpha/codex-home"}
	codexTargets := codexUsageTargets(codexConfig, "/default-codex")
	var codexNames []string
	var managedModes []bool
	for _, target := range codexTargets {
		codexNames = append(codexNames, target.Account.Label)
		managedModes = append(managedModes, target.Account.UseAppServer)
	}
	if !reflect.DeepEqual(codexNames, []string{"alpha", "zeta", "default"}) {
		t.Fatalf("Codex usage order: got %q", codexNames)
	}
	if !reflect.DeepEqual(managedModes, []bool{true, true, false}) {
		t.Fatalf("Codex source modes: got %v", managedModes)
	}

	claudeConfig := defaultClaudeConfig()
	claudeConfig.Profiles["zeta"] = claudeProfile{Name: "zeta", ConfigDir: "/profiles/zeta/config"}
	claudeConfig.Profiles["alpha"] = claudeProfile{Name: "alpha", ConfigDir: "/profiles/alpha/config"}
	claudeTargets := claudeUsageTargets(claudeConfig)
	var claudeNames []string
	for _, target := range claudeTargets {
		claudeNames = append(claudeNames, target.Name)
	}
	if !reflect.DeepEqual(claudeNames, []string{"alpha", "zeta", "default"}) {
		t.Fatalf("Claude usage order: got %q", claudeNames)
	}
}

func TestUsageAccountDisplayNamesHideEmailShapedProfileNames(t *testing.T) {
	codexConfig := DefaultConfig()
	codexConfig.Profiles["person@example.com"] = Profile{
		Name:      "person@example.com",
		CodexHome: "/profiles/person@example.com/codex-home",
	}
	targets := codexUsageTargets(codexConfig, "/default-codex")
	if targets[0].DisplayName != "[managed-1]" {
		t.Fatalf("email-shaped Codex profile display name: %q", targets[0].DisplayName)
	}
	if ValidateProfileName(targets[0].DisplayName) == nil {
		t.Fatalf("Codex privacy alias is inside the valid profile-name alphabet: %q", targets[0].DisplayName)
	}

	claudeConfig := defaultClaudeConfig()
	claudeConfig.Profiles["person@example.com"] = claudeProfile{
		Name:      "person@example.com",
		ConfigDir: "/profiles/person@example.com/config",
	}
	claudeTargets := claudeUsageTargets(claudeConfig)
	if claudeTargets[0].DisplayName != "[managed-1]" {
		t.Fatalf("email-shaped Claude profile display name: %q", claudeTargets[0].DisplayName)
	}
}

func TestUsageDisplayNamesAreCollisionFreeAndDeterministic(t *testing.T) {
	targets := []struct {
		name      string
		isDefault bool
	}{
		{name: "first@example.com"},
		{name: "[managed-1]"},
		{name: "alpha"},
		{name: "alpha"},
		{name: "second@example.com"},
		{name: defaultExecAccountLabel, isDefault: true},
	}
	allocate := func() []string {
		return allocateUsageDisplayNames(len(targets), func(index int) (string, bool) {
			return targets[index].name, targets[index].isDefault
		})
	}
	first := allocate()
	second := allocate()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("usage aliases are not deterministic: first=%q second=%q", first, second)
	}
	if first[0] != "[managed-2]" || first[1] != "[managed-1]" ||
		first[2] != "alpha" || first[3] != "[managed-3]" ||
		first[4] != "[managed-4]" || first[5] != defaultExecAccountLabel {
		t.Fatalf("unexpected collision-free aliases: %q", first)
	}
	seen := make(map[string]struct{}, len(first))
	for _, name := range first {
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate usage presentation name %q in %q", name, first)
		}
		seen[name] = struct{}{}
	}
}

func TestTamperedManagedCodexHomeCannotSuppressDefaultUsageTarget(t *testing.T) {
	root := t.TempDir()
	app := &App{store: NewStore(Paths{
		MultisubsHome:    filepath.Join(root, "multisubs"),
		ConfigPath:       filepath.Join(root, "multisubs", "config.json"),
		ProfilesDir:      filepath.Join(root, "multisubs", "profiles"),
		DefaultCodexHome: filepath.Join(root, "default-codex"),
	})}
	cfg := DefaultConfig()
	cfg.Profiles["tampered"] = Profile{
		Name:      "tampered",
		CodexHome: app.store.paths.DefaultCodexHome,
	}
	if err := app.store.Save(cfg); err != nil {
		t.Fatalf("save tampered registry fixture: %v", err)
	}

	sourceCalls := 0
	app.codexUsageSource = func(account monitorusage.MonitorAccount) monitorusage.Source {
		sourceCalls++
		if account.Label != defaultExecAccountLabel {
			t.Fatalf("unsafe managed target reached usage source: %+v", account)
		}
		return &fakeCodexUsageSource{summary: &monitorusage.Summary{
			WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 20},
		}}
	}
	report := app.collectCodexUsage()
	if len(report.Accounts) != 2 {
		t.Fatalf("tampered registry usage account count: got %d want 2", len(report.Accounts))
	}
	if report.Accounts[0].Name != "tampered" ||
		report.Accounts[0].Failure != "profile state unavailable" {
		t.Fatalf("tampered managed account row: %+v", report.Accounts[0])
	}
	if report.Accounts[1].Name != defaultExecAccountLabel || report.Accounts[1].Failure != "" {
		t.Fatalf("default account was suppressed or failed: %+v", report.Accounts[1])
	}
	if sourceCalls != 1 {
		t.Fatalf("usage source calls: got %d want one default probe", sourceCalls)
	}
}

func TestAdaptCodexUsageShowsSessionWeeklyAndSortedModelLimits(t *testing.T) {
	sessionMinutes := 300
	summary := &monitorusage.Summary{
		SessionWindow: monitorusage.WindowSummary{
			UsedPercent:        24,
			WindowDurationMins: &sessionMinutes,
		},
		WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 61},
		RateLimitWindows: map[string]monitorusage.RateLimitWindow{
			"codex": {
				LimitID: "codex",
				SessionWindow: monitorusage.WindowSummary{
					UsedPercent:        24,
					WindowDurationMins: &sessionMinutes,
				},
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 61},
			},
			"zeta": {
				LimitID:      "zeta",
				LimitName:    "Zeta",
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 11},
			},
			"codex_bengalfox": {
				LimitID:      "codex_bengalfox",
				LimitName:    "Spark",
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 52},
			},
		},
	}
	account := adaptCodexUsageAccount("work", summary)
	var labels []string
	for _, window := range account.Windows {
		labels = append(labels, window.Label)
	}
	if !reflect.DeepEqual(labels, []string{"Session (5h)", "Weekly", "Spark weekly", "Zeta weekly"}) {
		t.Fatalf("Codex report windows: got %q", labels)
	}
	if account.Windows[0].UsedPercent == nil || *account.Windows[0].UsedPercent != 24 {
		t.Fatalf("Codex session was not adapted: %+v", account.Windows[0])
	}
}

func TestAdaptCodexUsageDoesNotExposeIdentityLikeLimitNames(t *testing.T) {
	summary := &monitorusage.Summary{
		SessionWindow: monitorusage.WindowSummary{UsedPercent: -1},
		WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 20},
		RateLimitWindows: map[string]monitorusage.RateLimitWindow{
			"codex": {
				SessionWindow: monitorusage.WindowSummary{UsedPercent: -1},
				WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 20},
			},
			"opaque-account-id": {
				LimitName:    "person@example.com",
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 45},
			},
		},
	}
	account := adaptCodexUsageAccount("work", summary)
	rendered := ""
	for _, window := range account.Windows {
		rendered += window.Label
	}
	if strings.Contains(rendered, "person@example.com") || strings.Contains(rendered, "opaque-account-id") {
		t.Fatalf("Codex adapter exposed an identity-like limit label: %q", rendered)
	}
}

func TestPrintUsageReportCombinedGolden(t *testing.T) {
	location := time.FixedZone("CEST", 2*60*60)
	now := time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC)
	sessionReset := time.Date(2026, time.July, 23, 22, 29, 0, 0, time.UTC)
	weeklyReset := time.Date(2026, time.July, 27, 7, 0, 0, 0, time.UTC)
	report := usageReport{
		Command:   "multisubs usage",
		UpdatedAt: now.In(location),
		Providers: []usageProviderReport{
			{
				Name: "Codex",
				Accounts: []usageAccountReport{{
					Name: "egcom",
					Windows: []usageWindowReport{
						{Label: "Session (5h)", UsedPercent: testFloat64Ptr(24), ResetAt: &sessionReset},
						{Label: "Weekly", UsedPercent: testFloat64Ptr(61), ResetAt: &weeklyReset},
						{Label: "Spark weekly"},
					},
				}},
			},
			{
				Name: "Claude",
				Accounts: []usageAccountReport{{
					Name: "gmail",
					Windows: []usageWindowReport{
						{Label: "Session (~5h)", UsedPercent: testFloat64Ptr(18), ResetText: "Resets in 1 hour"},
						{Label: "Weekly all models", UsedPercent: testFloat64Ptr(37), ResetText: "Resets Monday at 9:00 AM"},
						{Label: "Fable weekly", UsedPercent: testFloat64Ptr(52), ResetText: "Resets Tuesday at 10:00 AM"},
					},
				}},
			},
		},
	}

	var output bytes.Buffer
	printUsageReport(&output, report, now, location)
	want := "" +
		"multisubs usage\n" +
		"Updated: Thu 23 Jul 2026 22:15 CEST\n" +
		"\n" +
		"Codex\n" +
		"  egcom\n" +
		"    Session (5h)  24% used · resets in 2h 14m (Fri 24 Jul 00:29 CEST)\n" +
		"    Weekly        61% used · resets in 3d 10h (Mon 27 Jul 09:00 CEST)\n" +
		"    Spark weekly  not reported\n" +
		"\n" +
		"Claude\n" +
		"  gmail\n" +
		"    Session (~5h)      18% used · Resets in 1 hour\n" +
		"    Weekly all models  37% used · Resets Monday at 9:00 AM\n" +
		"    Fable weekly       52% used · Resets Tuesday at 10:00 AM\n" +
		"\n" +
		"Result: complete · 2 of 2 accounts available\n"
	if output.String() != want {
		t.Fatalf("combined usage output:\n--- got ---\n%s--- want ---\n%s", output.String(), want)
	}
}

func TestPrintUsageReportProviderOnlyAndResetStates(t *testing.T) {
	location := time.UTC
	now := time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	report := usageReport{
		Command:   "multisubs codex usage",
		UpdatedAt: now,
		Providers: []usageProviderReport{{
			Name: "Codex",
			Accounts: []usageAccountReport{
				{
					Name: "alpha",
					Windows: []usageWindowReport{
						{Label: "Session", UsedPercent: testFloat64Ptr(10)},
						{Label: "Weekly", UsedPercent: testFloat64Ptr(20), ResetAt: &expired},
					},
				},
				{Name: "default", Failure: "not logged in"},
			},
		}},
	}
	var output bytes.Buffer
	printUsageReport(&output, report, now, location)
	want := "" +
		"multisubs codex usage\n" +
		"Updated: Thu 23 Jul 2026 20:15 UTC\n" +
		"\n" +
		"Codex\n" +
		"  alpha\n" +
		"    Session  10% used · reset unknown\n" +
		"    Weekly   20% used · reset due\n" +
		"\n" +
		"  default\n" +
		"    unavailable · not logged in\n" +
		"\n" +
		"Result: partial · 1 of 2 accounts available\n"
	if output.String() != want {
		t.Fatalf("provider usage output:\n--- got ---\n%s--- want ---\n%s", output.String(), want)
	}
}

func TestPrintUsageReportAllAccountsFailed(t *testing.T) {
	now := time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC)
	report := usageReport{
		Command:   "multisubs usage",
		UpdatedAt: now,
		Providers: []usageProviderReport{
			{Name: "Codex", Accounts: []usageAccountReport{{Name: "default", Failure: "Codex unavailable"}}},
			{Name: "Claude", Accounts: []usageAccountReport{{Name: "default", Failure: "Claude unavailable"}}},
		},
	}
	var output bytes.Buffer
	printUsageReport(&output, report, now, time.UTC)
	if !strings.Contains(output.String(), "Result: partial · 0 of 2 accounts available") {
		t.Fatalf("all-failure result: %s", output.String())
	}
	if !usageReportHasFailures(report) {
		t.Fatal("all-failure report must cause exit 1")
	}
}

func TestPrintUsageReportShowsRetainedSessionAsPartialWhenWeeklyIsUnavailable(t *testing.T) {
	now := time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC)
	report := usageReport{
		Command:   "multisubs codex usage",
		UpdatedAt: now,
		Providers: []usageProviderReport{{
			Name: "Codex",
			Accounts: []usageAccountReport{{
				Name:    "work",
				Failure: "weekly usage unavailable",
				Windows: []usageWindowReport{
					{Label: "Session (5h)", UsedPercent: testFloat64Ptr(10)},
					{Label: "Weekly"},
				},
			}},
		}},
	}
	var output bytes.Buffer
	printUsageReport(&output, report, now, time.UTC)
	for _, want := range []string{
		"Session (5h)  10% used",
		"Weekly        not reported",
		"partial · weekly usage unavailable",
		"Result: partial · 0 of 1 accounts available",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("partial session output missing %q:\n%s", want, output.String())
		}
	}
}

func TestCollectConcurrentPreservesTargetOrder(t *testing.T) {
	release := make(chan struct{})
	started := make(chan int, 3)
	resultsDone := make(chan []usageAccountReport, 1)
	go func() {
		resultsDone <- collectConcurrent([]int{0, 1, 2}, func(target int) usageAccountReport {
			started <- target
			<-release
			return usageAccountReport{Name: string(rune('a' + target))}
		})
	}()
	for range []int{0, 1, 2} {
		<-started
	}
	close(release)
	results := <-resultsDone
	var names []string
	for _, result := range results {
		names = append(names, result.Name)
	}
	if !reflect.DeepEqual(names, []string{"a", "b", "c"}) {
		t.Fatalf("concurrent results were reordered: %q", names)
	}
}

func TestClaudeUsageCollectorHandlesOptionalFableAndSafeFailures(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "alpha", "beta")
	runner.capture = func(ctx context.Context, _ []string, env []string) ([]byte, []byte, error) {
		switch claudeConfigDirFromEnv(env) {
		case profiles["alpha"].ConfigDir:
			return fakeClaudeUsageEnvelope(10, 20, nil), nil, nil
		case profiles["beta"].ConfigDir:
			return fakeMalformedClaudeUsageEnvelope("synthetic-secret"), nil, nil
		default:
			return nil, nil, &exec.Error{Name: "claude", Err: exec.ErrNotFound}
		}
	}
	report := app.collectClaudeUsage()
	if len(report.Accounts) != 3 {
		t.Fatalf("Claude account count: got %d", len(report.Accounts))
	}
	if report.Accounts[0].Name != "alpha" ||
		report.Accounts[0].Windows[2].Label != "Fable weekly" ||
		report.Accounts[0].Windows[2].UsedPercent != nil {
		t.Fatalf("missing Fable was not optional: %+v", report.Accounts[0])
	}
	if report.Accounts[1].Failure != "usage response malformed" {
		t.Fatalf("malformed Claude response category: %q", report.Accounts[1].Failure)
	}
	if report.Accounts[2].Failure != "Claude unavailable" {
		t.Fatalf("missing Claude binary category: %q", report.Accounts[2].Failure)
	}
	for _, account := range report.Accounts {
		if strings.Contains(account.Failure, "synthetic-secret") ||
			strings.Contains(account.Failure, profiles["alpha"].ConfigDir) {
			t.Fatalf("Claude collector exposed sensitive text: %+v", account)
		}
	}
}

func TestClaudeUsageCollectorCategorizesTimeoutAndLoggedOut(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "logged-out")
	oldTimeout := usageAccountTimeout
	usageAccountTimeout = time.Millisecond
	t.Cleanup(func() { usageAccountTimeout = oldTimeout })
	runner.capture = func(ctx context.Context, _ []string, env []string) ([]byte, []byte, error) {
		if claudeConfigDirFromEnv(env) == profiles["logged-out"].ConfigDir {
			return []byte(`{"is_error":true,"result":"Please log in to Claude."}`), nil, nil
		}
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	report := app.collectClaudeUsage()
	if report.Accounts[0].Failure != "not logged in" {
		t.Fatalf("logged-out category: %q", report.Accounts[0].Failure)
	}
	if report.Accounts[1].Failure != "timed out" {
		t.Fatalf("timeout category: %q", report.Accounts[1].Failure)
	}
}

func TestClaudeUsageCollectorIsolatesManagedProfilePathFailure(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profile := createClaudeProfiles(t, app, "missing")["missing"]
	if err := os.Remove(profile.ConfigDir); err != nil {
		t.Fatalf("remove synthetic profile directory: %v", err)
	}
	runner.capture = func(context.Context, []string, []string) ([]byte, []byte, error) {
		return fakeClaudeUsageEnvelope(10, 20, nil), nil, nil
	}
	report := app.collectClaudeUsage()
	if report.Accounts[0].Failure != "profile state unavailable" {
		t.Fatalf("profile path failure category: %+v", report.Accounts[0])
	}
	if report.Accounts[1].Failure != "" {
		t.Fatalf("default Claude account should remain available: %+v", report.Accounts[1])
	}
}

func TestUsageCommandsRejectEveryArgumentWithExitTwo(t *testing.T) {
	app := &App{}
	for _, test := range []struct {
		provider string
		args     []string
	}{
		{provider: usageProviderAll, args: []string{"--json"}},
		{provider: usageProviderCodex, args: []string{"--json"}},
		{provider: usageProviderCodex, args: []string{"--help"}},
		{provider: usageProviderClaude, args: []string{"unexpected"}},
	} {
		err := app.cmdUsage(test.args, test.provider)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 2 {
			t.Fatalf("cmdUsage(%q, %q) = %T %v, want exit 2", test.args, test.provider, err, err)
		}
	}
}

func TestCodexUsageCommandIsReadOnlyAndReturnsPartialExit(t *testing.T) {
	root := t.TempDir()
	multisubsHome := filepath.Join(root, "missing-multisubs")
	app := &App{
		store: NewStore(Paths{
			MultisubsHome:    multisubsHome,
			ConfigPath:       filepath.Join(multisubsHome, "config.json"),
			ProfilesDir:      filepath.Join(multisubsHome, "profiles"),
			DefaultCodexHome: filepath.Join(root, "missing-default"),
		}),
		codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
			return &fakeCodexUsageSource{err: errors.New("auth.json not found in synthetic path")}
		},
		usageClock:    func() time.Time { return time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC) },
		usageLocation: time.UTC,
	}
	_, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Codex partial usage exit: %T %v", err, err)
	}
	if _, statErr := os.Lstat(multisubsHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("usage command created product state: %v", statErr)
	}
}

func TestCodexUsageCollectorClosesSourceOnceAcrossOutcomes(t *testing.T) {
	target := codexUsageTarget{
		codexRoutingTarget: codexRoutingTarget{
			Kind: codexRoutingTargetDefault,
			Account: monitorusage.MonitorAccount{
				Label:     defaultExecAccountLabel,
				CodexHome: "/synthetic/default",
			},
		},
		DisplayName: defaultExecAccountLabel,
	}
	tests := []struct {
		name        string
		source      *fakeCodexUsageSource
		wantFailure string
	}{
		{
			name: "success",
			source: &fakeCodexUsageSource{summary: &monitorusage.Summary{
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 20},
			}},
		},
		{
			name:        "fetch failure",
			source:      &fakeCodexUsageSource{err: errors.New("synthetic provider failure")},
			wantFailure: "usage probe failed",
		},
		{
			name: "timeout",
			source: &fakeCodexUsageSource{fetch: func(ctx context.Context) (*monitorusage.Summary, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}},
			wantFailure: "timed out",
		},
		{
			name: "close failure",
			source: &fakeCodexUsageSource{
				summary: &monitorusage.Summary{
					WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 20},
				},
				closeErr: errors.New("synthetic cleanup secret"),
			},
			wantFailure: "usage cleanup failed",
		},
	}
	oldTimeout := usageAccountTimeout
	usageAccountTimeout = time.Millisecond
	t.Cleanup(func() { usageAccountTimeout = oldTimeout })

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{
				codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
					return test.source
				},
			}
			account := app.collectCodexUsageTarget(target)
			if account.Failure != test.wantFailure {
				t.Fatalf("failure category: got %q want %q", account.Failure, test.wantFailure)
			}
			if test.source.closeCalls != 1 {
				t.Fatalf("source close calls: got %d want 1", test.source.closeCalls)
			}
			if strings.Contains(account.Failure, "synthetic") {
				t.Fatalf("account failure exposed source error: %q", account.Failure)
			}
		})
	}
}

func TestCodexUsageCollectorKeepsSessionPartialWhenWeeklyFallbackIsUnavailable(t *testing.T) {
	source := &fakeCodexUsageSource{
		summary: &monitorusage.Summary{
			SessionWindow: monitorusage.WindowSummary{UsedPercent: 10},
			WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: -1},
		},
		err: monitorusage.ErrWeeklyUsageUnavailable,
	}
	app := &App{
		codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
			return source
		},
	}
	target := codexUsageTarget{
		codexRoutingTarget: codexRoutingTarget{
			Kind: codexRoutingTargetDefault,
			Account: monitorusage.MonitorAccount{
				Label: defaultExecAccountLabel,
			},
		},
		DisplayName: defaultExecAccountLabel,
	}

	account := app.collectCodexUsageTarget(target)
	if account.Failure != "weekly usage unavailable" {
		t.Fatalf("missing-weekly failure category: %q", account.Failure)
	}
	if len(account.Windows) < 2 ||
		account.Windows[0].UsedPercent == nil ||
		*account.Windows[0].UsedPercent != 10 ||
		account.Windows[1].UsedPercent != nil {
		t.Fatalf("partial session/weekly windows: %+v", account.Windows)
	}
	if source.closeCalls != 1 {
		t.Fatalf("source close calls: got %d want 1", source.closeCalls)
	}
}

func TestCombinedUsagePrintsCodexWhenClaudeBinaryIsMissing(t *testing.T) {
	root := t.TempDir()
	multisubsHome := filepath.Join(root, "missing-multisubs")
	sessionMinutes := 300
	runner := &fakeClaudeRunner{
		capture: func(context.Context, []string, []string) ([]byte, []byte, error) {
			return nil, nil, &exec.Error{Name: "claude", Err: exec.ErrNotFound}
		},
	}
	app := &App{
		store: NewStore(Paths{
			MultisubsHome:    multisubsHome,
			ConfigPath:       filepath.Join(multisubsHome, "config.json"),
			ProfilesDir:      filepath.Join(multisubsHome, "profiles"),
			DefaultCodexHome: filepath.Join(root, "default-codex"),
		}),
		claudeRunner: runner,
		codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
			return &fakeCodexUsageSource{summary: &monitorusage.Summary{
				SessionWindow: monitorusage.WindowSummary{
					UsedPercent:        10,
					WindowDurationMins: &sessionMinutes,
				},
				WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 20},
			}}
		},
		usageClock:    func() time.Time { return time.Date(2026, time.July, 23, 20, 15, 0, 0, time.UTC) },
		usageLocation: time.UTC,
	}
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderAll)
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("combined partial usage exit: %T %v", err, err)
	}
	for _, want := range []string{
		"Codex",
		"10% used",
		"Claude",
		"unavailable · Claude unavailable",
		"Result: partial · 1 of 2 accounts available",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("combined partial output missing %q:\n%s", want, output)
		}
	}
}

func TestSanitizeClaudeResetTextRejectsIdentityAndSecretLikeText(t *testing.T) {
	for _, unsafe := range []string{
		"Resets for person@example.com tomorrow",
		"Resets with bearer synthetic-secret",
		"Resets using token synthetic-secret",
		"Resets from account/private-data",
		"Resets at sk-" + "ant-api03-synthetic",
		"Resets at org_synthetic",
		"\x00",
		"\x85",
		"Resets in 1 hour\x00",
		"\x1b[31mResets in 1 hour\x1b[0m",
		"Resets in 1\u0085hour",
		"Resets in 1 hour (Europe//Rome)",
		"Resets in 1 hour (Europe/../Rome)",
		"Resets in 1 hour (token/synthetic-secret)",
		"Resets Monday at 9:00 AM (token/synthetic-secret)",
		"Resets July 20 at 4:20pm (token/synthetic-secret)",
		"Resets at 4am (token/synthetic-secret)",
		"Resets in 1 hour (Not_A_Real_Region/Not_A_Real_City)",
		"Resets tomorrow after lunch",
		strings.Repeat("Resets in 1 hour ", 10),
	} {
		if got := sanitizeClaudeResetText(unsafe); got != "" {
			t.Fatalf("unsafe reset text was preserved: %q", got)
		}
	}
}

func TestSanitizeClaudeResetTextAcceptsOnlySupportedGrammar(t *testing.T) {
	for _, reset := range []string{
		"Resets in 1 minute",
		"resets in 2 hours",
		"Resets in 3 days (Europe/Rome)",
		"Resets Monday at 9:00 AM",
		"Resets Mon at 09:00",
		"Resets Monday at 9:00 AM (UTC)",
		"Resets July 20 at 4:20pm",
		"Resets Jul 20 at 4am",
		"Resets at 23:15",
		"Resets at 4am (Europe/Rome)",
		"Resets at 4:20 PM (America/Argentina/Buenos_Aires)",
	} {
		if got := sanitizeClaudeResetText(reset); got != reset {
			t.Fatalf("supported reset text changed or rejected: input=%q output=%q", reset, got)
		}
	}
}

func TestClaudeUsageWindowRendersUnsupportedResetAsUnknown(t *testing.T) {
	window := adaptClaudeUsageWindow("Session (~5h)", claudeUsageWindow{
		UsedPercent: 20,
		ResetText:   "Resets with bearer synthetic-provider-secret",
	})
	rendered := formatUsageWindow(window, time.Now(), time.UTC)
	if !strings.Contains(rendered, "reset unknown") {
		t.Fatalf("unsupported Claude reset did not become unknown: %q", rendered)
	}
	if strings.Contains(rendered, "bearer") || strings.Contains(rendered, "synthetic-provider-secret") {
		t.Fatalf("unsupported Claude reset text reached output: %q", rendered)
	}
}

func testFloat64Ptr(value float64) *float64 {
	return &value
}
