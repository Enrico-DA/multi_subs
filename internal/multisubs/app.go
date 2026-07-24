package multisubs

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/codexstate"
)

// App wires command handlers with persistent config.
type App struct {
	store               *Store
	claudeRunner        claudeCommandRunner
	claudeFableResolver claudeFableApplicabilityResolver
	codexUsageSource    codexUsageSourceFactory
	usageClock          func() time.Time
	usageLocation       *time.Location
}

func NewApp() (*App, error) {
	return newApp(false)
}

func NewReadOnlyApp() (*App, error) {
	return newApp(true)
}

func newApp(readOnly bool) (*App, error) {
	var (
		paths Paths
		err   error
	)
	if readOnly {
		paths, err = ResolvePathsReadOnly()
	} else {
		paths, err = ResolvePaths()
	}
	if err != nil {
		return nil, err
	}
	return &App{
		store:               NewStore(paths),
		claudeRunner:        osClaudeCommandRunner{},
		claudeFableResolver: newLocalClaudeFableApplicabilityResolver(),
	}, nil
}

func RunCLI(args []string) error {
	if err := rejectLegacyProductEnvironment(os.Environ()); err != nil {
		return err
	}
	if len(args) == 0 {
		printHelp()
		return nil
	}
	if err := rejectTopLevelArguments(args); err != nil {
		return err
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) == 1 {
			printHelp()
			return nil
		}
		return printCommandHelp(args[1:])
	case "version", "-v", "--version":
		printVersion()
		return nil
	case "completion":
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdCompletion(args[1:])
	case "init":
		app, err := newApp(false)
		if err != nil {
			return err
		}
		return app.cmdInit()
	case "doctor":
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdAggregateDoctor(args[1:])
	case "usage":
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdUsage(args[1:], usageProviderAll)
	case "codex":
		return runCodexCLI(args[1:])
	case "claude":
		return runClaudeCLI(args[1:])
	case "__complete-codex-profiles":
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdCompleteCodexProfiles()
	case "__complete-claude-profiles":
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdCompleteClaudeProfiles()
	}
	if bareCodexCommand(args[0]) {
		return &ExitError{
			Code:    2,
			Message: fmt.Sprintf("bare Codex command %q was removed; run \"multisubs codex %s\" instead", args[0], strings.Join(args, " ")),
		}
	}
	return &ExitError{Code: 2, Message: fmt.Sprintf("unknown command: %s\nrun \"multisubs help\" for available commands", args[0])}
}

func runCodexCLI(args []string) error {
	if len(args) == 0 {
		printCodexHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" {
		if len(args) != 1 {
			return &ExitError{Code: 2, Message: "usage: multisubs codex"}
		}
		printCodexHelp()
		return nil
	}
	if args[0] == "cli" {
		if handled, err := runCodexCLIHelpFastPath(args[1:]); handled {
			return err
		}
	}
	if args[0] == "login" {
		if handled, err := runCodexLoginHelpFastPath(args[1:]); handled {
			return err
		}
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printCodexHelp()
			return nil
		}
		if len(args) != 2 {
			return &ExitError{Code: 2, Message: "usage: multisubs codex help [command]"}
		}
		return printCommandHelp([]string{"codex", args[1]})
	}
	if args[0] == "exec" && execArgsAreHelpRequest(args[1:]) {
		app, err := newApp(true)
		if err != nil {
			return err
		}
		return app.cmdExec(args[1:])
	}
	if args[0] != "usage" && len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		return printCommandHelp([]string{"codex", args[0]})
	}
	if err := rejectCodexArguments(args); err != nil {
		return err
	}
	readOnlyStartup, known := codexCommandReadOnlyStartup(args[0])
	if !known {
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown Codex command: %s\nrun \"multisubs codex help\" for available commands", args[0])}
	}
	app, err := newApp(readOnlyStartup)
	if err != nil {
		return err
	}
	return app.cmdCodex(args)
}

