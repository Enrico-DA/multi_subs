package multicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type fakeClaudeCall struct {
	Kind string
	Args []string
	Env  []string
}

type fakeClaudeRunner struct {
	mu             sync.Mutex
	calls          []fakeClaudeCall
	capture        func(context.Context, []string, []string) ([]byte, []byte, error)
	run            func(context.Context, []string, []string) error
	runInteractive func([]string, []string) error
}

func (f *fakeClaudeRunner) Capture(ctx context.Context, args, env []string) ([]byte, []byte, error) {
	f.record("capture", args, env)
	if f.capture == nil {
		return nil, nil, fmt.Errorf("unexpected Claude capture call: %v", args)
	}
	return f.capture(ctx, append([]string(nil), args...), append([]string(nil), env...))
}

func (f *fakeClaudeRunner) Run(ctx context.Context, args, env []string) error {
	f.record("run", args, env)
	if f.run == nil {
		return fmt.Errorf("unexpected Claude run call: %v", args)
	}
	return f.run(ctx, append([]string(nil), args...), append([]string(nil), env...))
}

func (f *fakeClaudeRunner) RunReserved(ctx context.Context, args, env []string, _ *os.File) error {
	return f.Run(ctx, args, env)
}

func (f *fakeClaudeRunner) RunInteractive(args, env []string) error {
	f.record("interactive", args, env)
	if f.runInteractive == nil {
		return fmt.Errorf("unexpected interactive Claude call: %v", args)
	}
	return f.runInteractive(append([]string(nil), args...), append([]string(nil), env...))
}

func (f *fakeClaudeRunner) record(kind string, args, env []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeClaudeCall{Kind: kind, Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
}

func (f *fakeClaudeRunner) Calls() []fakeClaudeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeClaudeCall(nil), f.calls...)
}

func newClaudeTestApp(t *testing.T) (*App, *fakeClaudeRunner, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("MULTICODEX_HOME", filepath.Join(root, "multicodex"))
	t.Setenv("MULTICODEX_DEFAULT_CODEX_HOME", filepath.Join(root, "codex-default"))
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	runner := &fakeClaudeRunner{}
	app.claudeRunner = runner
	return app, runner, root
}

func createClaudeProfiles(t *testing.T, app *App, names ...string) map[string]claudeProfile {
	t.Helper()
	store := newClaudeStore(app.store.paths)
	if err := store.EnsureBaseDirs(); err != nil {
		t.Fatalf("EnsureBaseDirs: %v", err)
	}
	cfg := defaultClaudeConfig()
	profiles := make(map[string]claudeProfile, len(names))
	for _, name := range names {
		profile, err := store.CreateProfile(name)
		if err != nil {
			t.Fatalf("CreateProfile(%s): %v", name, err)
		}
		cfg.Profiles[name] = profile
		profiles[name] = profile
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save Claude config: %v", err)
	}
	return profiles
}

func fakeClaudeUsageEnvelope(session, weekly float64, fable *float64) []byte {
	result := fmt.Sprintf("Current session\n%.2f%% used\nResets in 2 hours\n\nCurrent week (all models)\n%.2f%% used\nResets Monday at 09:00", session, weekly)
	if fable != nil {
		result += fmt.Sprintf("\n\nCurrent week (Fable)\n%.2f%% used\nResets Tuesday at 10:00", *fable)
	}
	payload, _ := json.Marshal(map[string]any{
		"type":     "result",
		"is_error": false,
		"result":   result,
	})
	return payload
}

func fakeMalformedClaudeUsageEnvelope(marker string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"type":     "result",
		"is_error": false,
		"result": "Current session\n10% used " + marker + " 20% used\nResets in 2 hours\n\n" +
			"Current week (all models)\n30% used\nResets Monday at 09:00",
	})
	return payload
}

func fakeClaudeAuthJSON(loggedIn bool, email string) []byte {
	return fakeClaudeAuthJSONWithOrg(loggedIn, email, "org-"+email)
}

func fakeClaudeAuthJSONWithOrg(loggedIn bool, email, orgID string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"loggedIn":         loggedIn,
		"email":            email,
		"authMethod":       "claude.ai",
		"apiProvider":      "firstParty",
		"subscriptionType": "max",
		"orgId":            orgID,
	})
	return payload
}

func claudeConfigDirFromEnv(env []string) string {
	for _, item := range env {
		if len(item) >= len("CLAUDE_CONFIG_DIR=") && item[:len("CLAUDE_CONFIG_DIR=")] == "CLAUDE_CONFIG_DIR=" {
			return item[len("CLAUDE_CONFIG_DIR="):]
		}
	}
	return ""
}

func envContainsKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func assertPrivateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got %o want %o", path, got, want)
	}
}
