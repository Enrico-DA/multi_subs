package multisubs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	var sourceModes []monitorusage.SourceMode
	for _, target := range codexTargets {
		codexNames = append(codexNames, target.Account.Label)
		sourceModes = append(sourceModes, target.Account.SourceMode)
	}
	if !reflect.DeepEqual(codexNames, []string{"alpha", "zeta", "default"}) {
		t.Fatalf("Codex usage order: got %q", codexNames)
	}
	if !reflect.DeepEqual(sourceModes, []monitorusage.SourceMode{
		monitorusage.SourceModeManagedAppServer,
		monitorusage.SourceModeManagedAppServer,
		monitorusage.SourceModeDefaultAccount,
	}) {
		t.Fatalf("Codex source modes: got %v", sourceModes)
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

func TestCmdCodexUsageMeasuresDefaultWithoutAuthThroughUnmanagedAppServer(t *testing.T) {
	app, _, root := newExecSelectionTestApp(t)
	writeExecSelectionDefaultUsage(t, app, 27, time.Hour)
	appServerLog := filepath.Join(root, "unmanaged-usage-app-server.log")
	t.Setenv("TEST_UNMANAGED_APP_SERVER_LOG", appServerLog)
	t.Setenv("OPENAI_API_KEY", "synthetic-denied-value")
	t.Setenv("CODEX_AUTH_TOKEN", "synthetic-denied-value")
	t.Setenv("MULTISUBS_ACTIVE_PROFILE", "synthetic-denied-value")

	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	if err != nil {
		t.Fatalf("Codex usage through unmanaged app-server: %v", err)
	}
	if !strings.Contains(output, "default · default@example.com") || !strings.Contains(output, "Weekly        27% used") {
		t.Fatalf("unmanaged default usage output:\n%s", output)
	}
	data, err := os.ReadFile(appServerLog)
	if err != nil {
		t.Fatalf("read unmanaged app-server log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "args=-s read-only -a untrusted app-server") {
		t.Fatalf("unmanaged app-server arguments: %q", log)
	}
	if strings.Contains(log, managedCodexAuthConfig) {
		t.Fatalf("unmanaged app-server received managed auth override: %q", log)
	}
}

func TestCmdCodexUsageMeasuresDefaultWithAuthThroughOAuthOnly(t *testing.T) {
	app, _, root := newExecSelectionTestApp(t)
	writeExecSelectionDefaultData(t, app, 0, 31, time.Hour)
	appServerLog := filepath.Join(root, "unexpected-usage-app-server.log")
	t.Setenv("TEST_UNMANAGED_APP_SERVER_LOG", appServerLog)

	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	if err != nil {
		t.Fatalf("Codex usage through OAuth: %v", err)
	}
	if !strings.Contains(output, "default · default@example.com") || !strings.Contains(output, "Weekly        31% used") {
		t.Fatalf("OAuth default usage output:\n%s", output)
	}
	if _, err := os.Lstat(appServerLog); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OAuth default usage started app-server: %v", err)
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

func TestCollapseCodexUsageCollectionsMergesDefaultWithManagedSubscription(t *testing.T) {
	alpha := codexUsageCollectionFixture(
		"alpha",
		codexRoutingTargetManaged,
		"alpha-account",
		"alpha@example.com",
		25,
	)
	personal := codexUsageCollectionFixture(
		"personal",
		codexRoutingTargetManaged,
		"shared-account",
		"owner@example.com",
		70,
	)
	defaultAccount := codexUsageCollectionFixture(
		defaultExecAccountLabel,
		codexRoutingTargetDefault,
		"",
		" OWNER@example.com ",
		10,
	)

	accounts := collapseCodexUsageCollections([]codexUsageCollection{
		alpha,
		personal,
		defaultAccount,
	})
	if len(accounts) != 2 {
		t.Fatalf("logical account count: got %d want 2", len(accounts))
	}
	if accounts[0].Name != "alpha" || accounts[0].Identity != "alpha@example.com" {
		t.Fatalf("independent account: %+v", accounts[0])
	}
	duplicate := accounts[1]
	if duplicate.Name != "personal (also default)" ||
		duplicate.Identity != "owner@example.com" ||
		duplicate.Failure != "" {
		t.Fatalf("default duplicate row: %+v", duplicate)
	}
	if duplicate.Windows[1].UsedPercent == nil || *duplicate.Windows[1].UsedPercent != 70 {
		t.Fatalf("quota snapshots were averaged or reordered: %+v", duplicate.Windows)
	}

	report := usageReport{Providers: []usageProviderReport{{Name: "Codex", Accounts: accounts}}}
	available, total := usageReportAvailability(report)
	if available != 2 || total != 2 {
		t.Fatalf("logical availability: got %d of %d want 2 of 2", available, total)
	}
}

func TestCollapseCodexUsageCollectionsUsesStableSortedAliases(t *testing.T) {
	collected := []codexUsageCollection{
		codexUsageCollectionFixture("zeta", codexRoutingTargetManaged, "shared", "person@example.com", 40),
		codexUsageCollectionFixture("alpha", codexRoutingTargetManaged, "shared", "person@example.com", 90),
		codexUsageCollectionFixture(defaultExecAccountLabel, codexRoutingTargetDefault, "shared", "person@example.com", 10),
	}
	accounts := collapseCodexUsageCollections(collected)
	if len(accounts) != 1 {
		t.Fatalf("logical account count: got %d want 1", len(accounts))
	}
	if got, want := accounts[0].Name, "alpha (also zeta) (also default)"; got != want {
		t.Fatalf("stable aliases: got %q want %q", got, want)
	}
	if accounts[0].Windows[1].UsedPercent == nil || *accounts[0].Windows[1].UsedPercent != 40 {
		t.Fatalf("representative changed with alias sorting: %+v", accounts[0].Windows)
	}
}

func TestCollapseCodexUsageCollectionsKeepsDuplicateFailurePartial(t *testing.T) {
	success := codexUsageCollectionFixture("alpha", codexRoutingTargetManaged, "shared", "person@example.com", 40)
	partial := codexUsageCollectionFixture("beta", codexRoutingTargetManaged, "shared", "person@example.com", 90)
	partial.Account.Failure = "weekly usage unavailable"

	accounts := collapseCodexUsageCollections([]codexUsageCollection{success, partial})
	if len(accounts) != 1 ||
		accounts[0].Windows[1].UsedPercent == nil ||
		*accounts[0].Windows[1].UsedPercent != 40 ||
		accounts[0].Failure != "weekly usage unavailable" {
		t.Fatalf("duplicate failure did not stay partial with successful snapshot: %+v", accounts)
	}
}

func TestCodexUsageSuccessfulQuotaWithoutIdentityIsPartial(t *testing.T) {
	collected := codexUsageCollectionFixture(
		"work",
		codexRoutingTargetManaged,
		"",
		"malformed identity",
		20,
	)
	accounts := collapseCodexUsageCollections([]codexUsageCollection{collected})
	if len(accounts) != 1 ||
		accounts[0].Identity != "" ||
		accounts[0].Failure != "identity unavailable" ||
		len(accounts[0].Windows) == 0 {
		t.Fatalf("missing identity did not retain partial quota: %+v", accounts)
	}
	report := usageReport{Providers: []usageProviderReport{{Name: "Codex", Accounts: accounts}}}
	if !usageReportHasFailures(report) {
		t.Fatal("missing identity must preserve strict partial failure")
	}
}

func TestCodexUsageWeeklyFailureRecoversIdentityAndAvoidsDuplicateCount(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(codexHome, "auth.json"),
		[]byte(`{"email":" Person@Example.com "}`),
		0o600,
	); err != nil {
		t.Fatalf("write synthetic auth identity: %v", err)
	}
	partialSummary := &monitorusage.Summary{
		SessionWindow: monitorusage.WindowSummary{UsedPercent: 10},
		WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: -1},
	}
	source := &fakeCodexUsageSource{
		summary: partialSummary,
		err:     monitorusage.ErrWeeklyUsageUnavailable,
	}
	app := &App{codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
		return source
	}}
	target := codexUsageTarget{
		codexRoutingTarget: codexRoutingTarget{
			Kind: codexRoutingTargetDefault,
			Account: monitorusage.MonitorAccount{
				Label:     defaultExecAccountLabel,
				CodexHome: codexHome,
			},
		},
		DisplayName: defaultExecAccountLabel,
	}

	partial := app.collectCodexUsageCollection(target)
	if partial.Summary == nil || partial.Summary.AccountEmail != "person@example.com" {
		t.Fatalf("weekly failure did not recover normalized identity: %+v", partial.Summary)
	}
	if partial.Account.Failure != "weekly usage unavailable" ||
		len(partial.Account.Windows) == 0 {
		t.Fatalf("weekly failure lost partial quota: %+v", partial.Account)
	}

	success := codexUsageCollectionFixture(
		"work",
		codexRoutingTargetManaged,
		"shared-account",
		"person@example.com",
		40,
	)
	accounts := collapseCodexUsageCollections([]codexUsageCollection{success, partial})
	if len(accounts) != 1 ||
		accounts[0].Name != "work (also default)" ||
		accounts[0].Failure != "weekly usage unavailable" {
		t.Fatalf("recovered failed duplicate inflated logical count: %+v", accounts)
	}
	available, total := usageReportAvailability(usageReport{
		Providers: []usageProviderReport{{Name: "Codex", Accounts: accounts}},
	})
	if available != 0 || total != 1 {
		t.Fatalf("recovered failed duplicate availability: got %d of %d want 0 of 1", available, total)
	}
}

