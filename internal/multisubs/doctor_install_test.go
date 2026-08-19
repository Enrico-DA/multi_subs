package multisubs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoInstallTargetCheckOKWhenGOBINMatchesRunningBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runningDir := filepath.Join(root, "local-bin")
	running := filepath.Join(runningDir, "multisubs")
	check := goInstallTargetCheck(goInstallTargetInput{
		runningPath: running,
		goBin:       runningDir,
		goPath:      filepath.Join(root, "go"),
		home:        root,
		installFileExists: func(string) bool {
			return false
		},
	})
	if check.Name != "go install target" || check.Status != "ok" {
		t.Fatalf("matching GOBIN: %+v", check)
	}
	if !strings.Contains(check.Details, runningDir) {
		t.Fatalf("matching GOBIN details: %q", check.Details)
	}
}

func TestGoInstallTargetCheckWarnsWhenGoInstallWouldWriteElsewhere(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	running := filepath.Join(root, "local-bin", "multisubs")
	goPath := filepath.Join(root, "go")
	check := goInstallTargetCheck(goInstallTargetInput{
		runningPath: running,
		goPath:      goPath,
		home:        root,
		installFileExists: func(string) bool {
			return false
		},
	})
	if check.Status != "warn" {
		t.Fatalf("expected warn when install dir differs, got %+v", check)
	}
	if !strings.Contains(check.Details, filepath.Join(goPath, "bin")) ||
		!strings.Contains(check.Details, running) ||
		!strings.Contains(check.Details, "run multisubs install") {
		t.Fatalf("differing install dir details: %q", check.Details)
	}
}

func TestGoInstallTargetCheckWarnsAboutLeftoverBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runningDir := filepath.Join(root, "local-bin")
	running := filepath.Join(runningDir, "multisubs")
	leftover := filepath.Join(root, "go", "bin", "multisubs")
	check := goInstallTargetCheck(goInstallTargetInput{
		runningPath: running,
		goBin:       runningDir,
		goPath:      filepath.Join(root, "go"),
		home:        root,
		installFileExists: func(path string) bool {
			return filepath.Clean(path) == leftover
		},
	})
	if check.Status != "warn" {
		t.Fatalf("expected leftover warn, got %+v", check)
	}
	if !strings.Contains(check.Details, leftover) ||
		!strings.Contains(check.Details, "second binary") ||
		!strings.Contains(check.Details, "run multisubs install") {
		t.Fatalf("leftover details: %q", check.Details)
	}
}

func TestGoInstallTargetCheckSkipsNonMultisubsProcess(t *testing.T) {
	t.Parallel()

	check := goInstallTargetCheck(goInstallTargetInput{
		runningPath: filepath.Join(t.TempDir(), "multisubs.test"),
		home:        t.TempDir(),
	})
	if check.Status != "ok" || !strings.Contains(check.Details, "skipped") {
		t.Fatalf("test process skip: %+v", check)
	}
}

func TestGoInstallBinDirUsesHomeGoWhenGOPATHIsEmpty(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "home")
	got := goInstallBinDir("", "", home)
	want := filepath.Join(home, "go", "bin")
	if got != want {
		t.Fatalf("default install dir: got %q want %q", got, want)
	}
}
