package multisubs

import (
	"fmt"
	"strings"
)

func (a *App) cmdCompletion(args []string) error {
	if len(args) != 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs completion <bash|zsh|fish>"}
	}
	switch args[0] {
	case "bash":
		fmt.Print(renderBashCompletion())
	case "zsh":
		fmt.Print(renderZshCompletion())
	case "fish":
		fmt.Print(renderFishCompletion())
	default:
		return &ExitError{Code: 2, Message: "unsupported shell. expected one of: bash, zsh, fish"}
	}
	return nil
}

func (a *App) cmdCompleteCodexProfiles() error {
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		return nil
	}
	for _, name := range sortedProfileNames(cfg) {
		fmt.Println(name)
	}
	return nil
}

func (a *App) cmdCompleteClaudeProfiles() error {
	cfg, err := newClaudeStore(a.store.paths).LoadIfExists()
	if err != nil {
		return nil
	}
	for _, name := range sortedClaudeProfileNames(cfg) {
		fmt.Println(name)
	}
	return nil
}

func renderBashCompletion() string {
	return strings.TrimSpace(`
_multisubs_codex_profiles() {
  "${COMP_WORDS[0]}" __complete-codex-profiles 2>/dev/null
}

_multisubs_claude_profiles() {
  "${COMP_WORDS[0]}" __complete-claude-profiles 2>/dev/null
}

_multisubs_complete() {
  local cur top provider command
  cur="${COMP_WORDS[COMP_CWORD]}"
  top="${COMP_WORDS[1]:-}"
  provider="${COMP_WORDS[2]:-}"
  command="${COMP_WORDS[3]:-}"

  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W "init doctor codex claude completion version help" -- "$cur") )
    return 0
  fi

  case "$top" in
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
      ;;
    doctor)
      COMPREPLY=( $(compgen -W "--json --timeout" -- "$cur") )
      ;;
    codex)
      if (( COMP_CWORD == 2 )); then
        COMPREPLY=( $(compgen -W "init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help" -- "$cur") )
        return 0
      fi
      case "$provider" in
        login|cli)
          if (( COMP_CWORD == 3 )); then
            COMPREPLY=( $(compgen -W "$(_multisubs_codex_profiles)" -- "$cur") )
          fi
          ;;
        doctor)
          COMPREPLY=( $(compgen -W "--json --timeout" -- "$cur") )
          ;;
        dry-run)
          if (( COMP_CWORD == 3 )); then
            COMPREPLY=( $(compgen -W "login" -- "$cur") )
          elif (( COMP_CWORD == 4 )) && [[ "$command" == "login" ]]; then
            COMPREPLY=( $(compgen -W "$(_multisubs_codex_profiles)" -- "$cur") )
          fi
          ;;
        monitor)
          if (( COMP_CWORD == 3 )); then
            COMPREPLY=( $(compgen -W "doctor completion help tui --interval --timeout --no-color --no-alt-screen --include-default --include-active --discover" -- "$cur") )
          else
            case "$command" in
              completion)
                COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
                ;;
              doctor)
                COMPREPLY=( $(compgen -W "--json --timeout --include-default --include-active --discover --app-server" -- "$cur") )
                ;;
              help)
                COMPREPLY=( $(compgen -W "doctor completion tui help" -- "$cur") )
                ;;
              tui)
                COMPREPLY=( $(compgen -W "--interval --timeout --no-color --no-alt-screen --include-default --include-active --discover" -- "$cur") )
                ;;
            esac
          fi
          ;;
        help)
          COMPREPLY=( $(compgen -W "init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help" -- "$cur") )
          ;;
      esac
      ;;
    claude)
      if (( COMP_CWORD == 2 )); then
        COMPREPLY=( $(compgen -W "add login cli exec status usage doctor help" -- "$cur") )
        return 0
      fi
      case "$provider" in
        login)
          if (( COMP_CWORD == 3 )); then
            COMPREPLY=( $(compgen -W "$(_multisubs_claude_profiles)" -- "$cur") )
          fi
          ;;
        cli)
          if (( COMP_CWORD == 3 )); then
            COMPREPLY=( $(compgen -W "default $(_multisubs_claude_profiles)" -- "$cur") )
          fi
          ;;
        help)
          COMPREPLY=( $(compgen -W "add login cli exec status usage doctor help" -- "$cur") )
          ;;
      esac
      ;;
    help)
      if (( COMP_CWORD == 2 )); then
        COMPREPLY=( $(compgen -W "init doctor codex claude completion version help" -- "$cur") )
      elif (( COMP_CWORD == 3 )); then
        case "${COMP_WORDS[2]}" in
          codex)
            COMPREPLY=( $(compgen -W "init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help" -- "$cur") )
            ;;
          claude)
            COMPREPLY=( $(compgen -W "add login cli exec status usage doctor help" -- "$cur") )
            ;;
        esac
      elif (( COMP_CWORD == 4 )) && [[ "${COMP_WORDS[2]}" == "codex" && "${COMP_WORDS[3]}" == "monitor" ]]; then
        COMPREPLY=( $(compgen -W "doctor completion tui help" -- "$cur") )
      fi
      ;;
  esac
}

complete -F _multisubs_complete multisubs
`) + "\n"
}

