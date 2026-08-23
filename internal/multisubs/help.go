package multisubs

import (
	"fmt"
	"sort"
	"strings"
)

type commandHelp struct {
	Usage       string
	Description string
	Examples    []string
}

var commandSummaries = []struct {
	Name    string
	Summary string
}{
	{Name: "init", Summary: "initialize shared multisubs state"},
	{Name: "install [ref]", Summary: "replace the running binary and persist GOBIN"},
	{Name: "doctor [flags]", Summary: "run aggregate shared, Codex, and Claude checks"},
	{Name: "status", Summary: "show quota and the next command when an account is not ready"},
	{Name: "usage", Summary: "show one quota snapshot for every routed account"},
	{Name: "codex <command>", Summary: "manage and route isolated Codex accounts"},
	{Name: "claude <command>", Summary: "manage and route isolated Claude accounts"},
	{Name: "completion <shell>", Summary: "print shell completion for bash, zsh, or fish"},
	{Name: "version", Summary: "print the multisubs version"},
	{Name: "help [topic]", Summary: "show global or topic-specific help"},
}

var codexCommandSummaries = []struct {
	Name    string
	Summary string
}{
	{Name: "init", Summary: "initialize shared multisubs and Codex profile state"},
	{Name: "add <name>", Summary: "add a named Codex account profile"},
	{Name: "login <name> [args...]", Summary: "log in through the official Codex flow"},
	{Name: "login-all", Summary: "run login for every known Codex profile"},
	{Name: "cli [<name>] [args...]", Summary: "run interactive Codex on the best available account"},
	{Name: "exec [args...]", Summary: "run codex exec on the best available account"},
	{Name: "generate [args...]", Summary: "generate one ChatGPT subscription reply, optionally with native web search"},
	{Name: "status", Summary: "show Codex profile authentication states"},
	{Name: "usage", Summary: "show Codex quota for every routed account"},
	{Name: "reconcile", Summary: "reconcile resources for all Codex profiles"},
	{Name: "monitor [args...]", Summary: "show live Codex subscription usage"},
	{Name: "doctor [flags]", Summary: "run focused, read-only Codex checks"},
	{Name: "dry-run [operation]", Summary: "preview Codex operations without changing state"},
	{Name: "help [command]", Summary: "show Codex namespace help"},
}

