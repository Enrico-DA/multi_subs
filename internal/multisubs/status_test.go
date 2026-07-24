package multisubs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmailFromAuthFileTopLevel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	content := `{"email":"top@example.com","tokens":{"id_token":""}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	email, err := emailFromAuthFile(path)
	if err != nil {
		t.Fatalf("emailFromAuthFile returned error: %v", err)
	}
	if email != "top@example.com" {
		t.Fatalf("unexpected email: %q", email)
	}
}

func TestEmailFromAuthFileIDToken(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	idToken := syntheticJWT(t, map[string]any{"email": "claim@example.com"})
	content := `{"tokens":{"id_token":"` + idToken + `"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	email, err := emailFromAuthFile(path)
	if err != nil {
		t.Fatalf("emailFromAuthFile returned error: %v", err)
	}
	if email != "claim@example.com" {
		t.Fatalf("unexpected email: %q", email)
	}
}

func TestEmailFromAuthFileMissingEmail(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	idToken := syntheticJWT(t, map[string]any{"sub": "abc123"})
	content := `{"tokens":{"id_token":"` + idToken + `"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	email, err := emailFromAuthFile(path)
	if err != nil {
		t.Fatalf("emailFromAuthFile returned error: %v", err)
	}
	if email != "" {
		t.Fatalf("expected empty email, got %q", email)
	}
}

func TestCmdStatusRejectsAuthSymlinkBeforeCodexStatus(t *testing.T) {
	app, logPath := newStatusTestApp(t)
	writeDefaultFileStoreConfig(t, app)
	createTestProfiles(t, app, "work")
	profileHome := filepath.Join(app.store.paths.ProfilesDir, "work", "codex-home")
	target := filepath.Join(t.TempDir(), "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write target auth: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(profileHome, "auth.json")); err != nil {
		t.Fatalf("symlink auth: %v", err)
	}

	out, err := captureStdout(t, app.cmdStatus)
	if err != nil {
		t.Fatalf("cmdStatus: %v", err)
	}
	if !strings.Contains(out, "auth path is a symlink") {
		t.Fatalf("expected symlink error in status output, got %q", out)
	}
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected codex status not to be invoked, stat err=%v", statErr)
	}
}

func TestCmdStatusRequiresFileStoreBeforeCodexStatus(t *testing.T) {
	app, logPath := newStatusTestApp(t)
	defaultConfigPath := writeDefaultConfig(t, app, "model = \"global\"\n")
	createTestProfiles(t, app, "work")
	profileConfigPath := filepath.Join(app.store.paths.ProfilesDir, "work", "codex-home", "config.toml")
	if err := os.Symlink(defaultConfigPath, profileConfigPath); err != nil {
		t.Fatalf("symlink profile config: %v", err)
	}

	out, err := captureStdout(t, app.cmdStatus)
	if err != nil {
		t.Fatalf("cmdStatus: %v", err)
	}
	if !strings.Contains(out, "requires file-backed auth") {
		t.Fatalf("expected file-store error in status output, got %q", out)
	}
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected codex status not to be invoked, stat err=%v", statErr)
	}
}

func TestCmdStatusRejectsHardLinkedConfigBeforeCodexStatus(t *testing.T) {
	app, logPath := newStatusTestApp(t)
	writeDefaultFileStoreConfig(t, app)
	createTestProfiles(t, app, "work")
	configPath := filepath.Join(app.store.paths.ProfilesDir, "work", "codex-home", "config.toml")
	if err := os.WriteFile(configPath, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write profile config: %v", err)
	}
	linkFileOrSkipUnsupported(t, configPath, filepath.Join(t.TempDir(), "config-alias.toml"))

	out, err := captureStdout(t, app.cmdStatus)
	if err != nil {
		t.Fatalf("cmdStatus: %v", err)
	}
	if !strings.Contains(out, "multiple hard links") {
		t.Fatalf("expected managed config error in status output, got %q", out)
	}
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected codex status not to be invoked, stat err=%v", statErr)
	}
}

func TestCodexLoginStatusTreatsZeroExitNegativeOutputAsLoggedOut(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	script := "#!/bin/sh\nprintf 'not logged in\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	state, _, detail := codexLoginStatus(filepath.Join(root, "codex-home"))
	if state != "logged-out" {
		t.Fatalf("expected logged-out for zero-exit negative status, got state=%q detail=%q", state, detail)
	}
	if detail != "not logged in" {
		t.Fatalf("expected negative status detail to be preserved, got %q", detail)
	}
}

func TestCodexLoginStatusRedactsFailureOutput(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	script := "#!/bin/sh\nprintf 'opaque-provider-diagnostic\\n' >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	state, _, detail := codexLoginStatus(filepath.Join(root, "codex-home"))
	if state != "error" {
		t.Fatalf("expected error state, got %q", state)
	}
	if strings.Contains(detail, "opaque-provider-diagnostic") {
		t.Fatalf("failure output leaked into detail: %q", detail)
	}
	if !strings.Contains(detail, "exit code 7") {
		t.Fatalf("expected safe exit-code detail, got %q", detail)
	}
}

func TestCodexLoginStatusForcesFileBackedAuth(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	argsPath := filepath.Join(root, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + shellQuote(argsPath) + "\nprintf 'logged in\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	state, _, _ := codexLoginStatus(filepath.Join(root, "codex-home"))
	if state != "logged-in" {
		t.Fatalf("expected logged-in state, got %q", state)
	}
	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	want := "login status -c " + managedCodexAuthConfig + "\n"
	if string(data) != want {
		t.Fatalf("managed login status args: got %q want %q", data, want)
	}
}

func newStatusTestApp(t *testing.T) (*App, string) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex-status.log")
	script := "#!/bin/sh\nprintf 'codex login status invoked\\n' > " + shellQuote(logPath) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return app, logPath
}

func TestStatusDoesNotResolveOrMutateProfileResources(t *testing.T) {
	app, _ := newStatusTestApp(t)
	if err := app.store.EnsureBaseDirs(); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{"missing"}
	cfg := DefaultConfig()
	cfg.ProfileResources = &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}
	if err := app.store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdStatus(); err != nil {
		t.Fatalf("auth-only status should not resolve resource paths: %v", err)
	}
	entries, err := os.ReadDir(app.store.paths.ProfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("status mutated profile state: %v", entries)
	}
}

func syntheticJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	header := map[string]any{"alg": "none", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	enc := base64.RawURLEncoding
	return enc.EncodeToString(headerJSON) + "." + enc.EncodeToString(claimsJSON) + "."
}
