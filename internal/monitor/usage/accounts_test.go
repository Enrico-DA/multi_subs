package usage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestLoadMonitorAccountsEmptyWhenFileMissingAndNoProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected no default monitor accounts, got %#v", accounts)
	}
}

func TestLoadMonitorAccountsIncludesOnlyGlobalHomeByDefault(t *testing.T) {
	tmp := t.TempDir()
	defaultHome := filepath.Join(tmp, ".codex")
	activeHome := filepath.Join(tmp, "active-codex")
	discoveredHome := filepath.Join(tmp, "profiles", "work", "codex-home")
	for _, home := range []string{defaultHome, activeHome, discoveredHome} {
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatalf("mkdir codex home: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
			t.Fatalf("write auth file: %v", err)
		}
	}
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", activeHome)
	t.Setenv(defaultCodexHomeEnvVar, defaultHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected only the global home by default, got %#v", accounts)
	}
	if accounts[0].Label != "global" || accounts[0].CodexHome != normalizeHome(defaultHome) {
		t.Fatalf("expected global account for %q, got %#v", normalizeHome(defaultHome), accounts[0])
	}
}

func TestLoadMonitorAccountsUsesConfiguredDefaultCodexHome(t *testing.T) {
	tmp := t.TempDir()
	configuredHome := filepath.Join(tmp, "custom-default-codex")
	if err := os.MkdirAll(configuredHome, 0o700); err != nil {
		t.Fatalf("mkdir configured home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configuredHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write configured auth: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(defaultCodexHomeEnvVar, configuredHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one default account, got %#v", accounts)
	}
	if accounts[0].Label != "global" {
		t.Fatalf("expected global label, got %q", accounts[0].Label)
	}
	if accounts[0].CodexHome != normalizeHome(configuredHome) {
		t.Fatalf("expected configured default codex home %q, got %q", normalizeHome(configuredHome), accounts[0].CodexHome)
	}
}

func TestLoadMonitorAccountsCanExcludeGlobalHome(t *testing.T) {
	tmp := t.TempDir()
	defaultHome := filepath.Join(tmp, ".codex")
	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatalf("mkdir default home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write default auth: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(defaultCodexHomeEnvVar, defaultHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, warning, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected the global home to be excluded, got %#v", accounts)
	}
}

func TestLoadMonitorAccountsIncludesActiveCodexHomeWhenRequested(t *testing.T) {
	tmp := t.TempDir()
	activeHome := filepath.Join(tmp, "active-codex")
	if err := os.MkdirAll(activeHome, 0o700); err != nil {
		t.Fatalf("mkdir active home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(activeHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write active auth file: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", activeHome)
	t.Setenv(defaultCodexHomeEnvVar, "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, warning, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{IncludeActive: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one active account, got %#v", accounts)
	}
	if accounts[0].Label != "active" {
		t.Fatalf("expected active CODEX_HOME label active, got %q", accounts[0].Label)
	}
	if accounts[0].CodexHome != normalizeHome(activeHome) {
		t.Fatalf("expected active codex home %q, got %q", normalizeHome(activeHome), accounts[0].CodexHome)
	}
}

