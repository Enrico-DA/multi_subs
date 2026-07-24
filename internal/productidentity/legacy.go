package productidentity

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	legacyProduct           = "multi" + "codex"
	legacyEnvironmentPrefix = "MULTI" + "CODEX"
)

type legacyCategory string

const (
	legacyAttribution    legacyCategory = "attribution"
	legacyUpstreamMap    legacyCategory = "upstream-map"
	legacyRejection      legacyCategory = "startup-rejection"
	legacyDenylist       legacyCategory = "child-environment-denylist"
	legacyIgnore         legacyCategory = "ignore"
	legacyLeakProtection legacyCategory = "legacy-state-leak-protection"
	legacyNegativeTest   legacyCategory = "explicit-negative-test"
)

type legacyRule struct {
	path     string
	line     string
	category legacyCategory
	count    int
}

func requiredLegacyLine(path string, category legacyCategory, template string) legacyRule {
	return legacyRule{
		path:     path,
		line:     expandLegacyTemplate(template),
		category: category,
		count:    1,
	}
}

func expandLegacyTemplate(template string) string {
	expanded := strings.ReplaceAll(template, "{legacy}", legacyProduct)
	return strings.ReplaceAll(expanded, "{LEGACY}", legacyEnvironmentPrefix)
}

var requiredLegacyLines = []legacyRule{
	requiredLegacyLine(".gitignore", legacyIgnore, ".{legacy}/"),
	requiredLegacyLine(".gitignore", legacyIgnore, "**/.{legacy}/config.json"),
	requiredLegacyLine(".gitignore", legacyIgnore, "**/.{legacy}/profiles/"),
	requiredLegacyLine(".gitignore", legacyIgnore, "**/{legacy}/config.json"),
	requiredLegacyLine(".gitignore", legacyIgnore, "**/{legacy}/profiles/"),
	requiredLegacyLine(".gitignore", legacyIgnore, "**/{legacy}/providers/claude/"),
	requiredLegacyLine(".gitignore", legacyIgnore, "/{legacy}"),

	requiredLegacyLine("AGENTS.md", legacyAttribution, "- The upstream project is `olliecrow/{legacy}`; preserve its attribution and license."),
	requiredLegacyLine("AGENTS.md", legacyAttribution, "- A read-only upstream remote may reference `github.com/olliecrow/{legacy}`."),
	requiredLegacyLine("CONTRIBUTING.md", legacyUpstreamMap, "Pull-request CI runs a non-publishing identity check. When syncing from `olliecrow/{legacy}`, apply the translation map in `docs/upstream-sync.md` and preserve upstream attribution and license text."),

	requiredLegacyLine("README.md", legacyRejection, "Any legacy `{LEGACY}_*` variable causes startup to fail before state access. Clear it before running `multisubs`. Runtime never reads the old product home or old environment namespace. All legacy `{LEGACY}_*` controls are still removed from provider child environments as a denylist."),
	requiredLegacyLine("README.md", legacyDenylist, "- Provider child environments remove credential overrides, inherited active product controls, and all legacy `{LEGACY}_*` controls. Multisubs adds only `MULTISUBS_ACTIVE_PROFILE` to managed Codex children for the selected profile; default-account Codex, neutral Codex help, and Claude children do not receive it."),
	requiredLegacyLine("README.md", legacyLeakProtection, "Filesystem monitor discovery prunes both `~/{legacy}` and `~/.{legacy}`, including canonical targets reached through aliases."),
	requiredLegacyLine("README.md", legacyAttribution, "This fork is based on [olliecrow/{legacy}](https://github.com/olliecrow/{legacy}) and preserves its attribution and license."),

	requiredLegacyLine("docs/README.md", legacyUpstreamMap, "- [`upstream-sync.md`](upstream-sync.md) records the durable identity and command translation from `olliecrow/{legacy}`."),
	requiredLegacyLine("docs/command-spec.md", legacyRejection, "Startup checks the environment before path resolution. If any `{LEGACY}_*` variable is present, the command exits with code 2 and tells the user to clear it."),
	requiredLegacyLine("docs/command-spec.md", legacyDenylist, "Runtime never reads the old environment namespace or the old `~/{legacy}` state root. All old `{LEGACY}_*` variables remain on provider child-environment denylists to prevent account-routing leakage."),
	requiredLegacyLine("docs/command-spec.md", legacyLeakProtection, "Monitor discovery also prunes `~/.{legacy}` and the canonical alias targets of both legacy roots before descent."),

	requiredLegacyLine("docs/decisions.md", legacyAttribution, "Decision: Preserve the Apache 2.0 license terms and attribution to `olliecrow/{legacy}`."),
	requiredLegacyLine("docs/decisions.md", legacyRejection, "Decision: Old `{LEGACY}_*` variables cause startup to fail before state access."),
	requiredLegacyLine("docs/decisions.md", legacyLeakProtection, "Enforcement: Top-level startup rejects any old product-prefixed variable. Provider child environments still strip old controls. The old `~/{legacy}` and `.{legacy}` patterns remain only as legacy-sensitive ignore and leak protection."),

	requiredLegacyLine("docs/security-and-privacy.md", legacyDenylist, "- all legacy `{LEGACY}_*` controls."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyRejection, "- Any `{LEGACY}_*` variable rejects top-level startup before state access."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyLeakProtection, "- Runtime never reads `{LEGACY}_HOME`."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyLeakProtection, "- Runtime never defaults to `~/{legacy}`."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyLeakProtection, "- Monitor discovery prunes `~/{legacy}`, `~/.{legacy}`, and candidates canonically inside either root before reading usage signals."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyLeakProtection, "- `.{legacy}`, `{legacy}` state paths, and old environment names remain only in ignore, leak, denylist, and rejection tests so old credentials cannot be committed or inherited."),
	requiredLegacyLine("docs/security-and-privacy.md", legacyAttribution, "Tests and examples use synthetic values and dummy paths. Upstream attribution to `olliecrow/{legacy}` is not a runtime compatibility reference."),

	requiredLegacyLine("docs/upstream-sync.md", legacyAttribution, "This fork keeps `olliecrow/{legacy}` as its read-only upstream source and preserves Ollie's attribution and license. Fork work is pushed only to `Enrico-DA/multi_subs`."),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `olliecrow/{legacy}` | `Enrico-DA/multi_subs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `github.com/olliecrow/{legacy}` | `github.com/Enrico-DA/multi_subs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `{legacy}` executable and product text | `multisubs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `cmd/{legacy}` | `cmd/multisubs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `internal/{legacy}` package and directory | `internal/multisubs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `package {legacy}` | `package multisubs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `~/{legacy}` active state | `~/multisubs` |"),
	requiredLegacyLine("docs/upstream-sync.md", legacyUpstreamMap, "| `{LEGACY}_*` active controls | `MULTISUBS_*` |"),

	requiredLegacyLine("internal/codexstate/env.go", legacyDenylist, "\tif strings.HasPrefix(key, \"{LEGACY}_\") {"),
	requiredLegacyLine("internal/codexstate/env_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_HOME=/legacy-product-state\","),
	requiredLegacyLine("internal/codexstate/env_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_ACTIVE_PROFILE=legacy\","),
	requiredLegacyLine("internal/codexstate/env_test.go", legacyNegativeTest, "\tfor _, forbidden := range []string{\"CODEX_HOME=/stale\", \"MULTISUBS_HOME=\", \"MULTISUBS_ACTIVE_PROFILE=\", \"{LEGACY}_HOME=\", \"{LEGACY}_ACTIVE_PROFILE=\", \"CODEX_USAGE_MONITOR_ACCOUNTS_FILE=\", \"OPENAI_API_KEY=\", \"CODEX_AUTH_TOKEN=\", \"INVALID_ENTRY\"} {"),

	requiredLegacyLine("internal/monitor/usage/accounts.go", legacyLeakProtection, "\tfor _, name := range []string{\"{legacy}\", \".{legacy}\"} {"),
	requiredLegacyLine("internal/monitor/usage/accounts_test.go", legacyNegativeTest, "\tfor _, rootName := range []string{\"{legacy}\", \".{legacy}\"} {"),

	requiredLegacyLine("internal/multisubs/app.go", legacyRejection, "\t\tif ok && strings.HasPrefix(name, \"{LEGACY}_\") {"),
	requiredLegacyLine("internal/multisubs/app.go", legacyRejection, "\t\tMessage: fmt.Sprintf(\"legacy {LEGACY}_* environment variable(s) are set: %s; clear them before running multisubs\", strings.Join(names, \", \")),"),
	requiredLegacyLine("internal/multisubs/claude_process.go", legacyDenylist, "\tif strings.HasPrefix(key, \"{LEGACY}_\") {"),
	requiredLegacyLine("internal/multisubs/claude_process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_HOME=/tmp/legacy\","),
	requiredLegacyLine("internal/multisubs/claude_process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_CLAUDE_PROFILE=legacy\","),
	requiredLegacyLine("internal/multisubs/claude_process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_HOME\","),
	requiredLegacyLine("internal/multisubs/claude_process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_CLAUDE_PROFILE\","),
	requiredLegacyLine("internal/multisubs/cli_test.go", legacyNegativeTest, "\t\t\t\tt.Setenv(\"{LEGACY}_HOME\", filepath.Join(root, \"legacy-product-state\"))"),
	requiredLegacyLine("internal/multisubs/cli_test.go", legacyNegativeTest, "\t\t\t\tt.Setenv(\"{LEGACY}_ACTIVE_PROFILE\", \"legacy\")"),
	requiredLegacyLine("internal/multisubs/cli_test.go", legacyNegativeTest, "\t\t\t\tt.Setenv(\"{LEGACY}_UNKNOWN_CONTROL\", \"legacy\")"),
	requiredLegacyLine("internal/multisubs/cli_test.go", legacyNegativeTest, "\t\tforbidden = append(forbidden, \"{LEGACY}_HOME\", \"{LEGACY}_ACTIVE_PROFILE\", \"{LEGACY}_UNKNOWN_CONTROL\")"),

	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\".{legacy}/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"**/{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"**/{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"**/{legacy}/providers/claude/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"**/.{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"**/.{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\".{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/olliecrow/{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/olliecrow/.{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/enrico-da/{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/enrico-da/.{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/{legacy}/config.json\") ||"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/.{legacy}/config.json\") {"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\".{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/olliecrow/{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/olliecrow/.{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/enrico-da/{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\t\"github.com/enrico-da/.{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/{legacy}/profiles/\") ||"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/.{legacy}/profiles/\") {"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/.{legacy}/providers/claude/\") ||"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.HasPrefix(clean, \".{legacy}/providers/claude/\") ||"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.Contains(clean, \"/{legacy}/providers/claude/\") ||"),
	requiredLegacyLine("internal/multisubs/doctor.go", legacyLeakProtection, "\t\tstrings.HasPrefix(clean, \"{legacy}/providers/claude/\") {"),

	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\"**/{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\"**/{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\"**/{legacy}/providers/claude/\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\"**/.{legacy}/config.json\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\"**/.{legacy}/profiles/\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t\".{legacy}/\","),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\tfor _, want := range []string{\".multisubs/\", \".{legacy}/\", \"**/multisubs/config.json\", \"**/{legacy}/config.json\", \"**/auth.json\"} {"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\tfor _, want := range []string{\".{legacy}/\", \"**/{legacy}/config.json\", \"**/{legacy}/profiles/\"} {"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \"github.com/olliecrow/{legacy}/config.json\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \"github.com/olliecrow/{legacy}/profiles/work/codex-home/config.toml\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \".{legacy}/config.json\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \".{legacy}/profiles/work/codex-home/config.toml\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \".{legacy}/providers/claude/config.json\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \"workspace/.{legacy}/providers/claude/config.json\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \"github.com/olliecrow/.{legacy}/config.json\", sensitive: true},"),
	requiredLegacyLine("internal/multisubs/doctor_test.go", legacyNegativeTest, "\t\t{path: \"github.com/olliecrow/{legacy}/docs/readme.md\", sensitive: false},"),

	requiredLegacyLine("internal/multisubs/help_completion_test.go", legacyNegativeTest, "\tif strings.Contains(out, \"{legacy}\") {"),
	requiredLegacyLine("internal/multisubs/help_completion_test.go", legacyNegativeTest, "\tif !strings.HasPrefix(out, \"multisubs \") || strings.Contains(out, \"{legacy}\") {"),
	requiredLegacyLine("internal/multisubs/help_completion_test.go", legacyNegativeTest, "\t\t\tif strings.Contains(test.out, \"{legacy}\") || strings.Contains(test.out, \"__complete-profiles\") {"),

	requiredLegacyLine("internal/multisubs/paths_test.go", legacyNegativeTest, "\tlegacyHome := filepath.Join(t.TempDir(), \"{legacy}\")"),
	requiredLegacyLine("internal/multisubs/paths_test.go", legacyNegativeTest, "\tt.Setenv(\"{LEGACY}_HOME\", legacyHome)"),
	requiredLegacyLine("internal/multisubs/paths_test.go", legacyNegativeTest, "\tt.Setenv(\"{LEGACY}_DEFAULT_CODEX_HOME\", filepath.Join(t.TempDir(), \"legacy-codex\"))"),
	requiredLegacyLine("internal/multisubs/process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_HOME=/tmp/legacy\","),
	requiredLegacyLine("internal/multisubs/process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_ACTIVE_PROFILE=legacy\","),
	requiredLegacyLine("internal/multisubs/process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_HOME=\","),
	requiredLegacyLine("internal/multisubs/process_test.go", legacyNegativeTest, "\t\t\"{LEGACY}_ACTIVE_PROFILE=\","),
	requiredLegacyLine("internal/multisubs/run_cli_test.go", legacyNegativeTest, "\tlegacyHome := filepath.Join(home, \"{legacy}\")"),
	requiredLegacyLine("internal/multisubs/run_cli_test.go", legacyNegativeTest, "\tt.Setenv(\"{LEGACY}_HOME\", legacyHome)"),
}

func (repo *repository) checkLegacyOccurrences() {
	type ruleKey struct {
		path string
		line string
	}

	ruleByKey := make(map[ruleKey]int, len(requiredLegacyLines))
	counts := make([]int, len(requiredLegacyLines))
	for index, rule := range requiredLegacyLines {
		key := ruleKey{path: rule.path, line: rule.line}
		if _, duplicate := ruleByKey[key]; duplicate {
			repo.errors = append(repo.errors, fmt.Sprintf("%s: duplicate reviewed legacy occurrence rule", rule.path))
			continue
		}
		ruleByKey[key] = index
	}

	legacyBytes := []byte(strings.ToLower(legacyProduct))
	for _, relativePath := range repo.trackedPaths {
		data, ok := repo.readFile(relativePath)
		if !ok || !bytes.Contains(bytes.ToLower(data), legacyBytes) {
			continue
		}
		if !utf8.Valid(data) {
			repo.errors = append(repo.errors, fmt.Sprintf("%s: cannot decode tracked legacy identity occurrence as UTF-8", relativePath))
			continue
		}
		for lineNumber, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(strings.ToLower(line), legacyProduct) {
				continue
			}
			index, approved := ruleByKey[ruleKey{path: relativePath, line: line}]
			if !approved {
				repo.errors = append(repo.errors, fmt.Sprintf("%s:%d: legacy product text is outside the exact reviewed occurrence rules", relativePath, lineNumber+1))
				continue
			}
			counts[index]++
		}
	}

	for index, rule := range requiredLegacyLines {
		if counts[index] != rule.count {
			repo.errors = append(repo.errors, fmt.Sprintf("%s: required %s legacy occurrence %q must appear exactly %d time(s); found %d", rule.path, rule.category, rule.line, rule.count, counts[index]))
		}
	}
}
