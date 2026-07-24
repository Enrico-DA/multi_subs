package multisubs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorReportHasFailures(t *testing.T) {
	t.Parallel()

	report := DoctorReport{Checks: []DoctorCheck{{Name: "a", Status: "ok"}, {Name: "b", Status: "warn"}}}
	if report.HasFailures() {
		t.Fatalf("expected no failures")
	}
	report.Checks = append(report.Checks, DoctorCheck{Name: "c", Status: "fail"})
	if !report.HasFailures() {
		t.Fatalf("expected failures")
	}
}

func TestCheckFileStoreConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := filepath.Join(root, "config.toml")

	missingReq := checkFileStoreConfig("req", cfg, true)
	if missingReq.Status != "fail" {
		t.Fatalf("expected fail for required missing config, got %s", missingReq.Status)
	}

	if err := os.WriteFile(cfg, []byte("model = \"o4\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	bad := checkFileStoreConfig("req", cfg, true)
	if bad.Status != "fail" {
		t.Fatalf("expected fail for missing file-store setting, got %s", bad.Status)
	}

	if err := os.WriteFile(cfg, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	ok := checkFileStoreConfig("req", cfg, true)
	if ok.Status != "ok" {
		t.Fatalf("expected ok for file-store config, got %s", ok.Status)
	}

	link := filepath.Join(root, "linked-config.toml")
	if err := os.Symlink(cfg, link); err != nil {
		t.Fatalf("symlink config: %v", err)
	}
	linked := checkFileStoreConfig("req", link, true)
	if linked.Status != "ok" {
		t.Fatalf("expected ok for symlinked file-store config, got %s", linked.Status)
	}
	if !strings.Contains(linked.Details, "symlink ->") {
		t.Fatalf("expected symlink details, got %q", linked.Details)
	}
}

func TestRunDoctorMinimal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "codex"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	store := NewStore(paths)
	if err := store.EnsureBaseDirs(); err != nil {
		t.Fatalf("EnsureBaseDirs: %v", err)
	}

	cfg := DefaultConfig()
	report := RunDoctor(store, cfg, 50*time.Millisecond)
	if len(report.Checks) == 0 {
		t.Fatalf("expected non-empty checks")
	}
}

func TestProfileDoctorChecksUseSuppliedShortAndLongTimeouts(t *testing.T) {
	root := t.TempDir()
	paths := Paths{ProfilesDir: filepath.Join(root, "profiles")}
	codexHome := filepath.Join(paths.ProfilesDir, "work", "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(generatedProfileConfigContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	previousCommandContext := codexLoginStatusCommandContext
	t.Cleanup(func() {
		codexLoginStatusCommandContext = previousCommandContext
	})

	var remainingTimeouts []time.Duration
	codexLoginStatusCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "codex" {
			t.Fatalf("unexpected command: %s", name)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("doctor login status command has no deadline")
		}
		remainingTimeouts = append(remainingTimeouts, time.Until(deadline))
		return exec.Command("sh", "-c", "printf 'logged in\\n'")
	}

	timeouts := []time.Duration{250 * time.Millisecond, 45 * time.Second}
	for _, timeout := range timeouts {
		checks := profileDoctorChecks(paths, "work", Profile{Name: "work", CodexHome: codexHome}, true, timeout)
		foundLoginStatus := false
		for _, check := range checks {
			if check.Name == "profile work login status" {
				foundLoginStatus = true
				if check.Status != "ok" {
					t.Fatalf("timeout %s produced login check %v", timeout, check)
				}
			}
		}
		if !foundLoginStatus {
			t.Fatalf("timeout %s did not run the login status check: %v", timeout, checks)
		}
	}

	if len(remainingTimeouts) != len(timeouts) {
		t.Fatalf("captured %d timeouts, want %d", len(remainingTimeouts), len(timeouts))
	}
	for index, timeout := range timeouts {
		remaining := remainingTimeouts[index]
		if remaining <= 0 || remaining > timeout {
			t.Fatalf("timeout %s reached command as %s", timeout, remaining)
		}
		if timeout-remaining > 100*time.Millisecond {
			t.Fatalf("timeout %s lost too much time before command creation: remaining=%s", timeout, remaining)
		}
	}
}

func TestAggregateAndFocusedDoctorParseTheSameTimeout(t *testing.T) {
	const timeoutArgument = "375ms"
	for _, command := range []string{"multisubs doctor", "multisubs codex doctor"} {
		jsonOutput, timeout, err := parseDoctorArguments([]string{"--timeout", timeoutArgument}, command)
		if err != nil {
			t.Fatalf("%s rejected timeout: %v", command, err)
		}
		if jsonOutput {
			t.Fatalf("%s unexpectedly enabled JSON output", command)
		}
		if timeout != 375*time.Millisecond {
			t.Fatalf("%s parsed timeout as %s", command, timeout)
		}
	}
}

func TestAggregateAndFocusedDoctorScopesAreReadOnly(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	runner.capture = func(_ context.Context, args, _ []string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("claude test\n"), nil, nil
		}
		return fakeClaudeAuthJSON(false, ""), nil, nil
	}
	stateHome := app.store.paths.MultisubsHome

	aggregate, _ := captureStdout(t, func() error {
		return app.cmdAggregateDoctor(nil)
	})
	for _, section := range []string{"== shared/base ==", "== Codex ==", "== Claude =="} {
		if !strings.Contains(aggregate, section) {
			t.Errorf("aggregate doctor missing %q:\n%s", section, aggregate)
		}
	}
	if _, err := os.Lstat(stateHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aggregate doctor created product state: %v", err)
	}

	codex, _ := captureStdout(t, func() error {
		return app.cmdDoctor(nil)
	})
	if !strings.Contains(codex, "multisubs codex doctor") || strings.Contains(codex, "Claude binary") {
		t.Fatalf("Codex doctor scope is not focused:\n%s", codex)
	}
	if _, err := os.Lstat(stateHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex doctor created product state: %v", err)
	}

	claude, _ := captureStdout(t, func() error {
		return app.cmdClaudeDoctor(nil)
	})
	if !strings.Contains(claude, "multisubs claude doctor") || strings.Contains(claude, "default codex home") {
		t.Fatalf("Claude doctor scope is not focused:\n%s", claude)
	}
	if _, err := os.Lstat(stateHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Claude doctor created product state: %v", err)
	}
}

