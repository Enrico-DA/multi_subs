package usage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Enrico-DA/multi_subs/internal/buildinfo"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOAuthSourceFetchLeavesNonWeeklyPrimaryOnlyWindowUnknown(t *testing.T) {
	codexHome := t.TempDir()
	authJSON := `{"tokens":{"access_token":"test-token"}}`
	if err := os.WriteFile(codexHome+"/auth.json", []byte(authJSON), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	source := NewOAuthSourceForHome(codexHome)
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("expected bearer token header, got %q", got)
		}
		if got := req.Header.Get("User-Agent"); got != clientName+"/"+buildinfo.Version {
			t.Fatalf("expected versioned user agent, got %q", got)
		}
		body := `{
			"email": "user@example.com",
			"plan_type": "pro",
			"rate_limit": {
				"primary_window": {
					"used_percent": 12,
					"limit_window_seconds": 18000,
					"reset_at": 1893456000
				}
			}
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	summary, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if summary.WeeklyWindow.UsedPercent != unavailableUsedPercent {
		t.Fatalf("expected missing weekly window to be unavailable, got %d", summary.WeeklyWindow.UsedPercent)
	}
	if summary.SessionWindow.UsedPercent != 12 ||
		summary.SessionWindow.WindowDurationMins == nil ||
		*summary.SessionWindow.WindowDurationMins != 300 {
		t.Fatalf("expected OAuth five-hour session window, got %+v", summary.SessionWindow)
	}
	codexWindow, ok := summary.RateLimitWindows["codex"]
	if !ok {
		t.Fatalf("expected codex rate limit window")
	}
	if codexWindow.WeeklyWindow.UsedPercent != unavailableUsedPercent {
		t.Fatalf("expected codex weekly window to be unavailable, got %d", codexWindow.WeeklyWindow.UsedPercent)
	}
	if codexWindow.SessionWindow.UsedPercent != 12 {
		t.Fatalf("expected per-limit OAuth session window, got %+v", codexWindow.SessionWindow)
	}
}

func TestOAuthSourceMissingAuthKeepsExistingSafeErrorText(t *testing.T) {
	codexHome := t.TempDir()
	source := NewOAuthSourceForHome(codexHome)

	_, err := source.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected missing auth error")
	}
	want := "auth.json not found in " + codexHome + "/auth.json"
	if err.Error() != want {
		t.Fatalf("missing auth error: got %q want %q", err, want)
	}
	if !errors.Is(err, errOAuthAuthFileUnavailable) {
		t.Fatalf("missing auth error was not classified for default fallback: %v", err)
	}
}

func TestUsableAuthFileRejectsLoosePermissions(t *testing.T) {
	codexHome := t.TempDir()
	authPath := codexHome + "/auth.json"
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"test-token"}}`), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	ok, err := usableAuthFile(authPath)
	if err == nil {
		t.Fatal("expected loose auth permissions to fail")
	}
	if ok {
		t.Fatal("expected loose auth file not to be usable")
	}
	if !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("expected permissions error, got %v", err)
	}
}

func TestBuildRateLimitWindowsFromOAuthAdditionalLimitsKeepsPrimaryOnlyLimit(t *testing.T) {
	windows := buildRateLimitWindowsFromOAuthAdditionalLimits([]oauthAdditionalRateLimit{
		{
			LimitName: "Spark",
			RateLimit: &oauthRateLimitDetails{
				PrimaryWindow: &oauthWindowSnapshot{
					UsedPercent:        42,
					LimitWindowSeconds: 5 * 60 * 60,
				},
			},
		},
	})

	window, ok := windows["Spark"]
	if !ok {
		t.Fatalf("expected primary-only additional limit to be preserved")
	}
	if window.Primary == nil || window.Primary.UsedPercent != 42 {
		t.Fatalf("expected primary usage 42, got %#v", window.Primary)
	}
	if window.Secondary != nil {
		t.Fatalf("expected missing secondary window to stay nil for normalizer fallback, got %#v", window.Secondary)
	}
}

