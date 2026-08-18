package multisubs

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const codexCLIUsage = "usage: multisubs codex cli [<name>|--account <name>] [codex args...]"

func (a *App) cmdCLI(args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return a.cmdHelp([]string{"codex", "cli"})
	}
	if handled, err := runCodexCLIHelpFastPath(args); handled {
		return err
	}
	if len(args) > 0 && args[0] == "--account" {
		return a.cmdCLIWithAccount(args[1:])
	}

	name, remaining, explicit := parseCLIExplicitProfile(args)
	if explicit {
		return a.cmdCLIWithNamedProfile(name, remaining)
	}

	selected, err := a.selectAccountForCodexArgs(context.Background(), remaining)
	if err != nil {
		return err
	}
	if selected.IsProfile {
		if err := ensureProfileCodexExecutionReady(a.store.paths, selected.Profile); err != nil {
			return err
		}
	}

	activeProfile := selected.Name
	if !selected.IsProfile {
		activeProfile = ""
	}
	return RunInteractiveCodexWithProfile(selected.CodexHome, activeProfile, remaining)
}

func (a *App) cmdCLIWithAccount(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return &ExitError{Code: 2, Message: codexCLIUsage}
	}
	name := args[0]
	codexArgs := args[1:]
	if len(codexArgs) > 0 && codexArgs[0] == "--" {
		codexArgs = codexArgs[1:]
	}
	return a.cmdCLIWithNamedProfile(name, codexArgs)
}

func (a *App) cmdCLIWithNamedProfile(name string, codexArgs []string) error {
	// A rejected profile name must not create product state, so the registry
	// is read without initializing it. A profile can only exist in a saved
	// registry, so a missing one is always an unknown name.
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		return err
	}
	profile, ok := cfg.Profiles[name]
	if !ok {
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown profile: %s", name)}
	}
	changes, err := a.store.EnsureProfileDir(profile, cfg.ProfileResources)
	if err != nil {
		return err
	}
	printResourceChanges(changes)
	if err := ensureProfileCodexExecutionReady(a.store.paths, profile); err != nil {
		return err
	}

	return RunInteractiveCodexWithProfile(profile.CodexHome, name, codexArgs)
}

func parseCLIExplicitProfile(args []string) (name string, remaining []string, explicit bool) {
	if len(args) == 0 {
		return "", nil, false
	}
	if args[0] == "--" {
		return "", args[1:], false
	}
	if strings.HasPrefix(args[0], "-") {
		return "", args, false
	}
	remaining = args[1:]
	if len(remaining) > 0 && remaining[0] == "--" {
		remaining = remaining[1:]
	}
	return args[0], remaining, true
}

func runCodexCLIHelpFastPath(args []string) (bool, error) {
	if len(args) > 1 && (args[0] == "-h" || args[0] == "--help") {
		return true, &ExitError{Code: 2, Message: codexCLIUsage}
	}
	if len(args) < 2 {
		return false, nil
	}
	helpIndex := -1
	for index, arg := range args[1:] {
		if arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" {
			helpIndex = index + 1
			break
		}
	}
	if helpIndex == -1 {
		return false, nil
	}
	if len(args) != 2 || helpIndex != 1 {
		return true, &ExitError{Code: 2, Message: codexCLIUsage}
	}
	if strings.HasPrefix(args[0], "-") {
		return true, &ExitError{Code: 2, Message: codexCLIUsage}
	}
	return true, runCommandWithEnv("codex", []string{args[1]}, neutralCodexEnv(os.Environ()), "Codex help command failed")
}
