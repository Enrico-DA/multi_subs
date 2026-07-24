package multisubs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/codexstate"
	"github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

const (
	execSelectionTimeout    = 10 * time.Second
	envSelectedProfilePath  = "MULTISUBS_SELECTED_PROFILE_PATH"
	defaultExecAccountLabel = codexDefaultAccountName
)

type execAccountSelector func(context.Context, []usage.MonitorAccount, string) (usage.SelectedAccount, error)

type execSelectionMetadata struct {
	Profile           string `json:"profile"`
	SelectionSource   string `json:"selection_source,omitempty"`
	WeeklyUsedPercent *int   `json:"weekly_used_percent,omitempty"`
}

type execSelection struct {
	Name      string
	CodexHome string
	IsProfile bool
	Profile   Profile
	Metadata  execSelectionMetadata
}

var defaultExecAccountSelector execAccountSelector = func(ctx context.Context, accounts []usage.MonitorAccount, model string) (usage.SelectedAccount, error) {
	return usage.SelectBestAccountForModel(ctx, accounts, model)
}

func (a *App) cmdExec(args []string) error {
	if execArgsAreHelpRequest(args) {
		return runCommandWithEnv("codex", append([]string{"exec"}, args...), neutralCodexEnv(os.Environ()), fmt.Sprintf("command failed: %s", strings.Join(append([]string{"codex", "exec"}, args...), " ")))
	}
	routingArgs, err := parseExecRoutingArgs(args)
	if err != nil {
		return &ExitError{Code: 2, Message: err.Error()}
	}
	if routingArgs.ProfileExplicit && !routingArgs.ModelExplicit {
		return &ExitError{
			Code:    2,
			Message: "Codex --profile can change the effective model; pass --model <model> so multisubs can route the matching weekly quota",
		}
	}

	cfg, err := a.loadOrInitConfig()
	if err != nil {
		return err
	}
	cfg, err = a.execReadyConfig(cfg)
	if err != nil {
		return err
	}
	model := routingArgs.Model
	if !routingArgs.ModelExplicit {
		model, err = commonConfiguredExecModel(a.store.paths, cfg)
		if err != nil {
			return err
		}
	}

	selected, err := a.selectExecProfile(cfg, defaultExecAccountSelector, model)
	if err != nil {
		return err
	}
	if selected.IsProfile {
		if err := ensureProfileCodexExecutionReady(a.store.paths, selected.Profile); err != nil {
			return err
		}
	}
	if err := writeSelectedProfileMetadata(a.store.paths, os.Getenv(envSelectedProfilePath), selected.Metadata); err != nil {
		return err
	}

	activeProfile := selected.Name
	if !selected.IsProfile {
		activeProfile = ""
	}
	return RunCodexWithProfile(selected.CodexHome, activeProfile, append([]string{"exec"}, args...))
}

func (a *App) execReadyConfig(cfg *Config) (*Config, error) {
	ready := DefaultConfig()
	ready.ProfileResources = cfg.ProfileResources
	for _, name := range sortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		changes, err := a.store.EnsureProfileDir(profile, cfg.ProfileResources)
		if err != nil {
			return nil, err
		}
		printResourceChangesToStderr(changes)
		if err := ensureProfileCodexExecutionReady(a.store.paths, profile); err != nil {
			return nil, err
		}
		ready.Profiles[name] = profile
	}
	return ready, nil
}

