package multisubs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestClaudeFableResolver(homeDirectory, workingDirectory string) *localClaudeFableApplicabilityResolver {
	resolver := newLocalClaudeFableApplicabilityResolver()
	resolver.homeDirectory = func() (string, error) { return homeDirectory, nil }
	resolver.workingDirectory = func() (string, error) { return workingDirectory, nil }
	resolver.inspectLocalManagedSettings = false
	resolver.opaqueSettings = func(claudeTarget) claudeOpaqueSettings {
		return claudeOpaqueSettings{
			Settings:       emptyClaudeRelevantSettings(),
			AccountDefault: knownClaudeSetting("claude-sonnet-4-5-20250929"),
		}
	}
	return resolver
}

func writeClaudeSettingsTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create settings parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

func TestClaudeFableApplicabilityUsesCandidateUserSettingsRoot(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workingDirectory := filepath.Join(root, "work")
	managedConfig := filepath.Join(root, "managed", "config")
	if err := os.MkdirAll(workingDirectory, 0o700); err != nil {
		t.Fatalf("create working directory: %v", err)
	}
	writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	writeClaudeSettingsTestFile(t, filepath.Join(managedConfig, "settings.json"), `{"model":"fable"}`)

	resolver := newTestClaudeFableResolver(home, workingDirectory)
	intent := resolver.ParseIntent([]string{"--setting-sources=user", "prompt"}, nil)
	if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fableNotApplicable {
		t.Fatalf("default applicability: got %v want %v", got, fableNotApplicable)
	}
	if got := resolver.Resolve(intent, claudeTarget{Name: "work", Kind: "managed", ConfigDir: managedConfig}); got != fableApplicable {
		t.Fatalf("managed applicability: got %v want %v", got, fableApplicable)
	}
}

func TestClaudeFableApplicabilityIncludesPersistentFallbackWithCLIPrimary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), `{"fallbackModel":["fable"]}`)
	resolver := newTestClaudeFableResolver(home, root)

	intent := resolver.ParseIntent([]string{"--model", "sonnet", "--setting-sources=user", "prompt"}, nil)
	if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fableApplicable {
		t.Fatalf("applicability: got %v want %v", got, fableApplicable)
	}
}

func TestClaudeFableApplicabilityCanProveKnownAbsentFallback(t *testing.T) {
	root := t.TempDir()
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
	intent := resolver.ParseIntent([]string{"--model=claude-sonnet-4-5-20250929", "--setting-sources=", "prompt"}, nil)

	if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fableNotApplicable {
		t.Fatalf("applicability: got %v want %v", got, fableNotApplicable)
	}
}

func TestClaudeFableApplicabilityUsesCandidateEnvironmentSettingsAndAliases(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		args     []string
		want     fableApplicability
	}{
		{
			name:     "candidate ANTHROPIC_MODEL",
			settings: `{"env":{"ANTHROPIC_MODEL":"fable"}}`,
			args:     []string{"--setting-sources=user", "prompt"},
			want:     fableApplicable,
		},
		{
			name:     "Sonnet alias maps to Fable",
			settings: `{"env":{"ANTHROPIC_DEFAULT_SONNET_MODEL":"fable"}}`,
			args:     []string{"--model=sonnet", "--fallback-model=claude-haiku-3-5-20241022", "--setting-sources=user", "prompt"},
			want:     fableApplicable,
		},
		{
			name:     "opaque ID equals Fable mapping",
			settings: `{"env":{"ANTHROPIC_DEFAULT_FABLE_MODEL":"team-model-17"}}`,
			args:     []string{"--model=team-model-17", "--fallback-model=claude-haiku-3-5-20241022", "--setting-sources=user", "prompt"},
			want:     fableApplicable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), test.settings)
			resolver := newTestClaudeFableResolver(home, root)
			intent := resolver.ParseIntent(test.args, nil)
			if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != test.want {
				t.Fatalf("applicability: got %v want %v", got, test.want)
			}
		})
	}
}