func codexCommandReadOnlyStartup(command string) (bool, bool) {
	switch command {
	case "status", "usage", "doctor", "dry-run", "monitor":
		return true, true
	case "init", "add", "login", "login-all", "cli", "exec", "heartbeat", "reconcile":
		return false, true
	default:
		return false, false
	}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	if err := rejectTopLevelArguments(args); err != nil {
		return err
	}

	switch args[0] {
	case "help", "-h", "--help":
		return a.cmdHelp(args[1:])
	case "version", "-v", "--version":
		printVersion()
		return nil
	case "init":
		return a.cmdInit()
	case "completion":
		return a.cmdCompletion(args[1:])
	case "doctor":
		return a.cmdAggregateDoctor(args[1:])
	case "usage":
		return a.cmdUsage(args[1:], usageProviderAll)
	case "__complete-codex-profiles":
		return a.cmdCompleteCodexProfiles()
	case "__complete-claude-profiles":
		return a.cmdCompleteClaudeProfiles()
	case "codex":
		return a.cmdCodex(args[1:])
	case "claude":
		return a.cmdClaude(args[1:])
	default:
		if bareCodexCommand(args[0]) {
			return &ExitError{Code: 2, Message: fmt.Sprintf("bare Codex command %q was removed; run \"multisubs codex %s\" instead", args[0], strings.Join(args, " "))}
		}
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown command: %s\nrun \"multisubs help\" for available commands", args[0])}
	}
}

func (a *App) cmdCodex(args []string) error {
	if len(args) == 0 {
		printCodexHelp()
		return nil
	}
	if args[0] == "-h" || args[0] == "--help" {
		if len(args) != 1 {
			return &ExitError{Code: 2, Message: "usage: multisubs codex"}
		}
		printCodexHelp()
		return nil
	}
	if args[0] == "cli" {
		if handled, err := runCodexCLIHelpFastPath(args[1:]); handled {
			return err
		}
	}
	if args[0] == "login" {
		if handled, err := runCodexLoginHelpFastPath(args[1:]); handled {
			return err
		}
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printCodexHelp()
			return nil
		}
		if len(args) != 2 {
			return &ExitError{Code: 2, Message: "usage: multisubs codex help [command]"}
		}
		return printCommandHelp([]string{"codex", args[1]})
	}
	if args[0] == "exec" && execArgsAreHelpRequest(args[1:]) {
		return a.cmdExec(args[1:])
	}
	if args[0] != "usage" && len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		return printCommandHelp([]string{"codex", args[0]})
	}
	if err := rejectCodexArguments(args); err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return a.cmdInit()
	case "add":
		return a.cmdAdd(args[1:])
	case "login":
		return a.cmdLogin(args[1:])
	case "login-all":
		return a.cmdLoginAll()
	case "cli":
		return a.cmdCLI(args[1:])
	case "exec":
		return a.cmdExec(args[1:])
	case "status":
		return a.cmdStatus()
	case "usage":
		return a.cmdUsage(args[1:], usageProviderCodex)
	case "reconcile":
		return a.cmdReconcile(args[1:])
	case "heartbeat":
		return a.cmdHeartbeat(args[1:])
	case "monitor":
		return a.cmdMonitor(args[1:])
	case "doctor":
		return a.cmdDoctor(args[1:])
	case "dry-run":
		return a.cmdDryRun(args[1:])
	default:
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown Codex command: %s\nrun \"multisubs codex help\" for available commands", args[0])}
	}
}

func rejectArguments(args []string, usage string) error {
	if len(args) == 0 {
		return nil
	}
	return &ExitError{Code: 2, Message: usage}
}

func rejectTopLevelArguments(args []string) error {
	if len(args) < 2 {
		return nil
	}
	switch args[0] {
	case "init", "__complete-codex-profiles", "__complete-claude-profiles":
		return rejectArguments(args[1:], "usage: multisubs "+args[0])
	case "version", "-v", "--version":
		return rejectArguments(args[1:], "usage: multisubs version")
	case "-h", "--help":
		return rejectArguments(args[1:], "usage: multisubs help [topic]")
	default:
		return nil
	}
}

func rejectCodexArguments(args []string) error {
	if len(args) < 2 {
		return nil
	}
	switch args[0] {
	case "init", "login-all", "status", "usage":
		return rejectArguments(args[1:], "usage: multisubs codex "+args[0])
	default:
		return nil
	}
}

func bareCodexCommand(command string) bool {
	switch command {
	case "add", "login", "login-all", "cli", "exec", "status", "reconcile", "heartbeat", "monitor", "dry-run":
		return true
	default:
		return false
	}
}

func rejectLegacyProductEnvironment(env []string) error {
	names := make([]string, 0, 4)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "MULTICODEX_") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return &ExitError{
		Code:    2,
		Message: fmt.Sprintf("legacy MULTICODEX_* environment variable(s) are set: %s; clear them before running multisubs", strings.Join(names, ", ")),
	}
}

func printVersion() {
	fmt.Printf("%s %s\n", appName, version())
}

