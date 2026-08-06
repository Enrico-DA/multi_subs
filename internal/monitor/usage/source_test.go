package usage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageSourceForManagedAccountUsesManagedAppServerWithOAuthFallback(t *testing.T) {
	t.Parallel()

	source := NewUsageSourceForAccount(MonitorAccount{
		Label:      "managed",
		CodexHome:  "/managed",
		SourceMode: SourceModeManagedAppServer,
	})
	usageSource, ok := source.(*UsageSource)
	if !ok {
		t.Fatalf("managed account source type: got %T want *UsageSource", source)
	}
	appServer, ok := usageSource.primary.(*AppServerSource)
	if !ok {
		t.Fatalf("managed primary source type: got %T want *AppServerSource", usageSource.primary)
	}
	if !appServer.managedProfile {
		t.Fatal("managed account primary app-server is not managed")
	}
	if _, ok := usageSource.fallback.(*OAuthSource); !ok {
		t.Fatalf("managed fallback source type: got %T want *OAuthSource", usageSource.fallback)
	}
}

func TestUsageSourceWithoutManagedProofUsesOAuthOnly(t *testing.T) {
	unverified := NewUsageSourceForAccount(MonitorAccount{
		Label:     "unverified",
		CodexHome: "/unverified",
	})
	if _, ok := unverified.(*OAuthSource); !ok {
		t.Fatalf("unverified account source type: got %T want *OAuthSource", unverified)
	}

	if _, ok := NewReportUsageSourceForAccount(MonitorAccount{CodexHome: "/unverified"}).(*OAuthSource); !ok {
		t.Fatal("unverified report source did not stay OAuth-only")
	}
}

func TestDefaultAccountWithoutAuthUsesUnmanagedAppServerFallback(t *testing.T) {
	home := t.TempDir()
	source := NewUsageSourceForHome(home)
	appServer := &fakeSource{name: "app-server", out: &Summary{
		Source:       "app-server",
		WeeklyWindow: WindowSummary{UsedPercent: 37},
	}}
	source.appServer = appServer

	summary, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("default source fetch: %v", err)
	}
	if summary.Source != "app-server" || summary.WeeklyWindow.UsedPercent != 37 {
		t.Fatalf("default app-server summary: %+v", summary)
	}
	if appServer.fetches != 1 {
		t.Fatalf("default app-server fetches: got %d want 1", appServer.fetches)
	}
	if source.AllowsAuthFileIdentityFallback() {
		t.Fatal("unmanaged default source allowed a later credential-file identity read")
	}

	rawAppServer := NewUsageSourceForHome(home).appServer
	actualAppServer, ok := rawAppServer.(*AppServerSource)
	if !ok {
		t.Fatalf("default app-server type: got %T want *AppServerSource", rawAppServer)
	}
	if actualAppServer.managedProfile {
		t.Fatal("default app-server source was forced into managed mode")
	}
	if _, ok := NewUsageSourceForAccount(MonitorAccount{CodexHome: home, SourceMode: SourceModeDefaultAccount}).(*DefaultAccountSource); !ok {
		t.Fatal("routing default did not receive the typed default source")
	}
	if _, ok := NewReportUsageSourceForAccount(MonitorAccount{CodexHome: home, SourceMode: SourceModeDefaultAccount}).(*DefaultAccountSource); !ok {
		t.Fatal("report default did not receive the typed default source")
	}
}

