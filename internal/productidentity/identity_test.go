package productidentity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var identityFixturePaths = []string{
	".gitignore",
	".github/workflows/release.yml",
	"AGENTS.md",
	"CONTRIBUTING.md",
	"README.md",
	"cmd/multisubs/main.go",
	"docs/README.md",
	"docs/command-spec.md",
	"docs/decisions.md",
	"docs/security-and-privacy.md",
	"docs/upstream-sync.md",
	"go.mod",
	"internal/codexstate/env.go",
	"internal/codexstate/env_test.go",
	"internal/monitor/tui/model.go",
	"internal/monitor/usage/accounts.go",
	"internal/monitor/usage/accounts_test.go",
	"internal/monitor/usage/appserver.go",
	"internal/monitor/usage/oauth.go",
	"internal/multisubs/app.go",
	"internal/multisubs/claude_process.go",
	"internal/multisubs/claude_process_test.go",
	"internal/multisubs/cli_test.go",
	"internal/multisubs/doctor.go",
	"internal/multisubs/doctor_test.go",
	"internal/multisubs/help_completion_test.go",
	"internal/multisubs/paths_test.go",
	"internal/multisubs/process_test.go",
	"internal/multisubs/run_cli_test.go",
	"internal/multisubs/version.go",
}

func TestRepositoryIdentity(t *testing.T) {
	root := sourceRepositoryRoot(t)
	trackedPaths, err := trackedPathsFromGit(root)
	if err != nil {
		t.Fatalf("product identity cannot inspect the Git-tracked file universe: %v", err)
	}
	if errors := validateRepository(root, trackedPaths); len(errors) != 0 {
		t.Fatalf("product identity validation failed:\n%s", strings.Join(errors, "\n"))
	}
}

