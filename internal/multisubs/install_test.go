package multisubs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInstallRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
		code    int
	}{
		{name: "default latest", want: "latest"},
		{name: "explicit latest", args: []string{"latest"}, want: "latest"},
		{name: "tag", args: []string{"v0.1.0"}, want: "v0.1.0"},
		{name: "commit", args: []string{"7e89a4635ca0"}, want: "7e89a4635ca0"},
		{name: "slash branch", args: []string{"cursor/topic"}, wantErr: true, code: 2},
		{name: "at ref", args: []string{"latest@main"}, wantErr: true, code: 2},
		{name: "extra args", args: []string{"latest", "extra"}, wantErr: true, code: 2},
		{name: "flag", args: []string{"--json"}, wantErr: true, code: 2},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseInstallRef(test.args)
			if test.wantErr {
				var exitErr *ExitError
				if !errors.As(err, &exitErr) || exitErr.Code != test.code {
					t.Fatalf("parseInstallRef(%q) = %q, %T (%v)", test.args, got, err, err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parseInstallRef(%q) = %q, %v; want %q", test.args, got, err, test.want)
			}
		})
	}
}

func TestDefaultInstallShellRC(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bashrc := filepath.Join(root, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("export PATH=1\n"), 0o600); err != nil {
		t.Fatalf("write bashrc: %v", err)
	}

	path, fish := defaultInstallShellRC(root, "/usr/bin/fish")
	if !fish || path != filepath.Join(root, ".config", "fish", "config.fish") {
		t.Fatalf("fish rc: %q fish=%v", path, fish)
	}
	path, fish = defaultInstallShellRC(root, "/bin/bash")
	if fish || path != bashrc {
		t.Fatalf("bash rc with bashrc: %q fish=%v", path, fish)
	}
	empty := filepath.Join(root, "empty")
	path, fish = defaultInstallShellRC(empty, "/bin/bash")
	if fish || path != filepath.Join(empty, ".bash_profile") {
		t.Fatalf("bash rc without bashrc: %q fish=%v", path, fish)
	}
	path, fish = defaultInstallShellRC(root, "/bin/zsh")
	if fish || path != filepath.Join(root, ".zshrc") {
		t.Fatalf("zsh rc: %q fish=%v", path, fish)
	}
}

func TestPersistInstallShellBlockCreatesUpdatesAndSkipsDuplicate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rcPath := filepath.Join(root, ".zshrc")
	gobin := filepath.Join(root, "local-bin")

	wrote, err := persistInstallShellBlock(rcPath, gobin, false)
	if err != nil || !wrote {
		t.Fatalf("create zshrc: wrote=%v err=%v", wrote, err)
	}
	first, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read zshrc: %v", err)
	}
	if strings.Count(string(first), installShellBlockBegin) != 1 ||
		!strings.Contains(string(first), "export GOBIN='"+gobin+"'") ||
		!strings.Contains(string(first), "export GOPRIVATE='"+installPrivateModule+"'") {
		t.Fatalf("created zshrc: %s", first)
	}

	wrote, err = persistInstallShellBlock(rcPath, gobin, false)
	if err != nil || wrote {
		t.Fatalf("duplicate zshrc write: wrote=%v err=%v", wrote, err)
	}
	second, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("reread zshrc: %v", err)
	}
	if string(second) != string(first) {
		t.Fatalf("duplicate changed zshrc:\n%s", second)
	}

	nextBin := filepath.Join(root, "other-bin")
	wrote, err = persistInstallShellBlock(rcPath, nextBin, false)
	if err != nil || !wrote {
		t.Fatalf("update zshrc: wrote=%v err=%v", wrote, err)
	}
	updated, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read updated zshrc: %v", err)
	}
	if strings.Count(string(updated), installShellBlockBegin) != 1 ||
		!strings.Contains(string(updated), "export GOBIN='"+nextBin+"'") ||
		strings.Contains(string(updated), gobin) {
		t.Fatalf("updated zshrc: %s", updated)
	}
}