var commandHelpByName = map[string]commandHelp{
	"init": {
		Usage:       "multisubs init",
		Description: "Create shared multisubs state and the Codex profile registry. This does not change either default provider account.",
	},
	"install": {
		Usage:       "multisubs install [ref]",
		Description: "Replace the running multisubs binary with go install, using that binary's directory as GOBIN. Default ref is latest. After a successful install, persist GOPRIVATE and GOBIN in the login shell profile and delete leftover regular copies at the default Go bin path. Doctor never deletes leftovers. Raw go install output is discarded. Provider credentials are not changed.",
		Examples: []string{
			"multisubs install",
			"multisubs install latest",
			"multisubs install v0.1.0",
		},
	},
	"doctor": {
		Usage:       "multisubs doctor [--json] [--timeout 8s]",
		Description: "Run one read-only product check with shared/base, Codex, and Claude sections. Shared/base includes the running binary path and whether go install would replace that file. Doctor never deletes leftover binaries; run `multisubs install` to replace the running copy and remove leftovers.",
	},
	"status": {
		Usage:       "multisubs status",
		Description: "Show the same Codex and Claude quota snapshot as `multisubs usage`. When any account is unavailable, print a Next section with the exact command to run. Partial account failures exit 1.",
	},
	"usage": {
		Usage:       "multisubs usage",
		Description: "Show a Codex and Claude quota snapshot with validated full local account emails for managed profiles and both default accounts. When any account is unavailable, print a Next section with the exact command to run. The default Codex app-server fallback may write non-credential operational state. Partial account failures exit 1. JSON output is not available yet.",
	},
	"completion": {
		Usage:       "multisubs completion <bash|zsh|fish>",
		Description: "Print completion for both provider namespaces, their nested topics, and dynamic provider profile names.",
		Examples: []string{
			`eval "$(multisubs completion zsh)"`,
			`eval "$(multisubs completion bash)"`,
			"multisubs completion fish > ~/.config/fish/completions/multisubs.fish",
		},
	},
	"version": {
		Usage:       "multisubs version",
		Description: "Print the multisubs build version. A release tag wins. A go-install module version or short Git revision is used when the compile-time default would otherwise hide which binary is running. Status, usage, and doctor print the same string.",
	},
	"help": {
		Usage:       "multisubs help [topic]",
		Description: "Show global help or detailed help for a provider or command topic.",
		Examples: []string{
			"multisubs help codex exec",
			"multisubs help codex monitor doctor",
			"multisubs help claude usage",
		},
	},
	"codex": {
		Usage:       "multisubs codex <command> [args]",
		Description: "Manage isolated Codex profiles and route Codex work across the default account and managed profiles.",
	},
	"codex init": {
		Usage:       "multisubs codex init",
		Description: "Run the same shared product and Codex profile initialization path as `multisubs init`.",
	},
	"codex add": {
		Usage:       "multisubs codex add <name>",
		Description: "Create a named Codex profile with an isolated profile-local CODEX_HOME.",
	},
	"codex login": {
		Usage:       "multisubs codex login <name> [codex login args]",
		Description: "Run official `codex login` in one profile and enforce file-backed authentication isolation.",
	},
	"codex login-all": {
		Usage:       "multisubs codex login-all",
		Description: "Run login for all configured Codex profiles in sorted order.",
	},
	"codex cli": {
		Usage:       "multisubs codex cli [<name>|--account <name>] [codex args...]",
		Description: "Run the official interactive Codex CLI after selecting the default account or a managed profile with the same weekly-usage rules as `multisubs codex exec`. A leading profile name or `--account <name>` bypasses routing and launches that managed profile. Default login uses the same two bounded checks as exec.",
		Examples: []string{
			"multisubs codex cli",
			"multisubs codex cli -m gpt-5-codex-spark",
			"multisubs codex cli personal",
			"multisubs codex cli --account work -- \"check this repo\"",
		},
	},
	"codex exec": {
		Usage:       "multisubs codex exec [--search] [codex exec args]",
		Description: "Run `codex exec` after selecting the default account or a managed profile by weekly usage. `--search` is moved before `exec` because Codex defines it as a global flag. Default login gets two bounded checks. If neither confirms login, multisubs warns on stderr, excludes default, and selects once more from the remaining accounts.",
	},
	"codex generate": {
		Usage:       "multisubs codex generate [--search] [--account <name>] [-m|--model <model>] [--effort <effort>] [--base-instructions-file <path>] [--developer-instructions-file <path>] [--output-schema <path>] [--json] [prompt]",
		Description: "Send one text prompt through Codex App Server using ChatGPT subscription authentication. Automatic mode uses the same weekly routing as `multisubs codex exec`. `--account <name>` selects one managed profile. `--search` enables only native live web search; other tools stay off. Requires Codex CLI 0.147.0 or 0.148.0. Resource notices and errors go to stderr so generated text can stay on stdout.",
		Examples: []string{
			"multisubs codex generate \"Summarize this change.\"",
			"multisubs codex generate --account work --json \"Name three risks.\"",
			"multisubs codex generate --search \"What changed this week?\"",
		},
	},
	"codex status": {
		Usage:       "multisubs codex status",
		Description: "Show profile-local Codex login status for managed profiles and the default account. When a row is not logged in, print a Next section with the exact command to run.",
	},
	"codex usage": {
		Usage:       "multisubs codex usage",
		Description: "Show session, weekly, and reported model-specific Codex quota with validated full local account emails for managed profiles and the default account. When any account is unavailable, print a Next section with the exact command to run. This snapshot does not change weekly-only routing.",
	},
	"codex reconcile": {
		Usage:       "multisubs codex reconcile",
		Description: "Apply configured guidance and skill links to every Codex profile without reading credentials or launching Codex.",
	},
	"codex monitor": {
		Usage:       "multisubs codex monitor",
		Description: "Run the Codex subscription-usage terminal interface.",
	},
	"codex monitor doctor": {
		Usage:       "multisubs codex monitor doctor [--json] [--timeout 60s] [--include-default] [--include-active] [--discover] [--app-server]",
		Description: "Run read-only checks against configured Codex usage sources.",
	},
	"codex monitor completion": {
		Usage:       "multisubs codex monitor completion [bash|zsh|fish]",
		Description: "Print the full multisubs completion script from the Codex monitor namespace.",
	},
	"codex doctor": {
		Usage:       "multisubs codex doctor [--json] [--timeout 8s]",
		Description: "Run focused, non-mutating Codex binary, profile, config, and authentication checks.",
	},
	"codex dry-run": {
		Usage:       "multisubs codex dry-run [operation]",
		Description: "Preview Codex commands and filesystem work without making changes.",
	},
	"codex help": {
		Usage:       "multisubs codex help [command]",
		Description: "Show Codex namespace or command help without creating product state.",
	},
	"codex monitor help": {
		Usage:       "multisubs codex monitor help",
		Description: "Show detailed Codex monitor commands and flags without creating product state.",
	},
	"claude": {
		Usage:       "multisubs claude <command> [args]",
		Description: "Manage isolated Claude profiles and route Claude print-mode work across the default account and managed profiles.",
	},
	"claude add": {
		Usage:       "multisubs claude add <name>",
		Description: "Create a managed Claude profile with a private, derived CLAUDE_CONFIG_DIR.",
	},
	"claude login": {
		Usage:       "multisubs claude login <name> [claude auth login args]",
		Description: "Run the official Claude.ai login flow for one managed profile without reading or copying credentials.",
	},
	"claude cli": {
		Usage:       "multisubs claude cli <name|default> [claude args...]",
		Description: "Run the official interactive Claude CLI with a managed profile or the default account.",
	},
	"claude exec": {
		Usage:       "multisubs claude exec [claude -p args...]",
		Description: "Run official `claude -p` after fresh target-scoped session, weekly all-model, and Fable usage checks.",
	},
	"claude status": {
		Usage:       "multisubs claude status",
		Description: "Show official authentication status for the default Claude account and every managed profile. When a target is not logged in, print a Next section with the exact command to run.",
	},
	"claude usage": {
		Usage:       "multisubs claude usage",
		Description: "Show fresh session, weekly all-model, and optional Fable quota with validated full local account emails for every managed profile and the default account through the shared usage report. When any account is unavailable, print a Next section with the exact command to run.",
	},
	"claude doctor": {
		Usage:       "multisubs claude doctor",
		Description: "Run focused, read-only Claude binary, sidecar, path, and authentication checks.",
	},
	"claude help": {
		Usage:       "multisubs claude help [command]",
		Description: "Show Claude namespace or command help without creating product state.",
	},
}