func TestCodexUsageSessionOnlySuccessRecoversIdentity(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(codexHome, "auth.json"),
		[]byte(`{"email":" Person@Example.com "}`),
		0o600,
	); err != nil {
		t.Fatalf("write synthetic auth identity: %v", err)
	}
	app := &App{codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
		return &fakeCodexUsageSource{summary: &monitorusage.Summary{
			SessionWindow: monitorusage.WindowSummary{UsedPercent: 10},
			WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: -1},
		}}
	}}
	target := codexUsageTarget{
		codexRoutingTarget: codexRoutingTarget{
			Kind: codexRoutingTargetDefault,
			Account: monitorusage.MonitorAccount{
				Label:     defaultExecAccountLabel,
				CodexHome: codexHome,
			},
		},
		DisplayName: defaultExecAccountLabel,
	}

	collected := app.collectCodexUsageCollection(target)
	if collected.Summary == nil ||
		collected.Summary.AccountEmail != "person@example.com" {
		t.Fatalf("session-only success did not recover identity: %+v", collected.Summary)
	}
	if collected.Account.Failure != "weekly usage unavailable" {
		t.Fatalf("session-only failure category: %q", collected.Account.Failure)
	}
}

func TestRecoverCodexUsageIdentityFillsEmailBesideStrongAccountID(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(codexHome, "auth.json"),
		[]byte(`{"email":"person@example.com"}`),
		0o600,
	); err != nil {
		t.Fatalf("write synthetic auth identity: %v", err)
	}
	collected := codexUsageCollection{
		Target: codexUsageTarget{codexRoutingTarget: codexRoutingTarget{
			Account: monitorusage.MonitorAccount{CodexHome: codexHome},
		}},
		Summary: &monitorusage.Summary{AccountID: "strong-account-id"},
	}

	recoverCodexUsageCollectionIdentity(&collected, nil)
	if collected.Summary.AccountEmail != "person@example.com" {
		t.Fatalf("strong account ID blocked safe display-email recovery: %+v", collected.Summary)
	}
}

