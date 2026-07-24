package codexstate

import (
	"strings"
	"testing"
)

func TestSanitizedEnv(t *testing.T) {
	t.Parallel()

	env := SanitizedEnv([]string{
		"PATH=/bin",
		"CODEX_HOME=/stale",
		"MULTISUBS_HOME=/active-product-state",
		"MULTISUBS_ACTIVE_PROFILE=stale",
		"MULTISUBS_FUTURE_CONTROL=stale",
		"MULTICODEX_HOME=/legacy-product-state",
		"MULTICODEX_ACTIVE_PROFILE=legacy",
		"CODEX_USAGE_MONITOR_ACCOUNTS_FILE=/legacy-accounts.json",
		"OPENAI_API_KEY=placeholder",
		"CODEX_AUTH_TOKEN=placeholder",
		"INVALID_ENTRY",
	}, "/isolated")
	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"CODEX_HOME=/stale", "MULTISUBS_HOME=", "MULTISUBS_ACTIVE_PROFILE=", "MULTISUBS_FUTURE_CONTROL=", "MULTICODEX_HOME=", "MULTICODEX_ACTIVE_PROFILE=", "CODEX_USAGE_MONITOR_ACCOUNTS_FILE=", "OPENAI_API_KEY=", "CODEX_AUTH_TOKEN=", "INVALID_ENTRY"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("SanitizedEnv retained %q in %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "CODEX_HOME=/isolated") {
		t.Fatalf("SanitizedEnv removed required entries: %q", joined)
	}
}
