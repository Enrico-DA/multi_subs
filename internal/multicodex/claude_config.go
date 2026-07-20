package multicodex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	claudeConfigVersion = 1
	claudeDefaultTarget = "default"
)

// claudeConfig is a provider-owned sidecar. The legacy config.json remains
// exclusively owned by the Codex implementation.
type claudeConfig struct {
	Version  int                      `json:"version"`
	Profiles map[string]claudeProfile `json:"profiles"`
}

type claudeProfile struct {
	Name      string `json:"name"`
	ConfigDir string `json:"config_dir"`
}

type claudeStore struct {
	paths Paths
}

func newClaudeStore(paths Paths) *claudeStore {
	return &claudeStore{paths: withClaudePaths(paths)}
}

func withClaudePaths(paths Paths) Paths {
	if paths.ClaudeProviderDir == "" {
		paths.ClaudeProviderDir = filepath.Join(paths.MulticodexHome, "providers", "claude")
	}
	if paths.ClaudeConfigPath == "" {
		paths.ClaudeConfigPath = filepath.Join(paths.ClaudeProviderDir, "config.json")
	}
	if paths.ClaudeProfilesDir == "" {
		paths.ClaudeProfilesDir = filepath.Join(paths.ClaudeProviderDir, "profiles")
	}
	if paths.ClaudeRunDir == "" {
		paths.ClaudeRunDir = filepath.Join(paths.ClaudeProviderDir, "run")
	}
	return paths
}

func defaultClaudeConfig() *claudeConfig {
	return &claudeConfig{Version: claudeConfigVersion, Profiles: map[string]claudeProfile{}}
}

func (s *claudeStore) LoadIfExists() (*claudeConfig, error) {
	info, err := os.Lstat(s.paths.ClaudeConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := s.validateProviderTreeIfPresent(); err != nil {
				return nil, err
			}
			return defaultClaudeConfig(), nil
		}
		return nil, fmt.Errorf("inspect Claude sidecar: %w", err)
	}
	if err := validatePrivateRegularFileInfo(s.paths.ClaudeConfigPath, "Claude sidecar", info); err != nil {
		return nil, err
	}
	if err := s.validateProviderTree(); err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(s.paths.ClaudeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read Claude sidecar: %w", err)
	}
	cfg := &claudeConfig{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse Claude sidecar: %w", err)
	}
	if cfg.Version != claudeConfigVersion {
		return nil, fmt.Errorf("unsupported Claude sidecar version %d (supported: %d)", cfg.Version, claudeConfigVersion)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]claudeProfile{}
	}
	for name, profile := range cfg.Profiles {
		if err := validateClaudeProfileName(name); err != nil {
			return nil, fmt.Errorf("invalid stored Claude profile name %q: %w", name, err)
		}
		if profile.Name != name {
			return nil, fmt.Errorf("stored Claude profile %q has mismatched name %q", name, profile.Name)
		}
		if err := s.validateProfileStoragePath(profile); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (s *claudeStore) Save(cfg *claudeConfig) error {
	if cfg == nil {
		return errors.New("Claude sidecar config is nil")
	}
	if err := s.EnsureBaseDirs(); err != nil {
		return err
	}
	if err := ensurePrivateRegularFileForWrite(s.paths.ClaudeConfigPath, "Claude sidecar"); err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]claudeProfile{}
	}
	for name, profile := range cfg.Profiles {
		if name != profile.Name {
			return fmt.Errorf("stored Claude profile %q has mismatched name %q", name, profile.Name)
		}
		if err := s.validateProfileStoragePath(profile); err != nil {
			return err
		}
	}
	cfg.Version = claudeConfigVersion
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Claude sidecar: %w", err)
	}

	tmp, err := os.CreateTemp(s.paths.ClaudeProviderDir, "config.json.tmp.")
	if err != nil {
		return fmt.Errorf("create Claude sidecar temp file: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure Claude sidecar temp file: %w", err)
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write Claude sidecar temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close Claude sidecar temp file: %w", err)
	}
	closed = true
	if err := os.Rename(tmpPath, s.paths.ClaudeConfigPath); err != nil {
		return fmt.Errorf("replace Claude sidecar: %w", err)
	}
	return nil
}

