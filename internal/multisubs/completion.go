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

func renderFishCompletion() string {
	return strings.TrimSpace(`
function __multisubs_codex_profiles
    multisubs __complete-codex-profiles 2>/dev/null
end

function __multisubs_claude_profiles
    multisubs __complete-claude-profiles 2>/dev/null
end

complete -c multisubs -f -n '__fish_use_subcommand' -a 'init doctor codex claude completion version help'
complete -c multisubs -f -n '__fish_seen_subcommand_from codex' -a 'init add login login-all cli exec status reconcile heartbeat monitor doctor dry-run help'
complete -c multisubs -f -n '__fish_seen_subcommand_from claude' -a 'add login cli exec status usage doctor help'
complete -c multisubs -f -n '__fish_seen_subcommand_from help' -a 'init doctor codex claude completion version help'
complete -c multisubs -f -n '__fish_seen_subcommand_from codex; and __fish_seen_subcommand_from login cli' -a '(__multisubs_codex_profiles)'
complete -c multisubs -f -n '__fish_seen_subcommand_from claude; and __fish_seen_subcommand_from login' -a '(__multisubs_claude_profiles)'
complete -c multisubs -f -n '__fish_seen_subcommand_from claude; and __fish_seen_subcommand_from cli' -a 'default (__multisubs_claude_profiles)'
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -a 'doctor completion help tui'
complete -c multisubs -f -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
complete -c multisubs -f -n '__fish_seen_subcommand_from doctor' -l json
complete -c multisubs -f -n '__fish_seen_subcommand_from doctor' -l timeout
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l interval
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l timeout
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l no-color
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l no-alt-screen
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l include-default
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l include-active
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor' -l discover
complete -c multisubs -f -n '__fish_seen_subcommand_from monitor; and __fish_seen_subcommand_from doctor' -l app-server
`) + "\n"
}
