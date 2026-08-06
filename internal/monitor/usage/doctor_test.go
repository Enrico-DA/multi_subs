package usage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
			if got := tc.report.Healthy(); got != (tc.want == "healthy") {
				t.Fatalf("Healthy() = %v, want %v", got, tc.want == "healthy")
			}
		})
	}
}

func TestCheckSourceFetchFormatsWeeklyUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		weekly     WindowSummary
		want       string
		wantWeekly *int
	}{
		{name: "unused", weekly: WindowSummary{UsedPercent: 0}, want: "plan=pro weekly=0% source=app-server", wantWeekly: doctorTestInt(0)},
		{name: "partly used", weekly: WindowSummary{UsedPercent: 24}, want: "plan=pro weekly=24% source=app-server", wantWeekly: doctorTestInt(24)},
		{name: "exhausted", weekly: WindowSummary{UsedPercent: 100}, want: "plan=pro weekly=100% source=app-server", wantWeekly: doctorTestInt(100)},
		{name: "unavailable", weekly: unavailableWindowSummary(), want: "plan=pro weekly=unavailable source=app-server"},
		{name: "invalid negative", weekly: WindowSummary{UsedPercent: -2}, want: "plan=pro weekly=-2% source=app-server"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := checkSourceFetch(context.Background(), MonitorAccount{Label: "personal"}, &fakeSource{
				name: "usage",
				out: &Summary{
					PlanType:     "pro",
					Source:       "app-server",
					WeeklyWindow: tc.weekly,
				},
			})

			if !check.OK {
				t.Fatalf("expected successful check, got %q", check.Details)
			}
			if check.Details != tc.want {
				t.Fatalf("Details = %q, want %q", check.Details, tc.want)
			}
			if check.PlanType != "pro" || check.Source != "app-server" {
				t.Fatalf("structured fields = plan %q, source %q", check.PlanType, check.Source)
			}
			assertDoctorPercentage(t, check.WeeklyUsedPercent, tc.wantWeekly)

			encoded, err := json.Marshal(check)
			if err != nil {
				t.Fatalf("marshal check: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("unmarshal check fields: %v", err)
			}
			if string(fields["plan_type"]) != `"pro"` || string(fields["source"]) != `"app-server"` {
				t.Fatalf("JSON structured fields are wrong: %s", encoded)
			}
			weeklyJSON, weeklyExists := fields["weekly_used_percent"]
			if tc.wantWeekly == nil {
				if weeklyExists {
					t.Fatalf("JSON included unavailable weekly percentage: %s", encoded)
				}
			} else {
				if !weeklyExists {
					t.Fatalf("JSON omitted available weekly percentage: %s", encoded)
				}
				var weeklyUsedPercent int
				if err := json.Unmarshal(weeklyJSON, &weeklyUsedPercent); err != nil {
					t.Fatalf("unmarshal JSON weekly percentage: %v", err)
				}
				if weeklyUsedPercent != *tc.wantWeekly {
					t.Fatalf("JSON weekly percentage is wrong: %s", encoded)
				}
			}
		})
	}
}

func TestCheckSourceFetchFailureHasNoStructuredFields(t *testing.T) {
	t.Parallel()

	check := checkSourceFetch(context.Background(), MonitorAccount{Label: "personal"}, &fakeSource{
		name: "app-server",
		err:  errors.New("synthetic fetch failure"),
	})

	if check.OK {
		t.Fatal("expected failed check")
	}
	if check.Details != "synthetic fetch failure" {
		t.Fatalf("Details = %q, want synthetic fetch failure", check.Details)
	}
	if check.PlanType != "" || check.Source != "" || check.WeeklyUsedPercent != nil {
		t.Fatalf("failed check carried structured fields: %+v", check)
	}
	encoded, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("marshal failed check: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal failed check fields: %v", err)
	}
	for _, field := range []string{"plan_type", "source", "weekly_used_percent"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("failed check JSON included structured field %q: %s", field, encoded)
		}
	}
}

func TestCheckSourceFetchOmitsUnavailableTextFields(t *testing.T) {
	t.Parallel()

	check := checkSourceFetch(context.Background(), MonitorAccount{Label: "personal"}, &fakeSource{
		name: "usage",
		out: &Summary{
			WeeklyWindow: WindowSummary{UsedPercent: 24},
		},
	})
	if !check.OK {
		t.Fatalf("expected successful check, got %q", check.Details)
	}
	if check.Details != "plan= weekly=24% source=" {
		t.Fatalf("Details = %q, want empty plan and source values", check.Details)
	}

	encoded, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("marshal check: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal check fields: %v", err)
	}
	for _, field := range []string{"plan_type", "source"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("JSON included unavailable field %q: %s", field, encoded)
		}
	}
	if string(fields["weekly_used_percent"]) != "24" {
		t.Fatalf("JSON weekly percentage is wrong: %s", encoded)
	}
}

func TestDoctorCheckJSONRoundTripKeepsAbsentFieldsOmitted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputJSON    string
		absentFields []string
	}{
		{
			name:         "unavailable weekly window",
			inputJSON:    `{"name":"usage fetch: personal","ok":true,"details":"plan=pro weekly=unavailable source=app-server","plan_type":"pro","source":"app-server"}`,
			absentFields: []string{"weekly_used_percent"},
		},
		{
			name:         "failed fetch",
			inputJSON:    `{"name":"usage fetch: personal","ok":false,"details":"synthetic fetch failure"}`,
			absentFields: []string{"plan_type", "source", "weekly_used_percent"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var roundTripped DoctorCheck
			if err := json.Unmarshal([]byte(tc.inputJSON), &roundTripped); err != nil {
				t.Fatalf("unmarshal check: %v", err)
			}
			roundTrippedJSON, err := json.Marshal(roundTripped)
			if err != nil {
				t.Fatalf("marshal round-tripped check: %v", err)
			}

			var fields map[string]json.RawMessage
			if err := json.Unmarshal(roundTrippedJSON, &fields); err != nil {
				t.Fatalf("unmarshal round-tripped fields: %v", err)
			}
			for _, field := range tc.absentFields {
				if _, exists := fields[field]; exists {
					t.Fatalf("round-tripped JSON included absent field %q: %s", field, roundTrippedJSON)
				}
			}
		})
	}
}

func doctorTestInt(value int) *int {
	return &value
}

func assertDoctorPercentage(t *testing.T, got, want *int) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("weekly used percentage = %v, want %v", got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("weekly used percentage = %d, want %d", *got, *want)
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
	t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")
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
	for _, forbidden := range []string{"CODEX_HOME", "MULTISUBS_ACTIVE_PROFILE", "OPENAI_API_KEY", "CODEX_AUTH_TOKEN"} {
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