func TestClassifyClaudeModelValue(t *testing.T) {
	emptyEnvironment := emptyClaudeRelevantSettings().Environment
	uncertainDefaultContext := claudeModelClassificationContext{
		Environment:    emptyEnvironment,
		AccountDefault: uncertainClaudeSetting(),
	}
	tests := []struct {
		model string
		want  fableApplicability
	}{
		{model: "fable", want: fableApplicable},
		{model: "claude-fable-latest", want: fableApplicable},
		{model: "best", want: fablePossible},
		{model: "default", want: fablePossible},
		{model: "sonnet", want: fableNotApplicable},
		{model: "opus", want: fableNotApplicable},
		{model: "haiku", want: fableNotApplicable},
		{model: "claude-sonnet-4-5-20250929", want: fableNotApplicable},
		{model: "claude-3-5-haiku-20241022", want: fableNotApplicable},
		{model: "custom-model", want: fablePossible},
		{model: "", want: fablePossible},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			if got := classifyClaudeModelValue(test.model, uncertainDefaultContext, make(map[string]bool)); got != test.want {
				t.Fatalf("classification: got %v want %v", got, test.want)
			}
		})
	}

	cyclicEnvironment := emptyClaudeRelevantSettings().Environment
	cyclicEnvironment["ANTHROPIC_DEFAULT_SONNET_MODEL"] = knownClaudeSetting("opus")
	cyclicEnvironment["ANTHROPIC_DEFAULT_OPUS_MODEL"] = knownClaudeSetting("sonnet")
	if got := classifyClaudeModelValue("sonnet", claudeModelClassificationContext{
		Environment:    cyclicEnvironment,
		AccountDefault: uncertainClaudeSetting(),
	}, make(map[string]bool)); got != fablePossible {
		t.Fatalf("cyclic alias classification: got %v want %v", got, fablePossible)
	}
}

func TestClaudeFableApplicabilityCLIAndFallbackPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		args     []string
		want     fableApplicability
	}{
		{
			name:     "CLI model overrides settings environment model",
			settings: `{"env":{"ANTHROPIC_MODEL":"fable"}}`,
			args:     []string{"--model=claude-sonnet-4-5-20250929", "--fallback-model=claude-haiku-3-5-20241022", "--setting-sources=user"},
			want:     fableNotApplicable,
		},
		{
			name:     "CLI fallback overrides persistent fallback",
			settings: `{"fallbackModel":["fable"]}`,
			args:     []string{"--model=claude-sonnet-4-5-20250929", "--fallback-model=claude-haiku-3-5-20241022", "--setting-sources=user"},
			want:     fableNotApplicable,
		},
		{
			name:     "primary override does not suppress persistent fallback",
			settings: `{"fallbackModel":["fable"]}`,
			args:     []string{"--model=claude-sonnet-4-5-20250929", "--setting-sources=user"},
			want:     fableApplicable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), test.settings)
			resolver := newTestClaudeFableResolver(home, root)
			intent := resolver.ParseIntent(test.args, nil)
			if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != test.want {
				t.Fatalf("applicability: got %v want %v", got, test.want)
			}
		})
	}
}

func TestClaudeFableApplicabilityReadsInlineAndPathSettings(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "routing.json")
	writeClaudeSettingsTestFile(t, settingsPath, `{"model":"fable"}`)
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
	for _, args := range [][]string{
		{"--setting-sources=", "--settings", `{"model":"fable"}`},
		{"--setting-sources=", "--settings=routing.json"},
	} {
		intent := resolver.ParseIntent(args, nil)
		if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fableApplicable {
			t.Fatalf("args %#v applicability: got %v want %v", args, got, fableApplicable)
		}
	}
}

