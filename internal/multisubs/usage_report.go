package multisubs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	monitorusage "github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

const (
	usageProviderAll    = ""
	usageProviderCodex  = "codex"
	usageProviderClaude = "claude"

	usageAccountWorkerLimit = 4
)

var usageAccountTimeout = 15 * time.Second

type usageReport struct {
	Command   string
	UpdatedAt time.Time
	Providers []usageProviderReport
}

type usageProviderReport struct {
	Name     string
	Accounts []usageAccountReport
	Failure  string
}

type usageAccountReport struct {
	Name    string
	Windows []usageWindowReport
	Failure string
}

type usageWindowReport struct {
	Label       string
	UsedPercent *float64
	ResetAt     *time.Time
	ResetText   string
}

type codexUsageSourceFactory func(monitorusage.MonitorAccount) monitorusage.Source

type codexUsageTarget struct {
	codexRoutingTarget
	DisplayName string
}

func (a *App) cmdUsage(args []string, provider string) error {
	command := "multisubs usage"
	switch provider {
	case usageProviderCodex:
		command = "multisubs codex usage"
	case usageProviderClaude:
		command = "multisubs claude usage"
	}
	if len(args) != 0 {
		return &ExitError{Code: 2, Message: "usage: " + command}
	}

	now := time.Now()
	if a.usageClock != nil {
		now = a.usageClock()
	}
	location := time.Local
	if a.usageLocation != nil {
		location = a.usageLocation
	}
	report := usageReport{
		Command:   command,
		UpdatedAt: now.In(location),
	}

	switch provider {
	case usageProviderCodex:
		report.Providers = []usageProviderReport{a.collectCodexUsage()}
	case usageProviderClaude:
		report.Providers = []usageProviderReport{a.collectClaudeUsage()}
	default:
		report.Providers = make([]usageProviderReport, 2)
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			report.Providers[0] = a.collectCodexUsage()
		}()
		go func() {
			defer wait.Done()
			report.Providers[1] = a.collectClaudeUsage()
		}()
		wait.Wait()
	}

	printUsageReport(os.Stdout, report, now, location)
	if usageReportHasFailures(report) {
		return &ExitError{Code: 1}
	}
	return nil
}

func (a *App) collectCodexUsage() usageProviderReport {
	provider := usageProviderReport{Name: "Codex"}
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		provider.Failure = "configuration unavailable"
		cfg = DefaultConfig()
	}

	targets := codexUsageTargets(cfg, a.store.paths.DefaultCodexHome)
	provider.Accounts = collectConcurrent(targets, func(target codexUsageTarget) usageAccountReport {
		return a.collectCodexUsageTarget(target)
	})
	return provider
}

func codexUsageTargets(cfg *Config, defaultHome string) []codexUsageTarget {
	routingTargets := codexRoutingTargets(cfg, defaultHome)
	targets := make([]codexUsageTarget, 0, len(routingTargets))
	for _, target := range routingTargets {
		targets = append(targets, codexUsageTarget{codexRoutingTarget: target})
	}
	displayNames := allocateUsageDisplayNames(len(targets), func(index int) (string, bool) {
		target := targets[index]
		return target.Account.Label, target.Kind == codexRoutingTargetDefault
	})
	for index := range targets {
		targets[index].DisplayName = displayNames[index]
	}
	return targets
}

func (a *App) collectCodexUsageTarget(target codexUsageTarget) usageAccountReport {
	displayName := target.DisplayName
	if displayName == "" {
		displayName = target.Account.Label
	}
	account := usageAccountReport{Name: displayName}
	if target.Profile != nil {
		if err := ensureProfileCodexExecutionReady(a.store.paths, *target.Profile); err != nil {
			account.Failure = "profile state unavailable"
			return account
		}
	}

	sourceFactory := a.codexUsageSource
	if sourceFactory == nil {
		sourceFactory = monitorusage.NewReportUsageSourceForAccount
	}
	source := sourceFactory(target.Account)
	if source == nil {
		account.Failure = "usage source unavailable"
		return account
	}

	ctx, cancel := context.WithTimeout(context.Background(), usageAccountTimeout)
	summary, err := source.Fetch(ctx)
	cancel()
	closeErr := source.Close()
	if closeErr != nil {
		account.Failure = "usage cleanup failed"
		return account
	}
	if err != nil {
		if errors.Is(err, monitorusage.ErrWeeklyUsageUnavailable) && summary != nil {
			account = adaptCodexUsageAccount(displayName, summary)
			account.Failure = "weekly usage unavailable"
			return account
		}
		account.Failure = safeCodexUsageFailure(ctx, err)
		return account
	}
	if summary == nil {
		account.Failure = "usage response unavailable"
		return account
	}
	account = adaptCodexUsageAccount(displayName, summary)
	if !codexSummaryHasWeeklyData(summary) {
		account.Failure = "weekly usage unavailable"
	}
	return account
}