func (s *claudeStore) WithConfigLock(fn func() error) error {
	if err := s.EnsureBaseDirs(); err != nil {
		return err
	}
	lockPath := filepath.Join(s.paths.ClaudeProviderDir, "config.lock")
	if err := ensurePrivateRegularFileForWrite(lockPath, "Claude config lock"); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open Claude config lock: %w", err)
	}
	defer lockFile.Close()
	if err := lockFile.Chmod(0o600); err != nil {
		return fmt.Errorf("secure Claude config lock: %w", err)
	}
	info, err := lockFile.Stat()
	if err != nil {
		return fmt.Errorf("inspect Claude config lock: %w", err)
	}
	if err := validatePrivateRegularFileInfo(lockPath, "Claude config lock", info); err != nil {
		return err
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock Claude config: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func (s *claudeStore) EnsureBaseDirs() error {
	providersDir := filepath.Dir(s.paths.ClaudeProviderDir)
	paths := []string{s.paths.MulticodexHome, providersDir, s.paths.ClaudeProviderDir, s.paths.ClaudeProfilesDir}
	for _, path := range paths {
		if err := ensureClaudePrivateDir(path, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *claudeStore) CreateProfile(name string) (claudeProfile, error) {
	if err := validateClaudeProfileName(name); err != nil {
		return claudeProfile{}, err
	}
	if err := s.EnsureBaseDirs(); err != nil {
		return claudeProfile{}, err
	}
	profile := claudeProfile{
		Name:      name,
		ConfigDir: filepath.Join(s.paths.ClaudeProfilesDir, name, "config"),
	}
	if err := s.validateProfileStoragePath(profile); err != nil {
		return claudeProfile{}, err
	}
	for _, path := range []string{filepath.Dir(profile.ConfigDir), profile.ConfigDir} {
		if err := ensureClaudePrivateDir(path, true); err != nil {
			return claudeProfile{}, err
		}
	}
	return profile, nil
}

func (s *claudeStore) EnsureProfileReady(profile claudeProfile) error {
	if err := s.validateProfileStoragePath(profile); err != nil {
		return err
	}
	info, err := os.Lstat(profile.ConfigDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Claude profile %q config directory is missing: %s", profile.Name, profile.ConfigDir)
		}
		return fmt.Errorf("inspect Claude profile %q config directory: %w", profile.Name, err)
	}
	if err := validatePrivateDirectoryInfo(profile.ConfigDir, "Claude profile config directory", info); err != nil {
		return err
	}
	return nil
}

func (s *claudeStore) validateProviderTreeIfPresent() error {
	info, err := os.Lstat(s.paths.ClaudeProviderDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect Claude provider directory: %w", err)
	}
	if err := validatePrivateDirectoryInfo(s.paths.ClaudeProviderDir, "Claude provider directory", info); err != nil {
		return err
	}
	return s.validateProviderTree()
}

func (s *claudeStore) validateProviderTree() error {
	providersDir := filepath.Dir(s.paths.ClaudeProviderDir)
	for _, path := range []string{s.paths.MulticodexHome, providersDir, s.paths.ClaudeProviderDir, s.paths.ClaudeProfilesDir} {
		if err := ensureClaudePrivateDir(path, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *claudeStore) validateProfileStoragePath(profile claudeProfile) error {
	if err := validateClaudeProfileName(profile.Name); err != nil {
		return fmt.Errorf("invalid Claude profile name %q: %w", profile.Name, err)
	}
	expected := filepath.Join(s.paths.ClaudeProfilesDir, profile.Name, "config")
	if profile.ConfigDir == "" {
		return fmt.Errorf("Claude profile %q config directory is empty", profile.Name)
	}
	if filepath.Clean(profile.ConfigDir) != profile.ConfigDir {
		return fmt.Errorf("Claude profile %q config directory is not a clean path: %s", profile.Name, profile.ConfigDir)
	}
	if !sameProfilePath(profile.ConfigDir, expected) {
		return fmt.Errorf("Claude profile %q config directory %s does not match derived path %s", profile.Name, profile.ConfigDir, expected)
	}
	rel, err := filepath.Rel(s.paths.ClaudeProfilesDir, profile.ConfigDir)
	if err != nil || rel != filepath.Join(profile.Name, "config") {
		return fmt.Errorf("Claude profile %q config directory must stay under %s", profile.Name, s.paths.ClaudeProfilesDir)
	}
	for _, path := range []string{s.paths.MulticodexHome, filepath.Dir(s.paths.ClaudeProviderDir), s.paths.ClaudeProviderDir, s.paths.ClaudeProfilesDir, filepath.Dir(expected), expected} {
		if err := ensureClaudePrivateDir(path, false); err != nil {
			return err
		}
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(s.paths.MulticodexHome, expected); err != nil {
		return fmt.Errorf("unsafe Claude profile path: %w", err)
	}
	return nil
}

func validateClaudeProfileName(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if name == claudeDefaultTarget {
		return fmt.Errorf("profile name %q is reserved for the built-in Claude default account", name)
	}
	return nil
}

func sortedClaudeProfileNames(cfg *claudeConfig) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensureClaudePrivateDir(path string, create bool) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return errors.New("Claude provider directory path is empty")
	}
	info, err := os.Lstat(path)
	if err == nil {
		return validatePrivateDirectoryInfo(path, "Claude provider path", info)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Claude provider path %s: %w", path, err)
	}
	if !create {
		return nil
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create Claude provider path %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure Claude provider path %s: %w", path, err)
	}
	info, err = os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect created Claude provider path %s: %w", path, err)
	}
	return validatePrivateDirectoryInfo(path, "Claude provider path", info)
}

func validatePrivateDirectoryInfo(path, label string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s permissions are %o, expected no group/world permissions: %s", label, info.Mode().Perm(), path)
	}
	return nil
}

func validatePrivateRegularFileInfo(path, label string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink: %s", label, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file: %s", label, path)
	}
	if fileHasMultipleLinks(info) {
		return fmt.Errorf("%s has multiple hard links: %s", label, path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s permissions are %o, expected no group/world permissions: %s", label, info.Mode().Perm(), path)
	}
	return nil
}

func ensurePrivateRegularFileForWrite(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	return validatePrivateRegularFileInfo(path, label, info)
}
