package codexstate

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestValidateManagedConfigPathRegularSingleLink(t *testing.T) {
	root := t.TempDir()
	profileConfig := filepath.Join(root, "profile.toml")
	defaultConfig := filepath.Join(root, "default.toml")
	writeManagedConfigTestFile(t, profileConfig)

	details, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
	if err != nil {
		t.Fatalf("ValidateManagedConfigPath: %v", err)
	}
	if details.IsSymlink || details.RawLinkTarget != "" {
		t.Fatalf("unexpected details: %+v", details)
	}
}

func TestValidateManagedConfigPathDefaultSymlinkForms(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default")
	profileDir := filepath.Join(root, "profile")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatalf("create default dir: %v", err)
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	defaultConfig := filepath.Join(defaultDir, "config.toml")
	writeManagedConfigTestFile(t, defaultConfig)

	tests := []struct {
		name       string
		makeTarget func(t *testing.T, profileConfig string) string
	}{
		{
			name: "absolute",
			makeTarget: func(t *testing.T, profileConfig string) string {
				return defaultConfig
			},
		},
		{
			name: "relative",
			makeTarget: func(t *testing.T, profileConfig string) string {
				target, err := filepath.Rel(filepath.Dir(profileConfig), defaultConfig)
				if err != nil {
					t.Fatalf("make relative target: %v", err)
				}
				return target
			},
		},
		{
			name: "symlink chain",
			makeTarget: func(t *testing.T, profileConfig string) string {
				first := filepath.Join(root, "first.toml")
				second := filepath.Join(root, "second.toml")
				if err := os.Symlink(defaultConfig, second); err != nil {
					t.Fatalf("create second link: %v", err)
				}
				if err := os.Symlink(second, first); err != nil {
					t.Fatalf("create first link: %v", err)
				}
				return first
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profileConfig := filepath.Join(profileDir, test.name+".toml")
			rawTarget := test.makeTarget(t, profileConfig)
			if err := os.Symlink(rawTarget, profileConfig); err != nil {
				t.Fatalf("create profile link: %v", err)
			}
			details, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
			if err != nil {
				t.Fatalf("ValidateManagedConfigPath: %v", err)
			}
			if !details.IsSymlink || details.RawLinkTarget != rawTarget {
				t.Fatalf("details = %+v, want raw target %q", details, rawTarget)
			}
		})
	}
}

func TestValidateManagedConfigPathRejectsHardLinkedRegularFile(t *testing.T) {
	root := t.TempDir()
	profileConfig := filepath.Join(root, "profile.toml")
	alias := filepath.Join(root, "alias.toml")
	writeManagedConfigTestFile(t, profileConfig)
	linkManagedConfigTestFile(t, profileConfig, alias)

	_, err := ValidateManagedConfigPath(profileConfig, filepath.Join(root, "default.toml"))
	if err == nil {
		t.Fatal("expected hard-linked config to fail")
	}
}

func TestValidateManagedConfigPathRejectsArbitraryReadableSymlink(t *testing.T) {
	root := t.TempDir()
	defaultConfig := filepath.Join(root, "default.toml")
	otherConfig := filepath.Join(root, "other.toml")
	profileConfig := filepath.Join(root, "profile.toml")
	writeManagedConfigTestFile(t, defaultConfig)
	writeManagedConfigTestFile(t, otherConfig)
	if err := os.Symlink(otherConfig, profileConfig); err != nil {
		t.Fatalf("create profile link: %v", err)
	}

	details, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
	if err == nil {
		t.Fatal("expected arbitrary symlink to fail")
	}
	if !details.IsSymlink || details.RawLinkTarget != otherConfig {
		t.Fatalf("failure details = %+v", details)
	}
}

func TestValidateManagedConfigPathRejectsSymlinkToHardLinkAliasOfDefault(t *testing.T) {
	root := t.TempDir()
	defaultConfig := filepath.Join(root, "default.toml")
	defaultAlias := filepath.Join(root, "default-alias.toml")
	profileConfig := filepath.Join(root, "profile.toml")
	writeManagedConfigTestFile(t, defaultConfig)
	linkManagedConfigTestFile(t, defaultConfig, defaultAlias)
	if err := os.Symlink(defaultAlias, profileConfig); err != nil {
		t.Fatalf("create profile link: %v", err)
	}

	_, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
	if err == nil {
		t.Fatal("expected alias path to fail exact path comparison")
	}
}