func TestLoadMonitorAccountsPrefersConfiguredDefaultOverActiveCodexHome(t *testing.T) {
	tmp := t.TempDir()
	configuredHome := filepath.Join(tmp, "custom-default-codex")
	activeHome := filepath.Join(tmp, "stale-profile-codex")
	if err := os.MkdirAll(configuredHome, 0o700); err != nil {
		t.Fatalf("mkdir configured home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configuredHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write configured auth: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", activeHome)
	t.Setenv(defaultCodexHomeEnvVar, configuredHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{
		IncludeDefault: true,
		IncludeActive:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byLabel := map[string]string{}
	for _, account := range accounts {
		byLabel[account.Label] = account.CodexHome
	}
	expectedConfiguredHome := normalizeHome(configuredHome)
	if byLabel["global"] != expectedConfiguredHome {
		t.Fatalf("expected global account from configured home %q, got %q", expectedConfiguredHome, byLabel["global"])
	}
	expectedActiveHome := normalizeHome(activeHome)
	if byLabel["active"] != expectedActiveHome {
		t.Fatalf("expected active account from CODEX_HOME %q, got %q", expectedActiveHome, byLabel["active"])
	}
}

func TestLoadMonitorAccountsFromFileWithDedup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	accountsPath := filepath.Join(tmp, "accounts.json")
	t.Setenv(accountsFileEnvVar, accountsPath)

	content := `{
  "version": 1,
  "accounts": [
    {"label":"personal","codex_home":"~/codex/a"},
    {"label":"work","codex_home":"` + filepath.Join(tmp, "codex", "b") + `"},
    {"label":"dupe","codex_home":"` + filepath.Join(tmp, "codex", "b") + `"}
  ]
}`
	if err := os.WriteFile(accountsPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write accounts file: %v", err)
	}

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts after dedup, got %d", len(accounts))
	}
	if accounts[0].Label != "personal" {
		t.Fatalf("expected first label personal, got %q", accounts[0].Label)
	}
	if !strings.HasSuffix(accounts[0].CodexHome, filepath.Join("codex", "a")) {
		t.Fatalf("expected expanded home path, got %q", accounts[0].CodexHome)
	}
	if accounts[1].Label != "work" {
		t.Fatalf("expected second label work, got %q", accounts[1].Label)
	}
	for _, account := range accounts {
		if account.UseAppServer {
			t.Fatalf("expected unverified account-file entry not to use app-server: %#v", account)
		}
	}
}

func TestLoadMonitorAccountsWarnsOnEmptyAccounts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	accountsPath := filepath.Join(tmp, "accounts.json")
	t.Setenv(accountsFileEnvVar, accountsPath)
	if err := os.WriteFile(accountsPath, []byte(`{"version":1,"accounts":[]}`), 0o600); err != nil {
		t.Fatalf("write accounts file: %v", err)
	}

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected no fallback default account, got %#v", accounts)
	}
	if warning == "" {
		t.Fatalf("expected warning for empty accounts list")
	}
}

func TestLoadMonitorAccountsRejectsUnsupportedAccountsVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	accountsPath := filepath.Join(tmp, "accounts.json")
	t.Setenv(accountsFileEnvVar, accountsPath)
	if err := os.WriteFile(accountsPath, []byte(`{"version":2,"accounts":[]}`), 0o600); err != nil {
		t.Fatalf("write accounts file: %v", err)
	}

	_, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected aggregate load error: %v", err)
	}
	if !strings.Contains(warning, "unsupported accounts file version 2") {
		t.Fatalf("expected unsupported-version warning, got %q", warning)
	}
}