func codexSummaryHasWeeklyData(summary *monitorusage.Summary) bool {
	if summary == nil {
		return false
	}
	if summary.WeeklyWindow.UsedPercent >= 0 {
		return true
	}
	for _, window := range summary.RateLimitWindows {
		if window.WeeklyWindow.UsedPercent >= 0 {
			return true
		}
	}
	return false
}

func adaptCodexUsageAccount(name string, summary *monitorusage.Summary) usageAccountReport {
	account := usageAccountReport{Name: name}
	session := summary.SessionWindow
	weekly := summary.WeeklyWindow
	standardID := ""
	if id, standard, ok := summary.RateLimitWindowForModel(""); ok {
		standardID = id
		session = standard.SessionWindow
		weekly = standard.WeeklyWindow
	}

	account.Windows = append(account.Windows,
		adaptCodexUsageWindow(codexSessionLabel(session), session),
		adaptCodexUsageWindow("Weekly", weekly),
	)

	type modelWindow struct {
		label  string
		window monitorusage.WindowSummary
	}
	models := make([]modelWindow, 0, len(summary.RateLimitWindows))
	modelIndexByLabel := make(map[string]int, len(summary.RateLimitWindows))
	sparkReported := false
	limitIDs := make([]string, 0, len(summary.RateLimitWindows))
	for id := range summary.RateLimitWindows {
		limitIDs = append(limitIDs, id)
	}
	sort.Strings(limitIDs)
	for _, id := range limitIDs {
		if id == standardID || id == "codex" {
			continue
		}
		limit := summary.RateLimitWindows[id]
		label := safeCodexLimitLabel(limit.LimitName)
		if strings.Contains(strings.ToLower(id), "spark") ||
			strings.Contains(strings.ToLower(id), "bengalfox") ||
			strings.EqualFold(label, "spark") {
			label = "Spark"
			sparkReported = true
		}
		if label == "" {
			continue
		}
		label += " weekly"
		labelKey := strings.ToLower(label)
		if index, exists := modelIndexByLabel[labelKey]; exists {
			if models[index].window.UsedPercent < 0 && limit.WeeklyWindow.UsedPercent >= 0 {
				models[index].window = limit.WeeklyWindow
			}
			continue
		}
		modelIndexByLabel[labelKey] = len(models)
		models = append(models, modelWindow{label: label, window: limit.WeeklyWindow})
	}
	if !sparkReported {
		models = append(models, modelWindow{label: "Spark weekly", window: monitorusage.WindowSummary{UsedPercent: -1}})
	}
	sort.Slice(models, func(i, j int) bool {
		left := strings.ToLower(models[i].label)
		right := strings.ToLower(models[j].label)
		if left != right {
			return left < right
		}
		return models[i].label < models[j].label
	})
	for _, model := range models {
		account.Windows = append(account.Windows, adaptCodexUsageWindow(model.label, model.window))
	}
	return account
}

func safeCodexLimitLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 40 || emailRe.MatchString(label) {
		return ""
	}
	for _, character := range label {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == ' ', character == '-', character == '.':
		default:
			return ""
		}
	}
	return label
}

func adaptCodexUsageWindow(label string, window monitorusage.WindowSummary) usageWindowReport {
	out := usageWindowReport{Label: label}
	if window.UsedPercent < 0 {
		return out
	}
	used := float64(window.UsedPercent)
	out.UsedPercent = &used
	if window.ResetsAt != nil {
		reset := window.ResetsAt.UTC()
		out.ResetAt = &reset
	}
	return out
}

func codexSessionLabel(window monitorusage.WindowSummary) string {
	if window.WindowDurationMins == nil || *window.WindowDurationMins <= 0 {
		return "Session"
	}
	minutes := *window.WindowDurationMins
	if minutes == 300 {
		return "Session (5h)"
	}
	return "Session (" + formatWindowDuration(minutes) + ")"
}