func TestCodexUsageAmbiguousEmailFallbackKeepsConservativePartialCount(t *testing.T) {
	first := codexUsageCollectionFixture(
		"alpha",
		codexRoutingTargetManaged,
		"account-one",
		"shared@example.com",
		20,
	)
	second := codexUsageCollectionFixture(
		"beta",
		codexRoutingTargetManaged,
		"account-two",
		"shared@example.com",
		30,
	)
	ambiguous := codexUsageCollectionFixture(
		"fallback",
		codexRoutingTargetManaged,
		"",
		"shared@example.com",
		40,
	)

	accounts := collapseCodexUsageCollections([]codexUsageCollection{first, second, ambiguous})
	if len(accounts) != 3 {
		t.Fatalf("ambiguous email fallback was unsafely collapsed: %+v", accounts)
	}
	if accounts[2].Name != "fallback" ||
		accounts[2].Identity != "" ||
		accounts[2].Failure != "identity unavailable" ||
		len(accounts[2].Windows) == 0 {
		t.Fatalf("ambiguous fallback did not retain a separate partial quota row: %+v", accounts[2])
	}
	available, total := usageReportAvailability(usageReport{
		Providers: []usageProviderReport{{Name: "Codex", Accounts: accounts}},
	})
	if available != 2 || total != 3 {
		t.Fatalf("ambiguous fallback availability: got %d of %d want conservative 2 of 3", available, total)
	}
}

func codexUsageCollectionFixture(name string, kind codexRoutingTargetKind, accountID, email string, weekly float64) codexUsageCollection {
	summary := &monitorusage.Summary{
		AccountID:     accountID,
		AccountEmail:  email,
		SessionWindow: monitorusage.WindowSummary{UsedPercent: 10},
		WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: int(weekly)},
	}
	target := codexUsageTarget{
		codexRoutingTarget: codexRoutingTarget{
			Kind: kind,
			Account: monitorusage.MonitorAccount{
				Label:     name,
				CodexHome: "/synthetic/" + name,
			},
		},
		DisplayName: name,
	}
	return codexUsageCollection{
		Target:  target,
		Account: adaptCodexUsageAccount(name, summary),
		Summary: summary,
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
			AccountEmail: "default@example.com",
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
	if !reflect.DeepEqual(labels, []string{"Session (5h)", "Weekly", "Spark weekly"}) {
		t.Fatalf("Codex report windows: got %q", labels)
	}
	if account.Windows[0].UsedPercent == nil || *account.Windows[0].UsedPercent != 24 {
		t.Fatalf("Codex session was not adapted: %+v", account.Windows[0])
	}
}

func TestAdaptCodexUsageDoesNotExposeIdentityLikeLimitNames(t *testing.T) {
	unknownNames := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"account-org-opaque-id",
		"sk-" + strings.Repeat("x", 32),
		"/synthetic/private/provider/path",
	}
	rateLimits := map[string]monitorusage.RateLimitWindow{
		"codex": {
			SessionWindow: monitorusage.WindowSummary{UsedPercent: -1},
			WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 20},
		},
	}
	for index, limitName := range unknownNames {
		rateLimits["unknown-"+strconv.Itoa(index)] = monitorusage.RateLimitWindow{
			LimitName:    limitName,
			WeeklyWindow: monitorusage.WindowSummary{UsedPercent: 45},
		}
	}
	summary := &monitorusage.Summary{
		SessionWindow:    monitorusage.WindowSummary{UsedPercent: -1},
		WeeklyWindow:     monitorusage.WindowSummary{UsedPercent: 20},
		RateLimitWindows: rateLimits,
	}
	account := adaptCodexUsageAccount("work", summary)
	rendered := ""
	for _, window := range account.Windows {
		rendered += window.Label
	}
	for _, forbidden := range unknownNames {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Codex adapter exposed provider limit name %q: %q", forbidden, rendered)
		}
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
					Name:     "personal",
					Identity: "personal@example.com",
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
					Name:     "work",
					Identity: "owner@example.com",
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
		"  personal · personal@example.com\n" +
		"    Session (5h)  24% used · resets in 2h 14m (Fri 24 Jul 00:29 CEST)\n" +
		"    Weekly        61% used · resets in 3d 10h (Mon 27 Jul 09:00 CEST)\n" +
		"    Spark weekly  not reported\n" +
		"\n" +
		"Claude\n" +
		"  work · owner@example.com\n" +
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
					Name:     "alpha",
					Identity: "alpha@example.com",
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
		"  alpha · alpha@example.com\n" +
		"    Session  10% used · reset unknown\n" +
		"    Weekly   20% used · reset due\n" +
		"\n" +
		"  default · identity unavailable\n" +
		"    unavailable · not logged in\n" +
		"\n" +
		"Result: partial · 1 of 2 accounts available\n" +
		"\n" +
		"Next:\n" +
		"  Codex default · not logged in\n" +
		"    Run: codex login\n"
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
		"Run: multisubs doctor",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("partial session output missing %q:\n%s", want, output.String())
		}
	}
}

