package multicodex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type claudeUsageWindow struct {
	UsedPercent float64
	ResetText   string
}

type claudeUsage struct {
	Session   claudeUsageWindow
	WeeklyAll claudeUsageWindow
	Fable     *claudeUsageWindow
}

var (
	claudeANSIControlRE = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)
	claudePercentRE     = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*%`)
	claudeResetRE       = regexp.MustCompile(`(?i)\breset(?:s|ting)?\b.*`)
)

func fetchClaudeUsage(ctx context.Context, runner claudeCommandRunner, configDir string) (claudeUsage, error) {
	stdout, _, err := runner.Capture(ctx, claudeUsageProbeArgs(), claudeEnv(os.Environ(), configDir))
	if err != nil {
		return claudeUsage{}, fmt.Errorf("Claude usage command failed: %s", claudeProbeFailure(ctx, err))
	}
	usage, err := parseClaudeUsageEnvelope(stdout)
	if err != nil {
		return claudeUsage{}, err
	}
	return usage, nil
}

func claudeUsageProbeArgs() []string {
	return []string{
		"-p",
		"--no-session-persistence",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--mcp-config", `{"mcpServers":{}}`,
		"--output-format", "json",
		"/usage",
	}
}

func parseClaudeUsageEnvelope(raw []byte) (claudeUsage, error) {
	if len(raw) > 1<<20 {
		return claudeUsage{}, errors.New("parse Claude usage JSON: response exceeds 1 MiB")
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed[0] != '{' {
		return claudeUsage{}, errors.New("parse Claude usage JSON: expected an object")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return claudeUsage{}, errors.New("parse Claude usage JSON: invalid JSON")
	}
	if envelope == nil {
		return claudeUsage{}, errors.New("parse Claude usage JSON: expected an object")
	}

	isErrorRaw, ok := envelope["is_error"]
	if !ok {
		return claudeUsage{}, errors.New("parse Claude usage JSON: missing is_error")
	}
	var isError bool
	if err := json.Unmarshal(isErrorRaw, &isError); err != nil {
		return claudeUsage{}, errors.New("parse Claude usage JSON: is_error must be a boolean")
	}
	if isError {
		return claudeUsage{}, errors.New("Claude usage response reported an error")
	}

	resultRaw, ok := envelope["result"]
	if !ok {
		return claudeUsage{}, errors.New("parse Claude usage JSON: missing result string")
	}
	var result string
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return claudeUsage{}, errors.New("parse Claude usage JSON: result must be a string")
	}
	if strings.TrimSpace(result) == "" {
		return claudeUsage{}, errors.New("parse Claude usage JSON: result string is empty")
	}
	return parseClaudeUsageResult(result)
}

type claudeUsageSection int

const (
	claudeUsageSectionNone claudeUsageSection = iota
	claudeUsageSectionSession
	claudeUsageSectionWeeklyAll
	claudeUsageSectionFable
)

type claudeUsageWindowBuilder struct {
	seenHeading bool
	percent     *float64
	reset       string
}

func parseClaudeUsageResult(result string) (claudeUsage, error) {
	cleaned := claudeANSIControlRE.ReplaceAllString(strings.ReplaceAll(result, "\r\n", "\n"), "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	builders := map[claudeUsageSection]*claudeUsageWindowBuilder{
		claudeUsageSectionSession:   {},
		claudeUsageSectionWeeklyAll: {},
		claudeUsageSectionFable:     {},
	}
	section := claudeUsageSectionNone
	for _, rawLine := range strings.Split(cleaned, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "what's contributing to your limits usage") {
			break
		}
		if detected := detectClaudeUsageSection(line); detected != claudeUsageSectionNone {
			section = detected
			builders[section].seenHeading = true
		}
		if section == claudeUsageSectionNone {
			continue
		}
		builder := builders[section]
		matches := claudePercentRE.FindAllStringSubmatch(line, -1)
		if len(matches) > 1 {
			return claudeUsage{}, errors.New("parse Claude usage result: multiple percentages in one line")
		}
		if len(matches) == 1 {
			value, err := strconv.ParseFloat(matches[0][1], 64)
			if err != nil || value < 0 || value > 100 {
				return claudeUsage{}, errors.New("parse Claude usage result: invalid percentage")
			}
			if builder.percent != nil && *builder.percent != value {
				return claudeUsage{}, fmt.Errorf("parse Claude usage result: conflicting percentages in one section")
			}
			builder.percent = &value
		}
		if reset := claudeResetRE.FindString(line); reset != "" {
			reset = strings.TrimSpace(reset)
			if builder.reset != "" && builder.reset != reset {
				return claudeUsage{}, errors.New("parse Claude usage result: conflicting reset text in one section")
			}
			builder.reset = reset
		}
	}

	session, err := completeClaudeUsageWindow("session", builders[claudeUsageSectionSession])
	if err != nil {
		return claudeUsage{}, err
	}
	weekly, err := completeClaudeUsageWindow("weekly all-model", builders[claudeUsageSectionWeeklyAll])
	if err != nil {
		return claudeUsage{}, err
	}
	parsed := claudeUsage{Session: session, WeeklyAll: weekly}
	fableBuilder := builders[claudeUsageSectionFable]
	if fableBuilder.seenHeading {
		fable, err := completeClaudeUsageWindowAllowMissingReset("Fable", fableBuilder)
		if err != nil {
			return claudeUsage{}, err
		}
		parsed.Fable = &fable
	}
	return parsed, nil
}

func detectClaudeUsageSection(line string) claudeUsageSection {
	lower := strings.ToLower(strings.TrimSpace(line))
	lower = strings.Trim(lower, "#*_-=:|[]() ")
	if strings.Contains(lower, "fable") {
		return claudeUsageSectionFable
	}
	if strings.Contains(lower, "all models") && (strings.Contains(lower, "week") || strings.Contains(lower, "weekly")) {
		return claudeUsageSectionWeeklyAll
	}
	if lower == "session" || strings.HasPrefix(lower, "session ") || strings.Contains(lower, "current session") {
		return claudeUsageSectionSession
	}
	return claudeUsageSectionNone
}

func completeClaudeUsageWindow(name string, builder *claudeUsageWindowBuilder) (claudeUsageWindow, error) {
	return completeClaudeUsageWindowAllowMissingReset(name, builder)
}

func completeClaudeUsageWindowAllowMissingReset(name string, builder *claudeUsageWindowBuilder) (claudeUsageWindow, error) {
	if !builder.seenHeading {
		return claudeUsageWindow{}, fmt.Errorf("parse Claude usage result: missing %s section", name)
	}
	if builder.percent == nil {
		return claudeUsageWindow{}, fmt.Errorf("parse Claude usage result: missing %s percentage", name)
	}
	return claudeUsageWindow{UsedPercent: *builder.percent, ResetText: builder.reset}, nil
}

func formatClaudePercent(value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%d%%", int(value))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".") + "%"
}
