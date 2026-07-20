package multicodex

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseClaudeUsageEnvelopeStrictlyParsesResultString(t *testing.T) {
	result := "\x1b[1mCurrent session\x1b[0m\n12.5% used\nResets in 2 hours\nCurrent week (all models)\n80% used\nResets Monday at 09:00\nCurrent week (Fable)\n31% used\nResets Tuesday at 10:00"
	raw, err := json.Marshal(map[string]any{
		"type":               "result",
		"is_error":           false,
		"result":             result,
		"session_percentage": 99,
		"weekly_percentage":  99,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	usage, err := parseClaudeUsageEnvelope(raw)
	if err != nil {
		t.Fatalf("parseClaudeUsageEnvelope: %v", err)
	}
	if usage.Session.UsedPercent != 12.5 || usage.Session.ResetText != "Resets in 2 hours" {
		t.Fatalf("unexpected session usage: %+v", usage.Session)
	}
	if usage.WeeklyAll.UsedPercent != 80 || usage.WeeklyAll.ResetText != "Resets Monday at 09:00" {
		t.Fatalf("unexpected weekly usage: %+v", usage.WeeklyAll)
	}
	if usage.Fable == nil || usage.Fable.UsedPercent != 31 || usage.Fable.ResetText != "Resets Tuesday at 10:00" {
		t.Fatalf("unexpected Fable usage: %+v", usage.Fable)
	}
}

func TestParseClaudeUsageEnvelopeAllowsMissingFableSection(t *testing.T) {
	usage, err := parseClaudeUsageEnvelope(fakeClaudeUsageEnvelope(10, 20, nil))
	if err != nil {
		t.Fatalf("parse missing-Fable usage: %v", err)
	}
	if usage.Fable != nil {
		t.Fatalf("expected unavailable Fable window, got %+v", usage.Fable)
	}
	if !claudeUsageIsEligible(usage, false) {
		t.Fatal("missing Fable should not exclude non-Fable work")
	}
	if claudeUsageIsEligible(usage, true) {
		t.Fatal("missing Fable must exclude Fable work")
	}
}

func TestParseClaudeUsageEnvelopeRejectsMalformedAndErrorShapes(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		message string
	}{
		{"malformed JSON", `{`, "parse Claude usage JSON"},
		{"non-object", `[]`, "expected an object"},
		{"missing is_error", `{"result":"text"}`, "missing is_error"},
		{"non-boolean is_error", `{"is_error":"false","result":"text"}`, "must be a boolean"},
		{"reported error", `{"is_error":true,"result":"failed"}`, "reported an error"},
		{"missing result", `{"is_error":false}`, "missing result string"},
		{"non-string result", `{"is_error":false,"result":{"session":1}}`, "result must be a string"},
		{"empty result", `{"is_error":false,"result":"  "}`, "result string is empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseClaudeUsageEnvelope([]byte(test.raw))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q error, got %v", test.message, err)
			}
		})
	}
}

func TestParseClaudeUsageResultRejectsMissingOrInvalidRequiredWindows(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		message string
	}{
		{
			"missing session",
			"Current week (all models)\n20% used\nResets tomorrow",
			"missing session section",
		},
		{
			"missing weekly",
			"Current session\n20% used\nResets soon",
			"missing weekly all-model section",
		},
		{
			"weekly missing percentage",
			"Current session\n20% used\nCurrent week (all models)\nResets tomorrow",
			"missing weekly all-model percentage",
		},
		{
			"over one hundred",
			"Current session\n101% used\nResets soon\nCurrent week (all models)\n30% used\nResets tomorrow",
			"invalid percentage",
		},
		{
			"Fable missing percentage",
			"Current session\n10% used\nResets soon\nCurrent week (all models)\n20% used\nResets tomorrow\nCurrent week (Fable)",
			"missing Fable percentage",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseClaudeUsageResult(test.result)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q error, got %v", test.message, err)
			}
		})
	}
}

func TestParseClaudeUsageAllowsFableWithoutResetText(t *testing.T) {
	usage, err := parseClaudeUsageResult(
		"Current session: 0% used · resets later\n" +
			"Current week (all models): 0% used · resets Sunday\n" +
			"Current week (Fable): 0% used",
	)
	if err != nil {
		t.Fatalf("parse Fable usage without reset: %v", err)
	}
	if usage.Fable == nil || usage.Fable.UsedPercent != 0 || usage.Fable.ResetText != "" {
		t.Fatalf("unexpected Fable usage: %+v", usage.Fable)
	}
}

func TestParseClaudeUsageStopsBeforeActivityBreakdown(t *testing.T) {
	usage, err := parseClaudeUsageResult(
		"You are currently using your subscription to power your Claude Code usage\n\n" +
			"Current session: 20% used · resets Jul 20 at 4:20pm\n" +
			"Current week (all models): 24% used · resets Jul 25 at 4am\n" +
			"Current week (Fable): 48% used · resets Jul 25 at 4am\n\n" +
			"What's contributing to your limits usage?\n" +
			"Last 24h · 1719 requests\n" +
			"  100% of your usage came from subagent-heavy sessions\n" +
			"  Top skills: /dev-cycle 4%\n",
	)
	if err != nil {
		t.Fatalf("parse usage with activity breakdown: %v", err)
	}
	if usage.Session.UsedPercent != 20 || usage.WeeklyAll.UsedPercent != 24 ||
		usage.Fable == nil || usage.Fable.UsedPercent != 48 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestClaudeUsageEligibilityAndWorstPercentage(t *testing.T) {
	fable := claudeUsageWindow{UsedPercent: 70, ResetText: "Resets later"}
	usage := claudeUsage{
		Session:   claudeUsageWindow{UsedPercent: 35},
		WeeklyAll: claudeUsageWindow{UsedPercent: 60},
		Fable:     &fable,
	}
	if got := claudeUsageWorstPercent(usage, false); got != 60 {
		t.Fatalf("non-Fable score: got %v want 60", got)
	}
	if got := claudeUsageWorstPercent(usage, true); got != 70 {
		t.Fatalf("Fable score: got %v want 70", got)
	}
	usage.Session.UsedPercent = 100
	if claudeUsageIsEligible(usage, false) {
		t.Fatal("100% session usage must be ineligible")
	}
}
