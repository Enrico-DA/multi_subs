package multisubs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var claudeProbeTimeout = 15 * time.Second

type claudeTarget struct {
	Name      string
	Kind      string
	ConfigDir string
	Profile   *claudeProfile
}

func runClaudeCLI(args []string) error {
	if len(args) == 0 {
		printClaudeHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" {
		if len(args) != 1 {
			return &ExitError{Code: 2, Message: "usage: multisubs claude"}
		}
		printClaudeHelp()
		return nil
	}
	if args[0] == "login" {
		if handled, err := runClaudeLoginHelpFastPath(osClaudeCommandRunner{}, args[1:]); handled {
			return err
		}
	}
	if officialArgs, ok := claudeOfficialHelpFastPath(args); ok {
		runner := osClaudeCommandRunner{}
		err := runner.Run(context.Background(), officialArgs, claudeEnv(os.Environ(), ""))
		return claudeChildError(err, "Claude help command failed")
	}
	app, err := newApp(args[0] != "add")
	if err != nil {
		return err
	}
	return app.cmdClaude(args)
}

func (a *App) cmdClaude(args []string) error {
	if len(args) == 0 {
		printClaudeHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" {
		if len(args) != 1 {
			return &ExitError{Code: 2, Message: "usage: multisubs claude"}
		}
		printClaudeHelp()
		return nil
	}
	if args[0] == "login" {
		if handled, err := runClaudeLoginHelpFastPath(a.claudeCommandRunner(), args[1:]); handled {
			return err
		}
	}
	if officialArgs, ok := claudeOfficialHelpFastPath(args); ok {
		err := a.claudeCommandRunner().Run(context.Background(), officialArgs, claudeEnv(os.Environ(), ""))
		return claudeChildError(err, "Claude help command failed")
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printClaudeHelp()
			return nil
		}
		if len(args) != 2 {
			return &ExitError{Code: 2, Message: "usage: multisubs claude help [command]"}
		}
		return printClaudeCommandHelp(args[1])
	}
	if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		return printClaudeCommandHelp(args[0])
	}

	switch args[0] {
	case "add":
		return a.cmdClaudeAdd(args[1:])
	case "login":
		return a.cmdClaudeLogin(args[1:])
	case "cli":
		return a.cmdClaudeCLI(args[1:])
	case "exec":
		return a.cmdClaudeExec(args[1:])
	case "status":
		return a.cmdClaudeStatus(args[1:])
	case "usage":
		return a.cmdClaudeUsage(args[1:])
	case "doctor":
		return a.cmdClaudeDoctor(args[1:])
	default:
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown Claude command: %s\nrun \"multisubs claude help\" for available commands", args[0])}
	}
}

func (a *App) claudeCommandRunner() claudeCommandRunner {
	if a.claudeRunner == nil {
		return osClaudeCommandRunner{}
	}
	return a.claudeRunner
}

func (a *App) cmdClaudeAdd(args []string) error {
	if len(args) != 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude add <name>"}
	}
	name := strings.TrimSpace(args[0])
	if err := validateClaudeProfileName(name); err != nil {
		return &ExitError{Code: 2, Message: err.Error()}
	}
	store := newClaudeStore(a.store.paths)
	var profile claudeProfile
	if err := store.WithConfigLock(func() error {
		cfg, err := store.LoadIfExists()
		if err != nil {
			return err
		}
		if _, exists := cfg.Profiles[name]; exists {
			return &ExitError{Code: 2, Message: fmt.Sprintf("Claude profile already exists: %s", name)}
		}
		profile, err = store.CreateProfile(name)
		if err != nil {
			return err
		}
		cfg.Profiles[name] = profile
		return store.Save(cfg)
	}); err != nil {
		return err
	}

	fmt.Printf("added Claude profile: %s\n", name)
	fmt.Printf("Claude config directory: %s\n", profile.ConfigDir)
	return nil
}