func TestDoctorProfileResourcesIsReadOnly(t *testing.T) {
	root := t.TempDir()
	store := NewStore(Paths{ConfigPath: filepath.Join(root, "state", "config.json")})
	inherit := true
	sources := []string{"missing"}
	check := checkProfileResources(store, &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}})
	if check.Status != "fail" || !strings.Contains(check.Details, "resolve profile_resources.skills.sources[0]: lstat") {
		t.Fatalf("unexpected check: %#v", check)
	}
	if _, err := os.Lstat(filepath.Join(root, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("doctor resource check mutated filesystem: %v", err)
	}
}

func TestDoctorProfileResourcesRejectsWrongType(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "skills")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(Paths{ConfigPath: filepath.Join(root, "state", "config.json")})
	inherit := true
	sources := []string{file}
	check := checkProfileResources(store, &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}})
	if check.Status != "fail" || !strings.Contains(check.Details, "not a directory") {
		t.Fatalf("unexpected check: %#v", check)
	}
}

func TestRunDoctorScrubsCodexVersionEnvironment(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex-env.log")
	script := "#!/bin/sh\nenv > \"$MULTISUBS_TEST_ENV_LOG\"\nprintf 'codex-test-version\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MULTISUBS_TEST_ENV_LOG", logPath)
	t.Setenv("CODEX_HOME", filepath.Join(root, "stale-codex"))
	t.Setenv("MULTISUBS_ACTIVE_PROFILE", "stale")
	t.Setenv("OPENAI_API_KEY", "stale")
	t.Setenv("CODEX_AUTH_TOKEN", "stale")
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "codex"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	report := RunDoctor(NewStore(paths), DefaultConfig(), time.Second)
	found := false
	for _, check := range report.Checks {
		if check.Name == "codex binary" {
			found = true
			if check.Status != "ok" {
				t.Fatalf("expected codex binary check ok, got %s (%s)", check.Status, check.Details)
			}
		}
	}
	if !found {
		t.Fatalf("expected codex binary check in report")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read codex env log: %v", err)
	}
	log := string(data)
	for _, forbidden := range []string{"CODEX_HOME", "MULTISUBS_ACTIVE_PROFILE", "OPENAI_API_KEY", "CODEX_AUTH_TOKEN"} {
		if envLogContainsKey(log, forbidden) {
			t.Fatalf("expected %s to be scrubbed from codex version env", forbidden)
		}
	}
}

func envLogContainsKey(log, key string) bool {
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, key+"=") {
			return true
		}
	}
	return false
}