func formatWindowDuration(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	remainder := minutes % 60
	if remainder == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, remainder)
}

func safeCodexUsageFailure(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "start codex app-server"),
		strings.Contains(message, "executable file not found"):
		return "Codex unavailable"
	case strings.Contains(message, "authentication expired"), strings.Contains(message, "token_expired"):
		return "authentication expired"
	case strings.Contains(message, "authentication rejected"), strings.Contains(message, "unauthorized"):
		return "authentication rejected"
	case strings.Contains(message, "auth.json not found"),
		strings.Contains(message, "missing tokens.access_token"),
		strings.Contains(message, "requires openai authentication"):
		return "not logged in"
	default:
		return "usage probe failed"
	}
}

func (a *App) collectClaudeUsage() usageProviderReport {
	provider := usageProviderReport{Name: "Claude"}
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		provider.Failure = "configuration unavailable"
		cfg = defaultClaudeConfig()
	}
	targets := claudeUsageTargets(cfg)
	provider.Accounts = collectConcurrent(targets, func(target claudeTarget) usageAccountReport {
		return a.collectClaudeUsageTarget(store, target)
	})
	return provider
}

func claudeUsageTargets(cfg *claudeConfig) []claudeTarget {
	sharedTargets := claudeTargets(cfg)
	targets := make([]claudeTarget, 0, len(sharedTargets))
	for _, target := range sharedTargets {
		if target.Kind == "managed" {
			targets = append(targets, target)
		}
	}
	for _, target := range sharedTargets {
		if target.Kind == "default" {
			targets = append(targets, target)
		}
	}
	displayNames := allocateUsageDisplayNames(len(targets), func(index int) (string, bool) {
		return targets[index].Name, targets[index].Kind == "default"
	})
	for index := range targets {
		targets[index].DisplayName = displayNames[index]
	}
	return targets
}

func (a *App) collectClaudeUsageTarget(store *claudeStore, target claudeTarget) usageAccountReport {
	displayName := target.DisplayName
	if displayName == "" {
		displayName = target.Name
	}
	account := usageAccountReport{Name: displayName}
	if target.Profile != nil {
		if err := store.EnsureProfileReady(*target.Profile); err != nil {
			account.Failure = "profile state unavailable"
			return account
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), usageAccountTimeout)
	providerUsage, err := fetchClaudeUsage(ctx, a.claudeCommandRunner(), target.ConfigDir)
	cancel()
	if err != nil {
		account.Failure = safeClaudeUsageFailure(ctx, err)
		return account
	}
	account.Windows = []usageWindowReport{
		adaptClaudeUsageWindow(claudeSessionLabel(providerUsage.Session), providerUsage.Session),
		adaptClaudeUsageWindow("Weekly all models", providerUsage.WeeklyAll),
		{Label: "Fable weekly"},
	}
	if providerUsage.Fable != nil {
		account.Windows[2] = adaptClaudeUsageWindow("Fable weekly", *providerUsage.Fable)
	}
	return account
}

func allocateUsageDisplayNames(count int, target func(int) (name string, isDefault bool)) []string {
	names := make([]string, count)
	defaults := make([]bool, count)
	reserved := make(map[string]struct{}, count)
	for index := 0; index < count; index++ {
		name, isDefault := target(index)
		names[index] = name
		defaults[index] = isDefault
		reserved[name] = struct{}{}
	}

	out := make([]string, count)
	used := map[string]struct{}{defaultExecAccountLabel: {}}
	nextAlias := 1
	allocateAlias := func() string {
		for {
			candidate := fmt.Sprintf("[managed-%d]", nextAlias)
			nextAlias++
			if _, exists := reserved[candidate]; exists {
				continue
			}
			if _, exists := used[candidate]; exists {
				continue
			}
			used[candidate] = struct{}{}
			return candidate
		}
	}
	for index, name := range names {
		if defaults[index] {
			out[index] = defaultExecAccountLabel
			continue
		}
		_, duplicate := used[name]
		if name == "" || name == defaultExecAccountLabel || emailRe.MatchString(name) || duplicate {
			out[index] = allocateAlias()
			continue
		}
		out[index] = name
		used[name] = struct{}{}
	}
	return out
}

func claudeSessionLabel(window claudeUsageWindow) string {
	if window.DurationMins == nil || *window.DurationMins <= 0 {
		return "Session (~5h)"
	}
	return "Session (" + formatWindowDuration(*window.DurationMins) + ")"
}