func TestCollectConcurrentPreservesTargetOrder(t *testing.T) {
	release := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	started := make(chan int, 3)
	resultsDone := make(chan []usageAccountReport, 1)
	go func() {
		resultsDone <- collectConcurrent([]int{0, 1, 2}, func(target int) usageAccountReport {
			started <- target
			<-release[target]
			return usageAccountReport{Name: string(rune('a' + target))}
		})
	}()
	for range []int{0, 1, 2} {
		<-started
	}
	close(release[2])
	close(release[0])
	close(release[1])
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
	runner.capture = func(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
		configDir := claudeConfigDirFromEnv(env)
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			switch configDir {
			case profiles["alpha"].ConfigDir:
				return fakeClaudeAuthJSONWithOrg(true, "alpha@example.com", "alpha-org"), nil, nil
			case profiles["beta"].ConfigDir:
				return fakeClaudeAuthJSONWithOrg(true, "beta@example.com", "beta-org"), nil, nil
			default:
				return nil, nil, &exec.Error{Name: "claude", Err: exec.ErrNotFound}
			}
		}
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("unexpected Claude usage args: %#v", args)
		}
		switch configDir {
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

func TestClaudeUsageCollectorPrefersLoggedOutAuthOverMalformedUsage(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	runner.capture = func(_ context.Context, args []string, env []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			return fakeClaudeAuthJSONWithOrg(false, "", ""), nil, nil
		}
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("unexpected Claude usage args: %#v", args)
		}
		return fakeMalformedClaudeUsageEnvelope("synthetic-secret"), nil, nil
	}
	report := app.collectClaudeUsage()
	if len(report.Accounts) != 1 {
		t.Fatalf("Claude account count: got %d", len(report.Accounts))
	}
	if report.Accounts[0].Name != "default" || report.Accounts[0].Failure != "not logged in" {
		t.Fatalf("logged-out default category: %+v", report.Accounts[0])
	}
	if len(report.Accounts[0].Windows) != 0 {
		t.Fatalf("logged-out default kept usage windows: %+v", report.Accounts[0])
	}
	if strings.Contains(report.Accounts[0].Failure, "synthetic-secret") {
		t.Fatalf("logged-out classification exposed provider text: %+v", report.Accounts[0])
	}
}