func TestLoadMonitorAccountsAutoDiscoversSystemCodexHomes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	discoveredHome := filepath.Join(tmp, "profiles", "work", "codex-home")
	if err := os.MkdirAll(discoveredHome, 0o755); err != nil {
		t.Fatalf("mkdir discovered home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(discoveredHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{Discover: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	expectedHome := normalizeHome(discoveredHome)
	for _, account := range accounts {
		if account.CodexHome == expectedHome {
			found = true
			if account.Label != "work" {
				t.Fatalf("expected discovered label work, got %q", account.Label)
			}
		}
	}
	if !found {
		t.Fatalf("expected discovered codex home to be included")
	}
}

func TestLoadMonitorAccountsSkipsTransientAutoDiscoveredHomes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	stableHome := filepath.Join(tmp, "profiles", "work", "codex-home")
	if err := os.MkdirAll(stableHome, 0o755); err != nil {
		t.Fatalf("mkdir stable home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stableHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write stable auth file: %v", err)
	}

	transientHome := filepath.Join(tmp, "loopy", "launches", "20260317T071323Z-ba3a94ce", "codex-home")
	if err := os.MkdirAll(transientHome, 0o755); err != nil {
		t.Fatalf("mkdir transient home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(transientHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write transient auth file: %v", err)
	}

	accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{Discover: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stableFound := false
	transientFound := false
	for _, account := range accounts {
		switch account.CodexHome {
		case normalizeHome(stableHome):
			stableFound = true
		case normalizeHome(transientHome):
			transientFound = true
		}
	}
	if !stableFound {
		t.Fatalf("expected stable discovered home to be included")
	}
	if transientFound {
		t.Fatalf("expected transient loopy launch home to be excluded")
	}
}

func TestLoadMonitorAccountsDiscoveryDoesNotDescendIntoSymlinkedDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	outsideHome := filepath.Join(t.TempDir(), "outside", "codex-home")
	if err := os.MkdirAll(outsideHome, 0o755); err != nil {
		t.Fatalf("mkdir outside home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write outside auth file: %v", err)
	}
	if err := os.Symlink(filepath.Dir(outsideHome), filepath.Join(tmp, "linked-outside")); err != nil {
		t.Fatalf("symlink outside dir: %v", err)
	}

	accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{Discover: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, account := range accounts {
		if account.CodexHome == normalizeHome(outsideHome) {
			t.Fatalf("expected symlinked outside home not to be discovered: %#v", accounts)
		}
	}
}

func TestLoadMonitorAccountsDiscoveryPrunesLargeCommonDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	prunedHome := filepath.Join(tmp, "node_modules", "nested", "codex-home")
	if err := os.MkdirAll(prunedHome, 0o755); err != nil {
		t.Fatalf("mkdir pruned home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prunedHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write pruned auth file: %v", err)
	}
	stableHome := filepath.Join(tmp, "profiles", "work", "codex-home")
	if err := os.MkdirAll(stableHome, 0o755); err != nil {
		t.Fatalf("mkdir stable home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stableHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write stable auth file: %v", err)
	}

	accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{Discover: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prunedFound := false
	stableFound := false
	for _, account := range accounts {
		switch account.CodexHome {
		case normalizeHome(prunedHome):
			prunedFound = true
		case normalizeHome(stableHome):
			stableFound = true
		}
	}
	if prunedFound {
		t.Fatalf("expected node_modules home not to be discovered")
	}
	if !stableFound {
		t.Fatalf("expected stable discovered home")
	}
}

func TestLoadMonitorAccountsDiscoveryNeverReadsLegacyProductRoots(t *testing.T) {
	for _, rootName := range []string{"multicodex", ".multicodex"} {
		rootName := rootName
		for _, alias := range []bool{false, true} {
			alias := alias
			name := rootName
			if alias {
				name += " symlink alias"
			}
			t.Run(name, func(t *testing.T) {
				tmp := t.TempDir()
				t.Setenv("HOME", tmp)
				t.Setenv("CODEX_HOME", "")
				t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
				t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

				legacyRoot := filepath.Join(tmp, rootName)
				discoveryRoot := legacyRoot
				if alias {
					discoveryRoot = filepath.Join(tmp, "legacy-target-"+strings.TrimPrefix(rootName, "."))
					if err := os.Symlink(discoveryRoot, legacyRoot); err != nil {
						t.Fatalf("symlink legacy root: %v", err)
					}
				}
				legacyHome := filepath.Join(discoveryRoot, "profiles", "old", "codex-home")
				if err := os.MkdirAll(filepath.Join(legacyHome, "sessions"), 0o700); err != nil {
					t.Fatalf("mkdir legacy session signals: %v", err)
				}
				if err := os.WriteFile(filepath.Join(legacyHome, "auth.json"), []byte(`{"tokens":{"access_token":"legacy"}}`), 0o600); err != nil {
					t.Fatalf("write legacy auth signal: %v", err)
				}

				stableHome := filepath.Join(tmp, "current", "work", "codex-home")
				if err := os.MkdirAll(filepath.Join(stableHome, "sessions"), 0o700); err != nil {
					t.Fatalf("mkdir stable session signals: %v", err)
				}

				accounts, _, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{Discover: true})
				if err != nil {
					t.Fatalf("discover accounts: %v", err)
				}
				stableFound := false
				for _, account := range accounts {
					if pathInsideRoot(account.CodexHome, discoveryRoot) {
						t.Fatalf("legacy Codex home was returned through %s: %#v", rootName, accounts)
					}
					if account.CodexHome == normalizeHome(stableHome) {
						stableFound = true
					}
				}
				if !stableFound {
					t.Fatalf("general filesystem discovery stopped outside the legacy root: %#v", accounts)
				}
			})
		}
	}
}