func TestDefaultAccountWithUsableAuthUsesOAuthWithoutAppServer(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"synthetic-token"}}`), 0o600); err != nil {
		t.Fatalf("write auth fixture: %v", err)
	}
	source := NewUsageSourceForHome(home)
	oauth := source.oauth.(*OAuthSource)
	oauth.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer synthetic-token" {
			t.Fatalf("OAuth authorization header: %q", got)
		}
		body := `{"email":"user@example.test","plan_type":"pro","rate_limit":{"primary_window":{"used_percent":1,"limit_window_seconds":18000},"secondary_window":{"used_percent":23,"limit_window_seconds":604800}}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	appServer := &fakeSource{name: "app-server", out: &Summary{Source: "app-server"}}
	source.appServer = appServer

	summary, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("default OAuth fetch: %v", err)
	}
	if summary.Source != "oauth" || summary.WeeklyWindow.UsedPercent != 23 {
		t.Fatalf("default OAuth summary: %+v", summary)
	}
	if appServer.fetches != 0 {
		t.Fatalf("usable auth started app-server %d time(s)", appServer.fetches)
	}
	if appServer.closeCount != 0 {
		t.Fatalf("usable auth touched app-server cleanup %d time(s)", appServer.closeCount)
	}
	if !source.AllowsAuthFileIdentityFallback() {
		t.Fatal("OAuth-backed default unexpectedly blocked protected identity recovery")
	}
}

func TestDefaultAccountDoesNotUseAppServerAfterOAuthProviderFailure(t *testing.T) {
	oauth := &fakeSource{name: "oauth", err: context.DeadlineExceeded}
	source := &DefaultAccountSource{
		oauth:                         oauth,
		oauthCredential:               func() (string, error) { return "synthetic-token", nil },
		oauthFetchWithCredential:      func(ctx context.Context, _ string) (*Summary, error) { return oauth.Fetch(ctx) },
		appServer:                     &fakeSource{name: "app-server", out: &Summary{}},
		allowAuthFileIdentityFallback: true,
	}

	_, err := source.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected OAuth provider failure")
	}
	if source.appServer.(*fakeSource).fetches != 0 {
		t.Fatal("OAuth provider failure started the unmanaged app-server")
	}
}

func TestDefaultAccountClosesOldAppServerBeforeReturningToOAuth(t *testing.T) {
	appServer := &fakeSource{name: "app-server", out: &Summary{Source: "app-server"}}
	authChecks := 0
	source := &DefaultAccountSource{
		oauth: &fakeSource{name: "oauth"},
		oauthCredential: func() (string, error) {
			authChecks++
			if authChecks == 1 {
				return "", errOAuthAuthFileUnavailable
			}
			return "synthetic-token", nil
		},
		oauthFetchWithCredential: func(context.Context, string) (*Summary, error) {
			if appServer.closeCount != 1 {
				t.Fatalf("app-server closes before OAuth request: got %d want 1", appServer.closeCount)
			}
			return &Summary{Source: "oauth"}, nil
		},
		appServer:                     appServer,
		allowAuthFileIdentityFallback: true,
	}

	first, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unmanaged fallback fetch: %v", err)
	}
	if first.Source != "app-server" {
		t.Fatalf("unmanaged fallback source: got %q want app-server", first.Source)
	}
	second, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("OAuth fetch after auth file appeared: %v", err)
	}
	if second.Source != "oauth" {
		t.Fatalf("source after auth file appeared: got %q want oauth", second.Source)
	}
	if appServer.fetches != 1 || appServer.closeCount != 1 {
		t.Fatalf("app-server lifecycle: fetches=%d closes=%d want 1 each", appServer.fetches, appServer.closeCount)
	}
}

func TestDefaultAccountRetainsTransitionCloseFailure(t *testing.T) {
	closeFailure := errors.New("synthetic app-server close failure")
	appServer := &closeSequenceSource{
		fakeSource:  fakeSource{name: "app-server"},
		closeErrors: []error{closeFailure, nil},
	}
	source := &DefaultAccountSource{
		oauth:                         &fakeSource{name: "oauth"},
		oauthCredential:               func() (string, error) { return "synthetic-token", nil },
		oauthFetchWithCredential:      func(context.Context, string) (*Summary, error) { return &Summary{Source: "oauth"}, nil },
		appServer:                     appServer,
		appServerUsed:                 true,
		allowAuthFileIdentityFallback: false,
	}

	if _, err := source.Fetch(context.Background()); !errors.Is(err, closeFailure) {
		t.Fatalf("transition close failure: got %v want %v", err, closeFailure)
	}
	if err := source.Close(); !errors.Is(err, closeFailure) {
		t.Fatalf("retained close failure: got %v want %v", err, closeFailure)
	}
	if appServer.closeCount != 2 {
		t.Fatalf("app-server close attempts: got %d want 2", appServer.closeCount)
	}
}

