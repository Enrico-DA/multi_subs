package multisubs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductIdentityInterlock(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate identity test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))

	goMod := readIdentityFile(t, filepath.Join(root, "go.mod"))
	if !strings.HasPrefix(goMod, "module github.com/Enrico-DA/multi_subs\n") {
		t.Fatalf("unexpected module identity: %q", strings.SplitN(goMod, "\n", 2)[0])
	}

	commandEntries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd directory: %v", err)
	}
	if len(commandEntries) != 1 || commandEntries[0].Name() != "multisubs" || !commandEntries[0].IsDir() {
		t.Fatalf("cmd must contain only cmd/multisubs, got %v", commandEntries)
	}

	mainSource := readIdentityFile(t, filepath.Join(root, "cmd", "multisubs", "main.go"))
	for _, want := range []string{
		`"github.com/Enrico-DA/multi_subs/internal/multisubs"`,
		"multisubs.RunCLI",
	} {
		if !strings.Contains(mainSource, want) {
			t.Errorf("entrypoint missing %q", want)
		}
	}
	coreSource := readIdentityFile(t, filepath.Join(root, "internal", "multisubs", "app.go"))
	if !strings.HasPrefix(coreSource, "package multisubs\n") {
		t.Fatal("core package identity is not multisubs")
	}

	release := readIdentityFile(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, want := range []string{
		"github.repository == 'Enrico-DA/multi_subs'",
		"github.com/Enrico-DA/multi_subs/internal/buildinfo.Version",
		`./cmd/multisubs`,
		`multisubs_${version}_${os}_${arch}`,
		`/multisubs`,
	} {
		if !strings.Contains(release, want) {
			t.Errorf("release identity missing %q", want)
		}
	}

	if appName != "multisubs" {
		t.Fatalf("binary name is %q, want multisubs", appName)
	}
}

func readIdentityFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