func TestProductIdentityMutations(t *testing.T) {
	t.Run("explicit fixture manifest passes unchanged", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		fixture.requireValid(t)
	})

	t.Run("active Go identity ignores retained comments", func(t *testing.T) {
		mutations := []struct {
			name        string
			path        string
			active      string
			replacement string
			errorText   string
		}{
			{
				name:        "command import",
				path:        "cmd/multisubs/main.go",
				active:      "\t\"github.com/Enrico-DA/multi_subs/internal/multisubs\"",
				replacement: "\tother \"example.invalid/other/module\"",
				errorText:   "command must import",
			},
			{
				name:        "RunCLI relationship",
				path:        "cmd/multisubs/main.go",
				active:      "\tif err := multisubs.RunCLI(os.Args[1:]); err != nil {",
				replacement: "\tif err := other.RunCLI(os.Args[1:]); err != nil {",
				errorText:   "main must call RunCLI",
			},
			{
				name:        "application name",
				path:        "internal/multisubs/version.go",
				active:      "const appName = \"multisubs\"",
				replacement: "const appName = \"other-command\"",
				errorText:   "active appName constant",
			},
			{
				name:        "monitor client name",
				path:        "internal/monitor/usage/appserver.go",
				active:      "const clientName = \"multisubs-monitor\"",
				replacement: "const clientName = \"other-monitor\"",
				errorText:   "active clientName constant",
			},
			{
				name:        "monitor client relationship",
				path:        "internal/monitor/usage/appserver.go",
				active:      "\t\t\tName:    clientName,",
				replacement: "\t\t\tName:    \"other-client\",",
				errorText:   "construct clientInfo",
			},
			{
				name:        "monitor version relationship",
				path:        "internal/monitor/usage/appserver.go",
				active:      "\t\t\tVersion: buildinfo.Version,",
				replacement: "\t\t\tVersion: \"development\",",
				errorText:   "construct clientInfo",
			},
			{
				name:        "OAuth User-Agent",
				path:        "internal/monitor/usage/oauth.go",
				active:      "\treq.Header.Set(\"User-Agent\", clientName+\"/\"+buildinfo.Version)",
				replacement: "\treq.Header.Set(\"User-Agent\", \"other-client\")",
				errorText:   "OAuth User-Agent",
			},
			{
				name:        "full TUI name",
				path:        "internal/monitor/tui/model.go",
				active:      "\ttitle := m.styles.title.Render(\" multisubs codex monitor \")",
				replacement: "\ttitle := m.styles.title.Render(\" other monitor \")",
				errorText:   "active monitor TUI title",
			},
			{
				name:        "compact TUI name",
				path:        "internal/monitor/tui/model.go",
				active:      "\t\tleft := m.styles.accent.Render(\"multisubs\") + \" \" + stateStyle.Render(stateText)",
				replacement: "\t\tleft := m.styles.accent.Render(\"other\") + \" \" + stateStyle.Render(stateText)",
				errorText:   "active compact monitor TUI",
			},
		}
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(mutation.name, func(t *testing.T) {
				fixture := newIdentityFixture(t)
				fixture.mutateGoAndRetainComment(t, mutation.path, mutation.active, mutation.replacement)
				requireErrorContaining(t, fixture.errors(), mutation.errorText)
			})
		}
	})

	t.Run("module declaration ignores retained comment", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		fixture.replaceExactLine(t, "go.mod", "module github.com/Enrico-DA/multi_subs", "module example.invalid/other/module")
		fixture.appendText(t, "go.mod", "\n// retained expected text: module github.com/Enrico-DA/multi_subs\n")
		requireErrorContaining(t, fixture.errors(), "module declaration")
	})

	t.Run("release identity ignores YAML and shell comments", func(t *testing.T) {
		mutations := []struct {
			name        string
			stepName    string
			active      string
			replacement string
			errorText   string
		}{
			{
				name:        "repository guard",
				active:      "    if: ${{ github.repository == 'Enrico-DA/multi_subs' }}",
				replacement: "    if: ${{ github.repository == 'someone/other-repository' }}",
				errorText:   "repository guard",
			},
			{
				name:        "archive step",
				active:      "      - name: Build release archives",
				replacement: "      - name: Build other archives",
				errorText:   "release archive step",
			},
			{
				name:        "archive run block",
				stepName:    "Build release archives",
				active:      "        run: |",
				replacement: "        run: >",
				errorText:   "literal run block",
			},
			{
				name:        "archive name",
				active:      "            name=\"multisubs_${version}_${os}_${arch}\"",
				replacement: "            name=\"other_${version}_${os}_${arch}\"",
				errorText:   "archive name",
			},
			{
				name:        "build command",
				active:      "            CGO_ENABLED=0 GOOS=\"${os}\" GOARCH=\"${arch}\" go build \\",
				replacement: "            CGO_ENABLED=1 GOOS=\"${os}\" GOARCH=\"${arch}\" go build \\",
				errorText:   "release build command",
			},
			{
				name:        "linker target",
				active:      "              -ldflags=\"-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}\" \\",
				replacement: "              -ldflags=\"-s -w -X example.invalid/other/internal/buildinfo.Version=${RELEASE_TAG}\" \\",
				errorText:   "linker target",
			},
			{
				name:        "output binary",
				active:      "              -o \"dist/${name}/multisubs\" \\",
				replacement: "              -o \"dist/${name}/other-command\" \\",
				errorText:   "output binary",
			},
			{
				name:        "build target",
				active:      "              ./cmd/multisubs",
				replacement: "              ./cmd/other-command",
				errorText:   "build target",
			},
			{
				name:        "copied files",
				active:      "            cp LICENSE README.md \"dist/${name}/\"",
				replacement: "            cp LICENSE \"dist/${name}/\"",
				errorText:   "copied release files",
			},
			{
				name:        "archive members",
				active:      "            tar -C \"dist/${name}\" -czf \"dist/${name}.tar.gz\" multisubs LICENSE README.md",
				replacement: "            tar -C \"dist/${name}\" -czf \"dist/${name}.tar.gz\" other-command LICENSE README.md",
				errorText:   "archive members",
			},
			{
				name:        "version assertion",
				active:      "              test \"$(\"dist/${name}/multisubs\" version)\" = \"multisubs ${RELEASE_TAG}\"",
				replacement: "              test \"$(\"dist/${name}/other-command\" version)\" = \"other-command ${RELEASE_TAG}\"",
				errorText:   "version assertion",
			},
			{
				name:        "checksum command",
				active:      "          sha256sum ./*.tar.gz > SHA256SUMS",
				replacement: "          sha256sum ./multisubs > SHA256SUMS",
				errorText:   "checksum command",
			},
		}
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(mutation.name, func(t *testing.T) {
				fixture := newIdentityFixture(t)
				if mutation.stepName == "" {
					fixture.mutateWorkflowAndRetainComment(t, mutation.active, mutation.replacement)
				} else {
					fixture.mutateWorkflowStepLineAndRetainComment(t, mutation.stepName, mutation.active, mutation.replacement)
				}
				requireErrorContaining(t, fixture.errors(), mutation.errorText)
			})
		}
	})

	t.Run("tracked old product path fails in ignored-looking directories", func(t *testing.T) {
		for _, directory := range []string{"plan", "dist"} {
			directory := directory
			t.Run(directory, func(t *testing.T) {
				fixture := newIdentityFixture(t)
				oldName := "Multi" + "Codex"
				relativePath := filepath.ToSlash(filepath.Join(directory, oldName, "placeholder.txt"))
				fixture.writeTracked(t, relativePath, "synthetic fixture\n")
				requireErrorContaining(t, fixture.errors(), "prohibited in Git-tracked paths")
			})
		}
	})

	t.Run("untracked old product path is ignored", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		oldName := "multi" + "codex"
		fixture.writeUntracked(t, filepath.ToSlash(filepath.Join("plan", oldName, "local.txt")), "local-only fixture\n")
		fixture.requireValid(t)
	})

	t.Run("tracked occurrence in ignored-looking directory fails", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		fixture.writeTracked(t, "plan/legacy-note.txt", "prohibited "+legacyProduct+" identity\n")
		requireErrorContaining(t, fixture.errors(), "outside the exact reviewed occurrence rules")
	})

	t.Run("active legacy environment read fails", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		oldHome := "MULTI" + "CODEX" + "_HOME"
		active := "\nfunc prohibitedLegacyReadForMutationTest() string {\n\treturn os.Getenv(\"" + oldHome + "\")\n}\n"
		comment := "\n/* retained expected text:\n\treturn os.Getenv(\"" + oldHome + "\")\n*/\n"
		fixture.appendText(t, "internal/multisubs/app.go", active+comment)
		requireErrorContaining(t, fixture.errors(), "active os.Getenv read")
	})

	t.Run("active legacy environment lookup fails", func(t *testing.T) {
		fixture := newIdentityFixture(t)
		active := "\nfunc prohibitedLegacyLookupForMutationTest() {\n\t_, _ = os.LookupEnv(\"MULTI\" + \"CODEX_HOME\")\n}\n"
		comment := "\n// retained expected text: os.LookupEnv(\"MULTI\" + \"CODEX_HOME\")\n"
		fixture.appendText(t, "internal/multisubs/app.go", active+comment)
		requireErrorContaining(t, fixture.errors(), "active os.LookupEnv read")
	})

	t.Run("every mandatory legacy occurrence fails when deleted", func(t *testing.T) {
		categories := make(map[legacyCategory]bool)
		for index, rule := range requiredLegacyLines {
			index := index
			rule := rule
			categories[rule.category] = true
			t.Run(fmt.Sprintf("%03d_%s", index, rule.category), func(t *testing.T) {
				fixture := newIdentityFixture(t)
				fixture.deleteExactLine(t, rule.path, rule.line)
				errors := fixture.errors()
				requireErrorContaining(t, errors, "required "+string(rule.category)+" legacy occurrence")
			})
		}
		for _, category := range []legacyCategory{
			legacyAttribution,
			legacyUpstreamMap,
			legacyRejection,
			legacyDenylist,
			legacyIgnore,
			legacyLeakProtection,
			legacyNegativeTest,
		} {
			if !categories[category] {
				t.Errorf("mandatory legacy category %q has no mutation coverage", category)
			}
		}
	})
}