func TestAccountCollectorDeduplicatesSymlinkAndRealHomes(t *testing.T) {
	tmp := t.TempDir()
	realHome := filepath.Join(tmp, "profiles", "work", "codex-home")
	if err := os.MkdirAll(realHome, 0o755); err != nil {
		t.Fatalf("mkdir real home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	symlinkHome := filepath.Join(tmp, "symlink-home")
	if err := os.Symlink(realHome, symlinkHome); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	collector := newAccountCollector()
	collector.add("real", realHome, 50, false, false)
	collector.add("link", symlinkHome, 60, false, false)

	accounts := collector.toAccounts()
	if len(accounts) != 1 {
		t.Fatalf("expected one deduplicated account, got %d", len(accounts))
	}
	if accounts[0].Label != "link" {
		t.Fatalf("expected higher-priority symlink label to win, got %q", accounts[0].Label)
	}
}

func TestResolveAccountsFilePathPrefersDefaultDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, "")

	defaultDir := filepath.Join(tmp, defaultMultisubsHomeDirName, defaultMonitorSubdirName)
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatalf("mkdir default dir: %v", err)
	}
	defaultFile := filepath.Join(defaultDir, defaultAccountsFileName)
	if err := os.WriteFile(defaultFile, []byte(`{"version":1,"accounts":[]}`), 0o600); err != nil {
		t.Fatalf("write default accounts file: %v", err)
	}

	path, err := resolveAccountsFilePath()
	if err != nil {
		t.Fatalf("resolve accounts file path: %v", err)
	}
	if path != defaultFile {
		t.Fatalf("expected default path %q, got %q", defaultFile, path)
	}
}

func TestResolveAccountsFilePathDoesNotReadLegacyMonitorEnvironment(t *testing.T) {
	tmp := t.TempDir()
	legacyPath := filepath.Join(tmp, "legacy-accounts.json")
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, "")
	t.Setenv(accountsFileEnvVar, "")
	t.Setenv("CODEX_USAGE_MONITOR_ACCOUNTS_FILE", legacyPath)

	path, err := resolveAccountsFilePath()
	if err != nil {
		t.Fatalf("resolve accounts file path: %v", err)
	}
	want := filepath.Join(tmp, defaultMultisubsHomeDirName, defaultMonitorSubdirName, defaultAccountsFileName)
	if path != want {
		t.Fatalf("legacy monitor environment affected path: got=%q want=%q", path, want)
	}
}

func TestLoadMonitorAccountsPrefersMultisubsProfiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir multisubs dir: %v", err)
	}
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	found := false
	for _, account := range accounts {
		if account.Label == "personal" && account.CodexHome == normalizeHome(profileHome) {
			found = true
			if !account.UseAppServer {
				t.Fatalf("expected validated multisubs profile to use app-server")
			}
		}
	}
	if !found {
		t.Fatalf("expected multisubs profile account to be included, got %#v", accounts)
	}
}

func TestMonitorProfileNameRejectsBuiltInDefaultCodexAccountLabel(t *testing.T) {
	if monitorProfileNameValid("default") {
		t.Fatal("expected the built-in default Codex account label to be rejected as a managed profile")
	}
	if !monitorProfileNameValid("work") {
		t.Fatal("expected an ordinary managed profile name to stay valid")
	}
}

func TestLoadMonitorAccountsRejectsUnsupportedMultisubsVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	t.Setenv(multisubsHomeEnvVar, configDir)
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir multisubs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"version":2,"profiles":{}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected aggregate load error: %v", err)
	}
	if !strings.Contains(warning, "unsupported multisubs config version 2") {
		t.Fatalf("expected unsupported-version warning, got %q", warning)
	}
}

func TestAccountCollectorPreservesVerifiedAppServerUseAcrossHigherPriorityAlias(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	collector := newAccountCollector()
	collector.add("profile", home, 90, false, true)
	collector.add("alias", home, 100, true, false)

	accounts := collector.toAccounts()
	if len(accounts) != 1 {
		t.Fatalf("expected deduped account, got %#v", accounts)
	}
	if accounts[0].Label != "alias" {
		t.Fatalf("expected higher-priority label, got %q", accounts[0].Label)
	}
	if !accounts[0].UseAppServer {
		t.Fatalf("expected verified app-server use to survive higher-priority alias")
	}
}