func TestOAuthExplicitMainQuotaSignalsAreNotRoutable(t *testing.T) {
	tests := []struct {
		name        string
		eligibility string
	}{
		{name: "not allowed", eligibility: `"allowed": false`},
		{name: "limit reached", eligibility: `"limit_reached": true`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := fetchOAuthSummary(t, `{
				"plan_type": "pro",
				"rate_limit": {
					`+test.eligibility+`,
					"primary_window": {
						"used_percent": 1,
						"limit_window_seconds": 604800
					}
				}
			}`)

			provider := accountFetchResult{
				codexHome: "/provider",
				account: AccountSummary{
					Label:            "provider",
					WeeklyWindow:     summary.WeeklyWindow,
					RateLimitWindows: summary.RateLimitWindows,
				},
				snapshot: summary,
			}
			selected, err := selectBestAccountFromResultsForModel([]accountFetchResult{
				provider,
				selectionResult("measured", 0, 90, -1),
			}, "")
			if err != nil {
				t.Fatal(err)
			}
			if selected.Account.Label != "measured" {
				t.Fatalf("explicit OAuth quota signal was scored: %+v", selected)
			}
		})
	}
}

func TestOAuthExplicitAdditionalQuotaUnavailableIsNotRoutable(t *testing.T) {
	summary := fetchOAuthSummary(t, `{
		"plan_type": "pro",
		"rate_limit": {
			"allowed": true,
			"primary_window": {
				"used_percent": 10,
				"limit_window_seconds": 604800
			}
		},
		"additional_rate_limits": [{
			"limit_name": "codex_bengalfox",
			"rate_limit": {
				"allowed": false,
				"primary_window": {
					"used_percent": 1,
					"limit_window_seconds": 604800
				}
			}
		}]
	}`)
	provider := accountFetchResult{
		codexHome: "/provider",
		account: AccountSummary{
			Label:            "provider",
			WeeklyWindow:     summary.WeeklyWindow,
			RateLimitWindows: summary.RateLimitWindows,
		},
		snapshot: summary,
	}
	measured := selectionResult("measured", 0, 90, -1)
	measured.account.RateLimitWindows = map[string]RateLimitWindow{
		"codex_bengalfox": {
			LimitID:      "codex_bengalfox",
			LimitName:    "Spark",
			WeeklyWindow: weeklyWindow(90, -1),
		},
	}

	selected, err := selectBestAccountFromResultsForModel(
		[]accountFetchResult{provider, measured},
		"gpt-5.3-codex-spark",
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Account.Label != "measured" {
		t.Fatalf("explicit additional OAuth quota unavailability was scored: %+v", selected)
	}
}

func TestOAuthOmittedEligibilityFieldsKeepOlderResponseCompatibility(t *testing.T) {
	summary := fetchOAuthSummary(t, `{
		"plan_type": "pro",
		"rate_limit": {
			"primary_window": {
				"used_percent": 23,
				"limit_window_seconds": 604800
			}
		}
	}`)
	result := accountFetchResult{
		codexHome: "/provider",
		account: AccountSummary{
			Label:            "provider",
			WeeklyWindow:     summary.WeeklyWindow,
			RateLimitWindows: summary.RateLimitWindows,
		},
		snapshot: summary,
	}

	selected, err := selectBestAccountFromResultsForModel([]accountFetchResult{result}, "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Account.Label != "provider" || selected.WeeklyUsedPercent != 23 {
		t.Fatalf("older OAuth response lost weekly compatibility: %+v", selected)
	}
}

func fetchOAuthSummary(t *testing.T, body string) *Summary {
	t.Helper()
	codexHome := t.TempDir()
	if err := os.WriteFile(codexHome+"/auth.json", []byte(`{"tokens":{"access_token":"test-token"}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	source := NewOAuthSourceForHome(codexHome)
	source.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	summary, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	return summary
}