func TestClaudeUsageCollectorCategorizesTimeoutAndLoggedOut(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profiles := createClaudeProfiles(t, app, "logged-out")
	oldTimeout := usageAccountTimeout
	usageAccountTimeout = time.Millisecond
	t.Cleanup(func() { usageAccountTimeout = oldTimeout })
	runner.capture = func(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			configDir := claudeConfigDirFromEnv(env)
			email := "default@example.com"
			organization := "default-org"
			if configDir == profiles["logged-out"].ConfigDir {
				email = "logged-out@example.com"
				organization = "logged-out-org"
			}
			return fakeClaudeAuthJSONWithOrg(true, email, organization), nil, nil
		}
		if !reflect.DeepEqual(args, claudeUsageProbeArgs()) {
			t.Fatalf("unexpected Claude usage args: %#v", args)
		}
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
	runner.capture = func(_ context.Context, args []string, _ []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			return fakeClaudeAuthJSONWithOrg(true, "default@example.com", "default-org"), nil, nil
		}
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

func TestClaudeUsageCollectsIdentityWithBoundedProbesAndCollapsesOrganization(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	profile := createClaudeProfiles(t, app, "personal")["personal"]
	var authCalls atomic.Int32
	var usageCalls atomic.Int32
	var deadlineLock sync.Mutex
	deadlines := make(map[string]time.Time)
	probeCounts := make(map[string]int)
	deadlineMismatch := false
	runner.capture = func(ctx context.Context, args, env []string) ([]byte, []byte, error) {
		deadline, bounded := ctx.Deadline()
		if !bounded {
			t.Fatalf("unbounded Claude probe: %#v", args)
		}
		configDir := claudeConfigDirFromEnv(env)
		deadlineLock.Lock()
		if first, exists := deadlines[configDir]; exists {
			if !first.Equal(deadline) {
				deadlineMismatch = true
			}
		} else {
			deadlines[configDir] = deadline
		}
		probeCounts[configDir]++
		deadlineLock.Unlock()
		switch {
		case reflect.DeepEqual(args, []string{"auth", "status", "--json"}):
			authCalls.Add(1)
			email := "owner@example.com"
			if configDir == profile.ConfigDir {
				email = "OWNER@EXAMPLE.COM"
			}
			return fakeClaudeAuthJSONWithOrg(true, email, "opaque-shared-org"), nil, nil
		case reflect.DeepEqual(args, claudeUsageProbeArgs()):
			usageCalls.Add(1)
			if configDir == profile.ConfigDir {
				return fakeClaudeUsageEnvelope(30, 70, nil), nil, nil
			}
			return fakeClaudeUsageEnvelope(5, 10, nil), nil, nil
		default:
			return nil, nil, errors.New("unexpected probe")
		}
	}

	report := app.collectClaudeUsage()
	if authCalls.Load() != 4 || usageCalls.Load() != 2 {
		t.Fatalf("probe counts: auth=%d usage=%d want auth=4 usage=2", authCalls.Load(), usageCalls.Load())
	}
	deadlineLock.Lock()
	if deadlineMismatch || len(probeCounts) != 2 {
		t.Fatalf("Claude auth/usage/auth did not share one target deadline: deadlines=%v counts=%v", deadlines, probeCounts)
	}
	for configDir, count := range probeCounts {
		if count != 3 {
			t.Fatalf("probe sequence for %q: got %d calls want 3", configDir, count)
		}
	}
	deadlineLock.Unlock()
	if len(report.Accounts) != 1 {
		t.Fatalf("organization account count: got %d want 1", len(report.Accounts))
	}
	account := report.Accounts[0]
	if account.Name != "personal (also default)" ||
		account.Identity != "owner@example.com" ||
		account.Failure != "" {
		t.Fatalf("organization row: %+v", account)
	}
	if account.Windows[1].UsedPercent == nil || *account.Windows[1].UsedPercent != 70 {
		t.Fatalf("expected deterministic managed quota snapshot, got %+v", account.Windows)
	}
}

func TestValidateClaudeUsageIdentityHasNoRoutingRestrictions(t *testing.T) {
	status := claudeAuthStatus{
		LoggedIn:     true,
		Identity:     " PERSON@Example.com ",
		AuthMethod:   "api-key",
		APIProvider:  "thirdParty",
		Subscription: "pro",
		OrgID:        " organization-one ",
	}
	identity, err := validateClaudeUsageIdentity(status)
	if err != nil {
		t.Fatalf("usage identity rejected valid official fields: %v", err)
	}
	if identity.Organization != "organization-one" || identity.AccountEmail != "person@example.com" {
		t.Fatalf("usage identity: %+v", identity)
	}
	if validateClaudeRoutingAuth(status) == nil {
		t.Fatal("routing validation changed to accept a non-Max third-party method")
	}
}

func TestValidateClaudeUsageIdentityRequiresOfficialStatusOrganizationAndEmail(t *testing.T) {
	valid := claudeAuthStatus{
		LoggedIn: true,
		Identity: "person@example.com",
		OrgID:    "organization-one",
	}
	for _, test := range []struct {
		name   string
		status claudeAuthStatus
	}{
		{name: "logged out", status: func() claudeAuthStatus {
			status := valid
			status.LoggedIn = false
			return status
		}()},
		{name: "missing organization", status: func() claudeAuthStatus {
			status := valid
			status.OrgID = ""
			return status
		}()},
		{name: "malformed email", status: func() claudeAuthStatus {
			status := valid
			status.Identity = "not-an-email"
			return status
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if identity, err := validateClaudeUsageIdentity(test.status); err == nil {
				t.Fatalf("invalid usage identity accepted: %+v", identity)
			}
		})
	}
}

func TestClaudeUsageKeepsDifferentOrganizationsWithSameEmailSeparate(t *testing.T) {
	collected := []claudeUsageCollection{
		{
			Target:       claudeTarget{Name: "alpha", DisplayName: "alpha", Kind: "managed"},
			Account:      claudeUsageAccountFixture("alpha", 20),
			Organization: "organization-one",
			AccountEmail: "person@example.com",
		},
		{
			Target:       claudeTarget{Name: "beta", DisplayName: "beta", Kind: "managed"},
			Account:      claudeUsageAccountFixture("beta", 30),
			Organization: "organization-two",
			AccountEmail: "person@example.com",
		},
	}
	accounts := collapseClaudeUsageCollections(collected)
	if len(accounts) != 2 ||
		accounts[0].Name != "alpha" ||
		accounts[1].Name != "beta" {
		t.Fatalf("different organizations merged by email: %+v", accounts)
	}
}

func TestClaudeUsageOrganizationCollapseKeepsProbeFailurePartial(t *testing.T) {
	success := claudeUsageCollection{
		Target:       claudeTarget{Name: "alpha", DisplayName: "alpha", Kind: "managed"},
		Account:      claudeUsageAccountFixture("alpha", 20),
		Organization: "shared-organization",
		AccountEmail: "person@example.com",
	}
	failed := claudeUsageCollection{
		Target:       claudeTarget{Name: "beta", DisplayName: "beta", Kind: "managed"},
		Account:      usageAccountReport{Name: "beta", Failure: "usage probe failed"},
		Organization: "shared-organization",
		AccountEmail: "person@example.com",
	}
	accounts := collapseClaudeUsageCollections([]claudeUsageCollection{success, failed})
	if len(accounts) != 1 ||
		len(accounts[0].Windows) != 3 ||
		accounts[0].Failure != "usage probe failed" {
		t.Fatalf("duplicate Claude failure did not stay partial: %+v", accounts)
	}
}

func TestClaudeUsageIdentityFailureRetainsQuotaAndStrictPartial(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
		if reflect.DeepEqual(args, []string{"auth", "status", "--json"}) {
			return nil, nil, errors.New("synthetic auth failure with opaque-org")
		}
		return fakeClaudeUsageEnvelope(10, 20, nil), nil, nil
	}

	report := app.collectClaudeUsage()
	if len(report.Accounts) != 1 ||
		report.Accounts[0].Failure != "identity unavailable" ||
		len(report.Accounts[0].Windows) != 3 {
		t.Fatalf("identity failure discarded valid Claude quota: %+v", report.Accounts)
	}
	if !usageReportHasFailures(usageReport{Providers: []usageProviderReport{report}}) {
		t.Fatal("Claude identity failure must produce strict exit 1")
	}
}

