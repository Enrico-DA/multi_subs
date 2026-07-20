package multicodex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAddUsesSeparateV1SidecarAndPrivateDerivedPath(t *testing.T) {
	app, _, _ := newClaudeTestApp(t)
	legacy := DefaultConfig()
	legacy.Profiles["codex-work"] = Profile{
		Name:      "codex-work",
		CodexHome: filepath.Join(app.store.paths.ProfilesDir, "codex-work", "codex-home"),
	}
	if err := app.store.Save(legacy); err != nil {
		t.Fatalf("save legacy config: %v", err)
	}
	legacyBefore, err := os.ReadFile(app.store.paths.ConfigPath)
	if err != nil {
		t.Fatalf("read legacy config: %v", err)
	}

	if _, err := captureStdout(t, func() error { return app.cmdClaudeAdd([]string{"work"}) }); err != nil {
		t.Fatalf("Claude add: %v", err)
	}
	legacyAfter, err := os.ReadFile(app.store.paths.ConfigPath)
	if err != nil {
		t.Fatalf("read legacy config after Claude add: %v", err)
	}
	if string(legacyAfter) != string(legacyBefore) {
		t.Fatalf("legacy config changed during Claude add\nbefore=%s\nafter=%s", legacyBefore, legacyAfter)
	}

	store := newClaudeStore(app.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		t.Fatalf("load Claude sidecar: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("sidecar version: got %d want 1", cfg.Version)
	}
	profile, ok := cfg.Profiles["work"]
	if !ok {
		t.Fatal("managed Claude profile was not stored")
	}
	wantConfigDir := filepath.Join(app.store.paths.MulticodexHome, "providers", "claude", "profiles", "work", "config")
	if profile.ConfigDir != wantConfigDir {
		t.Fatalf("config dir: got %q want %q", profile.ConfigDir, wantConfigDir)
	}
	for _, path := range []string{
		app.store.paths.MulticodexHome,
		filepath.Join(app.store.paths.MulticodexHome, "providers"),
		store.paths.ClaudeProviderDir,
		store.paths.ClaudeProfilesDir,
		filepath.Dir(profile.ConfigDir),
		profile.ConfigDir,
	} {
		assertPrivateMode(t, path, 0o700)
	}
	assertPrivateMode(t, store.paths.ClaudeConfigPath, 0o600)
}

func TestClaudeSidecarLoadsVersionOne(t *testing.T) {
	app, _, _ := newClaudeTestApp(t)
	store := newClaudeStore(app.store.paths)
	if err := store.EnsureBaseDirs(); err != nil {
		t.Fatalf("EnsureBaseDirs: %v", err)
	}
	profileDir := filepath.Join(store.paths.ClaudeProfilesDir, "work", "config")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	raw := `{"version":1,"profiles":{"work":{"name":"work","config_dir":"` + profileDir + `"}}}`
	if err := os.WriteFile(store.paths.ClaudeConfigPath, []byte(raw+"\n"), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	cfg, err := store.LoadIfExists()
	if err != nil {
		t.Fatalf("LoadIfExists: %v", err)
	}
	if cfg.Profiles["work"].ConfigDir != profileDir {
		t.Fatalf("unexpected loaded profile: %+v", cfg.Profiles["work"])
	}
}

func TestClaudeSidecarRejectsUnknownOrMissingVersion(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"unknown", `{"version":2,"profiles":{}}`},
		{"missing", `{"profiles":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _, _ := newClaudeTestApp(t)
			store := newClaudeStore(app.store.paths)
			if err := store.EnsureBaseDirs(); err != nil {
				t.Fatalf("EnsureBaseDirs: %v", err)
			}
			if err := os.WriteFile(store.paths.ClaudeConfigPath, []byte(test.raw+"\n"), 0o600); err != nil {
				t.Fatalf("write sidecar: %v", err)
			}
			_, err := store.LoadIfExists()
			if err == nil || !strings.Contains(err.Error(), "unsupported Claude sidecar version") {
				t.Fatalf("expected version rejection, got %v", err)
			}
		})
	}
}

func TestClaudeSidecarRejectsEscapingManagedPath(t *testing.T) {
	app, _, root := newClaudeTestApp(t)
	store := newClaudeStore(app.store.paths)
	if err := store.EnsureBaseDirs(); err != nil {
		t.Fatalf("EnsureBaseDirs: %v", err)
	}
	escape := filepath.Join(root, "outside", "config")
	raw := `{"version":1,"profiles":{"work":{"name":"work","config_dir":"` + escape + `"}}}`
	if err := os.WriteFile(store.paths.ClaudeConfigPath, []byte(raw+"\n"), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	_, err := store.LoadIfExists()
	if err == nil || !strings.Contains(err.Error(), "derived path") {
		t.Fatalf("expected derived-path rejection, got %v", err)
	}
}

func TestClaudeSidecarRejectsSymlinksHardlinksAndOpenPermissions(t *testing.T) {
	t.Run("sidecar symlink", func(t *testing.T) {
		app, _, root := newClaudeTestApp(t)
		store := newClaudeStore(app.store.paths)
		if err := store.EnsureBaseDirs(); err != nil {
			t.Fatalf("EnsureBaseDirs: %v", err)
		}
		target := filepath.Join(root, "sidecar-target.json")
		if err := os.WriteFile(target, []byte(`{"version":1,"profiles":{}}`), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.Symlink(target, store.paths.ClaudeConfigPath); err != nil {
			t.Fatalf("symlink sidecar: %v", err)
		}
		_, err := store.LoadIfExists()
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
	})

	t.Run("sidecar hardlink", func(t *testing.T) {
		app, _, root := newClaudeTestApp(t)
		store := newClaudeStore(app.store.paths)
		if err := store.EnsureBaseDirs(); err != nil {
			t.Fatalf("EnsureBaseDirs: %v", err)
		}
		if err := os.WriteFile(store.paths.ClaudeConfigPath, []byte(`{"version":1,"profiles":{}}`), 0o600); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
		if err := os.Link(store.paths.ClaudeConfigPath, filepath.Join(root, "sidecar.link")); err != nil {
			t.Fatalf("hardlink sidecar: %v", err)
		}
		_, err := store.LoadIfExists()
		if err == nil || !strings.Contains(err.Error(), "multiple hard links") {
			t.Fatalf("expected hardlink rejection, got %v", err)
		}
	})

	t.Run("profile symlink", func(t *testing.T) {
		app, _, root := newClaudeTestApp(t)
		store := newClaudeStore(app.store.paths)
		if err := store.EnsureBaseDirs(); err != nil {
			t.Fatalf("EnsureBaseDirs: %v", err)
		}
		profileRoot := filepath.Join(store.paths.ClaudeProfilesDir, "work")
		if err := os.Mkdir(profileRoot, 0o700); err != nil {
			t.Fatalf("mkdir profile root: %v", err)
		}
		outside := filepath.Join(root, "outside")
		if err := os.Mkdir(outside, 0o700); err != nil {
			t.Fatalf("mkdir outside: %v", err)
		}
		configDir := filepath.Join(profileRoot, "config")
		if err := os.Symlink(outside, configDir); err != nil {
			t.Fatalf("symlink config dir: %v", err)
		}
		raw := `{"version":1,"profiles":{"work":{"name":"work","config_dir":"` + configDir + `"}}}`
		if err := os.WriteFile(store.paths.ClaudeConfigPath, []byte(raw), 0o600); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
		_, err := store.LoadIfExists()
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected profile symlink rejection, got %v", err)
		}
	})

	t.Run("open profile permissions", func(t *testing.T) {
		app, _, _ := newClaudeTestApp(t)
		profiles := createClaudeProfiles(t, app, "work")
		if err := os.Chmod(profiles["work"].ConfigDir, 0o750); err != nil {
			t.Fatalf("chmod config dir: %v", err)
		}
		_, err := newClaudeStore(app.store.paths).LoadIfExists()
		if err == nil || !strings.Contains(err.Error(), "expected no group/world permissions") {
			t.Fatalf("expected permissions rejection, got %v", err)
		}
	})
}

func TestClaudeAddRejectsProtectedDefaultName(t *testing.T) {
	app, _, _ := newClaudeTestApp(t)
	err := app.cmdClaudeAdd([]string{"default"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %v", err, err)
	}
	if !strings.Contains(exitErr.Message, "reserved") {
		t.Fatalf("unexpected error: %s", exitErr.Message)
	}
}
