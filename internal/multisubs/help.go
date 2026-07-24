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
	{Name: "doctor [flags]", Summary: "run aggregate shared, Codex, and Claude checks"},
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
	{Name: "cli <name> [args...]", Summary: "run the interactive Codex CLI with one profile"},
	{Name: "exec [args...]", Summary: "run codex exec on the best available account"},
	{Name: "status", Summary: "show Codex profile authentication states"},
	{Name: "usage", Summary: "show Codex quota for every routed account"},
	{Name: "reconcile", Summary: "reconcile resources for all Codex profiles"},
	{Name: "heartbeat", Summary: "send a small keepalive for logged-in Codex profiles"},
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
	"doctor": {
		Usage:       "multisubs doctor [--json] [--timeout 8s]",
		Description: "Run one read-only product check with shared/base, Codex, and Claude sections.",
	},
	"usage": {
		Usage:       "multisubs usage",
		Description: "Show a read-only Codex and Claude quota snapshot for managed profiles and both default accounts. Partial account failures exit 1. JSON output is not available yet.",
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
		Description: "Print the multisubs build version.",
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
		Usage:       "multisubs codex cli <name> [codex args...]",
		Description: "Run the official interactive Codex CLI with one profile-local CODEX_HOME.",
	},
	"codex exec": {
		Usage:       "multisubs codex exec [codex exec args]",
		Description: "Run `codex exec` after selecting the default account or a managed profile by weekly usage.",
	},
	"codex status": {
		Usage:       "multisubs codex status",
		Description: "Show profile-local Codex login status and safe account hints.",
	},
	"codex usage": {
		Usage:       "multisubs codex usage",
		Description: "Show session, weekly, and reported model-specific Codex quota for managed profiles and the default account. This snapshot does not change weekly-only routing.",
	},
	"codex reconcile": {
		Usage:       "multisubs codex reconcile",
		Description: "Apply configured guidance and skill links to every Codex profile without reading credentials or launching Codex.",
	},
	"codex heartbeat": {
		Usage:       "multisubs codex heartbeat",
		Description: "Send an ephemeral, read-only keepalive to every logged-in Codex profile with bounded retry and locking.",
	},
	"codex monitor": {
		Usage:       "multisubs codex monitor [--interval 60s] [--timeout 60s] [--no-color] [--no-alt-screen] [--include-default] [--include-active] [--discover]",
		Description: "Run the Codex subscription-usage terminal interface.",
	},
	"codex monitor doctor": {
		Usage:       "multisubs codex monitor doctor [--json] [--timeout 60s] [--include-default] [--include-active] [--discover] [--app-server]",
		Description: "Run read-only checks against configured Codex usage sources.",
	},
	"codex monitor tui": {
		Usage:       "multisubs codex monitor tui [flags]",
		Description: "Run the Codex usage terminal interface explicitly.",
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
		Description: "Show official authentication status for the default Claude account and every managed profile.",
	},
	"claude usage": {
		Usage:       "multisubs claude usage",
		Description: "Show fresh session, weekly all-model, and optional Fable quota for every managed profile and the default account through the shared usage report.",
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
	fmt.Println("  multisubs codex status")
	fmt.Println("  multisubs usage")
	fmt.Println("  multisubs codex exec -s read-only \"Summarize this repository.\"")
	fmt.Println("  multisubs claude status")
	fmt.Println("  multisubs doctor")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - Codex commands live under `multisubs codex`.")
	fmt.Println("  - Claude commands live under `multisubs claude`.")
	fmt.Println("  - Usage snapshots are read-only; JSON output is not available yet.")
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
