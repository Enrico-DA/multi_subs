package usage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourceFetchReportsOnlyAvailableUsageWindows(t *testing.T) {
	tests := []struct {
		name              string
		primary           WindowSummary
		secondary         WindowSummary
		wantDetails       string
		wantFiveHour      *int
		wantWeekly        *int
		wantAbsentJSONKey string
	}{
		{
			name:         "both windows",
			primary:      WindowSummary{UsedPercent: 12},
			secondary:    WindowSummary{UsedPercent: 34},
			wantDetails:  "plan=pro 5h=12% weekly=34% source=app-server",
			wantFiveHour: intPtr(12),
			wantWeekly:   intPtr(34),
		},
		{
			name:              "weekly only",
			primary:           unavailableWindowSummary(),
			secondary:         WindowSummary{UsedPercent: 24},
			wantDetails:       "plan=pro weekly=24% source=app-server",
			wantWeekly:        intPtr(24),
			wantAbsentJSONKey: "five_hour_used_percent",
		},
		{
			name:              "five hour only",
			primary:           WindowSummary{UsedPercent: 7},
			secondary:         unavailableWindowSummary(),
			wantDetails:       "plan=pro 5h=7% source=app-server",
			wantFiveHour:      intPtr(7),
			wantAbsentJSONKey: "weekly_used_percent",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := checkSourceFetch(context.Background(), MonitorAccount{Label: "personal"}, &fakeSource{
				name: "app-server",
				out: &Summary{
					PlanType:        "pro",
					Source:          "app-server",
					PrimaryWindow:   test.primary,
					SecondaryWindow: test.secondary,
				},
			})

			if !check.OK {
				t.Fatalf("expected successful check, got %s", check.Details)
			}
			if check.Details != test.wantDetails {
				t.Fatalf("details = %q, want %q", check.Details, test.wantDetails)
			}
			if check.PlanType != "pro" || check.Source != "app-server" {
				t.Fatalf("unexpected structured metadata: plan=%q source=%q", check.PlanType, check.Source)
			}
			assertOptionalIntEqual(t, "five-hour", check.FiveHourUsedPercent, test.wantFiveHour)
			assertOptionalIntEqual(t, "weekly", check.WeeklyUsedPercent, test.wantWeekly)

			encoded, err := json.Marshal(check)
			if err != nil {
				t.Fatalf("marshal check: %v", err)
			}
			if strings.Contains(string(encoded), "-1") {
				t.Fatalf("doctor JSON exposed unavailable sentinel: %s", encoded)
			}
			if test.wantAbsentJSONKey != "" && strings.Contains(string(encoded), `"`+test.wantAbsentJSONKey+`"`) {
				t.Fatalf("doctor JSON included unavailable field %q: %s", test.wantAbsentJSONKey, encoded)
			}
		})
	}
}

func assertOptionalIntEqual(t *testing.T, label string, got, want *int) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("%s value = %v, want %v", label, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s value = %d, want %d", label, *got, *want)
	}
}

func TestDoctorReportStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		report DoctorReport
		want   string
	}{
		{
			name: "healthy",
			report: DoctorReport{Checks: []DoctorCheck{
				{Name: "oauth fetch: personal", OK: true},
				{Name: "oauth fetch: work", OK: true},
			}},
			want: "healthy",
		},
		{
			name: "degraded",
			report: DoctorReport{Checks: []DoctorCheck{
				{Name: "oauth fetch: personal", OK: true},
				{Name: "oauth fetch: work", OK: false},
			}},
			want: "degraded",
		},
		{
			name: "degraded with setup failure",
			report: DoctorReport{Checks: []DoctorCheck{
				{Name: "codex binary", OK: false},
				{Name: "oauth fetch: personal", OK: true},
			}},
			want: "degraded",
		},
		{
			name: "failed",
			report: DoctorReport{Checks: []DoctorCheck{
				{Name: "account candidates", OK: true},
				{Name: "oauth fetch: personal", OK: false},
			}},
			want: "failed",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.report.Status(); got != tc.want {
				t.Fatalf("Status() = %q, want %q", got, tc.want)
			}
			if got := tc.report.Healthy(); got != (tc.want != "failed") {
				t.Fatalf("Healthy() = %v, want %v", got, tc.want != "failed")
			}
		})
	}
}

func TestCheckCodexBinaryScrubsCodexEnvironment(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex-env.log")
	script := "#!/bin/sh\nenv > \"$USAGE_TEST_ENV_LOG\"\nprintf 'codex-test-version\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("USAGE_TEST_ENV_LOG", logPath)
	t.Setenv("CODEX_HOME", filepath.Join(root, "stale-codex"))
	t.Setenv("MULTICODEX_ACTIVE_PROFILE", "stale")
	t.Setenv("OPENAI_API_KEY", "stale")
	t.Setenv("CODEX_AUTH_TOKEN", "stale")

	check := checkCodexBinary(context.Background())
	if !check.OK {
		t.Fatalf("expected codex binary check ok, got %s", check.Details)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read codex env log: %v", err)
	}
	log := string(data)
	for _, forbidden := range []string{"CODEX_HOME", "MULTICODEX_ACTIVE_PROFILE", "OPENAI_API_KEY", "CODEX_AUTH_TOKEN"} {
		if envLogContainsKey(log, forbidden) {
			t.Fatalf("expected %s to be scrubbed from codex version env", forbidden)
		}
	}
}

func envLogContainsKey(log, key string) bool {
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, key+"=") {
			return true
		}
	}
	return false
}
