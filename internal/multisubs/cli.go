package multisubs

import (
	"fmt"
	"os"
)

func (a *App) cmdCLI(args []string) error {
	if len(args) < 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex cli <name> [codex args...]"}
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return a.cmdHelp([]string{"codex", "cli"})
	}
	if handled, err := runCodexCLIHelpFastPath(args); handled {
		return err
	}

	name := args[0]
	cfg, err := a.loadOrInitConfig()
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

	return RunInteractiveCodexWithProfile(profile.CodexHome, name, args[1:])
}

func runCodexCLIHelpFastPath(args []string) (bool, error) {
	if len(args) > 1 && (args[0] == "-h" || args[0] == "--help") {
		return true, &ExitError{Code: 2, Message: "usage: multisubs codex cli <name> [codex args...]"}
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
		return true, &ExitError{Code: 2, Message: "usage: multisubs codex cli <name> [codex args...]"}
	}
	return true, runCommandWithEnv("codex", []string{args[1]}, neutralCodexEnv(os.Environ()), "Codex help command failed")
}