func adaptClaudeUsageWindow(label string, window claudeUsageWindow) usageWindowReport {
	used := window.UsedPercent
	return usageWindowReport{
		Label:       label,
		UsedPercent: &used,
		ResetText:   sanitizeClaudeResetText(window.ResetText),
	}
}

var (
	claudeResetCountdownTextRE = regexp.MustCompile(`(?i)^Resets in ([0-9]{1,4}) (minute|minutes|hour|hours|day|days)(?: \((UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)\))?$`)
	claudeResetWeekdayTextRE   = regexp.MustCompile(`(?i)^Resets (Mon(?:day)?|Tue(?:sday)?|Wed(?:nesday)?|Thu(?:rsday)?|Fri(?:day)?|Sat(?:urday)?|Sun(?:day)?) at ([0-9]{1,2})(?::([0-9]{2}))?(?: ?([ap]m))?(?: \((UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)\))?$`)
	claudeResetMonthTextRE     = regexp.MustCompile(`(?i)^Resets (Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?) ([0-9]{1,2}) at ([0-9]{1,2})(?::([0-9]{2}))?(?: ?([ap]m))?(?: \((UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)\))?$`)
	claudeResetAtTextRE        = regexp.MustCompile(`(?i)^Resets at ([0-9]{1,2})(?::([0-9]{2}))?(?: ?([ap]m))?(?: \((UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)\))?$`)
)

func sanitizeClaudeResetText(reset string) string {
	if len(reset) > 120 {
		return ""
	}
	for index := 0; index < len(reset); index++ {
		if reset[index] < 0x20 || reset[index] > 0x7e {
			return ""
		}
	}
	reset = strings.Join(strings.Fields(reset), " ")
	if reset == "" {
		return ""
	}

	if matches := claudeResetCountdownTextRE.FindStringSubmatch(reset); matches != nil {
		count, err := strconv.Atoi(matches[1])
		if err == nil && count > 0 && validClaudeResetTimezone(matches[3]) {
			return reset
		}
	}
	if matches := claudeResetWeekdayTextRE.FindStringSubmatch(reset); matches != nil {
		if validClaudeResetTime(matches[2], matches[3], matches[4]) &&
			validClaudeResetTimezone(matches[5]) {
			return reset
		}
	}
	if matches := claudeResetMonthTextRE.FindStringSubmatch(reset); matches != nil {
		day, err := strconv.Atoi(matches[2])
		if err == nil && day >= 1 && day <= 31 &&
			validClaudeResetTime(matches[3], matches[4], matches[5]) &&
			validClaudeResetTimezone(matches[6]) {
			return reset
		}
	}
	if matches := claudeResetAtTextRE.FindStringSubmatch(reset); matches != nil {
		if validClaudeResetTime(matches[1], matches[2], matches[3]) &&
			validClaudeResetTimezone(matches[4]) {
			return reset
		}
	}
	return ""
}

func validClaudeResetTime(hourText, minuteText, meridiem string) bool {
	hour, err := strconv.Atoi(hourText)
	if err != nil {
		return false
	}
	if minuteText == "" {
		if meridiem == "" {
			return false
		}
	} else {
		minute, minuteErr := strconv.Atoi(minuteText)
		if minuteErr != nil || minute < 0 || minute > 59 {
			return false
		}
	}
	if meridiem != "" {
		return hour >= 1 && hour <= 12
	}
	return hour >= 0 && hour <= 23
}

func validClaudeResetTimezone(timezone string) bool {
	if timezone == "" {
		return true
	}
	if len(timezone) > 64 {
		return false
	}
	if timezone == "UTC" {
		return true
	}
	segments := strings.Split(timezone, "/")
	if len(segments) < 2 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 || len(segment) > 32 || segment == "." || segment == ".." {
			return false
		}
		if !isASCIIAlphaNumeric(segment[0]) || !isASCIIAlphaNumeric(segment[len(segment)-1]) {
			return false
		}
		for index := 0; index < len(segment); index++ {
			character := segment[index]
			if isASCIIAlphaNumeric(character) ||
				character == '_' || character == '-' || character == '+' || character == '.' {
				continue
			}
			return false
		}
	}
	_, err := time.LoadLocation(timezone)
	return err == nil
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func safeClaudeUsageFailure(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not logged in"):
		return "not logged in"
	case strings.Contains(message, "launch failure"):
		return "Claude unavailable"
	case strings.Contains(message, "parse claude usage"):
		return "usage response malformed"
	case strings.Contains(message, "reported an error"):
		return "usage unavailable"
	default:
		return "usage probe failed"
	}
}

