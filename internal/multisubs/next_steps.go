package multisubs

import (
	"fmt"
	"io"
	"strings"
)

type nextStep struct {
	Target  string
	Reason  string
	Command string
}

func isAuthFailure(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "authentication expired", "authentication rejected", "not logged in":
		return true
	default:
		return false
	}
}

func safeLoginProfileName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || name == defaultExecAccountLabel || name == claudeDefaultTarget {
		return "", false
	}
	if err := ValidateProfileName(name); err != nil {
		return "", false
	}
	return name, true
}

func accountLoginTargets(account usageAccountReport) (hasDefault bool, managed []string) {
	if account.HasDefault || len(account.ManagedNames) > 0 {
		hasDefault = account.HasDefault
		for _, name := range account.ManagedNames {
			if loginName, ok := safeLoginProfileName(name); ok {
				managed = append(managed, loginName)
			}
		}
		return hasDefault, managed
	}
	name := strings.TrimSpace(account.Name)
	if name == defaultExecAccountLabel || name == claudeDefaultTarget {
		return true, nil
	}
	if loginName, ok := safeLoginProfileName(name); ok {
		return false, []string{loginName}
	}
	return false, nil
}

func usageReportNextSteps(report usageReport) []nextStep {
	var steps []nextStep
	for _, provider := range report.Providers {
		providerLabel := strings.TrimSpace(provider.Name)
		if provider.Failure != "" {
			command := "multisubs doctor"
			if provider.Failure == "configuration unavailable" {
				command = "multisubs init"
			}
			steps = append(steps, nextStep{
				Target:  providerLabel,
				Reason:  provider.Failure,
				Command: command,
			})
		}
		for _, account := range provider.Accounts {
			if account.Failure == "" {
				continue
			}
			steps = append(steps, usageAccountNextSteps(providerLabel, account)...)
		}
	}
	return dedupeNextSteps(steps)
}

func usageAccountNextSteps(provider string, account usageAccountReport) []nextStep {
	reason := strings.TrimSpace(account.Failure)
	if reason == "" {
		return nil
	}
	hasDefault, managed := accountLoginTargets(account)
	if isAuthFailure(reason) {
		var steps []nextStep
		if hasDefault {
			steps = append(steps, nextStep{
				Target:  provider + " " + defaultExecAccountLabel,
				Reason:  reason,
				Command: defaultLoginCommand(provider),
			})
		}
		for _, name := range managed {
			steps = append(steps, nextStep{
				Target:  provider + " " + name,
				Reason:  reason,
				Command: managedLoginCommand(provider, name),
			})
		}
		if len(steps) > 0 {
			return steps
		}
	}
	return []nextStep{{
		Target:  usageAccountNextTarget(provider, account, hasDefault, managed),
		Reason:  reason,
		Command: "multisubs doctor",
	}}
}

func usageAccountNextTarget(provider string, account usageAccountReport, hasDefault bool, managed []string) string {
	switch {
	case hasDefault && len(managed) == 0:
		return provider + " " + defaultExecAccountLabel
	case !hasDefault && len(managed) == 1:
		return provider + " " + managed[0]
	case strings.TrimSpace(account.Name) != "":
		return provider + " " + strings.TrimSpace(account.Name)
	default:
		return provider
	}
}

func defaultLoginCommand(provider string) string {
	if strings.EqualFold(provider, "Claude") {
		return "claude auth login"
	}
	return "codex login"
}

func managedLoginCommand(provider, name string) string {
	if strings.EqualFold(provider, "Claude") {
		return "multisubs claude login " + name
	}
	return "multisubs codex login " + name
}

func statusRowReason(state, detail string) string {
	switch strings.TrimSpace(state) {
	case "", "logged-in", "ok":
		return ""
	case "logged-out":
		return "not logged in"
	case "missing":
		return "profile state unavailable"
	default:
		if isAuthFailure(detail) {
			return strings.TrimSpace(detail)
		}
		return "status check failed"
	}
}

func profileStatusNextSteps(provider string, rows []profileStatus) []nextStep {
	var steps []nextStep
	for _, row := range rows {
		reason := statusRowReason(row.State, row.Detail)
		if reason == "" {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if isAuthFailure(reason) {
			if name == defaultExecAccountLabel {
				steps = append(steps, nextStep{
					Target:  provider + " " + defaultExecAccountLabel,
					Reason:  reason,
					Command: defaultLoginCommand(provider),
				})
				continue
			}
			if loginName, ok := safeLoginProfileName(name); ok {
				steps = append(steps, nextStep{
					Target:  provider + " " + loginName,
					Reason:  reason,
					Command: managedLoginCommand(provider, loginName),
				})
				continue
			}
		}
		target := provider
		if name != "" {
			target = provider + " " + name
		}
		steps = append(steps, nextStep{
			Target:  target,
			Reason:  reason,
			Command: "multisubs doctor",
		})
	}
	return dedupeNextSteps(steps)
}

func dedupeNextSteps(steps []nextStep) []nextStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]nextStep, 0, len(steps))
	seen := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		if step.Command == "" || step.Reason == "" {
			continue
		}
		key := step.Command
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, step)
	}
	return out
}

func printNextSteps(writer io.Writer, steps []nextStep) {
	if len(steps) == 0 {
		return
	}
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Next:")
	for _, step := range steps {
		fmt.Fprintf(writer, "  %s · %s\n", step.Target, step.Reason)
		fmt.Fprintf(writer, "    Run: %s\n", step.Command)
	}
}