func TestClaudeFableApplicabilityHonorsSettingSources(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("create repository marker: %v", err)
	}
	writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"fable"}`)
	writeClaudeSettingsTestFile(t, filepath.Join(root, ".claude", "settings.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	resolver := newTestClaudeFableResolver(home, root)
	tests := []struct {
		name string
		args []string
		want fableApplicability
	}{
		{name: "omitted", args: []string{"prompt"}, want: fableNotApplicable},
		{name: "user only", args: []string{"--setting-sources=user", "prompt"}, want: fableApplicable},
		{name: "empty", args: []string{"--setting-sources=", "prompt"}, want: fableNotApplicable},
		{name: "invalid", args: []string{"--setting-sources=user,unknown", "prompt"}, want: fablePossible},
		{name: "duplicate option", args: []string{"--setting-sources=user", "--setting-sources=project", "prompt"}, want: fablePossible},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := resolver.ParseIntent(test.args, nil)
			if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != test.want {
				t.Fatalf("applicability: got %v want %v", got, test.want)
			}
		})
	}
}

func TestClaudeFableApplicabilityUsesNestedRepositoryRootAndInvocationDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("create repository marker: %v", err)
	}
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	writeClaudeSettingsTestFile(t, filepath.Join(root, ".claude", "settings.json"), `{"model":"fable"}`)
	writeClaudeSettingsTestFile(t, filepath.Join(nested, "override.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), nested)

	projectIntent := resolver.ParseIntent([]string{"--setting-sources=project"}, nil)
	if got := resolver.Resolve(projectIntent, claudeTarget{Name: "default", Kind: "default"}); got != fableApplicable {
		t.Fatalf("nested project applicability: got %v want %v", got, fableApplicable)
	}
	explicitIntent := resolver.ParseIntent([]string{"--setting-sources=project", "--settings=override.json"}, nil)
	if got := resolver.Resolve(explicitIntent, claudeTarget{Name: "default", Kind: "default"}); got != fableNotApplicable {
		t.Fatalf("relative explicit applicability: got %v want %v", got, fableNotApplicable)
	}

	outside := t.TempDir()
	writeClaudeSettingsTestFile(t, filepath.Join(outside, ".claude", "settings.json"), `{"model":"fable"}`)
	outsideResolver := newTestClaudeFableResolver(filepath.Join(outside, "home"), outside)
	outsideIntent := outsideResolver.ParseIntent([]string{"--setting-sources=project"}, nil)
	if got := outsideResolver.Resolve(outsideIntent, claudeTarget{Name: "default", Kind: "default"}); got != fableApplicable {
		t.Fatalf("outside-repository applicability: got %v want %v", got, fableApplicable)
	}
}

func TestClaudeFableApplicabilityHandlesSessionRestorationAndMalformedCLIFields(t *testing.T) {
	root := t.TempDir()
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
	tests := []struct {
		name string
		args []string
		want fableApplicability
	}{
		{
			name: "session with full overrides",
			args: []string{
				"--resume",
				"--model=claude-sonnet-4-5-20250929",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
			},
			want: fableNotApplicable,
		},
		{
			name: "session without conclusive primary",
			args: []string{
				"--continue",
				"--model=sonnet",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
			},
			want: fablePossible,
		},
		{
			name: "duplicate model",
			args: []string{
				"--model=claude-sonnet-4-5-20250929",
				"--model=claude-haiku-3-5-20241022",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
			},
			want: fablePossible,
		},
		{
			name: "unknown model form",
			args: []string{
				"--model:claude-sonnet-4-5-20250929",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
			},
			want: fablePossible,
		},
		{
			name: "short model equals form",
			args: []string{
				"-m=claude-sonnet-4-5-20250929",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
			},
			want: fableNotApplicable,
		},
		{
			name: "standalone separator",
			args: []string{
				"--model=claude-sonnet-4-5-20250929",
				"--fallback-model=claude-haiku-3-5-20241022",
				"--setting-sources=",
				"--",
				"--model=fable",
			},
			want: fableNotApplicable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := resolver.ParseIntent(test.args, nil)
			if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != test.want {
				t.Fatalf("applicability: got %v want %v", got, test.want)
			}
		})
	}
}

func TestClaudeFableApplicabilityReadsSortedLocalManagedSettings(t *testing.T) {
	root := t.TempDir()
	managedDirectory := filepath.Join(root, "policy")
	writeClaudeSettingsTestFile(t, filepath.Join(managedDirectory, "managed-settings.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	writeClaudeSettingsTestFile(t, filepath.Join(managedDirectory, "managed-settings.d", "20-fable.json"), `{"model":"fable"}`)
	writeClaudeSettingsTestFile(t, filepath.Join(managedDirectory, "managed-settings.d", "10-sonnet.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
	resolver.inspectLocalManagedSettings = true
	resolver.managedSettingsDirectory = managedDirectory
	intent := resolver.ParseIntent([]string{"--setting-sources="}, nil)

	if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fableApplicable {
		t.Fatalf("applicability: got %v want %v", got, fableApplicable)
	}
}

type unreadableClaudeSettingsFileSystem struct {
	claudeSettingsFileSystem
	unreadablePath string
}

func (fileSystem unreadableClaudeSettingsFileSystem) Open(path string) (claudeSettingsFile, error) {
	if path == fileSystem.unreadablePath {
		return nil, errors.New("synthetic-secret-marker")
	}
	return fileSystem.claudeSettingsFileSystem.Open(path)
}

func TestClaudeSettingsInspectionFailuresBecomePossible(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"malformed.json":  `{"model":`,
		"wrong-type.json": `{"model":42}`,
		"unreadable.json": `{"model":"claude-sonnet-4-5-20250929"}`,
	}
	for name, contents := range files {
		writeClaudeSettingsTestFile(t, filepath.Join(root, name), contents)
	}
	oversizedPath := filepath.Join(root, "oversized.json")
	if err := os.WriteFile(oversizedPath, make([]byte, claudeSettingsFileLimit+1), 0o600); err != nil {
		t.Fatalf("write oversized settings: %v", err)
	}
	nonRegularPath := filepath.Join(root, "non-regular.json")
	if err := os.Mkdir(nonRegularPath, 0o700); err != nil {
		t.Fatalf("create non-regular settings: %v", err)
	}

	tests := []struct {
		name       string
		path       string
		unreadable bool
	}{
		{name: "malformed", path: filepath.Join(root, "malformed.json")},
		{name: "wrong type", path: filepath.Join(root, "wrong-type.json")},
		{name: "oversized", path: oversizedPath},
		{name: "non-regular", path: nonRegularPath},
		{name: "unreadable", path: filepath.Join(root, "unreadable.json"), unreadable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
			if test.unreadable {
				resolver.fileSystem = unreadableClaudeSettingsFileSystem{
					claudeSettingsFileSystem: resolver.fileSystem,
					unreadablePath:           test.path,
				}
			}
			intent := resolver.ParseIntent([]string{"--setting-sources=", "--settings=" + test.path}, nil)
			if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fablePossible {
				t.Fatalf("applicability: got %v want %v", got, fablePossible)
			}
		})
	}
}

func TestClaudeMalformedUserSettingsAreCandidateLocal(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	managedConfig := filepath.Join(root, "managed", "config")
	writeClaudeSettingsTestFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"synthetic-secret-marker"`)
	writeClaudeSettingsTestFile(t, filepath.Join(managedConfig, "settings.json"), `{"model":"claude-sonnet-4-5-20250929"}`)
	resolver := newTestClaudeFableResolver(home, root)
	intent := resolver.ParseIntent([]string{"--setting-sources=user"}, nil)

	if got := resolver.Resolve(intent, claudeTarget{Name: "default", Kind: "default"}); got != fablePossible {
		t.Fatalf("default applicability: got %v want %v", got, fablePossible)
	}
	if got := resolver.Resolve(intent, claudeTarget{Name: "work", Kind: "managed", ConfigDir: managedConfig}); got != fableNotApplicable {
		t.Fatalf("managed applicability: got %v want %v", got, fableNotApplicable)
	}
	if message := claudeSettingsReadError("user settings").Error(); strings.Contains(message, "synthetic-secret-marker") {
		t.Fatalf("safe settings error exposed marker: %q", message)
	}

	sharedRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(sharedRoot, ".git"), 0o700); err != nil {
		t.Fatalf("create shared repository marker: %v", err)
	}
	writeClaudeSettingsTestFile(t, filepath.Join(sharedRoot, ".claude", "settings.json"), `{"model":"synthetic-secret-marker"`)
	sharedResolver := newTestClaudeFableResolver(filepath.Join(sharedRoot, "home"), sharedRoot)
	sharedIntent := sharedResolver.ParseIntent([]string{"--setting-sources=project"}, nil)
	for _, target := range []claudeTarget{
		{Name: "default", Kind: "default"},
		{Name: "work", Kind: "managed", ConfigDir: filepath.Join(sharedRoot, "managed")},
	} {
		if got := sharedResolver.Resolve(sharedIntent, target); got != fablePossible {
			t.Fatalf("shared malformed applicability for %s: got %v want %v", target.Name, got, fablePossible)
		}
	}
}