func printHelp() {
	fmt.Println("multisubs")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multisubs <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, item := range commandSummaries {
		fmt.Printf("  %-26s %s\n", item.Name, item.Summary)
	}
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  multisubs init")
	fmt.Println("  multisubs install")
	fmt.Println("  multisubs status")
	fmt.Println("  multisubs usage")
	fmt.Println("  multisubs codex exec -s read-only \"Summarize this repository.\"")
	fmt.Println("  multisubs claude status")
	fmt.Println("  multisubs doctor")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - Codex commands live under `multisubs codex`.")
	fmt.Println("  - Claude commands live under `multisubs claude`.")
	fmt.Println("  - Status and usage print the next command when an account is not ready.")
	fmt.Println("  - Usage snapshots do not change credentials; the default Codex app-server fallback may write non-credential operational state. JSON output is not available yet.")
}

func printCodexHelp() {
	fmt.Println("multisubs codex")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multisubs codex <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, item := range codexCommandSummaries {
		fmt.Printf("  %-34s %s\n", item.Name, item.Summary)
	}
}

func printClaudeHelp() {
	fmt.Println("multisubs claude")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multisubs claude <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, item := range []struct {
		name    string
		summary string
	}{
		{"add <name>", "add a managed Claude profile"},
		{"login <name> [args...]", "run the official Claude.ai login flow"},
		{"cli <name|default> [args...]", "run the official interactive Claude CLI"},
		{"exec [args...]", "route official Claude print mode by fresh usage"},
		{"status", "show auth status for default and managed profiles"},
		{"usage", "show session, weekly, and Fable quota"},
		{"doctor", "run focused, read-only Claude checks"},
		{"help [command]", "show Claude namespace help"},
	} {
		fmt.Printf("  %-37s %s\n", item.name, item.summary)
	}
	fmt.Println()
	fmt.Println("The default target runs with CLAUDE_CONFIG_DIR absent.")
}

func printClaudeCommandHelp(command string) error {
	return printCommandHelp([]string{"claude", command})
}

func printCommandHelp(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	if len(args) > 3 {
		return &ExitError{Code: 2, Message: "usage: multisubs help [topic]"}
	}
	name := normalizeHelpTopic(strings.Join(args, " "))
	topic, ok := commandHelpByName[name]
	if !ok {
		known := make([]string, 0, len(commandHelpByName))
		for key := range commandHelpByName {
			known = append(known, key)
		}
		sort.Strings(known)
		return &ExitError{
			Code:    2,
			Message: fmt.Sprintf("unknown help topic: %s\nknown topics: %s", strings.Join(args, " "), strings.Join(known, ", ")),
		}
	}

	fmt.Println("multisubs", name)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s\n", topic.Usage)
	fmt.Println()
	fmt.Println("Description:")
	fmt.Printf("  %s\n", topic.Description)
	if len(topic.Examples) > 0 {
		fmt.Println()
		fmt.Println("Examples:")
		for _, example := range topic.Examples {
			fmt.Printf("  %s\n", example)
		}
	}
	return nil
}

func (a *App) cmdHelp(args []string) error {
	return printCommandHelp(args)
}

func normalizeHelpTopic(topic string) string {
	topic = strings.TrimSpace(strings.ToLower(topic))
	switch topic {
	case "--help", "-h":
		return "help"
	case "--version", "-v":
		return "version"
	default:
		return topic
	}
}