func renderZshCompletion() string {
	return strings.TrimSpace(`
autoload -U +X compinit && compinit
autoload -U +X bashcompinit && bashcompinit

_multisubs_codex_profiles() {
  local bin="${words[1]:-multisubs}"
  "$bin" __complete-codex-profiles 2>/dev/null
}

_multisubs_claude_profiles() {
  local bin="${words[1]:-multisubs}"
  "$bin" __complete-claude-profiles 2>/dev/null
}

_multisubs_complete() {
  local cur top provider command
  cur="${words[CURRENT]}"
  top="${words[2]:-}"
  provider="${words[3]:-}"
  command="${words[4]:-}"

  if (( CURRENT == 2 )); then
    compadd -- init doctor codex claude completion version help
    return
  fi

  case "$top" in
    completion)
      compadd -- bash zsh fish
      ;;
    doctor)
      compadd -- --json --timeout
      ;;
    codex)
      if (( CURRENT == 3 )); then
        compadd -- init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help
        return
      fi
      case "$provider" in
        login|cli)
          if (( CURRENT == 4 )); then
            compadd -- ${=($(_multisubs_codex_profiles))}
          fi
          ;;
        doctor)
          compadd -- --json --timeout
          ;;
        dry-run)
          if (( CURRENT == 4 )); then
            compadd -- login
          elif (( CURRENT == 5 )) && [[ "$command" == "login" ]]; then
            compadd -- ${=($(_multisubs_codex_profiles))}
          fi
          ;;
        monitor)
          if (( CURRENT == 4 )); then
            compadd -- doctor completion help tui --interval --timeout --no-color --no-alt-screen --include-default --include-active --discover
          else
            case "$command" in
              completion) compadd -- bash zsh fish ;;
              doctor) compadd -- --json --timeout --include-default --include-active --discover --app-server ;;
              help) compadd -- doctor completion tui help ;;
              tui) compadd -- --interval --timeout --no-color --no-alt-screen --include-default --include-active --discover ;;
            esac
          fi
          ;;
        help)
          compadd -- init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help
          ;;
      esac
      ;;
    claude)
      if (( CURRENT == 3 )); then
        compadd -- add login cli exec status usage doctor help
        return
      fi
      case "$provider" in
        login)
          if (( CURRENT == 4 )); then
            compadd -- ${=($(_multisubs_claude_profiles))}
          fi
          ;;
        cli)
          if (( CURRENT == 4 )); then
            compadd -- default ${=($(_multisubs_claude_profiles))}
          fi
          ;;
        help)
          compadd -- add login cli exec status usage doctor help
          ;;
      esac
      ;;
    help)
      if (( CURRENT == 3 )); then
        compadd -- init doctor codex claude completion version help
      elif (( CURRENT == 4 )); then
        case "${words[3]:-}" in
          codex) compadd -- init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help ;;
          claude) compadd -- add login cli exec status usage doctor help ;;
        esac
      elif (( CURRENT == 5 )) && [[ "${words[3]:-}" == "codex" && "${words[4]:-}" == "monitor" ]]; then
        compadd -- doctor completion tui help
      fi
      ;;
  esac
}

compdef _multisubs_complete multisubs
`) + "\n"
}

type fishCompletionEntry struct {
	path               []string
	matchPathPrefix    bool
	tokens             []string
	argumentExpression string
	longOptions        []string
}