type identityFixture struct {
	root    string
	tracked []string
}

func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	sourceRoot := sourceRepositoryRoot(t)
	fixture := &identityFixture{
		root:    t.TempDir(),
		tracked: append([]string(nil), identityFixturePaths...),
	}
	for _, relativePath := range fixture.tracked {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("read fixture source %s: %v", relativePath, err)
		}
		fixture.writeFile(t, relativePath, data)
	}
	return fixture
}

func sourceRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate product identity test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}

func (fixture *identityFixture) errors() []string {
	return validateRepository(fixture.root, fixture.tracked)
}

func (fixture *identityFixture) requireValid(t *testing.T) {
	t.Helper()
	if errors := fixture.errors(); len(errors) != 0 {
		t.Fatalf("identity fixture is invalid:\n%s", strings.Join(errors, "\n"))
	}
}

func (fixture *identityFixture) mutateGoAndRetainComment(t *testing.T, relativePath, active, replacement string) {
	t.Helper()
	fixture.replaceExactLine(t, relativePath, active, replacement)
	fixture.appendText(t, relativePath, "\n/* retained expected text:\n"+active+"\n*/\n")
}

func (fixture *identityFixture) mutateWorkflowAndRetainComment(t *testing.T, active, replacement string) {
	t.Helper()
	const relativePath = ".github/workflows/release.yml"
	data := fixture.readFile(t, relativePath)
	lines := strings.Split(string(data), "\n")
	found := 0
	var changed []string
	for _, line := range lines {
		if line == active {
			found++
			changed = append(changed, replacement)
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			changed = append(changed, indent+"# retained expected text: "+strings.TrimSpace(active))
			continue
		}
		changed = append(changed, line)
	}
	if found != 1 {
		t.Fatalf("%s: expected one active workflow line %q; found %d", relativePath, active, found)
	}
	fixture.writeFile(t, relativePath, []byte(strings.Join(changed, "\n")))
}

