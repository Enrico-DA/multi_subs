package multisubs

import (
	"bytes"
	"strings"
	"testing"
)

func TestUsageReportNextStepsPrintsDefaultCodexLogin(t *testing.T) {
	t.Parallel()

	report := usageReport{
		Providers: []usageProviderReport{{
			Name: "Codex",
			Accounts: []usageAccountReport{{
				Name:       "default",
				HasDefault: true,
				Failure:    "authentication expired",
			}},
		}},
	}
	var output bytes.Buffer
	printNextSteps(&output, usageReportNextSteps(report))
	got := output.String()
	for _, want := range []string{
		"Next:",
		"Codex default · authentication expired",
		"Run: codex login",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "multisubs default") || strings.Contains(got, "multisubs codex login default") {
		t.Fatalf("used a managed-profile login for the default account:\n%s", got)
	}
}

func TestUsageReportNextStepsPrintsManagedAndDefaultLogins(t *testing.T) {
	t.Parallel()

	report := usageReport{
		Providers: []usageProviderReport{
			{
				Name: "Codex",
				Accounts: []usageAccountReport{{
					Name:         "ehit (also default)",
					HasDefault:   true,
					ManagedNames: []string{"ehit"},
					Failure:      "not logged in",
				}},
			},
			{
				Name: "Claude",
				Accounts: []usageAccountReport{{
					Name:         "gmail",
					ManagedNames: []string{"gmail"},
					Failure:      "not logged in",
				}},
			},
		},
	}
	steps := usageReportNextSteps(report)
	var output bytes.Buffer
	printNextSteps(&output, steps)
	got := output.String()
	for _, want := range []string{
		"Run: codex login",
		"Run: multisubs codex login ehit",
		"Run: multisubs claude login gmail",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestUsageReportNextStepsRejectsUnsafeManagedNames(t *testing.T) {
	t.Parallel()

	report := usageReport{
		Providers: []usageProviderReport{{
			Name: "Codex",
			Accounts: []usageAccountReport{{
				Name:         "unsafe",
				ManagedNames: []string{"Not Valid", "default", ""},
				Failure:      "authentication expired",
			}},
		}},
	}
	var output bytes.Buffer
	printNextSteps(&output, usageReportNextSteps(report))
	got := output.String()
	if strings.Contains(got, "multisubs codex login") {
		t.Fatalf("interpolated an unsafe profile name:\n%s", got)
	}
	if !strings.Contains(got, "Run: multisubs doctor") {
		t.Fatalf("expected doctor fallback, got:\n%s", got)
	}
}

func TestUsageReportNextStepsDedupesDoctor(t *testing.T) {
	t.Parallel()

	report := usageReport{
		Providers: []usageProviderReport{{
			Name: "Codex",
			Accounts: []usageAccountReport{
				{Name: "alpha", Failure: "timed out"},
				{Name: "beta", Failure: "usage probe failed"},
			},
		}},
	}
	got := usageReportNextSteps(report)
	if len(got) != 1 || got[0].Command != "multisubs doctor" {
		t.Fatalf("expected one doctor step, got %#v", got)
	}
}

func TestProfileStatusNextStepsUsesOfficialDefaultLogin(t *testing.T) {
	t.Parallel()

	steps := profileStatusNextSteps("Codex", []profileStatus{
		{Name: "default", State: "logged-out", Detail: "not logged in"},
		{Name: "work", State: "error", Detail: "/synthetic/private/path"},
	})
	var output bytes.Buffer
	printNextSteps(&output, steps)
	got := output.String()
	if !strings.Contains(got, "Run: codex login") {
		t.Fatalf("missing default login:\n%s", got)
	}
	if !strings.Contains(got, "Run: multisubs doctor") {
		t.Fatalf("missing doctor fallback:\n%s", got)
	}
	if strings.Contains(got, "/synthetic/private/path") {
		t.Fatalf("leaked status detail:\n%s", got)
	}
}

func TestClaudeUnverifiedDefaultUsesDoctorInsteadOfLogin(t *testing.T) {
	t.Parallel()

	report := usageReport{
		Providers: []usageProviderReport{{
			Name: "Claude",
			Accounts: []usageAccountReport{{
				Name:       "default",
				HasDefault: true,
				Failure:    "identity unavailable",
			}},
		}},
	}
	var output bytes.Buffer
	printNextSteps(&output, usageReportNextSteps(report))
	got := output.String()
	if !strings.Contains(got, "Claude default · identity unavailable") ||
		!strings.Contains(got, "Run: multisubs doctor") {
		t.Fatalf("missing unverified-default recovery:\n%s", got)
	}
	if strings.Contains(got, "claude auth login") {
		t.Fatalf("treated unverified default identity as a proven logout:\n%s", got)
	}

	output.Reset()
	printNextSteps(&output, profileStatusNextSteps("Claude", []profileStatus{{
		Name:    "default",
		State:   "logged-in",
		Account: "identity unavailable",
		Detail:  claudeDefaultIdentityDetail,
	}}))
	got = output.String()
	if got != "" {
		t.Fatalf("logged-in unverified default produced a recovery step:\n%s", got)
	}
}

func TestPrintNextStepsOmitsEmptyList(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	printNextSteps(&output, nil)
	if output.Len() != 0 {
		t.Fatalf("expected no next-step output, got %q", output.String())
	}
}
