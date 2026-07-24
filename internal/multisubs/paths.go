package multisubs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Paths centralizes filesystem locations used by multisubs.
type Paths struct {
	MultisubsHome     string
	ConfigPath        string
	ProfilesDir       string
	DefaultCodexHome  string
	ClaudeProviderDir string
	ClaudeConfigPath  string
	ClaudeProfilesDir string
	ClaudeRunDir      string
}

func ResolvePaths() (Paths, error) {
	return resolvePaths()
}

func ResolvePathsReadOnly() (Paths, error) {
	return resolvePaths()
}

func resolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}

	defaultMultisubsHome := filepath.Join(home, "multisubs")
	defaultCodexHome, err := resolveConfiguredPath(os.Getenv("MULTISUBS_DEFAULT_CODEX_HOME"), home)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve MULTISUBS_DEFAULT_CODEX_HOME: %w", err)
	}
	if defaultCodexHome == "" {
		defaultCodexHome = filepath.Join(home, ".codex")
	}
	multisubsHome, err := resolveConfiguredPath(os.Getenv("MULTISUBS_HOME"), home)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve MULTISUBS_HOME: %w", err)
	}
	if multisubsHome == "" {
		multisubsHome = defaultMultisubsHome
	}

	claudeProviderDir := filepath.Join(multisubsHome, "providers", "claude")
	return Paths{
		MultisubsHome:     multisubsHome,
		ConfigPath:        filepath.Join(multisubsHome, "config.json"),
		ProfilesDir:       filepath.Join(multisubsHome, "profiles"),
		DefaultCodexHome:  defaultCodexHome,
		ClaudeProviderDir: claudeProviderDir,
		ClaudeConfigPath:  filepath.Join(claudeProviderDir, "config.json"),
		ClaudeProfilesDir: filepath.Join(claudeProviderDir, "profiles"),
		ClaudeRunDir:      filepath.Join(claudeProviderDir, "run"),
	}, nil
}

func resolveConfiguredPath(value, home string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value == "~" {
		value = home
	} else if strings.HasPrefix(value, "~/") {
		value = filepath.Join(home, value[2:])
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