func (fixture *identityFixture) mutateWorkflowStepLineAndRetainComment(t *testing.T, stepName, active, replacement string) {
	t.Helper()
	const relativePath = ".github/workflows/release.yml"
	data := fixture.readFile(t, relativePath)
	lines := strings.Split(string(data), "\n")
	stepLine := "      - name: " + stepName
	stepIndex := -1
	stepCount := 0
	for index, line := range lines {
		if line == stepLine {
			stepIndex = index
			stepCount++
		}
	}
	if stepCount != 1 {
		t.Fatalf("%s: expected one workflow step %q; found %d", relativePath, stepName, stepCount)
	}
	stepEnd := len(lines)
	for index := stepIndex + 1; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], "      - name: ") {
			stepEnd = index
			break
		}
	}
	found := 0
	var changed []string
	for index, line := range lines {
		if index > stepIndex && index < stepEnd && line == active {
			found++
			changed = append(changed, replacement)
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			changed = append(changed, indent+"# retained expected text: "+strings.TrimSpace(active))
			continue
		}
		changed = append(changed, line)
	}
	if found != 1 {
		t.Fatalf("%s: expected one active workflow line %q in step %q; found %d", relativePath, active, stepName, found)
	}
	fixture.writeFile(t, relativePath, []byte(strings.Join(changed, "\n")))
}

func (fixture *identityFixture) replaceExactLine(t *testing.T, relativePath, active, replacement string) {
	t.Helper()
	data := fixture.readFile(t, relativePath)
	lines := strings.Split(string(data), "\n")
	found := 0
	for index, line := range lines {
		if line == active {
			found++
			lines[index] = replacement
		}
	}
	if found != 1 {
		t.Fatalf("%s: expected one active line %q; found %d", relativePath, active, found)
	}
	fixture.writeFile(t, relativePath, []byte(strings.Join(lines, "\n")))
}

func (fixture *identityFixture) deleteExactLine(t *testing.T, relativePath, active string) {
	t.Helper()
	data := fixture.readFile(t, relativePath)
	lines := strings.Split(string(data), "\n")
	found := 0
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == active {
			found++
			continue
		}
		kept = append(kept, line)
	}
	if found != 1 {
		t.Fatalf("%s: expected one mandatory line %q; found %d", relativePath, active, found)
	}
	fixture.writeFile(t, relativePath, []byte(strings.Join(kept, "\n")))
}

func (fixture *identityFixture) appendText(t *testing.T, relativePath, text string) {
	t.Helper()
	data := fixture.readFile(t, relativePath)
	fixture.writeFile(t, relativePath, append(data, []byte(text)...))
}

func (fixture *identityFixture) writeTracked(t *testing.T, relativePath, contents string) {
	t.Helper()
	fixture.writeFile(t, relativePath, []byte(contents))
	fixture.tracked = append(fixture.tracked, relativePath)
}

func (fixture *identityFixture) writeUntracked(t *testing.T, relativePath, contents string) {
	t.Helper()
	fixture.writeFile(t, relativePath, []byte(contents))
}

func (fixture *identityFixture) readFile(t *testing.T, relativePath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read fixture file %s: %v", relativePath, err)
	}
	return data
}

func (fixture *identityFixture) writeFile(t *testing.T, relativePath string, data []byte) {
	t.Helper()
	target := filepath.Join(fixture.root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("write fixture file %s: %v", relativePath, err)
	}
}

func requireErrorContaining(t *testing.T, errors []string, expected string) {
	t.Helper()
	for _, err := range errors {
		if strings.Contains(err, expected) {
			return
		}
	}
	t.Fatalf("expected identity error containing %q; got:\n%s", expected, strings.Join(errors, "\n"))
}