func TestReplaceInstallShellBlockRejectsBrokenMarker(t *testing.T) {
	t.Parallel()

	_, _, err := replaceInstallShellBlock(installShellBlockBegin+"\nexport GOBIN=/tmp\n", "unused")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("broken block: %T (%v)", err, err)
	}
}

func TestInstallCommandEnvOverridesGOBINAndGOPRIVATE(t *testing.T) {
	t.Parallel()

	got := installCommandEnv([]string{
		"PATH=/usr/bin",
		"GOBIN=/wrong/bin",
		"GOPRIVATE=old.example.test",
		"HOME=/tmp/home",
	}, "/tmp/local-bin")
	joined := strings.Join(got, "\n")
	if strings.Count(joined, "GOBIN=") != 1 ||
		strings.Count(joined, "GOPRIVATE=") != 1 ||
		!strings.Contains(joined, "GOBIN=/tmp/local-bin") ||
		!strings.Contains(joined, "GOPRIVATE="+installPrivateModule) ||
		!strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("install env: %q", got)
	}
}

func TestRemoveInstallLeftoversDeletesDefaultGoBinCopy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// removeInstallLeftovers resolves symlinks before deleting, so the expected
	// paths must be resolved too. On macOS t.TempDir() sits under /var, which is
	// a symlink to /private/var.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	runningDir := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(runningDir, 0o700); err != nil {
		t.Fatalf("mkdir running: %v", err)
	}
	running := filepath.Join(runningDir, "multisubs")
	if err := os.WriteFile(running, []byte("path-copy"), 0o700); err != nil {
		t.Fatalf("write running: %v", err)
	}
	leftoverDir := filepath.Join(root, "go", "bin")
	if err := os.MkdirAll(leftoverDir, 0o700); err != nil {
		t.Fatalf("mkdir leftover: %v", err)
	}
	leftover := filepath.Join(leftoverDir, "multisubs")
	if err := os.WriteFile(leftover, []byte("go-bin-copy"), 0o700); err != nil {
		t.Fatalf("write leftover: %v", err)
	}

	removed, err := removeInstallLeftovers(running, "", "", root)
	if err != nil {
		t.Fatalf("remove leftovers: %v", err)
	}
	if len(removed) != 1 || removed[0] != leftover {
		t.Fatalf("removed: %q", removed)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover still present: %v", err)
	}
	if _, err := os.Stat(running); err != nil {
		t.Fatalf("running binary missing: %v", err)
	}
}