func (a *App) cmdClaudeLogin(args []string) error {
	if len(args) == 1 && isHelpFlag(args[0]) {
		return printClaudeCommandHelp("login")
	}
	if handled, err := runClaudeLoginHelpFastPath(a.claudeCommandRunner(), args); handled {
		return err
	}
	if len(args) < 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude login <name> [claude auth login args]"}
	}
	store := newClaudeStore(a.store.paths)
	profile, err := loadClaudeManagedProfile(store, strings.TrimSpace(args[0]))
	if err != nil {
		return err
	}
	if err := store.EnsureProfileReady(profile); err != nil {
		return err
	}
	fmt.Printf("logging in Claude profile %q\n", profile.Name)
	loginArgs := append([]string{"auth", "login", "--claudeai"}, args[1:]...)
	err = a.claudeCommandRunner().Run(context.Background(), loginArgs, claudeEnv(os.Environ(), profile.ConfigDir))
	if err != nil {
		return claudeChildError(err, "Claude auth login failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
	status, statusErr := fetchClaudeAuthStatus(ctx, a.claudeCommandRunner(), profile.ConfigDir)
	cancel()
	if statusErr != nil {
		return fmt.Errorf("verify Claude login: %w", statusErr)
	}
	if err := validateClaudeRoutingAuth(status); err != nil {
		return fmt.Errorf("verify Claude login: %w", err)
	}
	if err := a.rejectDuplicateClaudeOrganization(profile, status.OrgID); err != nil {
		return err
	}
	fmt.Printf("Claude login complete: %s (%s)\n", valueOrDash(status.Identity), status.OrgID)
	return nil
}

func runClaudeLoginHelpFastPath(runner claudeCommandRunner, args []string) (bool, error) {
	helpFlag, hasTargetScopedHelp, exact := targetScopedLoginHelp(args)
	if !hasTargetScopedHelp {
		return false, nil
	}
	if !exact {
		return true, &ExitError{Code: 2, Message: "usage: multisubs claude login <name> [claude auth login args]"}
	}
	err := runner.Run(
		context.Background(),
		[]string{"auth", "login", "--claudeai", helpFlag},
		claudeEnv(os.Environ(), ""),
	)
	return true, claudeChildError(err, "Claude auth login help failed")
}

func (a *App) rejectDuplicateClaudeOrganization(profile claudeProfile, orgID string) error {
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return err
	}
	targets := []claudeTarget{{Name: claudeDefaultTarget, Kind: "default"}}
	for _, name := range sortedClaudeProfileNames(cfg) {
		if name == profile.Name {
			continue
		}
		other := cfg.Profiles[name]
		if err := store.EnsureProfileReady(other); err != nil {
			return fmt.Errorf("Claude profile %q is unsafe: %w", name, err)
		}
		otherCopy := other
		targets = append(targets, claudeTarget{Name: name, Kind: "managed", ConfigDir: other.ConfigDir, Profile: &otherCopy})
	}
	for _, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		status, statusErr := fetchClaudeAuthStatus(ctx, a.claudeCommandRunner(), target.ConfigDir)
		cancel()
		description := claudeTargetDescription(target)
		if statusErr != nil {
			return fmt.Errorf("cannot verify Claude organization for %s: %w", description, statusErr)
		}
		if !status.LoggedIn {
			continue
		}
		if err := validateClaudeRoutingAuth(status); err != nil {
			return fmt.Errorf("cannot verify Claude organization for %s: %w", description, err)
		}
		if status.OrgID == orgID {
			return &ExitError{
				Code:    2,
				Message: fmt.Sprintf("Claude profile %q uses the same organization as the %s; log in with a different Max account", profile.Name, description),
			}
		}
	}
	return nil
}

func (a *App) cmdClaudeCLI(args []string) error {
	if len(args) < 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude cli <name|default> [claude args...]"}
	}
	name := strings.TrimSpace(args[0])
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return err
	}
	configDir := ""
	if name != claudeDefaultTarget {
		profile, ok := cfg.Profiles[name]
		if !ok {
			return &ExitError{Code: 2, Message: fmt.Sprintf("unknown Claude profile: %s", name)}
		}
		if err := store.EnsureProfileReady(profile); err != nil {
			return err
		}
		configDir = profile.ConfigDir
	}
	err = a.claudeCommandRunner().RunInteractive(args[1:], claudeEnv(os.Environ(), configDir))
	return claudeChildError(err, "Claude command failed")
}