func (a *App) loadOrInitConfig() (*Config, error) {
	cfg, err := a.store.Load()
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		cfg = DefaultConfig()
		if err := a.store.EnsureBaseDirs(); err != nil {
			return nil, err
		}
		if err := a.store.Save(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	return nil, err
}

func (a *App) loadConfigIfExists() (*Config, error) {
	cfg, err := a.store.Load()
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	return nil, err
}

func (a *App) cmdInit() error {
	var cfg *Config
	created := false
	if err := a.store.WithConfigLock(func() error {
		loaded, err := a.store.Load()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				cfg = DefaultConfig()
				if err := a.store.Save(cfg); err != nil {
					return err
				}
				created = true
				return nil
			}
			return err
		}
		cfg = loaded
		return nil
	}); err != nil {
		return err
	}
	if created {
		fmt.Println("initialized multisubs local state")
		fmt.Printf("home: %s\n", a.store.paths.MultisubsHome)
		return nil
	}

	fmt.Println("multisubs already initialized")
	fmt.Printf("home: %s\n", a.store.paths.MultisubsHome)
	fmt.Printf("profiles: %d\n", len(cfg.Profiles))
	return nil
}

func (a *App) cmdAdd(args []string) error {
	if len(args) != 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex add <name>"}
	}
	name := strings.TrimSpace(args[0])
	if err := ValidateCodexProfileName(name); err != nil {
		return &ExitError{Code: 2, Message: err.Error()}
	}

	var profile Profile
	var resourceChanges []ResourceChange
	if err := a.store.WithConfigLock(func() error {
		cfg, err := a.loadOrInitConfig()
		if err != nil {
			return err
		}

		if _, exists := cfg.Profiles[name]; exists {
			return &ExitError{Code: 2, Message: fmt.Sprintf("profile already exists: %s", name)}
		}

		profile, resourceChanges, err = a.store.CreateProfile(name, cfg.ProfileResources)
		if err != nil {
			return err
		}
		cfg.Profiles[name] = profile
		if err := a.store.Save(cfg); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("added profile: %s\n", name)
	fmt.Printf("codex home: %s\n", profile.CodexHome)
	printResourceChanges(resourceChanges)
	return nil
}

func (a *App) cmdLogin(args []string) error {
	if len(args) == 1 && isHelpFlag(args[0]) {
		return a.cmdHelp([]string{"codex", "login"})
	}
	if handled, err := runCodexLoginHelpFastPath(args); handled {
		return err
	}
	if len(args) < 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex login <name> [codex login args]"}
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
	resourceChanges, err := a.store.EnsureProfileDir(profile, cfg.ProfileResources)
	if err != nil {
		return err
	}
	printResourceChanges(resourceChanges)
	if err := ensureLoginConfigReady(a.store.paths, profile); err != nil {
		return err
	}
	if err := secureAuthFilePermissions(profile.CodexHome); err != nil {
		return err
	}

	fmt.Printf("logging in profile %q\n", name)
	if err := RunCodexLogin(profile.CodexHome, args[1:]); err != nil {
		return err
	}

	hasAuth, err := HasAuthFile(profile.CodexHome)
	if err != nil {
		return err
	}
	if hasAuth {
		if err := secureAuthFilePermissions(profile.CodexHome); err != nil {
			return err
		}
		fmt.Println("login complete")
	} else {
		fmt.Println("login command completed. auth file not detected. this may indicate keychain mode or an incomplete login")
	}
	return nil
}

func runCodexLoginHelpFastPath(args []string) (bool, error) {
	helpFlag, hasTargetScopedHelp, exact := targetScopedLoginHelp(args)
	if !hasTargetScopedHelp {
		return false, nil
	}
	if !exact {
		return true, &ExitError{Code: 2, Message: "usage: multisubs codex login <name> [codex login args]"}
	}
	return true, runCommandWithEnv(
		"codex",
		[]string{"login", helpFlag},
		neutralCodexEnv(os.Environ()),
		"Codex login help command failed",
	)
}

func targetScopedLoginHelp(args []string) (string, bool, bool) {
	if len(args) == 1 && isHelpFlag(args[0]) {
		return "", false, false
	}
	hasHelp := false
	for _, arg := range args {
		if isHelpFlag(arg) {
			hasHelp = true
			break
		}
	}
	if !hasHelp {
		return "", false, false
	}
	if len(args) == 2 && !isHelpFlag(args[0]) && isHelpFlag(args[1]) {
		return args[1], true, true
	}
	return "", true, false
}

func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func ensureLoginConfigReady(paths Paths, profile Profile) error {
	return ensureProfileCodexExecutionReady(paths, profile)
}

func ensureProfileCodexExecutionReady(paths Paths, profile Profile) error {
	if err := NewStore(paths).ensureProfileStoragePathSafe(profile); err != nil {
		return err
	}
	if _, _, err := ensureProfileAuthPathSafe(profile.CodexHome); err != nil {
		return err
	}
	configPath := filepath.Join(profile.CodexHome, "config.toml")
	defaultConfigPath := filepath.Join(paths.DefaultCodexHome, "config.toml")
	if _, err := codexstate.ValidateManagedConfigPath(configPath, defaultConfigPath); err != nil {
		return fmt.Errorf("validate managed profile config: %w", err)
	}
	ok, err := profileConfigUsesFileStore(configPath, defaultConfigPath)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return &ExitError{
		Code: 2,
		Message: fmt.Sprintf(
			"profile %q requires file-backed auth to keep auth isolated. set cli_auth_credentials_store = \"file\" in %s or create a per-profile override at %s",
			profile.Name,
			filepath.Join(paths.DefaultCodexHome, "config.toml"),
			configPath,
		),
	}
}

func (a *App) cmdLoginAll() error {
	cfg, err := a.loadOrInitConfig()
	if err != nil {
		return err
	}
	if len(cfg.Profiles) == 0 {
		return &ExitError{Code: 2, Message: "no profiles configured. add one with: multisubs codex add <name>"}
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	failed := 0
	for _, name := range names {
		fmt.Printf("\n== %s ==\n", name)
		if err := a.cmdLogin([]string{name}); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "login failed for %s: %v\n", name, err)
		}
	}
	if failed > 0 {
		return &ExitError{Code: 1, Message: fmt.Sprintf("login-all completed with %d failure(s)", failed)}
	}
	fmt.Println("login-all completed")
	return nil
}

