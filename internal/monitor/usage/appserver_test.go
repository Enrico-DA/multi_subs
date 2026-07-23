package usage

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Enrico-DA/multicodex/internal/codexstate"
)

func TestRawAndDefaultAppServerArgsDoNotForceManagedAuth(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]*AppServerSource{
		"default": NewAppServerSource(),
		"raw":     NewAppServerSourceForHome("/raw"),
	} {
		source := source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if source.managedProfile {
				t.Fatal("raw app-server source marked as managed")
			}
			got := newAppServerSession(source.codexHome, source.managedProfile).commandArgs()
			want := []string{"-s", "read-only", "-a", "untrusted", "app-server"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("raw app-server args: got %#v want %#v", got, want)
			}
			for _, arg := range got {
				if arg == codexstate.ManagedAuthConfig {
					t.Fatalf("raw app-server args received managed auth override: %#v", got)
				}
			}
		})
	}
}

func TestManagedAppServerArgsForceOneAuthOverrideBeforeSubcommand(t *testing.T) {
	t.Parallel()

	source := newManagedAppServerSourceForHome("/managed")
	if !source.managedProfile {
		t.Fatal("managed app-server source not marked as managed")
	}
	got := newAppServerSession(source.codexHome, source.managedProfile).commandArgs()
	want := []string{
		"-s", "read-only",
		"-a", "untrusted",
		"-c", codexstate.ManagedAuthConfig,
		"app-server",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("managed app-server args: got %#v want %#v", got, want)
	}
	overrideCount := 0
	overrideIndex := -1
	appServerIndex := -1
	for i, arg := range got {
		switch arg {
		case codexstate.ManagedAuthConfig:
			overrideCount++
			overrideIndex = i
		case "app-server":
			appServerIndex = i
		}
	}
	if overrideCount != 1 {
		t.Fatalf("managed auth override count: got %d in %#v", overrideCount, got)
	}
	if overrideIndex == -1 || appServerIndex == -1 || overrideIndex >= appServerIndex {
		t.Fatalf("managed auth override must precede app-server: %#v", got)
	}
}

func TestRefreshAuthStateFirstFingerprintNoWarning(t *testing.T) {
	s := &AppServerSource{
		authFingerprintFn: func() (string, error) {
			return "fp-a", nil
		},
	}

	warning, err := s.refreshAuthState()
	if err != nil {
		t.Fatalf("refreshAuthState: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if s.authFingerprint != "fp-a" {
		t.Fatalf("expected fingerprint to be stored")
	}
}

func TestRefreshAuthStateUnchangedNoWarning(t *testing.T) {
	s := &AppServerSource{
		authFingerprint: "fp-a",
		authFingerprintFn: func() (string, error) {
			return "fp-a", nil
		},
	}

	warning, err := s.refreshAuthState()
	if err != nil {
		t.Fatalf("refreshAuthState: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if s.authFingerprint != "fp-a" {
		t.Fatalf("expected fingerprint to remain unchanged")
	}
}

func TestRefreshAuthStateChangedReturnsWarning(t *testing.T) {
	s := &AppServerSource{
		authFingerprint: "fp-a",
		authFingerprintFn: func() (string, error) {
			return "fp-b", nil
		},
		session: &appServerSession{},
	}

	warning, err := s.refreshAuthState()
	if err != nil {
		t.Fatalf("refreshAuthState: %v", err)
	}
	if warning == "" {
		t.Fatalf("expected warning on fingerprint change")
	}
	if s.authFingerprint != "fp-b" {
		t.Fatalf("expected fingerprint to update")
	}
	if s.session != nil {
		t.Fatalf("expected session reset on fingerprint change")
	}
}

func TestRefreshAuthStateErrorAfterKnownFingerprintReturnsError(t *testing.T) {
	s := &AppServerSource{
		authFingerprint: "fp-a",
		authFingerprintFn: func() (string, error) {
			return "", errors.New("missing auth")
		},
		session: &appServerSession{},
	}

	warning, err := s.refreshAuthState()
	if err == nil {
		t.Fatal("expected auth-state error")
	}
	if warning != "" {
		t.Fatalf("expected no warning on auth-state error, got %q", warning)
	}
	if s.authFingerprint != "" {
		t.Fatalf("expected fingerprint to be cleared")
	}
	if s.session != nil {
		t.Fatalf("expected session reset on auth-state error")
	}
}

func TestRefreshAuthStateFirstErrorReturnsError(t *testing.T) {
	s := &AppServerSource{
		authFingerprintFn: func() (string, error) {
			return "", errors.New("unsafe auth")
		},
	}

	warning, err := s.refreshAuthState()
	if err == nil {
		t.Fatal("expected auth-state error")
	}
	if warning != "" {
		t.Fatalf("expected no warning before a known fingerprint, got %q", warning)
	}
}

func TestFetchBlocksAuthStateErrorBeforeStartingSession(t *testing.T) {
	s := &AppServerSource{
		authFingerprintFn: func() (string, error) {
			return "", errors.New("unsafe auth")
		},
	}

	_, err := s.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected auth-state error")
	}
	if !strings.Contains(err.Error(), "unsafe auth") {
		t.Fatalf("expected unsafe auth error, got %v", err)
	}
	if s.session != nil {
		t.Fatalf("expected app-server session not to start")
	}
}

func TestWithoutCodexProfileEnvRemovesStaleProfileState(t *testing.T) {
	env := withoutCodexProfileEnv([]string{
		"CODEX_HOME=/tmp/stale",
		"MULTICODEX_ACTIVE_PROFILE=stale",
		"MULTICODEX_SELECTED_PROFILE_PATH=/tmp/stale.json",
		"OPENAI_API_KEY=stale",
		"CODEX_AUTH_TOKEN=stale",
		"KEEP=value",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CODEX_HOME=") {
		t.Fatalf("expected CODEX_HOME to be removed, got %q", env)
	}
	if strings.Contains(joined, "MULTICODEX_ACTIVE_PROFILE=") {
		t.Fatalf("expected MULTICODEX_ACTIVE_PROFILE to be removed, got %q", env)
	}
	if strings.Contains(joined, "MULTICODEX_SELECTED_PROFILE_PATH=") {
		t.Fatalf("expected MULTICODEX_SELECTED_PROFILE_PATH to be removed, got %q", env)
	}
	if strings.Contains(joined, "OPENAI_API_KEY=") || strings.Contains(joined, "CODEX_AUTH_TOKEN=") {
		t.Fatalf("expected account override credentials to be removed, got %q", env)
	}
	if !strings.Contains(joined, "KEEP=value") {
		t.Fatalf("expected unrelated env to remain, got %q", env)
	}
}