func (a *App) cmdClaudeStatus(args []string) error {
	if len(args) != 0 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude status"}
	}
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return err
	}
	targets := claudeTargets(cfg)
	fmt.Println("multisubs claude status")
	fmt.Println()
	fmt.Printf("%-16s %-9s %-12s %-30s %s\n", "target", "kind", "state", "identity", "auth")
	for _, target := range targets {
		if target.Profile != nil {
			if err := store.EnsureProfileReady(*target.Profile); err != nil {
				fmt.Printf("%-16s %-9s %-12s %-30s %s\n", target.Name, target.Kind, "error", "-", truncate(err.Error(), 80))
				continue
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		status, statusErr := fetchClaudeAuthStatus(ctx, a.claudeCommandRunner(), target.ConfigDir)
		cancel()
		if statusErr != nil {
			fmt.Printf("%-16s %-9s %-12s %-30s %s\n", target.Name, target.Kind, "error", "-", truncate(statusErr.Error(), 80))
			continue
		}
		state := "logged-out"
		if status.LoggedIn {
			state = "logged-in"
		}
		identity := valueOrDash(status.Identity)
		auth := compactClaudeAuthDescription(status)
		fmt.Printf("%-16s %-9s %-12s %-30s %s\n", target.Name, target.Kind, state, truncate(identity, 30), truncate(auth, 80))
	}
	return nil
}

func (a *App) cmdClaudeUsage(args []string) error {
	if len(args) != 0 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude usage"}
	}
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return err
	}
	fmt.Println("multisubs claude usage")
	for _, target := range claudeTargets(cfg) {
		fmt.Println()
		fmt.Printf("%s (%s)\n", target.Name, target.Kind)
		if target.Profile != nil {
			if err := store.EnsureProfileReady(*target.Profile); err != nil {
				fmt.Printf("  unavailable: %s\n", err)
				continue
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		usage, usageErr := fetchClaudeUsage(ctx, a.claudeCommandRunner(), target.ConfigDir)
		cancel()
		if usageErr != nil {
			fmt.Printf("  unavailable: %s\n", usageErr)
			continue
		}
		printClaudeUsageWindow("session", usage.Session)
		printClaudeUsageWindow("weekly all models", usage.WeeklyAll)
		if usage.Fable == nil {
			fmt.Println("  Fable: unavailable")
		} else {
			printClaudeUsageWindow("Fable", *usage.Fable)
		}
	}
	return nil
}

func (a *App) cmdClaudeDoctor(args []string) error {
	if len(args) != 0 {
		return &ExitError{Code: 2, Message: "usage: multisubs claude doctor"}
	}
	report, err := a.runClaudeDoctorChecks(claudeProbeTimeout)
	if err != nil {
		return err
	}
	printDoctorHuman("multisubs claude doctor", report)
	if report.HasFailures() {
		return &ExitError{Code: 1, Message: "Claude doctor checks failed"}
	}
	return nil
}

func (a *App) runClaudeDoctorChecks(timeout time.Duration) (DoctorReport, error) {
	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return DoctorReport{}, err
	}
	checks := make([]DoctorCheck, 0, len(cfg.Profiles)+4)
	if _, err := os.Lstat(store.paths.ClaudeConfigPath); errors.Is(err, os.ErrNotExist) {
		checks = append(checks, DoctorCheck{Status: "warn", Name: "sidecar", Details: "not initialized; add a profile with multisubs claude add <name>"})
	} else {
		checks = append(checks, DoctorCheck{Status: "ok", Name: "sidecar", Details: fmt.Sprintf("version %d with %d managed profile(s)", cfg.Version, len(cfg.Profiles))})
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	stdout, _, versionErr := a.claudeCommandRunner().Capture(ctx, []string{"--version"}, claudeEnv(os.Environ(), ""))
	cancel()
	if versionErr != nil {
		checks = append(checks, DoctorCheck{Status: "fail", Name: "Claude binary", Details: claudeProbeFailure(ctx, versionErr)})
	} else {
		version := strings.TrimSpace(string(stdout))
		if version == "" {
			version = "version output was empty"
		}
		checks = append(checks, DoctorCheck{Status: "ok", Name: "Claude binary", Details: truncate(firstLineOrDash(version), 120)})
	}
	organizationTargets := make(map[string]string, len(cfg.Profiles)+1)
	for _, target := range claudeTargets(cfg) {
		if target.Profile != nil {
			if err := store.EnsureProfileReady(*target.Profile); err != nil {
				checks = append(checks, DoctorCheck{Status: "fail", Name: "target " + target.Name, Details: err.Error()})
				continue
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		status, statusErr := fetchClaudeAuthStatus(ctx, a.claudeCommandRunner(), target.ConfigDir)
		cancel()
		if statusErr != nil {
			checks = append(checks, DoctorCheck{Status: "warn", Name: "target " + target.Name, Details: statusErr.Error()})
			continue
		}
		if !status.LoggedIn {
			checks = append(checks, DoctorCheck{Status: "warn", Name: "target " + target.Name, Details: "not logged in"})
			continue
		}
		if err := validateClaudeRoutingAuth(status); err != nil {
			checks = append(checks, DoctorCheck{Status: "warn", Name: "target " + target.Name, Details: err.Error()})
			continue
		}
		if existing, duplicate := organizationTargets[status.OrgID]; duplicate {
			checks = append(checks, DoctorCheck{Status: "fail", Name: "target " + target.Name, Details: "duplicates Claude organization used by " + existing})
			continue
		}
		organizationTargets[status.OrgID] = target.Name
		checks = append(checks, DoctorCheck{Status: "ok", Name: "target " + target.Name, Details: "logged in as " + valueOrDash(status.Identity) + " (" + status.OrgID + ")"})
	}
	return DoctorReport{Checks: checks}, nil
}

func loadClaudeManagedProfile(store *claudeStore, name string) (claudeProfile, error) {
	if name == claudeDefaultTarget {
		return claudeProfile{}, &ExitError{Code: 2, Message: "the built-in Claude default account is not a managed login target; run claude auth login directly"}
	}
	cfg, err := store.LoadIfExists()
	if err != nil {
		return claudeProfile{}, err
	}
	profile, ok := cfg.Profiles[name]
	if !ok {
		return claudeProfile{}, &ExitError{Code: 2, Message: fmt.Sprintf("unknown Claude profile: %s", name)}
	}
	return profile, nil
}

func claudeTargets(cfg *claudeConfig) []claudeTarget {
	targets := []claudeTarget{{Name: claudeDefaultTarget, Kind: "default"}}
	for _, name := range sortedClaudeProfileNames(cfg) {
		profile := cfg.Profiles[name]
		profileCopy := profile
		targets = append(targets, claudeTarget{Name: name, Kind: "managed", ConfigDir: profile.ConfigDir, Profile: &profileCopy})
	}
	return targets
}

func claudeTargetDescription(target claudeTarget) string {
	if target.Kind == "default" {
		return "default account"
	}
	return fmt.Sprintf("managed profile %q", target.Name)
}

func compactClaudeAuthDescription(status claudeAuthStatus) string {
	parts := make([]string, 0, 3)
	for _, value := range []string{status.AuthMethod, status.APIProvider, status.Subscription} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func printClaudeUsageWindow(label string, window claudeUsageWindow) {
	if strings.TrimSpace(window.ResetText) == "" {
		fmt.Printf("  %s: %s used\n", label, formatClaudePercent(window.UsedPercent))
		return
	}
	fmt.Printf("  %s: %s used; %s\n", label, formatClaudePercent(window.UsedPercent), window.ResetText)
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func claudeOfficialHelpFastPath(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	switch args[0] {
	case "exec":
		if commandArgsContainHelp(args[1:]) {
			return append([]string{"-p"}, args[1:]...), true
		}
	case "cli":
		if len(args) >= 3 && commandArgsContainHelp(args[2:]) {
			return append([]string(nil), args[2:]...), true
		}
	}
	return nil, false
}

func commandArgsContainHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}