func writeSelectedProfileMetadata(paths Paths, path string, metadata execSelectionMetadata) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	runtimeRoot := selectedProfileMetadataRoot(paths)
	resolvedPath, err := resolvePathInsideRoot(runtimeRoot, path, "selected profile metadata path")
	if err != nil {
		return err
	}
	path = resolvedPath
	metadata.Profile = strings.TrimSpace(metadata.Profile)
	metadata.SelectionSource = strings.TrimSpace(metadata.SelectionSource)
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal selected profile metadata: %w", err)
	}
	if err := ensurePathNotSymlinkIfExists(paths.MultisubsHome); err != nil {
		return err
	}
	if err := ensurePathNotSymlinkIfExists(runtimeRoot); err != nil {
		return err
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(paths.MultisubsHome, filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create selected profile metadata dir: %w", err)
	}
	if err := ensurePathNotSymlinkIfExists(runtimeRoot); err != nil {
		return err
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(paths.MultisubsHome, filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		return fmt.Errorf("secure selected profile metadata root permissions: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure selected profile metadata dir permissions: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("selected profile metadata path is a symlink: %s", path)
		}
		if fileHasMultipleLinks(info) {
			return fmt.Errorf("selected profile metadata path has multiple hard links: %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("selected profile metadata path is not a regular file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect selected profile metadata: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.")
	if err != nil {
		return fmt.Errorf("create selected profile metadata temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmpClosed := false
	defer func() {
		if !tmpClosed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("set selected profile metadata permissions: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write selected profile metadata: %w", err)
	}
	if err := tmp.Close(); err != nil {
		tmpClosed = true
		return fmt.Errorf("close selected profile metadata: %w", err)
	}
	tmpClosed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace selected profile metadata: %w", err)
	}
	return nil
}

func selectedProfileMetadataRoot(paths Paths) string {
	return filepath.Join(paths.MultisubsHome, "run")
}

func execArgsAreHelpRequest(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "-h", "--help":
			return true
		}
	}
	return len(args) == 1 && args[0] == "help"
}

func (a *App) selectExecProfile(cfg *Config, selector execAccountSelector, model string) (execSelection, error) {
	names := sortedProfileNames(cfg)
	accounts := make([]usage.MonitorAccount, 0, len(names))
	for _, name := range names {
		profile := cfg.Profiles[name]
		accounts = append(accounts, usage.MonitorAccount{
			Label:        name,
			CodexHome:    profile.CodexHome,
			UseAppServer: true,
		})
	}
	defaultHome := normalizeExecCodexHome(a.store.paths.DefaultCodexHome)
	if defaultHome != "" && !execAccountsContainHome(accounts, defaultHome) {
		accounts = append(accounts, usage.MonitorAccount{
			Label:     defaultExecAccountLabel,
			CodexHome: defaultHome,
		})
	}

	if selector == nil {
		return execSelection{}, fmt.Errorf("missing exec account selector")
	}

	ctx, cancel := context.WithTimeout(context.Background(), execSelectionTimeout)
	defer cancel()

	selected, err := selector(ctx, accounts, model)
	if err != nil {
		return execSelection{}, err
	}
	if name, profile, ok := lookupSelectedExecProfile(cfg, selected); ok {
		metadata := execSelectionMetadata{
			Profile:           name,
			SelectionSource:   "usage_selector",
			WeeklyUsedPercent: availableUsedPercentPtr(selected.WeeklyUsedPercent),
		}
		return execSelection{Name: name, CodexHome: profile.CodexHome, IsProfile: true, Profile: profile, Metadata: metadata}, nil
	}
	if home, ok := lookupDefaultExecAccount(a.store.paths, selected); ok {
		metadata := execSelectionMetadata{
			Profile:           defaultExecAccountLabel,
			SelectionSource:   "usage_selector_default",
			WeeklyUsedPercent: availableUsedPercentPtr(selected.WeeklyUsedPercent),
		}
		return execSelection{Name: defaultExecAccountLabel, CodexHome: home, Metadata: metadata}, nil
	}
	return execSelection{}, fmt.Errorf("selected account %q is not an exec candidate", selected.Account.Label)
}

func execAccountsContainHome(accounts []usage.MonitorAccount, home string) bool {
	normalized := normalizeExecCodexHome(home)
	if normalized == "" {
		return false
	}
	for _, account := range accounts {
		if normalizeExecCodexHome(account.CodexHome) == normalized {
			return true
		}
	}
	return false
}

func lookupDefaultExecAccount(paths Paths, selected usage.SelectedAccount) (string, bool) {
	defaultHome := normalizeExecCodexHome(paths.DefaultCodexHome)
	selectedHome := normalizeExecCodexHome(selected.Account.CodexHome)
	if defaultHome == "" || selectedHome == "" || selectedHome != defaultHome {
		return "", false
	}
	return defaultHome, true
}

func parseModelFromExecArgs(args []string) string {
	routingArgs, err := parseExecRoutingArgs(args)
	if err != nil {
		return ""
	}
	return routingArgs.Model
}

type execRoutingArgs struct {
	Model           string
	ModelExplicit   bool
	ProfileExplicit bool
}

func parseExecRoutingArgs(args []string) (execRoutingArgs, error) {
	var parsed execRoutingArgs
	dedicatedModel := ""
	dedicatedModelFound := false
	configModel := ""
	configModelFound := false

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if arg == "--" {
			break
		}
		switch {
		case arg == "--model" || arg == "-m":
			if dedicatedModelFound {
				return execRoutingArgs{}, errors.New("Codex accepts one explicit model flag; pass a single --model <model>")
			}
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" || args[i+1] == "--" {
				return execRoutingArgs{}, errors.New("Codex model flag is missing its value; pass --model <model>")
			}
			dedicatedModel = strings.TrimSpace(args[i+1])
			dedicatedModelFound = true
			i++
		case strings.HasPrefix(arg, "--model="):
			if dedicatedModelFound {
				return execRoutingArgs{}, errors.New("Codex accepts one explicit model flag; pass a single --model <model>")
			}
			dedicatedModel = strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
			if dedicatedModel == "" {
				return execRoutingArgs{}, errors.New("Codex model flag is missing its value; pass --model <model>")
			}
			dedicatedModelFound = true
		case strings.HasPrefix(arg, "-m="):
			if dedicatedModelFound {
				return execRoutingArgs{}, errors.New("Codex accepts one explicit model flag; pass a single --model <model>")
			}
			dedicatedModel = strings.TrimSpace(strings.TrimPrefix(arg, "-m="))
			if dedicatedModel == "" {
				return execRoutingArgs{}, errors.New("Codex model flag is missing its value; pass --model <model>")
			}
			dedicatedModelFound = true
		case arg == "-c" || arg == "--config":
			if i+1 >= len(args) || args[i+1] == "--" {
				return execRoutingArgs{}, errors.New("Codex config override is missing key=value")
			}
			i++
			model, found, err := codexstate.ModelFromConfigOverride(args[i])
			if err != nil {
				return execRoutingArgs{}, errors.New("Codex model config override is invalid; pass --model <model>")
			}
			if found {
				configModel = strings.TrimSpace(model)
				configModelFound = true
			}
		case strings.HasPrefix(arg, "--config="):
			model, found, err := codexstate.ModelFromConfigOverride(strings.TrimPrefix(arg, "--config="))
			if err != nil {
				return execRoutingArgs{}, errors.New("Codex model config override is invalid; pass --model <model>")
			}
			if found {
				configModel = strings.TrimSpace(model)
				configModelFound = true
			}
		case strings.HasPrefix(arg, "-c") && len(arg) > len("-c"):
			model, found, err := codexstate.ModelFromConfigOverride(strings.TrimPrefix(arg, "-c"))
			if err != nil {
				return execRoutingArgs{}, errors.New("Codex model config override is invalid; pass --model <model>")
			}
			if found {
				configModel = strings.TrimSpace(model)
				configModelFound = true
			}
		case arg == "--profile" || arg == "-p":
			parsed.ProfileExplicit = true
			if i+1 < len(args) && args[i+1] != "--" {
				i++
			}
		case strings.HasPrefix(arg, "--profile=") || strings.HasPrefix(arg, "-p=") ||
			(strings.HasPrefix(arg, "-p") && len(arg) > len("-p")):
			parsed.ProfileExplicit = true
		}
	}

	if dedicatedModelFound {
		parsed.Model = dedicatedModel
		parsed.ModelExplicit = true
		return parsed, nil
	}
	if configModelFound {
		if configModel == "" {
			return execRoutingArgs{}, errors.New("Codex model config override is empty; pass --model <model>")
		}
		parsed.Model = configModel
		parsed.ModelExplicit = true
	}
	return parsed, nil
}

func commonConfiguredExecModel(paths Paths, cfg *Config) (string, error) {
	type modelConfigCandidate struct {
		path              string
		defaultConfigPath string
		managed           bool
	}
	defaultConfigPath := filepath.Join(paths.DefaultCodexHome, "config.toml")
	candidates := []modelConfigCandidate{{path: defaultConfigPath}}
	for _, name := range sortedProfileNames(cfg) {
		candidates = append(candidates, modelConfigCandidate{
			path:              filepath.Join(cfg.Profiles[name].CodexHome, "config.toml"),
			defaultConfigPath: defaultConfigPath,
			managed:           true,
		})
	}

	commonModel := ""
	commonFound := false
	haveCandidate := false
	for _, candidate := range candidates {
		var (
			model string
			found bool
			err   error
		)
		if candidate.managed {
			model, found, err = configuredManagedExecModel(candidate.path, candidate.defaultConfigPath)
		} else {
			model, found, err = configuredExecModel(candidate.path)
		}
		if err != nil {
			return "", &ExitError{
				Code:    2,
				Message: "could not safely determine the configured Codex model for every routing candidate; pass --model <model>",
			}
		}
		if !haveCandidate {
			commonModel = model
			commonFound = found
			haveCandidate = true
			continue
		}
		if found != commonFound || (found && model != commonModel) {
			return "", &ExitError{
				Code:    2,
				Message: "Codex routing candidates have different configured models; pass --model <model>",
			}
		}
	}
	if !commonFound {
		return "", nil
	}
	return commonModel, nil
}

func configuredManagedExecModel(configPath, defaultConfigPath string) (string, bool, error) {
	if _, err := codexstate.ValidateManagedConfigPath(configPath, defaultConfigPath); err != nil {
		return "", false, errors.New("managed Codex config path is invalid")
	}
	return readConfiguredExecModel(configPath)
}

func configuredExecModel(configPath string) (string, bool, error) {
	if err := ensureRegularFileOrSymlinkTarget(configPath, "Codex config"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, errors.New("Codex config is not a readable regular file")
	}
	return readConfiguredExecModel(configPath)
}

func readConfiguredExecModel(configPath string) (string, bool, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, errors.New("Codex config could not be read")
	}
	model, found, err := codexstate.ModelFromTOML(string(content))
	if err != nil {
		return "", false, errors.New("Codex config model could not be parsed")
	}
	model = strings.TrimSpace(model)
	if found && model == "" {
		return "", false, errors.New("Codex config model is empty")
	}
	return model, found, nil
}

func availableUsedPercentPtr(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

func lookupSelectedExecProfile(cfg *Config, selected usage.SelectedAccount) (string, Profile, bool) {
	if name := strings.TrimSpace(selected.Account.Label); name != "" {
		if name == codexDefaultAccountName {
			return "", Profile{}, false
		}
		if profile, ok := cfg.Profiles[name]; ok {
			return name, profile, true
		}
	}

	selectedHome := normalizeExecCodexHome(selected.Account.CodexHome)
	if selectedHome == "" {
		return "", Profile{}, false
	}

	for _, name := range sortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		if normalizeExecCodexHome(profile.CodexHome) == selectedHome {
			return name, profile, true
		}
	}
	return "", Profile{}, false
}

func normalizeExecCodexHome(home string) string {
	trimmed := strings.TrimSpace(home)
	if trimmed == "" {
		return ""
	}
	normalized := filepath.Clean(trimmed)
	if abs, err := filepath.Abs(normalized); err == nil {
		normalized = abs
	}
	if resolved, err := filepath.EvalSymlinks(normalized); err == nil && strings.TrimSpace(resolved) != "" {
		normalized = resolved
	}
	return filepath.Clean(normalized)
}