func TestValidateManagedConfigPathRejectsBrokenAndLoopingSymlinks(t *testing.T) {
	root := t.TempDir()
	defaultConfig := filepath.Join(root, "default.toml")
	writeManagedConfigTestFile(t, defaultConfig)

	tests := []struct {
		name      string
		rawTarget string
	}{
		{name: "broken", rawTarget: filepath.Join(root, "missing.toml")},
		{name: "loop", rawTarget: filepath.Join(root, "loop.toml")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profileConfig := filepath.Join(root, test.name+".toml")
			rawTarget := test.rawTarget
			if test.name == "loop" {
				rawTarget = profileConfig
			}
			if err := os.Symlink(rawTarget, profileConfig); err != nil {
				t.Fatalf("create profile link: %v", err)
			}
			details, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
			if err == nil {
				t.Fatal("expected symlink resolution to fail")
			}
			if !details.IsSymlink || details.RawLinkTarget != rawTarget {
				t.Fatalf("failure details = %+v", details)
			}
		})
	}
}

func TestValidateManagedConfigPathRejectsNonRegularEntries(t *testing.T) {
	root := t.TempDir()
	defaultConfig := filepath.Join(root, "default.toml")

	t.Run("directory", func(t *testing.T) {
		profileConfig := filepath.Join(root, "directory")
		if err := os.Mkdir(profileConfig, 0o700); err != nil {
			t.Fatalf("create directory: %v", err)
		}
		if _, err := ValidateManagedConfigPath(profileConfig, defaultConfig); err == nil {
			t.Fatal("expected directory to fail")
		}
	})

	t.Run("named pipe", func(t *testing.T) {
		profileConfig := filepath.Join(root, "pipe")
		if err := syscall.Mkfifo(profileConfig, 0o600); err != nil {
			if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
				t.Skipf("named pipes unsupported: %v", err)
			}
			t.Fatalf("create named pipe: %v", err)
		}
		if _, err := ValidateManagedConfigPath(profileConfig, defaultConfig); err == nil {
			t.Fatal("expected named pipe to fail")
		}
	})
}

func TestValidateManagedConfigPathMissingDefaultPreservesNotExistAndDetails(t *testing.T) {
	root := t.TempDir()
	defaultConfig := filepath.Join(root, "missing.toml")
	otherConfig := filepath.Join(root, "other.toml")
	profileConfig := filepath.Join(root, "profile.toml")
	writeManagedConfigTestFile(t, otherConfig)
	if err := os.Symlink(otherConfig, profileConfig); err != nil {
		t.Fatalf("create profile link: %v", err)
	}

	details, err := ValidateManagedConfigPath(profileConfig, defaultConfig)
	if err == nil {
		t.Fatal("expected missing default to fail")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected errors.Is(os.ErrNotExist), got %v", err)
	}
	if !details.IsSymlink || details.RawLinkTarget != otherConfig {
		t.Fatalf("failure details = %+v", details)
	}
}

func TestValidateManagedConfigPathMissingProfilePreservesNotExist(t *testing.T) {
	root := t.TempDir()
	_, err := ValidateManagedConfigPath(
		filepath.Join(root, "missing-profile.toml"),
		filepath.Join(root, "default.toml"),
	)
	if err == nil {
		t.Fatal("expected missing profile config to fail")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected errors.Is(os.ErrNotExist), got %v", err)
	}
}

func TestValidateManagedConfigPathRejectsEmptyPaths(t *testing.T) {
	if _, err := ValidateManagedConfigPath("", "default.toml"); err == nil {
		t.Fatal("expected empty profile path to fail")
	}
	if _, err := ValidateManagedConfigPath("profile.toml", ""); err == nil {
		t.Fatal("expected empty default path to fail")
	}
}

func writeManagedConfigTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("model = \"test\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func linkManagedConfigTestFile(t *testing.T, oldPath, newPath string) {
	t.Helper()
	if err := os.Link(oldPath, newPath); err != nil {
		if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
			t.Skipf("hard links unsupported: %v", err)
		}
		t.Fatalf("create hard link: %v", err)
	}
}
