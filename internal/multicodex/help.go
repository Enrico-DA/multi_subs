package multicodex

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
	{Name: "claude <command>", Summary: "manage and route isolated Claude accounts"},
	{Name: "init", Summary: "initialize multicodex local state"},
	{Name: "add <name>", Summary: "add a named account profile"},
	{Name: "login <name> [codex login args]", Summary: "login profile using official codex flow"},
	{Name: "login-all", Summary: "run login for every known profile"},
	{Name: "cli <name> [codex args...]", Summary: "run the interactive Codex CLI with one profile"},
	{Name: "exec [codex exec args]", Summary: "run codex exec on the best available account"},
	{Name: "status", Summary: "show all profile auth states"},
	{Name: "reconcile", Summary: "reconcile resources for all profiles"},
	{Name: "heartbeat", Summary: "send a minimal keepalive hello for logged-in profiles"},
	{Name: "monitor [flags]", Summary: "show live subscription usage across accounts"},
	{Name: "monitor tui [flags]", Summary: "run the monitor terminal UI explicitly"},
	{Name: "monitor doctor [flags]", Summary: "check usage-monitor data sources"},
	{Name: "monitor completion [shell]", Summary: "print shell completion script"},
	{Name: "doctor [--json] [--timeout 8s]", Summary: "run non-mutating setup and auth checks"},
	{Name: "dry-run [operation]", Summary: "print planned operations without mutating state"},
	{Name: "completion <shell>", Summary: "print shell completion script for bash, zsh, or fish"},
	{Name: "version", Summary: "print multicodex version"},
	{Name: "help [command [subcommand]]", Summary: "show global or command-specific help"},
}