func TestClaudeOpaqueSettingsStayCandidateLocalAndFullCLIOverridesAreConclusive(t *testing.T) {
	root := t.TempDir()
	resolver := newTestClaudeFableResolver(filepath.Join(root, "home"), root)
	resolver.opaqueSettings = func(target claudeTarget) claudeOpaqueSettings {
		if target.Name == "uncertain" {
			return claudeOpaqueSettings{
				Settings:       uncertainClaudeRelevantSettings(),
				AccountDefault: uncertainClaudeSetting(),
			}
		}
		return claudeOpaqueSettings{
			Settings:       emptyClaudeRelevantSettings(),
			AccountDefault: knownClaudeSetting("claude-sonnet-4-5-20250929"),
		}
	}

	withoutFallback := resolver.ParseIntent([]string{"--model=claude-sonnet-4-5-20250929", "--setting-sources="}, nil)
	if got := resolver.Resolve(withoutFallback, claudeTarget{Name: "uncertain", Kind: "managed", ConfigDir: filepath.Join(root, "uncertain")}); got != fablePossible {
		t.Fatalf("opaque fallback applicability: got %v want %v", got, fablePossible)
	}
	fullOverrides := resolver.ParseIntent([]string{
		"--model=claude-sonnet-4-5-20250929",
		"--fallback-model=claude-haiku-3-5-20241022",
		"--setting-sources=",
	}, nil)
	if got := resolver.Resolve(fullOverrides, claudeTarget{Name: "uncertain", Kind: "managed", ConfigDir: filepath.Join(root, "uncertain")}); got != fableNotApplicable {
		t.Fatalf("full CLI override applicability: got %v want %v", got, fableNotApplicable)
	}
	if got := resolver.Resolve(withoutFallback, claudeTarget{Name: "known", Kind: "managed", ConfigDir: filepath.Join(root, "known")}); got != fableNotApplicable {
		t.Fatalf("known candidate applicability: got %v want %v", got, fableNotApplicable)
	}
}

