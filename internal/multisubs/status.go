package multisubs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/codexstate"
	"github.com/Enrico-DA/multi_subs/internal/processprobe"
)

type profileStatus struct {
	Name     string
	AuthFile bool
	State    string
	Account  string
	Detail   string
}

var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

var codexLoginStatusTimeout = 5 * time.Second
var codexLoginStatusCommandContext = exec.CommandContext
var profileCheckWorkerLimit = 6

func PrintStatus(store *Store, cfg *Config) error {
	if len(cfg.Profiles) == 0 {
		fmt.Println("no profiles configured")
		fmt.Println("add a profile with: multisubs codex add <name>")
		return nil
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := collectProfileRows(store, cfg, names)

	fmt.Println("multisubs codex status")
	fmt.Println("profile-local auth status")
	fmt.Println()
	fmt.Printf("%-16s %-10s %-10s %-30s %s\n", "profile", "auth.json", "state", "account", "detail")
	for _, row := range rows {
		auth := "no"
		if row.AuthFile {
			auth = "yes"
		}
		fmt.Printf("%-16s %-10s %-10s %-30s %s\n", row.Name, auth, row.State, truncate(row.Account, 30), truncate(row.Detail, 80))
	}
	return nil
}

func collectProfileRows(store *Store, cfg *Config, names []string) []profileStatus {
	rows := make([]profileStatus, len(names))
	workers := parallelWorkers(len(names))
	if workers == 1 {
		for i, name := range names {
			rows[i] = buildProfileRow(store, name, cfg.Profiles[name])
		}
		return rows
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, name := range names {
		i := i
		name := name
		profile := cfg.Profiles[name]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = buildProfileRow(store, name, profile)
		}()
	}
	wg.Wait()
	return rows
}

func buildProfileRow(store *Store, name string, profile Profile) profileStatus {
	row := profileStatus{Name: name}
	if err := store.ensureProfileStoragePathSafe(profile); err != nil {
		row.State = "error"
		row.Detail = err.Error()
		return row
	}
	if _, err := os.Stat(profile.CodexHome); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			row.State = "missing"
			row.Detail = "profile codex home not found"
			return row
		}
		row.State = "error"
		row.Detail = err.Error()
		return row
	}
	_, hasAuth, err := ensureProfileAuthPathSafe(profile.CodexHome)
	if err != nil {
		row.State = "error"
		row.Detail = err.Error()
		return row
	}
	row.AuthFile = hasAuth
	if err := ensureProfileCodexExecutionReady(store.paths, profile); err != nil {
		row.State = "error"
		row.Detail = err.Error()
		return row
	}
	state, account, detail := codexLoginStatus(profile.CodexHome)
	row.State = state
	row.Account = account
	row.Detail = detail
	return row
}

func parallelWorkers(total int) int {
	if total <= 1 {
		return 1
	}
	if profileCheckWorkerLimit <= 1 {
		return 1
	}
	if total < profileCheckWorkerLimit {
		return total
	}
	return profileCheckWorkerLimit
}

func codexLoginStatus(codexHome string) (state, account, detail string) {
	return codexLoginStatusWithTimeout(codexHome, codexLoginStatusTimeout, true)
}

func defaultCodexLoginState(codexHome string) (state, detail string) {
	state, _, detail = probeCodexLoginStatusWithTimeout(codexHome, codexLoginStatusTimeout, false)
	return state, detail
}

func codexLoginStatusWithTimeout(codexHome string, timeout time.Duration, managed bool) (state, account, detail string) {
	state, account, detail = probeCodexLoginStatusWithTimeout(codexHome, timeout, managed)
	if account != "" {
		return state, account, detail
	}

	email, err := emailFromAuthFile(filepath.Join(codexHome, "auth.json"))
	if err == nil && email != "" {
		account = email
	} else {
		account = "-"
	}
	return state, account, detail
}

