package codexstate

import "strings"

// SanitizedEnv removes inherited profile and account overrides before an
// isolated Codex process is started. When codexHome is non-empty, it becomes
// the process's sole CODEX_HOME.
func SanitizedEnv(base []string, codexHome string) []string {
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || environmentVariableIsAccountScoped(key) {
			continue
		}
		env = append(env, entry)
	}
	if codexHome != "" {
		env = append(env, "CODEX_HOME="+codexHome)
	}
	return env
}

func environmentVariableIsAccountScoped(key string) bool {
	if strings.HasPrefix(key, "MULTICODEX_") {
		return true
	}
	switch key {
	case "CODEX_HOME",
		"MULTISUBS_HOME",
		"MULTISUBS_DEFAULT_CODEX_HOME",
		"MULTISUBS_ACTIVE_PROFILE",
		"MULTISUBS_SELECTED_PROFILE_PATH",
		"MULTISUBS_HEARTBEAT_LOCK_PATH",
		"MULTISUBS_HEARTBEAT_PROMPT",
		"MULTISUBS_HEARTBEAT_TIMEOUT_SECONDS",
		"MULTISUBS_HEARTBEAT_RETRIES",
		"MULTISUBS_HEARTBEAT_BACKOFF_SECONDS",
		"MULTISUBS_MONITOR_ACCOUNTS_FILE",
		"CODEX_USAGE_MONITOR_ACCOUNTS_FILE",
		"MULTISUBS_CLAUDE_PROFILE",
		"MULTISUBS_CLAUDE_CONFIG_DIR",
		"MULTISUBS_CLAUDE_TARGET",
		"MULTISUBS_CLAUDE_ACTIVE_PROFILE",
		"MULTISUBS_CLAUDE_SELECTED_PROFILE",
		"MULTISUBS_ACTIVE_CLAUDE_PROFILE",
		"MULTISUBS_SELECTED_CLAUDE_PROFILE",
		"MULTISUBS_ACTIVE_PROVIDER",
		"MULTISUBS_SELECTED_PROVIDER",
		"OPENAI_API_KEY",
		"OPENAI_ORG_ID",
		"OPENAI_ORGANIZATION",
		"OPENAI_PROJECT",
		"OPENAI_BASE_URL",
		"OPENAI_API_BASE",
		"OPENAI_HOST",
		"CODEX_API_KEY",
		"CODEX_AUTH_TOKEN",
		"CODEX_ACCESS_TOKEN",
		"CODEX_REFRESH_TOKEN",
		"CODEX_TOKEN",
		"CODEX_BASE_URL",
		"CODEX_API_BASE":
		return true
	default:
		return false
	}
}