func TestLoadMonitorAccountsRejectsSymlinkedMultisubsConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir multisubs dir: %v", err)
	}
	outsideConfig := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(outsideConfig, []byte(`{"version":1,"profiles":{}}`), 0o600); err != nil {
		t.Fatalf("write outside config: %v", err)
	}
	if err := os.Symlink(outsideConfig, filepath.Join(configDir, "config.json")); err != nil {
		t.Fatalf("symlink config: %v", err)
	}

	accounts, warning, err := loadMonitorAccounts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected symlinked multisubs config to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "multisubs profile discovery error") || !strings.Contains(warning, "symlink") {
		t.Fatalf("expected symlink warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsGroupReadableProfileDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o750); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected group-readable profile to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "expected no group/world permissions") {
		t.Fatalf("expected private-permissions warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsGroupReadableMultisubsHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.Chmod(configDir, 0o750); err != nil {
		t.Fatalf("chmod multisubs home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected group-readable multisubs home to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "expected no group/world permissions") {
		t.Fatalf("expected private-permissions warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsProfileConfigSymlinkOutsideDefault(t *testing.T) {
	tmp := t.TempDir()
	defaultHome := filepath.Join(tmp, "default-codex")
	t.Setenv("HOME", tmp)
	t.Setenv(defaultCodexHomeEnvVar, defaultHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatalf("mkdir default home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write default config: %v", err)
	}

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	otherConfig := filepath.Join(t.TempDir(), "other-config.toml")
	if err := os.WriteFile(otherConfig, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write other config: %v", err)
	}
	if err := os.Symlink(otherConfig, filepath.Join(profileHome, "config.toml")); err != nil {
		t.Fatalf("symlink profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected unsafe profile config to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "must point to default Codex config") {
		t.Fatalf("expected default config symlink warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsProfileConfigSymlinkTraversalThroughSymlink(t *testing.T) {
	tmp := t.TempDir()
	defaultHome := filepath.Join(tmp, "default-codex")
	t.Setenv("HOME", tmp)
	t.Setenv(defaultCodexHomeEnvVar, defaultHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatalf("mkdir default home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write default config: %v", err)
	}
	outsideDir := filepath.Join(tmp, "outside")
	outsideChild := filepath.Join(outsideDir, "child")
	if err := os.MkdirAll(outsideChild, 0o700); err != nil {
		t.Fatalf("mkdir outside child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write outside config: %v", err)
	}
	if err := os.Symlink(outsideChild, filepath.Join(defaultHome, "pivot")); err != nil {
		t.Fatalf("symlink default pivot: %v", err)
	}

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	rawTarget := defaultHome + string(os.PathSeparator) + "pivot" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "config.toml"
	if err := os.Symlink(rawTarget, filepath.Join(profileHome, "config.toml")); err != nil {
		t.Fatalf("symlink profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected unsafe profile config to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "must point to default Codex config") {
		t.Fatalf("expected default config symlink warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigAllowsProfileConfigSymlinkToDefault(t *testing.T) {
	tmp := t.TempDir()
	defaultHome := filepath.Join(tmp, "default-codex")
	t.Setenv("HOME", tmp)
	t.Setenv(defaultCodexHomeEnvVar, defaultHome)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatalf("mkdir default home: %v", err)
	}
	defaultConfigPath := filepath.Join(defaultHome, "config.toml")
	if err := os.WriteFile(defaultConfigPath, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write default config: %v", err)
	}

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.Symlink(defaultConfigPath, filepath.Join(profileHome, "config.toml")); err != nil {
		t.Fatalf("symlink profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if len(accounts) != 1 || accounts[0].Label != "personal" {
		t.Fatalf("expected profile to be loaded, got %#v", accounts)
	}
}

func TestLoadMonitorAccountsSkipsUnsafeMultisubsProfile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", "")
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))
	t.Setenv(accountsFileEnvVar, filepath.Join(tmp, "missing.json"))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	target := filepath.Join(tmp, "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(profileHome, "auth.json")); err != nil {
		t.Fatalf("symlink auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected unsafe multisubs profile to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "skipping multisubs profile") {
		t.Fatalf("expected skip warning, got %q", warning)
	}
}

func TestLoadMonitorAccountsRejectsInvalidManagedProfileAndAllAliases(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	configPath := filepath.Join(profileHome, "config.toml")
	if err := os.WriteFile(configPath, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	linkMonitorTestFileOrSkipUnsupported(t, configPath, filepath.Join(tmp, "config-alias.toml"))

	registryBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(registryBody), 0o600); err != nil {
		t.Fatalf("write multisubs config: %v", err)
	}
	homeAlias := filepath.Join(tmp, "profile-home-alias")
	if err := os.Symlink(profileHome, homeAlias); err != nil {
		t.Fatalf("symlink profile home alias: %v", err)
	}
	accountsPath := filepath.Join(tmp, "accounts.json")
	accountsBody := `{"version":1,"accounts":[{"label":"file-alias","codex_home":"` + homeAlias + `"}]}`
	if err := os.WriteFile(accountsPath, []byte(accountsBody), 0o600); err != nil {
		t.Fatalf("write accounts file: %v", err)
	}

	t.Setenv("HOME", tmp)
	t.Setenv("CODEX_HOME", homeAlias)
	t.Setenv(defaultCodexHomeEnvVar, profileHome)
	t.Setenv(multisubsHomeEnvVar, configDir)
	t.Setenv(accountsFileEnvVar, accountsPath)

	accounts, warning, err := loadMonitorAccountsWithOptions(MonitorAccountOptions{
		IncludeDefault: true,
		IncludeActive:  true,
	})
	if err != nil {
		t.Fatalf("load monitor accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("invalid managed home was reintroduced through an alias: %#v", accounts)
	}
	if !strings.Contains(warning, "multiple hard links") {
		t.Fatalf("expected managed config skip warning, got %q", warning)
	}
}

func linkMonitorTestFileOrSkipUnsupported(t *testing.T, oldPath, newPath string) {
	t.Helper()
	if err := os.Link(oldPath, newPath); err != nil {
		if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
			t.Skipf("hard links unsupported: %v", err)
		}
		t.Fatalf("create hard link: %v", err)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsInvalidProfileName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"../shared":{"name":"../shared","codex_home":"` + filepath.Join(configDir, "shared", "codex-home") + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected invalid profile to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "invalid profile name") {
		t.Fatalf("expected invalid-name warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsHardLinkedAuth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	target := filepath.Join(tmp, "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"x"}}`), 0o600); err != nil {
		t.Fatalf("write auth target: %v", err)
	}
	linkMonitorTestFileOrSkipUnsupported(t, target, filepath.Join(profileHome, "auth.json"))
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected hard-linked auth profile to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "multiple hard links") {
		t.Fatalf("expected hard-link warning, got %q", warning)
	}
}

func TestLoadAccountsFromMultisubsConfigRejectsLooseAuthPermissions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv(multisubsHomeEnvVar, filepath.Join(tmp, defaultMultisubsHomeDirName))

	configDir := filepath.Join(tmp, defaultMultisubsHomeDirName)
	profileHome := filepath.Join(configDir, "profiles", "personal", "codex-home")
	if err := os.MkdirAll(profileHome, 0o700); err != nil {
		t.Fatalf("mkdir profile home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"tokens":{"access_token":"x"}}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configBody := `{"version":1,"profiles":{"personal":{"name":"personal","codex_home":"` + profileHome + `"}}}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	accounts, warning, err := loadAccountsFromMultisubsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected loose-permission auth profile to be skipped, got %#v", accounts)
	}
	if !strings.Contains(warning, "permissions") {
		t.Fatalf("expected permissions warning, got %q", warning)
	}
}

func TestMonitorConfigUsesFileStoreRequiresExactRootKey(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := "cli_auth_credentials_store_backup = \"file\"\n[other]\ncli_auth_credentials_store = \"file\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ok, err := monitorConfigUsesFileStore(path)
	if err != nil {
		t.Fatalf("monitorConfigUsesFileStore: %v", err)
	}
	if ok {
		t.Fatal("expected lookalike or nested credential store key not to pass")
	}
}

func TestMonitorConfigUsesFileStoreUnquotesBasicStringValue(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `cli_auth_credentials_store = "f\u0069le"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ok, err := monitorConfigUsesFileStore(path)
	if err != nil {
		t.Fatalf("monitorConfigUsesFileStore: %v", err)
	}
	if !ok {
		t.Fatal("expected escaped file credential store value to pass")
	}
}

func TestMonitorConfigUsesFileStoreIgnoresEqualsInsideQuotedKey(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `"not=credential_store" = "x"` + "\ncli_auth_credentials_store = \"file\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ok, err := monitorConfigUsesFileStore(path)
	if err != nil {
		t.Fatalf("monitorConfigUsesFileStore: %v", err)
	}
	if !ok {
		t.Fatal("expected exact credential store key to pass after quoted key with equals")
	}
}

func TestMonitorConfigUsesFileStoreRejectsQuotedKeyWithExtraSpaces(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `" cli_auth_credentials_store " = "file"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ok, err := monitorConfigUsesFileStore(path)
	if err != nil {
		t.Fatalf("monitorConfigUsesFileStore: %v", err)
	}
	if ok {
		t.Fatal("expected quoted key with extra spaces not to pass")
	}
}