func TestDefaultAccountUnusableAuthUsesUnmanagedAppServer(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "malformed",
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte("{"), 0o600); err != nil {
					t.Fatalf("write malformed auth: %v", err)
				}
			},
		},
		{
			name: "missing token",
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
					t.Fatalf("write tokenless auth: %v", err)
				}
			},
		},
		{
			name: "loose permissions",
			setup: func(t *testing.T, home string) {
				t.Helper()
				path := filepath.Join(home, "auth.json")
				if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"synthetic-token"}}`), 0o600); err != nil {
					t.Fatalf("write loose auth: %v", err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("loosen auth permissions: %v", err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, home string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "auth-target.json")
				if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"synthetic-token"}}`), 0o600); err != nil {
					t.Fatalf("write symlink target: %v", err)
				}
				if err := os.Symlink(target, filepath.Join(home, "auth.json")); err != nil {
					t.Fatalf("create auth symlink: %v", err)
				}
			},
		},
		{
			name: "hard link",
			setup: func(t *testing.T, home string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "auth-target.json")
				if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"synthetic-token"}}`), 0o600); err != nil {
					t.Fatalf("write hard-link target: %v", err)
				}
				if err := os.Link(target, filepath.Join(home, "auth.json")); err != nil {
					t.Fatalf("create auth hard link: %v", err)
				}
			},
		},
		{
			name: "not regular",
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(home, "auth.json"), 0o700); err != nil {
					t.Fatalf("create auth directory: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.setup(t, home)
			source := NewUsageSourceForHome(home)
			appServer := &fakeSource{name: "app-server", out: &Summary{Source: "app-server"}}
			source.appServer = appServer

			summary, err := source.Fetch(context.Background())
			if err != nil {
				t.Fatalf("unmanaged fallback fetch: %v", err)
			}
			if summary.Source != "app-server" || appServer.fetches != 1 {
				t.Fatalf("unmanaged fallback result: summary=%+v fetches=%d", summary, appServer.fetches)
			}
			if source.AllowsAuthFileIdentityFallback() {
				t.Fatal("unmanaged fallback allowed auth-file identity recovery")
			}
		})
	}
}

func TestReportUsageSourceForManagedAccountUsesReportFallbackMode(t *testing.T) {
	t.Parallel()

	source := NewReportUsageSourceForAccount(MonitorAccount{
		Label:      "managed",
		CodexHome:  "/managed",
		SourceMode: SourceModeManagedAppServer,
	})
	reportSource, ok := source.(*UsageSource)
	if !ok {
		t.Fatalf("managed report source type: got %T want *UsageSource", source)
	}
	if !reportSource.report {
		t.Fatal("managed report source did not enable report fallback semantics")
	}
}

func TestUsageSourceCloseIsIdempotent(t *testing.T) {
	primary := &fakeSource{name: "primary"}
	fallback := &fakeSource{name: "fallback"}
	source := &UsageSource{primary: primary, fallback: fallback}

	if err := source.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if primary.closeCount != 1 || fallback.closeCount != 1 {
		t.Fatalf("source close counts: primary=%d fallback=%d", primary.closeCount, fallback.closeCount)
	}
}

func TestDefaultAccountSourceCloseIsIdempotent(t *testing.T) {
	oauth := &fakeSource{name: "oauth"}
	appServer := &fakeSource{name: "app-server"}
	source := &DefaultAccountSource{oauth: oauth, appServer: appServer, appServerUsed: true}

	if err := source.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if oauth.closeCount != 1 || appServer.closeCount != 1 {
		t.Fatalf("default source close counts: oauth=%d app-server=%d", oauth.closeCount, appServer.closeCount)
	}
}