func fishCompletionEntries() []fishCompletionEntry {
	codexCommands := []string{"init", "add", "login", "login-all", "cli", "exec", "status", "reconcile", "heartbeat", "monitor", "doctor", "dry-run", "help"}
	claudeCommands := []string{"add", "login", "cli", "exec", "status", "usage", "doctor", "help"}
	monitorCommands := []string{"doctor", "completion", "help", "tui"}
	monitorTUIOptions := []string{"interval", "timeout", "no-color", "no-alt-screen", "include-default", "include-active", "discover"}

	return []fishCompletionEntry{
		{tokens: []string{"init", "doctor", "codex", "claude", "completion", "version", "help"}},
		{path: []string{"codex"}, tokens: codexCommands},
		{path: []string{"claude"}, tokens: claudeCommands},
		{path: []string{"help"}, tokens: []string{"init", "doctor", "codex", "claude", "completion", "version", "help"}},
		{path: []string{"help", "codex"}, tokens: codexCommands},
		{path: []string{"help", "claude"}, tokens: claudeCommands},
		{path: []string{"help", "codex", "monitor"}, tokens: monitorCommands},
		{path: []string{"codex", "login"}, argumentExpression: "(__multisubs_codex_profiles)"},
		{path: []string{"codex", "cli"}, argumentExpression: "(__multisubs_codex_profiles)"},
		{path: []string{"claude", "login"}, argumentExpression: "(__multisubs_claude_profiles)"},
		{path: []string{"claude", "cli"}, argumentExpression: "default (__multisubs_claude_profiles)"},
		{path: []string{"codex", "dry-run"}, tokens: []string{"login"}},
		{path: []string{"codex", "dry-run", "login"}, argumentExpression: "(__multisubs_codex_profiles)"},
		{path: []string{"completion"}, tokens: []string{"bash", "zsh", "fish"}},
		{path: []string{"doctor"}, matchPathPrefix: true, longOptions: []string{"json", "timeout"}},
		{path: []string{"codex", "doctor"}, matchPathPrefix: true, longOptions: []string{"json", "timeout"}},
		{path: []string{"codex", "monitor"}, tokens: monitorCommands, longOptions: monitorTUIOptions},
		{path: []string{"codex", "monitor", "completion"}, tokens: []string{"bash", "zsh", "fish"}},
		{path: []string{"codex", "monitor", "doctor"}, matchPathPrefix: true, longOptions: []string{"json", "timeout", "include-default", "include-active", "discover", "app-server"}},
		{path: []string{"codex", "monitor", "help"}, tokens: monitorCommands},
		{path: []string{"codex", "monitor", "tui"}, matchPathPrefix: true, longOptions: monitorTUIOptions},
		{path: []string{"codex", "help"}, tokens: codexCommands},
		{path: []string{"claude", "help"}, tokens: claudeCommands},
	}
}

func fishCompletionEntryMatches(entry fishCompletionEntry, path []string) bool {
	if entry.matchPathPrefix {
		if len(path) < len(entry.path) {
			return false
		}
	} else if len(path) != len(entry.path) {
		return false
	}
	for index := range entry.path {
		if entry.path[index] != path[index] {
			return false
		}
	}
	return true
}

func fishCompletionTokens(path []string) []string {
	var tokens []string
	for _, entry := range fishCompletionEntries() {
		if !fishCompletionEntryMatches(entry, path) {
			continue
		}
		tokens = append(tokens, entry.tokens...)
		for _, option := range entry.longOptions {
			tokens = append(tokens, "--"+option)
		}
	}
	return tokens
}

func renderFishCompletion() string {
	var completion strings.Builder
	completion.WriteString(strings.TrimSpace(`
function __multisubs_codex_profiles
    multisubs __complete-codex-profiles 2>/dev/null
end

function __multisubs_claude_profiles
    multisubs __complete-claude-profiles 2>/dev/null
end

function __multisubs_path_is
    set -l words (commandline -opc)
    if test (count $words) -gt 0
        set -e words[1]
    end
    test (count $words) -eq (count $argv); or return 1
    set -l index 1
    while test $index -le (count $argv)
        test "$words[$index]" = "$argv[$index]"; or return 1
        set index (math "$index + 1")
    end
    return 0
end

function __multisubs_path_starts_with
    set -l words (commandline -opc)
    if test (count $words) -gt 0
        set -e words[1]
    end
    test (count $words) -ge (count $argv); or return 1
    set -l index 1
    while test $index -le (count $argv)
        test "$words[$index]" = "$argv[$index]"; or return 1
        set index (math "$index + 1")
    end
    return 0
end
`))
	completion.WriteString("\n")
	for _, entry := range fishCompletionEntries() {
		condition := "__multisubs_path_is"
		if entry.matchPathPrefix {
			condition = "__multisubs_path_starts_with"
		}
		if len(entry.path) > 0 {
			condition += " " + strings.Join(entry.path, " ")
		}
		arguments := entry.argumentExpression
		if arguments == "" {
			arguments = strings.Join(entry.tokens, " ")
		}
		if arguments != "" {
			fmt.Fprintf(&completion, "complete -c multisubs -f -n '%s' -a '%s'\n", condition, arguments)
		}
		for _, option := range entry.longOptions {
			fmt.Fprintf(&completion, "complete -c multisubs -f -n '%s' -l %s\n", condition, option)
		}
	}
	return completion.String()
}