func TestCmdInstallPersistsGOBINAndRemovesLeftover(t *testing.T) {
	root := t.TempDir()
	// The install path reports resolved paths, so the expected values must be
	// resolved too. See TestRemoveInstallLeftoversDeletesDefaultGoBinCopy.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	home := filepath.Join(root, "home")
	localBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(localBin, 0o700); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	running := filepath.Join(localBin, "multisubs")
	if err := os.WriteFile(running, []byte("path-copy"), 0o700); err != nil {
		t.Fatalf("write running: %v", err)
	}
	leftoverDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(leftoverDir, 0o700); err != nil {
		t.Fatalf("mkdir leftover: %v", err)
	}
	leftover := filepath.Join(leftoverDir, "multisubs")
	if err := os.WriteFile(leftover, []byte("go-bin-copy"), 0o700); err != nil {
		t.Fatalf("write leftover: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))

	oldExe := installExecutablePath
	oldRun := installRunGo
	t.Cleanup(func() {
		installExecutablePath = oldExe
		installRunGo = oldRun
	})
	var gotGobin, gotRef string
	installExecutablePath = func() string { return running }
	installRunGo = func(gobin, ref string) error {
		gotGobin = gobin
		gotRef = ref
		return nil
	}

	app, err := NewApp()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	out, err := captureStdout(t, func() error {
		return app.cmdInstall(nil)
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if gotGobin != localBin || gotRef != "latest" {
		t.Fatalf("go install target: gobin=%q ref=%q", gotGobin, gotRef)
	}
	if !strings.Contains(out, "installed "+installModulePath+"@latest to "+running) ||
		!strings.Contains(out, "set GOBIN in "+filepath.Join(home, ".zshrc")) ||
		!strings.Contains(out, "removed leftover binary "+leftover) {
		t.Fatalf("install output: %s", out)
	}
	zshrc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("read zshrc: %v", err)
	}
	if strings.Count(string(zshrc), installShellBlockBegin) != 1 ||
		!strings.Contains(string(zshrc), "export GOBIN='"+localBin+"'") {
		t.Fatalf("zshrc: %s", zshrc)
	}
	envBody, err := os.ReadFile(filepath.Join(root, "multi", "install.env"))
	if err != nil {
		t.Fatalf("read install.env: %v", err)
	}
	if !strings.Contains(string(envBody), "GOBIN="+localBin) ||
		!strings.Contains(string(envBody), "GOPRIVATE="+installPrivateModule) {
		t.Fatalf("install.env: %s", envBody)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover still present: %v", err)
	}

	out, err = captureStdout(t, func() error {
		return app.cmdInstall(nil)
	})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(out, "GOBIN already set in "+filepath.Join(home, ".zshrc")) {
		t.Fatalf("second install output: %s", out)
	}
	zshrc, err = os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("reread zshrc: %v", err)
	}
	if strings.Count(string(zshrc), installShellBlockBegin) != 1 {
		t.Fatalf("duplicate install block: %s", zshrc)
	}
}

func TestCmdInstallFailedGoInstallDoesNotTouchShellOrLeftover(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	localBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(localBin, 0o700); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	running := filepath.Join(localBin, "multisubs")
	if err := os.WriteFile(running, []byte("path-copy"), 0o700); err != nil {
		t.Fatalf("write running: %v", err)
	}
	leftoverDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(leftoverDir, 0o700); err != nil {
		t.Fatalf("mkdir leftover: %v", err)
	}
	leftover := filepath.Join(leftoverDir, "multisubs")
	if err := os.WriteFile(leftover, []byte("go-bin-copy"), 0o700); err != nil {
		t.Fatalf("write leftover: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("MULTISUBS_HOME", filepath.Join(root, "multi"))
	t.Setenv("MULTISUBS_DEFAULT_CODEX_HOME", filepath.Join(root, "default-codex"))

	oldExe := installExecutablePath
	oldRun := installRunGo
	t.Cleanup(func() {
		installExecutablePath = oldExe
		installRunGo = oldRun
	})
	installExecutablePath = func() string { return running }
	installRunGo = func(string, string) error {
		return &ExitError{Code: 1, Message: "go install failed"}
	}

	app, err := NewApp()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	err = app.cmdInstall(nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 || exitErr.Message != "go install failed" {
		t.Fatalf("failed install: %T (%v)", err, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed install wrote zshrc: %v", err)
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Fatalf("failed install removed leftover: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "multi")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed install created product state: %v", err)
	}
}

func TestCmdInstallRejectsExtraArgsWithoutInstall(t *testing.T) {
	app := newTestAppForCLI(t)
	oldRun := installRunGo
	t.Cleanup(func() {
		installRunGo = oldRun
	})
	installRunGo = func(string, string) error {
		t.Fatal("go install should not run for extra args")
		return nil
	}
	err := app.Run([]string{"install", "latest", "extra"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("extra args: %T (%v)", err, err)
	}
}

func TestQuoteInstallShellValueRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	if _, err := quoteInstallShellValue("/tmp/bin"); err != nil {
		t.Fatalf("safe path: %v", err)
	}
	if _, err := quoteInstallShellValue("/tmp/bin'oops"); err == nil {
		t.Fatal("quoted path should fail")
	}
}