func probeCodexLoginStatusWithTimeout(codexHome string, timeout time.Duration, managed bool) (state, account, detail string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"login", "status"}
	if managed {
		args = codexstate.WithManagedAuthOverride(args)
	}
	cmd := codexLoginStatusCommandContext(ctx, "codex", args...)
	var env []string
	if managed {
		env = profileCodexEnv(os.Environ(), codexHome, "")
	} else {
		env = sanitizedCodexEnv(os.Environ(), codexHome)
	}
	cmd.Env = env
	out, truncated, err := processprobe.CombinedOutput(cmd)
	if truncated {
		return "error", "", "codex login status output exceeded safe limit"
	}
	all := strings.TrimSpace(string(out))
	lower := strings.ToLower(all)

	account = emailRe.FindString(all)

	if err == nil {
		if loginStatusTextIndicatesLoggedOut(lower) {
			return "logged-out", account, firstLineOrDash(all)
		}
		if loginStatusTextIndicatesLoggedIn(lower) {
			return "logged-in", account, firstLineOrDash(all)
		}
		return "ok", account, firstLineOrDash(all)
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "error", account, fmt.Sprintf("codex login status timed out after %s", timeout)
	}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if loginStatusTextIndicatesLoggedOut(lower) {
			return "logged-out", account, "not logged in"
		}
		return "error", account, fmt.Sprintf("codex login status failed with exit code %d", ee.ExitCode())
	}

	return "error", account, "codex login status could not be started"
}

func loginStatusTextIndicatesLoggedOut(lower string) bool {
	for _, phrase := range []string{
		"not logged in",
		"not signed in",
		"logged out",
		"signed out",
		"not authenticated",
		"unauthenticated",
		"unauthorized",
		"no active account",
		"please log in",
		"please sign in",
	} {
		if loginStatusTextHasWholePhrase(lower, phrase) {
			return true
		}
	}
	return false
}

func loginStatusTextIndicatesLoggedIn(lower string) bool {
	for _, line := range strings.Split(lower, "\n") {
		line = strings.TrimRight(strings.TrimSpace(line), ".!,;")
		switch line {
		case "logged in", "signed in", "you are logged in", "you are signed in":
			return true
		}
		for _, prefix := range []string{
			"logged in using ",
			"logged in as ",
			"signed in as ",
			"authenticated as ",
			"you are logged in using ",
			"you are logged in as ",
			"you are signed in as ",
		} {
			if strings.HasPrefix(line, prefix) && strings.TrimSpace(strings.TrimPrefix(line, prefix)) != "" {
				return true
			}
		}
	}
	return false
}

func loginStatusTextHasWholePhrase(text, phrase string) bool {
	for searchFrom := 0; searchFrom <= len(text)-len(phrase); {
		relativeStart := strings.Index(text[searchFrom:], phrase)
		if relativeStart < 0 {
			return false
		}
		start := searchFrom + relativeStart
		end := start + len(phrase)
		beforePhrase := start == 0 || loginStatusPhraseBoundary(text[start-1])
		afterPhrase := end == len(text) || loginStatusPhraseBoundary(text[end])
		if beforePhrase && afterPhrase {
			return true
		}
		searchFrom = start + 1
	}
	return false
}

// loginStatusPhraseBoundary reports whether a byte ends a word, so a phrase is
// only matched as whole words. Sentence punctuation counts as a boundary, which
// keeps "not logged in." readable as a logged-out message.
func loginStatusPhraseBoundary(char byte) bool {
	return !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9')
}

func firstLineOrDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func emailFromAuthFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var payload struct {
		Email  string `json:"email"`
		Tokens struct {
			IDToken string `json:"id_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", err
	}
	if payload.Email != "" {
		return payload.Email, nil
	}
	if payload.Tokens.IDToken == "" {
		return "", nil
	}

	parts := strings.Split(payload.Tokens.IDToken, ".")
	if len(parts) < 2 {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return "", err
	}
	v, ok := claims["email"]
	if !ok {
		return "", nil
	}
	email, ok := v.(string)
	if !ok {
		return "", nil
	}
	return email, nil
}