func collectConcurrent[Target any](targets []Target, collect func(Target) usageAccountReport) []usageAccountReport {
	results := make([]usageAccountReport, len(targets))
	if len(targets) == 0 {
		return results
	}
	workers := usageAccountWorkerLimit
	if len(targets) < workers {
		workers = len(targets)
	}
	sem := make(chan struct{}, workers)
	var wait sync.WaitGroup
	for index, target := range targets {
		index := index
		target := target
		wait.Add(1)
		go func() {
			defer wait.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[index] = collect(target)
		}()
	}
	wait.Wait()
	return results
}

func printUsageReport(writer io.Writer, report usageReport, now time.Time, location *time.Location) {
	fmt.Fprintln(writer, report.Command)
	fmt.Fprintf(writer, "Updated: %s\n", report.UpdatedAt.Format("Mon 02 Jan 2006 15:04 MST"))
	for _, provider := range report.Providers {
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, provider.Name)
		if provider.Failure != "" {
			fmt.Fprintf(writer, "  unavailable · %s\n", provider.Failure)
		}
		width := usageProviderLabelWidth(provider)
		for accountIndex, account := range provider.Accounts {
			if accountIndex > 0 {
				fmt.Fprintln(writer)
			}
			fmt.Fprintf(writer, "  %s\n", account.Name)
			if account.Failure != "" && len(account.Windows) == 0 {
				fmt.Fprintf(writer, "    unavailable · %s\n", account.Failure)
				continue
			}
			for _, window := range account.Windows {
				fmt.Fprintf(writer, "    %-*s  %s\n", width, window.Label, formatUsageWindow(window, now, location))
			}
			if account.Failure != "" {
				fmt.Fprintf(writer, "    partial · %s\n", account.Failure)
			}
		}
	}
	available, total := usageReportAvailability(report)
	result := "complete"
	if available != total || usageReportHasProviderFailure(report) {
		result = "partial"
	}
	fmt.Fprintln(writer)
	fmt.Fprintf(writer, "Result: %s · %d of %d accounts available\n", result, available, total)
}

func usageProviderLabelWidth(provider usageProviderReport) int {
	width := 0
	for _, account := range provider.Accounts {
		for _, window := range account.Windows {
			if len(window.Label) > width {
				width = len(window.Label)
			}
		}
	}
	return width
}

func formatUsageWindow(window usageWindowReport, now time.Time, location *time.Location) string {
	if window.UsedPercent == nil {
		return "not reported"
	}
	out := formatUsagePercent(*window.UsedPercent) + " used"
	switch {
	case window.ResetAt != nil:
		reset := window.ResetAt.In(location)
		if !reset.After(now) {
			return out + " · reset due"
		}
		return out + " · resets in " + formatResetCountdown(reset.Sub(now)) +
			" (" + reset.Format("Mon 02 Jan 15:04 MST") + ")"
	case window.ResetText != "":
		return out + " · " + window.ResetText
	default:
		return out + " · reset unknown"
	}
}

func formatUsagePercent(value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%d%%", int(value))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".") + "%"
}

func formatResetCountdown(duration time.Duration) string {
	minutes := int((duration + time.Minute - 1) / time.Minute)
	if minutes < 1 {
		return "<1m"
	}
	days := minutes / (24 * 60)
	hours := (minutes % (24 * 60)) / 60
	remainder := minutes % 60
	if days > 0 {
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		if remainder == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, remainder)
	}
	return fmt.Sprintf("%dm", remainder)
}

func usageReportAvailability(report usageReport) (int, int) {
	available := 0
	total := 0
	for _, provider := range report.Providers {
		for _, account := range provider.Accounts {
			total++
			if account.Failure == "" {
				available++
			}
		}
	}
	return available, total
}

func usageReportHasProviderFailure(report usageReport) bool {
	for _, provider := range report.Providers {
		if provider.Failure != "" {
			return true
		}
	}
	return false
}

func usageReportHasFailures(report usageReport) bool {
	if usageReportHasProviderFailure(report) {
		return true
	}
	available, total := usageReportAvailability(report)
	return available != total
}