type recordingClaudeFableResolver struct {
	args        []string
	environment []string
	parseCalls  int
}

func (resolver *recordingClaudeFableResolver) ParseIntent(args, environment []string) claudeCLIIntent {
	resolver.parseCalls++
	resolver.args = append([]string(nil), args...)
	resolver.environment = append([]string(nil), environment...)
	return claudeCLIIntent{}
}

func (*recordingClaudeFableResolver) Resolve(claudeCLIIntent, claudeTarget) fableApplicability {
	return fableNotApplicable
}

func TestClaudeExecDoesNotMutateForwardedArgsOrSafeEnvironment(t *testing.T) {
	app, runner, _ := newClaudeTestApp(t)
	resolver := &recordingClaudeFableResolver{}
	app.claudeFableResolver = resolver
	t.Setenv("CLAUDE_FABLE_TEST_PASSTHROUGH", "synthetic-safe-value")
	t.Setenv("ANTHROPIC_MODEL", "claude-sonnet-4-5-20250929")
	setFakeUsageCapture(t, runner, map[string][]byte{
		"": fakeClaudeUsageEnvelope(1, 2, nil),
	})
	args := []string{"--model", "claude-sonnet-4-5-20250929", "prompt", "--", "--model=fable"}
	originalArgs := append([]string(nil), args...)
	runner.run = func(_ context.Context, childArgs, environment []string) error {
		if !reflect.DeepEqual(childArgs, append([]string{"-p"}, originalArgs...)) {
			t.Fatalf("child args changed: got %#v want %#v", childArgs, append([]string{"-p"}, originalArgs...))
		}
		if !envContainsKey(environment, "CLAUDE_FABLE_TEST_PASSTHROUGH") ||
			!envContainsKey(environment, "ANTHROPIC_MODEL") {
			t.Fatalf("safe environment settings were not preserved: %q", environment)
		}
		return nil
	}
	if err := app.cmdClaudeExec(args); err != nil {
		t.Fatalf("Claude exec: %v", err)
	}
	if !reflect.DeepEqual(args, originalArgs) || !reflect.DeepEqual(resolver.args, originalArgs) {
		t.Fatalf("original args changed: args=%#v parsed=%#v want=%#v", args, resolver.args, originalArgs)
	}
	if resolver.parseCalls != 1 {
		t.Fatalf("CLI intent parse count: got %d want 1", resolver.parseCalls)
	}
	if !containsClaudeEnvironmentEntry(resolver.environment, "CLAUDE_FABLE_TEST_PASSTHROUGH=synthetic-safe-value") {
		t.Fatalf("resolver did not receive the invocation environment")
	}
}

func containsClaudeEnvironmentEntry(environment []string, want string) bool {
	for _, entry := range environment {
		if entry == want {
			return true
		}
	}
	return false
}
