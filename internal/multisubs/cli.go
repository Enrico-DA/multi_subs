package multisubs

import (
	"fmt"
)

func (a *App) cmdCLI(args []string) error {
	if len(args) < 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex cli <name> [codex args...]"}
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return a.cmdHelp([]string{"codex", "cli"})
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