func TestCheckAuthFileStructured(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	content := `{"tokens":{"access_token":"a","refresh_token":"r","id_token":"i"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	check := checkAuthFile("profile test auth", path)
	if check.Status != "ok" {
		t.Fatalf("expected ok, got %s (%s)", check.Status, check.Details)
	}
}

func TestCheckAuthFileInvalidJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	check := checkAuthFile("profile test auth", path)
	if check.Status != "fail" {
		t.Fatalf("expected fail, got %s", check.Status)
	}
	if !strings.Contains(check.Details, "invalid JSON") {
		t.Fatalf("unexpected details: %s", check.Details)
	}
}

func TestCheckAuthFileTokensAndAPIKeyAllowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "auth.json")
	content := `{"OPENAI_API_KEY":"test_api_key_value","tokens":{"access_token":"a","refresh_token":"r","id_token":"i"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	check := checkAuthFile("profile test auth", path)
	if check.Status != "ok" {
		t.Fatalf("expected ok, got %s (%s)", check.Status, check.Details)
	}
}

func TestCheckAuthFileRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write target auth file: %v", err)
	}
	path := filepath.Join(root, "auth.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink auth file: %v", err)
	}

	check := checkAuthFile("profile test auth", path)
	if check.Status != "fail" {
		t.Fatalf("expected fail for symlink auth, got %s", check.Status)
	}
	if !strings.Contains(check.Details, "auth.json is a symlink") {
		t.Fatalf("expected symlink detail, got %q", check.Details)
	}
}

func TestCheckAuthFileRejectsHardLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write target auth file: %v", err)
	}
	path := filepath.Join(root, "auth.json")
	if err := os.Link(target, path); err != nil {
		t.Skipf("hard links are not supported here: %v", err)
	}

	check := checkAuthFile("profile test auth", path)
	if check.Status != "fail" {
		t.Fatalf("expected fail for hard-linked auth, got %s", check.Status)
	}
	if !strings.Contains(check.Details, "multiple hard links") {
		t.Fatalf("expected hard-link detail, got %q", check.Details)
	}
}

func TestProfileDoctorChecksSkipLoginStatusWhenConfigFails(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex.log")
	script := "#!/bin/sh\nprintf 'codex login status invoked\\n' > " + shellQuote(logPath) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	paths := Paths{ProfilesDir: filepath.Join(root, "profiles"), DefaultCodexHome: filepath.Join(root, "default-codex")}
	codexHome := filepath.Join(paths.ProfilesDir, "work", "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("model = \"global\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	checks := profileDoctorChecks(paths, "work", Profile{Name: "work", CodexHome: codexHome}, true, time.Second)
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected codex login status not to run, stat err=%v", err)
	}
	for _, check := range checks {
		if strings.Contains(check.Name, "login status") {
			t.Fatalf("expected login status check to be skipped after config failure, got %v", checks)
		}
	}
}

func TestCheckDirExistsRejectsStrictSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "real-home")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(root, "linked-home")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink home: %v", err)
	}

	check := checkDirExists("multisubs home", link, true)
	if check.Status != "fail" {
		t.Fatalf("expected strict symlink dir to fail, got %s (%s)", check.Status, check.Details)
	}
}

func TestProfileDoctorChecksSkipLoginStatusWhenAuthFails(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex.log")
	script := "#!/bin/sh\nprintf 'codex login status invoked\\n' > " + shellQuote(logPath) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	paths := Paths{ProfilesDir: filepath.Join(root, "profiles")}
	codexHome := filepath.Join(paths.ProfilesDir, "work", "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(generatedProfileConfigContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	target := filepath.Join(root, "shared-auth.json")
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write target auth file: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(codexHome, "auth.json")); err != nil {
		t.Fatalf("symlink auth file: %v", err)
	}

	checks := profileDoctorChecks(paths, "work", Profile{Name: "work", CodexHome: codexHome}, true, time.Second)
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected codex login status not to run, stat err=%v", err)
	}
	for _, check := range checks {
		if strings.Contains(check.Name, "login status") {
			t.Fatalf("expected login status check to be skipped after auth failure, got %v", checks)
		}
	}
}