var commandHelpByName = map[string]commandHelp{
	"claude": {
		Usage:       "multicodex claude <command> [args]",
		Description: "Manage isolated Claude profiles and route Claude print-mode work while keeping the normal Claude account as a protected reserve.",
		Examples: []string{
			"multicodex claude add work",
			"multicodex claude status",
			`multicodex claude exec "Summarize this repository."`,
		},
	},
	"claude add": {
		Usage:       "multicodex claude add <name>",
		Description: "Create a managed Claude profile with a private, derived CLAUDE_CONFIG_DIR and register it in the Claude v1 sidecar.",
	},
	"claude login": {
		Usage:       "multicodex claude login <name> [claude auth login args]",
		Description: "Run official `claude auth login --claudeai` for one managed profile. Extra arguments such as `--email` pass through. Multicodex does not read or copy Claude credentials.",
	},
	"claude cli": {
		Usage:       "multicodex claude cli <name|default> [claude args...]",
		Description: "Run the official interactive Claude CLI with the chosen account. The built-in default target runs with CLAUDE_CONFIG_DIR absent.",
	},
	"claude exec": {
		Usage:       "multicodex claude exec [claude -p args...]",
		Description: "Run official `claude -p` after fresh profile-scoped usage checks. Managed profiles rank by their worst applicable usage percentage; default is used only when no managed profile is quota-eligible.",
	},
	"claude status": {
		Usage:       "multicodex claude status",
		Description: "Call official `claude auth status --json` for the default account and every managed profile, then show identity and authentication state.",
	},
	"claude usage": {
		Usage:       "multicodex claude usage",
		Description: "Call the free profile-scoped `/usage` command and report session, weekly all-model, and Fable limits and reset text.",
	},
	"claude doctor": {
		Usage:       "multicodex claude doctor",
		Description: "Run read-only Claude binary, sidecar, profile-path, and authentication checks.",
	},
	"init": {
		Usage:       "multicodex init",
		Description: "Create local multicodex metadata directories and config. This does not change your default Codex session.",
		Examples: []string{
			"multicodex init",
		},
	},
	"add": {
		Usage:       "multicodex add <name>",
		Description: "Create a named profile with an isolated profile CODEX_HOME.",
		Examples: []string{
			"multicodex add personal",
			"multicodex add work",
		},
	},
	"login": {
		Usage:       "multicodex login <name> [codex login args]",
		Description: "Run official codex login inside the selected profile context. User arguments keep their order, followed by the enforced file-backed-auth config override.",
		Examples: []string{
			"multicodex login personal",
			"multicodex login personal --device-auth",
		},
	},
	"login-all": {
		Usage:       "multicodex login-all",
		Description: "Run login for all configured profiles in sorted order and show per-profile outcomes.",
		Examples: []string{
			"multicodex login-all",
		},
	},
	"cli": {
		Usage:       "multicodex cli <name> [codex args...]",
		Description: "Run the interactive Codex CLI with the selected profile. Codex defaults such as model, reasoning, approvals, and sandbox come from the shared Codex config unless you pass explicit Codex args. Multicodex adds only the enforced file-backed-auth config override.",
		Examples: []string{
			"multicodex cli personal",
			`multicodex cli work "check this repo"`,
		},
	},
	"exec": {
		Usage:       "multicodex exec [codex exec args]",
		Description: "Run `codex exec` after automatically selecting the best available account. A configured-profile child receives the enforced file-backed-auth config override; default-reserve exec and exact help do not. Configured profiles are considered before the protected default reserve account. Profiles at 100% weekly usage are skipped, and known weekly resets are tried soonest first. The default Codex home is used only when no configured profile has usable weekly usage left, and remains the final fallback when it is the only destination.",
		Examples: []string{
			`multicodex exec -s read-only "Summarize the README in 3 bullets."`,
			"multicodex exec --skip-git-repo-check -C /path/to/repo \"Review the latest diff.\"",
		},
	},
	"status": {
		Usage:       "multicodex status",
		Description: "Show profile-local login status and account hints.",
		Examples: []string{
			"multicodex status",
		},
	},
	"reconcile": {
		Usage:       "multicodex reconcile",
		Description: "Reconcile configured guidance and skill resources for every profile. This does not inspect auth, launch Codex, or change the default Codex home.",
		Examples: []string{
			"multicodex reconcile",
		},
	},
	"heartbeat": {
		Usage:       "multicodex heartbeat",
		Description: "Fire-and-forget ephemeral keepalive across logged-in profiles with cron-safe locking, retry/backoff, and per-profile summary output. Heartbeat requests do not persist Codex session files.",
		Examples: []string{
			"multicodex heartbeat",
		},
	},
	"monitor": {
		Usage:       "multicodex monitor [--interval 60s] [--timeout 60s] [--no-color] [--no-alt-screen] [--include-default] [--include-active] [--discover]",
		Description: "Run the live subscription-usage terminal UI. By default, the monitor reads the global Codex home, explicit monitor account overrides, and configured multicodex profiles. Active CODEX_HOME and filesystem discovery are opt-in. If one refresh loses official window data for every account, the last good official window cards stay visible and are marked stale.",
		Examples: []string{
			"multicodex monitor",
			"multicodex monitor --interval 30s",
			"multicodex monitor doctor",
		},
	},
	"monitor doctor": {
		Usage:       "multicodex monitor doctor [--json] [--timeout 60s] [--include-default] [--include-active] [--discover] [--app-server]",
		Description: "Run read-only monitor checks against the global Codex home, explicit monitor account overrides, and configured multicodex profiles. Validated profile homes use app-server first with direct OAuth fallback; other homes use direct OAuth unless they dedupe with a validated profile. Active CODEX_HOME, filesystem discovery, and extra raw app-server checks are opt-in. The command succeeds when at least one usage source works and reports degraded status when another fetch or setup check fails.",
		Examples: []string{
			"multicodex monitor doctor",
			"multicodex monitor doctor --json",
		},
	},
	"monitor tui": {
		Usage:       "multicodex monitor tui [--interval 60s] [--timeout 60s] [--no-color] [--no-alt-screen] [--include-default] [--include-active] [--discover]",
		Description: "Explicit alias for the live subscription-usage terminal UI. This behaves the same as `multicodex monitor` with no monitor subcommand.",
		Examples: []string{
			"multicodex monitor tui",
			"multicodex monitor tui --interval 30s",
		},
	},
	"monitor completion": {
		Usage:       "multicodex monitor completion [bash|zsh|fish]",
		Description: "Print the full multicodex completion script from the monitor namespace. Defaults to bash when no shell is provided.",
		Examples: []string{
			"multicodex monitor completion",
			"multicodex monitor completion zsh",
		},
	},
	"doctor": {
		Usage:       "multicodex doctor [--json] [--timeout 8s]",
		Description: "Run non-mutating setup, auth, and leak-guard checks.",
		Examples: []string{
			"multicodex doctor",
			"multicodex doctor --json",
			"multicodex doctor --timeout 12s",
		},
	},
	"dry-run": {
		Usage:       "multicodex dry-run [operation]",
		Description: "Preview commands and filesystem operations without making changes.",
		Examples: []string{
			"multicodex dry-run",
			"multicodex dry-run login personal",
		},
	},
	"completion": {
		Usage:       "multicodex completion <bash|zsh|fish>",
		Description: "Print completion script for your shell. This supports command names and profile name completion.",
		Examples: []string{
			`eval "$(multicodex completion zsh)"`,
			`eval "$(multicodex completion bash)"`,
			"multicodex completion fish > ~/.config/fish/completions/multicodex.fish",
		},
	},
	"version": {
		Usage:       "multicodex version",
		Description: "Print multicodex version.",
		Examples: []string{
			"multicodex version",
		},
	},
	"help": {
		Usage:       "multicodex help [command [subcommand]]",
		Description: "Show global help or detailed help for one command.",
		Examples: []string{
			"multicodex help",
			"multicodex help heartbeat",
			"multicodex help claude exec",
			"multicodex help monitor doctor",
			"multicodex help monitor tui",
		},
	},
}