func TestClaudeUsageSuppressesMalformedEmail(t *testing.T) {
	collected := []claudeUsageCollection{{
		Target:       claudeTarget{Name: "work", DisplayName: "work", Kind: "managed"},
		Account:      claudeUsageAccountFixture("work", 20),
		Organization: "opaque-organization",
		AccountEmail: monitorusage.NormalizeAccountEmail("not-an-email"),
	}}
	accounts := collapseClaudeUsageCollections(collected)
	if len(accounts) != 1 ||
		accounts[0].Identity != "" ||
		accounts[0].Failure != "identity unavailable" {
		t.Fatalf("malformed Claude email was not suppressed: %+v", accounts)
	}
}

func claudeUsageAccountFixture(name string, weekly float64) usageAccountReport {
	return usageAccountReport{
		Name: name,
		Windows: []usageWindowReport{
			{Label: "Session (~5h)", UsedPercent: testFloat64Ptr(10)},
			{Label: "Weekly all models", UsedPercent: testFloat64Ptr(weekly)},
			{Label: "Fable weekly"},
		},
	}
}

func TestUsageCommandsRejectEveryFlagBeforeStateOrProbes(t *testing.T) {
	root := t.TempDir()
	multisubsHome := filepath.Join(root, "missing-multisubs")
	var codexProbes atomic.Int32
	var claudeProbes atomic.Int32
	runner := &fakeClaudeRunner{}
	runner.capture = func(context.Context, []string, []string) ([]byte, []byte, error) {
		claudeProbes.Add(1)
		return nil, nil, errors.New("unexpected Claude probe")
	}
	app := &App{
		store: NewStore(Paths{
			MultisubsHome:    multisubsHome,
			ConfigPath:       filepath.Join(multisubsHome, "config.json"),
			ProfilesDir:      filepath.Join(multisubsHome, "profiles"),
			DefaultCodexHome: filepath.Join(root, "missing-default"),
		}),
		claudeRunner: runner,
		codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
			codexProbes.Add(1)
			return nil
		},
	}
	for _, test := range []struct {
		name    string
		args    []string
		message string
	}{
		{name: "combined -h", args: []string{"usage", "-h"}, message: "usage: multisubs usage"},
		{name: "combined --help", args: []string{"usage", "--help"}, message: "usage: multisubs usage"},
		{name: "combined --json", args: []string{"usage", "--json"}, message: "usage: multisubs usage"},
		{name: "Codex -h", args: []string{"codex", "usage", "-h"}, message: "usage: multisubs codex usage"},
		{name: "Codex --help", args: []string{"codex", "usage", "--help"}, message: "usage: multisubs codex usage"},
		{name: "Codex --json", args: []string{"codex", "usage", "--json"}, message: "usage: multisubs codex usage"},
		{name: "Claude -h", args: []string{"claude", "usage", "-h"}, message: "usage: multisubs claude usage"},
		{name: "Claude --help", args: []string{"claude", "usage", "--help"}, message: "usage: multisubs claude usage"},
		{name: "Claude --json", args: []string{"claude", "usage", "--json"}, message: "usage: multisubs claude usage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := captureStdout(t, func() error { return app.Run(test.args) })
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 || exitErr.Message != test.message {
				t.Fatalf("Run(%q) = %T %+v, want exit 2 message %q", test.args, err, err, test.message)
			}
			if output != "" {
				t.Fatalf("rejected usage invocation printed stdout: %q", output)
			}
			if codexProbes.Load() != 0 || claudeProbes.Load() != 0 {
				t.Fatalf("rejected usage invocation probed providers: codex=%d claude=%d", codexProbes.Load(), claudeProbes.Load())
			}
			if _, statErr := os.Lstat(multisubsHome); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("rejected usage invocation accessed product state: %v", statErr)
			}
		})
	}
}