func TestProfileDoctorChecksRejectSymlinkedHomeBeforeAuthProbe(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	logPath := filepath.Join(root, "codex.log")
	script := "#!/bin/sh\nprintf 'codex login status invoked\\n' > " + shellQuote(logPath) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	paths := Paths{ProfilesDir: filepath.Join(root, "profiles")}
	realProfileDir := filepath.Join(root, "real-profile")
	realHome := filepath.Join(realProfileDir, "codex-home")
	if err := os.MkdirAll(realHome, 0o700); err != nil {
		t.Fatalf("mkdir real home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realHome, "auth.json"), []byte(`{"tokens":{"access_token":"a"}}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.MkdirAll(paths.ProfilesDir, 0o700); err != nil {
		t.Fatalf("mkdir profiles dir: %v", err)
	}
	profileDir := filepath.Join(paths.ProfilesDir, "work")
	if err := os.Symlink(realProfileDir, profileDir); err != nil {
		t.Fatalf("symlink profile dir: %v", err)
	}
	linkHome := filepath.Join(profileDir, "codex-home")

	checks := profileDoctorChecks(paths, "work", Profile{Name: "work", CodexHome: linkHome}, true, time.Second)
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected codex login status not to run, stat err=%v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected name and codex-home checks only, got %v", checks)
	}
	if checks[1].Status != "fail" || !strings.Contains(checks[1].Details, "symlink") {
		t.Fatalf("expected symlinked home failure, got %v", checks[1])
	}
}

func TestMissingIgnorePatterns(t *testing.T) {
	t.Parallel()

	full := strings.Join([]string{
		"**/multisubs/config.json",
		"**/multisubs/profiles/",
		"**/multisubs/providers/claude/",
		"**/.multisubs/config.json",
		"**/.multisubs/profiles/",
		".multisubs/",
		"**/multicodex/config.json",
		"**/multicodex/profiles/",
		"**/multicodex/providers/claude/",
		"**/.multicodex/config.json",
		"**/.multicodex/profiles/",
		".multicodex/",
		".codex/",
		"**/auth.json",
		"**/.credentials.json",
		".env",
		".env.*",
	}, "\n")
	if got := missingIgnorePatterns(full); len(got) != 0 {
		t.Fatalf("expected no missing patterns, got %v", got)
	}

	minimal := ".codex/\n"
	got := missingIgnorePatterns(minimal)
	for _, want := range []string{".multisubs/", ".multicodex/", "**/multisubs/config.json", "**/multicodex/config.json", "**/auth.json"} {
		if !containsString(got, want) {
			t.Errorf("missing expected ignore recommendation %q from %v", want, got)
		}
	}

	minimalNew := "**/multisubs/profiles/\n**/multisubs/config.json\n"
	got = missingIgnorePatterns(minimalNew)
	for _, want := range []string{".multicodex/", "**/multicodex/config.json", "**/multicodex/profiles/"} {
		if !containsString(got, want) {
			t.Errorf("new-only ignore file did not require legacy-sensitive %q: %v", want, got)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestIsSensitiveTrackedPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path      string
		sensitive bool
	}{
		{path: "github.com/olliecrow/multicodex/config.json", sensitive: true},
		{path: "github.com/olliecrow/multicodex/profiles/work/codex-home/config.toml", sensitive: true},
		{path: "github.com/Enrico-DA/multi_subs/config.json", sensitive: true},
		{path: "github.com/Enrico-DA/multi_subs/providers/claude/config.json", sensitive: true},
		{path: ".multisubs/config.json", sensitive: true},
		{path: ".multisubs/profiles/work/codex-home/config.toml", sensitive: true},
		{path: ".multicodex/config.json", sensitive: true},
		{path: ".multicodex/profiles/work/codex-home/config.toml", sensitive: true},
		{path: "github.com/olliecrow/.multicodex/config.json", sensitive: true},
		{path: "github.com/olliecrow/multicodex/docs/readme.md", sensitive: false},
		{path: "foo/.codex/auth.json", sensitive: true},
		{path: "auth.json", sensitive: true},
		{path: ".credentials.json", sensitive: true},
		{path: ".env", sensitive: true},
		{path: ".env.local", sensitive: true},
		{path: ".env.example", sensitive: false},
		{path: "keys/prod.pem", sensitive: true},
		{path: "docs/readme.md", sensitive: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := isSensitiveTrackedPath(tc.path); got != tc.sensitive {
				t.Fatalf("unexpected sensitivity for %q: got=%v want=%v", tc.path, got, tc.sensitive)
			}
		})
	}
}

func TestIsSubpathWithSymlinkAliases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realRoot := filepath.Join(root, "real-root")
	if err := os.MkdirAll(filepath.Join(realRoot, "child"), 0o755); err != nil {
		t.Fatalf("mkdir real root: %v", err)
	}
	aliasRoot := filepath.Join(root, "alias-root")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Fatalf("symlink alias root: %v", err)
	}

	if !isSubpath(aliasRoot, filepath.Join(realRoot, "child")) {
		t.Fatalf("expected child under symlink alias root to be detected as subpath")
	}
	if isSubpath(aliasRoot, root) {
		t.Fatalf("expected temp root to not be subpath of alias root")
	}
}
