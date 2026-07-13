package usage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type DoctorReport struct {
	Checks []DoctorCheck `json:"checks"`
}

type DoctorOptions struct {
	Accounts         MonitorAccountOptions
	IncludeAppServer bool
}

func RunDoctor(ctx context.Context, options DoctorOptions) DoctorReport {
	var checks []DoctorCheck

	checks = append(checks, checkCodexBinary(ctx))
	accounts, warning, err := loadMonitorAccountsWithOptions(options.Accounts)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "account candidates", OK: false, Details: err.Error()})
	} else if len(accounts) == 0 {
		details := "no monitor accounts configured"
		if warning != "" {
			details += ": " + warning
		}
		checks = append(checks, DoctorCheck{Name: "account candidates", OK: false, Details: details})
	} else {
		details := fmt.Sprintf("%d account(s)", len(accounts))
		if warning != "" {
			details += "; " + warning
		}
		checks = append(checks, DoctorCheck{Name: "account candidates", OK: true, Details: details})
		sourceChecks := make(chan DoctorCheck, len(accounts)*2)
		expected := 0
		for _, account := range accounts {
			account := account
			usageSource := NewUsageSourceForAccount(account)
			defer usageSource.Close()
			expected++
			go func() { sourceChecks <- checkSourceFetch(ctx, account, usageSource) }()
			if options.IncludeAppServer {
				appSource := NewAppServerSourceForHome(account.CodexHome)
				defer appSource.Close()
				expected++
				go func() { sourceChecks <- checkSourceFetch(ctx, account, appSource) }()
			}
		}
		for i := 0; i < expected; i++ {
			checks = append(checks, <-sourceChecks)
		}
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })

	return DoctorReport{Checks: checks}
}

func (r DoctorReport) Healthy() bool {
	return r.Status() != "failed"
}

func (r DoctorReport) Status() string {
	var fetchOK, fetchFailed, setupFailed bool
	for _, c := range r.Checks {
		if strings.Contains(c.Name, " fetch") {
			if c.OK {
				fetchOK = true
			} else {
				fetchFailed = true
			}
		} else if !c.OK {
			setupFailed = true
		}
	}
	switch {
	case fetchOK && !fetchFailed && !setupFailed:
		return "healthy"
	case fetchOK:
		return "degraded"
	default:
		return "failed"
	}
}

func checkCodexBinary(ctx context.Context) DoctorCheck {
	cmd := exec.CommandContext(ctx, "codex", "--version")
	cmd.Env = withoutCodexProfileEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return DoctorCheck{
			Name:    "codex binary",
			OK:      false,
			Details: fmt.Sprintf("failed to execute codex --version: %v", err),
		}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		version = "version output is empty"
	}
	return DoctorCheck{
		Name:    "codex binary",
		OK:      true,
		Details: version,
	}
}

func checkSourceFetch(ctx context.Context, account MonitorAccount, source Source) DoctorCheck {
	summary, err := source.Fetch(ctx)
	name := fmt.Sprintf("%s fetch: %s", source.Name(), account.Label)
	if err != nil {
		return DoctorCheck{
			Name:    name,
			OK:      false,
			Details: err.Error(),
		}
	}
	fiveHourUsedPercent := availableUsedPercent(summary.PrimaryWindow)
	weeklyUsedPercent := availableUsedPercent(summary.SecondaryWindow)
	return DoctorCheck{
		Name:                name,
		OK:                  true,
		Details:             formatUsageDetails(summary.PlanType, summary.Source, fiveHourUsedPercent, weeklyUsedPercent),
		PlanType:            summary.PlanType,
		Source:              summary.Source,
		FiveHourUsedPercent: fiveHourUsedPercent,
		WeeklyUsedPercent:   weeklyUsedPercent,
	}
}

func availableUsedPercent(window WindowSummary) *int {
	if window.UsedPercent == unavailableUsedPercent {
		return nil
	}
	usedPercent := window.UsedPercent
	return &usedPercent
}

func formatUsageDetails(planType, source string, fiveHourUsedPercent, weeklyUsedPercent *int) string {
	parts := []string{fmt.Sprintf("plan=%s", planType)}
	if fiveHourUsedPercent != nil {
		parts = append(parts, fmt.Sprintf("5h=%d%%", *fiveHourUsedPercent))
	}
	if weeklyUsedPercent != nil {
		parts = append(parts, fmt.Sprintf("weekly=%d%%", *weeklyUsedPercent))
	}
	return strings.Join(append(parts, fmt.Sprintf("source=%s", source)), " ")
}