func TestCmdCodexUsageCollapsesManagedAndDefaultWithExactOutput(t *testing.T) {
	app := newCodexDuplicateUsageCommandApp(t, false)
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	if err != nil {
		t.Fatalf("Codex duplicate usage: %v", err)
	}
	want := "" +
		"multisubs codex usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Codex\n" +
		"  alpha (also default) · person@example.com\n" +
		"    Session       30% used · reset unknown\n" +
		"    Weekly        70% used · reset unknown\n" +
		"    Spark weekly  not reported\n" +
		"\n" +
		"Result: complete · 1 of 1 accounts available\n"
	if output != want {
		t.Fatalf("Codex duplicate output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func TestCmdCodexUsageCollapsesFailedDuplicateAsOnePartialRow(t *testing.T) {
	app := newCodexDuplicateUsageCommandApp(t, true)
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	requireExitCode(t, err, 1)
	want := "" +
		"multisubs codex usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Codex\n" +
		"  alpha (also default) · person@example.com\n" +
		"    Session       30% used · reset unknown\n" +
		"    Weekly        70% used · reset unknown\n" +
		"    Spark weekly  not reported\n" +
		"    partial · usage probe failed\n" +
		"\n" +
		"Result: partial · 0 of 1 accounts available\n" +
		"\n" +
		"Next:\n" +
		"  Codex alpha (also default) · usage probe failed\n" +
		"    Run: multisubs doctor\n"
	if output != want {
		t.Fatalf("Codex failed duplicate output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func TestCmdClaudeUsageCollapsesManagedAndDefaultWithExactOutput(t *testing.T) {
	app := newClaudeDuplicateUsageCommandApp(t, claudeUsageCommandScenario{})
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderClaude)
	})
	if err != nil {
		t.Fatalf("Claude duplicate usage: %v", err)
	}
	want := "" +
		"multisubs claude usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Claude\n" +
		"  personal (also default) · person@example.com\n" +
		"    Session (~5h)      30% used · Resets in 2 hours\n" +
		"    Weekly all models  70% used · Resets Monday at 09:00\n" +
		"    Fable weekly       not reported\n" +
		"\n" +
		"Result: complete · 1 of 1 accounts available\n"
	if output != want {
		t.Fatalf("Claude duplicate output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func TestCmdClaudeUsageCollapsesFailedDuplicateAsOnePartialRow(t *testing.T) {
	app := newClaudeDuplicateUsageCommandApp(t, claudeUsageCommandScenario{defaultUsageFails: true})
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderClaude)
	})
	requireExitCode(t, err, 1)
	want := "" +
		"multisubs claude usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Claude\n" +
		"  personal (also default) · person@example.com\n" +
		"    Session (~5h)      30% used · Resets in 2 hours\n" +
		"    Weekly all models  70% used · Resets Monday at 09:00\n" +
		"    Fable weekly       not reported\n" +
		"    partial · usage probe failed\n" +
		"\n" +
		"Result: partial · 0 of 1 accounts available\n" +
		"\n" +
		"Next:\n" +
		"  Claude personal (also default) · usage probe failed\n" +
		"    Run: multisubs doctor\n"
	if output != want {
		t.Fatalf("Claude failed duplicate output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func TestCmdClaudeUsageIdentityChangeRetainsQuotaWithoutGrouping(t *testing.T) {
	app := newClaudeDuplicateUsageCommandApp(t, claudeUsageCommandScenario{managedIdentityChanges: true})
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderClaude)
	})
	requireExitCode(t, err, 1)
	want := "" +
		"multisubs claude usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Claude\n" +
		"  personal · identity unavailable\n" +
		"    Session (~5h)      30% used · Resets in 2 hours\n" +
		"    Weekly all models  70% used · Resets Monday at 09:00\n" +
		"    Fable weekly       not reported\n" +
		"    partial · identity unavailable\n" +
		"\n" +
		"  default · person@example.com\n" +
		"    Session (~5h)      5% used · Resets in 2 hours\n" +
		"    Weekly all models  10% used · Resets Monday at 09:00\n" +
		"    Fable weekly       not reported\n" +
		"\n" +
		"Result: partial · 1 of 2 accounts available\n" +
		"\n" +
		"Next:\n" +
		"  Claude personal · identity unavailable\n" +
		"    Run: multisubs doctor\n"
	if output != want {
		t.Fatalf("Claude identity-change output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func newCodexDuplicateUsageCommandApp(t *testing.T, defaultUsageFails bool) *App {
	t.Helper()
	app := newTestAppForCLI(t)
	writeDefaultFileStoreConfig(t, app)
	createTestProfiles(t, app, "alpha")
	cfg, err := app.store.Load()
	if err != nil {
		t.Fatalf("load Codex usage fixture config: %v", err)
	}
	if _, err := app.store.EnsureProfileDir(cfg.Profiles["alpha"], nil); err != nil {
		t.Fatalf("prepare Codex usage fixture profile: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(app.store.paths.DefaultCodexHome, "auth.json"),
		[]byte(`{"email":"person@example.com"}`),
		0o600,
	); err != nil {
		t.Fatalf("write default Codex identity fixture: %v", err)
	}

	app.codexUsageSource = func(account monitorusage.MonitorAccount) monitorusage.Source {
		if account.Label == defaultExecAccountLabel && defaultUsageFails {
			return &fakeCodexUsageSource{err: errors.New("synthetic usage failure")}
		}
		if account.Label == "alpha" {
			return &fakeCodexUsageSource{summary: &monitorusage.Summary{
				AccountID:     "shared-account",
				AccountEmail:  "person@example.com",
				SessionWindow: monitorusage.WindowSummary{UsedPercent: 30},
				WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 70},
			}}
		}
		return &fakeCodexUsageSource{summary: &monitorusage.Summary{
			AccountEmail:  "PERSON@EXAMPLE.COM",
			SessionWindow: monitorusage.WindowSummary{UsedPercent: 5},
			WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 10},
		}}
	}
	app.usageClock = func() time.Time {
		return time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	}
	app.usageLocation = time.UTC
	return app
}

type claudeUsageCommandScenario struct {
	defaultUsageFails      bool
	managedIdentityChanges bool
}