func (a *App) cmdStatus() error {
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		return err
	}
	return PrintStatus(a.store, cfg)
}

func (a *App) cmdDoctor(args []string) error {
	jsonOutput, timeout, err := parseDoctorArguments(args, "multisubs codex doctor")
	if err != nil {
		return err
	}
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		return err
	}
	report := RunCodexDoctor(a.store, cfg, timeout)
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printDoctorHuman("multisubs codex doctor", report)
	}
	if report.HasFailures() {
		return &ExitError{Code: 1, Message: "Codex doctor checks failed"}
	}
	return nil
}

func (a *App) cmdAggregateDoctor(args []string) error {
	jsonOutput, timeout, err := parseDoctorArguments(args, "multisubs doctor")
	if err != nil {
		return err
	}
	cfg, registryErr := a.loadConfigIfExists()
	if registryErr != nil {
		cfg = DefaultConfig()
	}
	claudeReport, err := a.runClaudeDoctorChecks(timeout)
	if err != nil {
		claudeReport = DoctorReport{Checks: []DoctorCheck{{
			Name:    "Claude setup",
			Status:  "fail",
			Details: err.Error(),
		}}}
	}
	report := AggregateDoctorReport{
		Base:   runBaseDoctor(a.store, cfg, registryErr),
		Codex:  RunCodexDoctor(a.store, cfg, timeout),
		Claude: claudeReport,
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printAggregateDoctorHuman(report)
	}
	if report.HasFailures() {
		return &ExitError{Code: 1, Message: "aggregate doctor checks failed"}
	}
	return nil
}

func parseDoctorArguments(args []string, command string) (bool, time.Duration, error) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "output doctor report as JSON")
	timeout := fs.Duration("timeout", 8*time.Second, "timeout for command checks")
	if err := fs.Parse(args); err != nil {
		return false, 0, &ExitError{Code: 2, Message: "usage: " + command + " [--json] [--timeout 8s]"}
	}
	if fs.NArg() != 0 {
		return false, 0, &ExitError{Code: 2, Message: "usage: " + command + " [--json] [--timeout 8s]"}
	}
	if *timeout <= 0 {
		return false, 0, &ExitError{Code: 2, Message: "error: --timeout must be > 0"}
	}
	return *jsonOutput, *timeout, nil
}

func (a *App) cmdDryRun(args []string) error {
	cfg, err := a.loadConfigIfExists()
	if err != nil {
		return err
	}
	text, err := RenderDryRun(a.store, cfg, args)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}