func printHelp() {
	fmt.Println("multicodex")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multicodex <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, c := range commandSummaries {
		fmt.Printf("  %-36s %s\n", c.Name, c.Summary)
	}
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  multicodex claude status")
	fmt.Println("  multicodex init")
	fmt.Println("  multicodex add personal")
	fmt.Println("  multicodex cli personal")
	fmt.Println("  multicodex monitor")
	fmt.Println("  multicodex heartbeat")
	fmt.Println("  multicodex reconcile")
	fmt.Println(`  eval "$(multicodex completion zsh)"`)
	fmt.Println()
	fmt.Println("Help:")
	fmt.Println("  multicodex help <command> [subcommand]")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - bare commands continue to manage Codex profiles")
	fmt.Println("  - provider commands are isolated and do not copy credentials")
}

func printClaudeHelp() {
	fmt.Println("multicodex claude")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multicodex claude <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, item := range []struct {
		name    string
		summary string
	}{
		{"add <name>", "add a managed Claude profile"},
		{"login <name>", "run the official Claude.ai login flow"},
		{"cli <name|default> [claude args...]", "run the official interactive Claude CLI"},
		{"exec [claude -p args...]", "route official Claude print mode by fresh usage"},
		{"status", "show auth status for default and managed profiles"},
		{"usage", "show session, weekly, and Fable usage"},
		{"doctor", "run read-only Claude provider checks"},
		{"help [command]", "show Claude namespace help"},
	} {
		fmt.Printf("  %-39s %s\n", item.name, item.summary)
	}
	fmt.Println()
	fmt.Println("The default target is a protected reserve and always runs with CLAUDE_CONFIG_DIR absent.")
}

func printClaudeCommandHelp(command string) error {
	name := "claude " + strings.TrimSpace(strings.ToLower(command))
	topic, ok := commandHelpByName[name]
	if !ok {
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown Claude help topic: %s", command)}
	}
	fmt.Println("multicodex", name)
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
	if len(args) == 0 {
		printHelp()
		return nil
	}
	if len(args) > 2 {
		return &ExitError{Code: 2, Message: "usage: multicodex help [command [subcommand]]"}
	}

	name := normalizeHelpTopic(strings.Join(args, " "))
	topic, ok := commandHelpByName[name]
	if !ok {
		known := make([]string, 0, len(commandHelpByName))
		for k := range commandHelpByName {
			known = append(known, k)
		}
		sort.Strings(known)
		return &ExitError{
			Code:    2,
			Message: fmt.Sprintf("unknown help topic: %s\nknown topics: %s", args[0], strings.Join(known, ", ")),
		}
	}

	fmt.Println("multicodex", name)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s\n", topic.Usage)
	fmt.Println()
	fmt.Println("Description:")
	fmt.Printf("  %s\n", topic.Description)
	if len(topic.Examples) > 0 {
		fmt.Println()
		fmt.Println("Examples:")
		for _, ex := range topic.Examples {
			fmt.Printf("  %s\n", ex)
		}
	}
	return nil
}

func normalizeHelpTopic(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "--help", "-h":
		return "help"
	case "--version", "-v":
		return "version"
	case "monitor-doctor", "monitor/doctor":
		return "monitor doctor"
	case "monitor-tui", "monitor/tui":
		return "monitor tui"
	case "monitor-completion", "monitor/completion":
		return "monitor completion"
	default:
		return s
	}
}