func newClaudeDuplicateUsageCommandApp(t *testing.T, scenario claudeUsageCommandScenario) *App {
	t.Helper()
	app, runner, _ := newClaudeTestApp(t)
	profile := createClaudeProfiles(t, app, "personal")["personal"]
	var authLock sync.Mutex
	authCalls := make(map[string]int)
	runner.capture = func(_ context.Context, args, env []string) ([]byte, []byte, error) {
		configDir := claudeConfigDirFromEnv(env)
		switch {
		case reflect.DeepEqual(args, []string{"auth", "status", "--json"}):
			authLock.Lock()
			authCalls[configDir]++
			call := authCalls[configDir]
			authLock.Unlock()
			if scenario.managedIdentityChanges && configDir == profile.ConfigDir && call == 2 {
				return fakeClaudeAuthJSONWithOrg(true, "changed@example.com", "changed-organization"), nil, nil
			}
			return fakeClaudeAuthJSONWithOrg(true, "PERSON@EXAMPLE.COM", "shared-organization"), nil, nil
		case reflect.DeepEqual(args, claudeUsageProbeArgs()):
			if configDir == "" && scenario.defaultUsageFails {
				return nil, nil, errors.New("synthetic usage failure")
			}
			if configDir == profile.ConfigDir {
				return fakeClaudeUsageEnvelope(30, 70, nil), nil, nil
			}
			return fakeClaudeUsageEnvelope(5, 10, nil), nil, nil
		default:
			return nil, nil, errors.New("unexpected synthetic Claude probe")
		}
	}
	app.usageClock = func() time.Time {
		return time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	}
	app.usageLocation = time.UTC
	return app
}

func requireExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != want {
		t.Fatalf("exit error: got %T %v want code %d", err, err, want)
	}
}

func TestCodexUsageCommandDoesNotCreateProductStateAndReturnsPartialExit(t *testing.T) {
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

func TestCodexUsageMissingIdentityPrintsExactPartialAndExitsOne(t *testing.T) {
	root := t.TempDir()
	app := &App{
		store: NewStore(Paths{
			MultisubsHome:    filepath.Join(root, "multisubs"),
			ConfigPath:       filepath.Join(root, "multisubs", "config.json"),
			ProfilesDir:      filepath.Join(root, "multisubs", "profiles"),
			DefaultCodexHome: filepath.Join(root, "default-codex"),
		}),
		codexUsageSource: func(monitorusage.MonitorAccount) monitorusage.Source {
			return &fakeCodexUsageSource{summary: &monitorusage.Summary{
				SessionWindow: monitorusage.WindowSummary{UsedPercent: 10},
				WeeklyWindow:  monitorusage.WindowSummary{UsedPercent: 20},
			}}
		},
		usageClock:    func() time.Time { return time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC) },
		usageLocation: time.UTC,
	}
	output, err := captureStdout(t, func() error {
		return app.cmdUsage(nil, usageProviderCodex)
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("strict missing-identity exit: %T %v", err, err)
	}
	want := "" +
		"multisubs codex usage\n" +
		"Updated: Fri 24 Jul 2026 12:00 UTC\n" +
		"\n" +
		"Codex\n" +
		"  default · identity unavailable\n" +
		"    Session       10% used · reset unknown\n" +
		"    Weekly        20% used · reset unknown\n" +
		"    Spark weekly  not reported\n" +
		"    partial · identity unavailable\n" +
		"\n" +
		"Result: partial · 0 of 1 accounts available\n" +
		"\n" +
		"Next:\n" +
		"  Codex default · identity unavailable\n" +
		"    Run: multisubs doctor\n"
	if output != want {
		t.Fatalf("missing-identity output:\n--- got ---\n%s--- want ---\n%s", output, want)
	}
}

func TestUsageOutputNeverPrintsOpaqueIDsUserIDsOrPaths(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	account := codexUsageCollectionFixture(
		"work",
		codexRoutingTargetManaged,
		"opaque-account-id",
		"person@example.com",
		20,
	)
	account.Summary.UserID = "opaque-user-id"
	account.Target.Account.CodexHome = "/synthetic/private/codex-home"
	accounts := collapseCodexUsageCollections([]codexUsageCollection{account})
	claudeAccounts := collapseClaudeUsageCollections([]claudeUsageCollection{{
		Target:       claudeTarget{Name: "claude-work", DisplayName: "claude-work", Kind: "managed"},
		Account:      claudeUsageAccountFixture("claude-work", 30),
		Organization: "opaque-organization",
		AccountEmail: "claude@example.com",
	}})
	report := usageReport{
		Command:   "multisubs usage",
		UpdatedAt: now,
		Providers: []usageProviderReport{
			{Name: "Codex", Accounts: accounts},
			{Name: "Claude", Accounts: claudeAccounts},
		},
	}
	var output bytes.Buffer
	printUsageReport(&output, report, now, time.UTC)
	rendered := output.String()
	for _, forbidden := range []string{
		"opaque-account-id",
		"opaque-user-id",
		"/synthetic/private/codex-home",
		"opaque-organization",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("usage output exposed %q:\n%s", forbidden, rendered)
		}
	}
	if !strings.Contains(rendered, "work · person@example.com") {
		t.Fatalf("validated email missing from local usage output:\n%s", rendered)
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
				AccountEmail: "default@example.com",
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
