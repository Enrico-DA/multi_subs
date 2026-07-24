package multisubs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/codexstate"
)

type DoctorReport struct {
	Checks []DoctorCheck `json:"checks"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details"`
}

func (r DoctorReport) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

type AggregateDoctorReport struct {
	Base   DoctorReport `json:"base"`
	Codex  DoctorReport `json:"codex"`
	Claude DoctorReport `json:"claude"`
}

func (r AggregateDoctorReport) HasFailures() bool {
	return r.Base.HasFailures() || r.Codex.HasFailures() || r.Claude.HasFailures()
}

func RunBaseDoctor(store *Store, cfg *Config) DoctorReport {
	return runBaseDoctor(store, cfg, nil)
}

func runBaseDoctor(store *Store, cfg *Config, registryErr error) DoctorReport {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	checks := make([]DoctorCheck, 0, 12)
	checks = append(checks, checkDirExists("multisubs home", store.paths.MultisubsHome, true))
	if registryErr != nil {
		checks = append(checks, DoctorCheck{
			Name:    "Codex profile registry",
			Status:  "fail",
			Details: codexRegistryFailureDetails(registryErr),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "config",
			Status:  "ok",
			Details: fmt.Sprintf("loaded config with %d profile(s)", len(cfg.Profiles)),
		})
	}
	checks = append(checks, checkProfileResources(store, cfg.ProfileResources))
	checks = append(checks, checkRepositoryLeakGuards(store.paths)...)
	return DoctorReport{Checks: checks}
}

func codexRegistryFailureDetails(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "parse config"):
		return "Codex profile registry is invalid: malformed JSON"
	case strings.Contains(message, "unsupported config version"):
		return "Codex profile registry is invalid: unsupported version"
	case strings.Contains(message, "invalid stored codex profile name"):
		return "Codex profile registry is invalid: invalid stored profile name"
	case strings.Contains(message, "mismatched name"):
		return "Codex profile registry is invalid: stored profile name mismatch"
	default:
		return "Codex profile registry could not be loaded safely"
	}
}

func RunCodexDoctor(store *Store, cfg *Config, timeout time.Duration) DoctorReport {
	checks := make([]DoctorCheck, 0, 16)
	codexFound := false
	if path, err := exec.LookPath("codex"); err != nil {
		checks = append(checks, DoctorCheck{
			Name:    "codex binary",
			Status:  "fail",
			Details: "codex was not found in PATH",
		})
	} else {
		codexFound = true
		detail := path
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "codex", "--version")
		cmd.Env = neutralCodexEnv(os.Environ())
		out, err := cmd.CombinedOutput()
		if err != nil {
			detail = fmt.Sprintf("%s (codex --version failed: %v)", path, err)
			checks = append(checks, DoctorCheck{Name: "codex binary", Status: "warn", Details: detail})
		} else {
			version := strings.TrimSpace(string(out))
			if version == "" {
				version = "version output is empty"
			}
			checks = append(checks, DoctorCheck{Name: "codex binary", Status: "ok", Details: fmt.Sprintf("%s (%s)", path, version)})
		}
	}

	checks = append(checks, checkDirExists("default codex home", store.paths.DefaultCodexHome, false))
	checks = append(checks, checkDefaultFileStoreConfig("default codex config", filepath.Join(store.paths.DefaultCodexHome, "config.toml")))

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	profileChecks := collectProfileDoctorChecks(store.paths, cfg, names, codexFound, timeout)
	for i := range profileChecks {
		checks = append(checks, profileChecks[i]...)
	}

	return DoctorReport{Checks: checks}
}

func RunDoctor(store *Store, cfg *Config, timeout time.Duration) DoctorReport {
	base := RunBaseDoctor(store, cfg)
	codex := RunCodexDoctor(store, cfg, timeout)
	return DoctorReport{Checks: append(base.Checks, codex.Checks...)}
}

func checkProfileResources(store *Store, resources *ProfileResources) DoctorCheck {
	resolved, err := store.ResolveProfileResources(resources)
	if err != nil {
		return DoctorCheck{Name: "profile resources", Status: "fail", Details: err.Error()}
	}
	return DoctorCheck{Name: "profile resources", Status: "ok", Details: describeProfileResources(resources, resolved)}
}

func collectProfileDoctorChecks(paths Paths, cfg *Config, names []string, codexFound bool, timeout time.Duration) [][]DoctorCheck {
	result := make([][]DoctorCheck, len(names))
	workers := parallelWorkers(len(names))
	if workers == 1 {
		for i, name := range names {
			result[i] = profileDoctorChecks(paths, name, cfg.Profiles[name], codexFound, timeout)
		}
		return result
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
			result[i] = profileDoctorChecks(paths, name, profile, codexFound, timeout)
		}()
	}
	wg.Wait()
	return result
}

func profileDoctorChecks(paths Paths, name string, profile Profile, codexFound bool, timeout time.Duration) []DoctorCheck {
	prefix := "profile " + name
	out := make([]DoctorCheck, 0, 4)

	if err := ValidateCodexProfileName(name); err != nil {
		out = append(out, DoctorCheck{Name: prefix + " name", Status: "fail", Details: err.Error()})
	} else {
		out = append(out, DoctorCheck{Name: prefix + " name", Status: "ok", Details: "valid"})
	}

	homeCheck := checkProfileCodexHome(paths, prefix+" codex home", profile)
	out = append(out, homeCheck)
	if homeCheck.Status == "fail" {
		return out
	}
	configCheck := checkManagedFileStoreConfig(
		prefix+" config",
		filepath.Join(profile.CodexHome, "config.toml"),
		filepath.Join(paths.DefaultCodexHome, "config.toml"),
	)
	out = append(out, configCheck)
	authCheck := checkAuthFile(prefix+" auth", filepath.Join(profile.CodexHome, "auth.json"))
	out = append(out, authCheck)

	if codexFound && configCheck.Status != "fail" && authCheck.Status != "fail" {
		state, account, detail := codexLoginStatusWithTimeout(profile.CodexHome, timeout)
		status := "warn"
		if state == "logged-in" || state == "ok" {
			status = "ok"
		}
		out = append(out, DoctorCheck{
			Name:    prefix + " login status",
			Status:  status,
			Details: fmt.Sprintf("state=%s account=%s detail=%s", state, account, detail),
		})
	}
	return out
}

func checkProfileCodexHome(paths Paths, name string, profile Profile) DoctorCheck {
	if err := NewStore(paths).ensureProfileStoragePathSafe(profile); err != nil {
		return DoctorCheck{Name: name, Status: "fail", Details: err.Error()}
	}
	return checkDirExists(name, profile.CodexHome, true)
}

func checkRepositoryLeakGuards(paths Paths) []DoctorCheck {
	root, err := gitRootFromCWD()
	if err != nil {
		return []DoctorCheck{{
			Name:    "repo leak guard",
			Status:  "warn",
			Details: "git root not detected from current working directory. skipped tracked-file leak checks",
		}}
	}

	checks := make([]DoctorCheck, 0, 6)
	checks = append(checks, DoctorCheck{
		Name:    "repo leak guard git root",
		Status:  "ok",
		Details: root,
	})

	checks = append(checks, checkPathOutsideRepo("multisubs home path isolation", root, paths.MultisubsHome))
	checks = append(checks, checkPathOutsideRepo("default codex home path isolation", root, paths.DefaultCodexHome))
	checks = append(checks, checkRequiredIgnorePatterns(root))
	checks = append(checks, checkTrackedSensitiveFiles(root))
	return checks
}

func gitRootFromCWD() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func checkPathOutsideRepo(name, root, p string) DoctorCheck {
	if p == "" {
		return DoctorCheck{Name: name, Status: "warn", Details: "path is empty"}
	}
	if isSubpath(root, p) {
		return DoctorCheck{
			Name:    name,
			Status:  "fail",
			Details: fmt.Sprintf("path is inside git working tree: %s", p),
		}
	}
	return DoctorCheck{Name: name, Status: "ok", Details: p}
}

func isSubpath(root, p string) bool {
	absRoot, err := canonicalPath(root)
	if err != nil {
		return false
	}
	absPath, err := canonicalPath(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return false
	}
	return true
}

func canonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	current := abs
	var tail []string
	for {
		if _, err := os.Stat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}

	resolvedBase, err := filepath.EvalSymlinks(current)
	if err != nil {
		resolvedBase = current
	}
	if len(tail) == 0 {
		return resolvedBase, nil
	}
	parts := append([]string{resolvedBase}, tail...)
	return filepath.Join(parts...), nil
}

func checkRequiredIgnorePatterns(root string) DoctorCheck {
	content, err := collectGitignoreContent(root)
	if err != nil {
		return DoctorCheck{Name: "repo leak guard ignore patterns", Status: "warn", Details: err.Error()}
	}
	if strings.TrimSpace(content) == "" {
		return DoctorCheck{
			Name:    "repo leak guard ignore patterns",
			Status:  "warn",
			Details: "no .gitignore entries found from current directory to git root",
		}
	}
	missing := missingIgnorePatterns(content)
	if len(missing) > 0 {
		return DoctorCheck{
			Name:    "repo leak guard ignore patterns",
			Status:  "warn",
			Details: "missing recommended patterns: " + strings.Join(missing, ", "),
		}
	}
	return DoctorCheck{Name: "repo leak guard ignore patterns", Status: "ok", Details: "required patterns present"}
}

func collectGitignoreContent(root string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	var chunks []string
	current := cwd
	for {
		if !isSubpath(root, current) {
			break
		}
		p := filepath.Join(current, ".gitignore")
		if b, err := os.ReadFile(p); err == nil {
			chunks = append(chunks, string(b))
		}
		if filepath.Clean(current) == filepath.Clean(root) {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return strings.Join(chunks, "\n"), nil
}

func missingIgnorePatterns(content string) []string {
	required := []string{
		".codex/",
		".multisubs/",
		"**/multisubs/config.json",
		"**/multisubs/profiles/",
		"**/multisubs/providers/claude/",
		"**/.multisubs/config.json",
		"**/.multisubs/profiles/",
		// Legacy-sensitive state stays covered even though runtime never reads it.
		".multicodex/",
		"**/multicodex/config.json",
		"**/multicodex/profiles/",
		"**/multicodex/providers/claude/",
		"**/.multicodex/config.json",
		"**/.multicodex/profiles/",
		"**/auth.json",
		"**/.credentials.json",
		".env",
		".env.*",
	}
	missing := make([]string, 0, len(required))
	for _, pattern := range required {
		if !strings.Contains(content, pattern) {
			missing = append(missing, pattern)
		}
	}
	return missing
}

func checkTrackedSensitiveFiles(root string) DoctorCheck {
	cmd := exec.Command("git", "-C", root, "ls-files")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return DoctorCheck{Name: "repo leak guard tracked files", Status: "warn", Details: "could not enumerate tracked files"}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	found := make([]string, 0, 8)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isSensitiveTrackedPath(line) {
			found = append(found, line)
			if len(found) >= 8 {
				break
			}
		}
	}
	if len(found) > 0 {
		return DoctorCheck{
			Name:    "repo leak guard tracked files",
			Status:  "fail",
			Details: "tracked sensitive-looking files detected: " + strings.Join(found, ", "),
		}
	}
	return DoctorCheck{Name: "repo leak guard tracked files", Status: "ok", Details: "no sensitive-looking tracked files detected"}
}

func isSensitiveTrackedPath(p string) bool {
	clean := path.Clean(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")))
	base := path.Base(clean)
	if trackedPathMatchesAny(clean, []string{
		"multisubs/config.json",
		".multisubs/config.json",
		"multicodex/config.json",
		".multicodex/config.json",
		"github.com/olliecrow/multicodex/config.json",
		"github.com/olliecrow/.multicodex/config.json",
		"github.com/enrico-da/multi_subs/config.json",
		"github.com/enrico-da/.multisubs/config.json",
		"github.com/enrico-da/multicodex/config.json",
		"github.com/enrico-da/.multicodex/config.json",
	}) || strings.Contains(clean, "/multisubs/config.json") ||
		strings.Contains(clean, "/.multisubs/config.json") ||
		strings.Contains(clean, "/multicodex/config.json") ||
		strings.Contains(clean, "/.multicodex/config.json") {
		return true
	}
	if trackedPathHasAnyPrefix(clean, []string{
		"multisubs/profiles/",
		".multisubs/profiles/",
		"multicodex/profiles/",
		".multicodex/profiles/",
		"github.com/olliecrow/multicodex/profiles/",
		"github.com/olliecrow/.multicodex/profiles/",
		"github.com/enrico-da/multi_subs/profiles/",
		"github.com/enrico-da/.multisubs/profiles/",
		"github.com/enrico-da/multicodex/profiles/",
		"github.com/enrico-da/.multicodex/profiles/",
	}) || strings.Contains(clean, "/multisubs/profiles/") ||
		strings.Contains(clean, "/.multisubs/profiles/") ||
		strings.Contains(clean, "/multicodex/profiles/") ||
		strings.Contains(clean, "/.multicodex/profiles/") {
		return true
	}
	if strings.Contains(clean, "/multisubs/providers/claude/") ||
		strings.HasPrefix(clean, "multisubs/providers/claude/") ||
		strings.Contains(clean, "/.multisubs/providers/claude/") ||
		strings.HasPrefix(clean, ".multisubs/providers/claude/") ||
		strings.Contains(clean, "/multi_subs/providers/claude/") ||
		strings.Contains(clean, "/.multicodex/providers/claude/") ||
		strings.HasPrefix(clean, ".multicodex/providers/claude/") ||
		strings.Contains(clean, "/multicodex/providers/claude/") ||
		strings.HasPrefix(clean, "multicodex/providers/claude/") {
		return true
	}
	if strings.Contains(clean, "/.codex/") || strings.HasPrefix(clean, ".codex/") {
		return true
	}
	if base == "auth.json" || base == ".credentials.json" {
		return true
	}
	if base == ".env" {
		return true
	}
	if strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".example") && !strings.HasSuffix(base, ".sample") {
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".key") {
		return true
	}
	return false
}

func trackedPathMatchesAny(clean string, matches []string) bool {
	for _, match := range matches {
		if clean == match {
			return true
		}
	}
	return false
}

func trackedPathHasAnyPrefix(clean string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func checkDirExists(name, path string, strictPerms bool) DoctorCheck {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorCheck{Name: name, Status: "warn", Details: "not found"}
		}
		return DoctorCheck{Name: name, Status: "fail", Details: err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if strictPerms {
			return DoctorCheck{Name: name, Status: "fail", Details: "expected directory, got symlink"}
		}
		targetInfo, statErr := os.Stat(path)
		if statErr != nil {
			return DoctorCheck{Name: name, Status: "fail", Details: statErr.Error()}
		}
		info = targetInfo
	}
	if !info.IsDir() {
		return DoctorCheck{Name: name, Status: "fail", Details: "expected directory"}
	}
	if strictPerms && info.Mode().Perm()&0o077 != 0 {
		return DoctorCheck{Name: name, Status: "warn", Details: fmt.Sprintf("permissions are %o, recommend 700", info.Mode().Perm())}
	}
	return DoctorCheck{Name: name, Status: "ok", Details: path}
}

func checkDefaultFileStoreConfig(name, path string) DoctorCheck {
	linkTarget := ""
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return DoctorCheck{Name: name, Status: "fail", Details: fmt.Sprintf("config.toml symlink read failed: %v", readErr)}
		}
		linkTarget = target
	}

	if err := ensureRegularFileOrSymlinkTarget(path, "config.toml"); err != nil {
		if os.IsNotExist(err) {
			if linkTarget != "" {
				return DoctorCheck{Name: name, Status: "warn", Details: "config.toml symlink target not found: " + linkTarget}
			}
			return DoctorCheck{Name: name, Status: "warn", Details: "config.toml not found"}
		}
		return DoctorCheck{Name: name, Status: "fail", Details: err.Error()}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if linkTarget != "" {
				return DoctorCheck{Name: name, Status: "warn", Details: "config.toml symlink target not found: " + linkTarget}
			}
			return DoctorCheck{Name: name, Status: "warn", Details: "config.toml not found"}
		}
		return DoctorCheck{Name: name, Status: "fail", Details: err.Error()}
	}
	store, found, err := codexstate.CredentialStoreFromTOML(string(content))
	if err != nil {
		return DoctorCheck{Name: name, Status: "fail", Details: fmt.Sprintf("parse config.toml: %v", err)}
	}
	ok := found && store == "file"
	if !ok {
		return DoctorCheck{Name: name, Status: "warn", Details: "config present but file credential store is not configured"}
	}
	if linkTarget != "" {
		return DoctorCheck{Name: name, Status: "ok", Details: "file credential store configured via symlink -> " + linkTarget}
	}
	return DoctorCheck{Name: name, Status: "ok", Details: "file credential store configured"}
}

func checkManagedFileStoreConfig(name, path, defaultConfigPath string) DoctorCheck {
	details, err := codexstate.ValidateManagedConfigPath(path, defaultConfigPath)
	if err != nil {
		message := err.Error()
		if details.IsSymlink {
			if details.RawLinkTarget == "" {
				message += "; config.toml is a symlink whose raw target could not be read"
			} else {
				message += "; config.toml symlink -> " + details.RawLinkTarget
			}
		}
		return DoctorCheck{Name: name, Status: "fail", Details: message}
	}
	ok, err := profileConfigUsesFileStore(path, defaultConfigPath)
	if err != nil {
		message := err.Error()
		if details.IsSymlink {
			message += "; config.toml symlink -> " + details.RawLinkTarget
		}
		return DoctorCheck{Name: name, Status: "fail", Details: message}
	}
	if !ok {
		return DoctorCheck{Name: name, Status: "fail", Details: "config present but file credential store is not configured"}
	}
	if details.IsSymlink {
		return DoctorCheck{Name: name, Status: "ok", Details: "file credential store configured via symlink -> " + details.RawLinkTarget}
	}
	return DoctorCheck{Name: name, Status: "ok", Details: "file credential store configured via single-link manual override"}
}

func checkAuthFile(name, path string) DoctorCheck {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorCheck{Name: name, Status: "warn", Details: "auth.json not found. run multisubs codex login <name>"}
		}
		return DoctorCheck{Name: name, Status: "fail", Details: err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return DoctorCheck{Name: name, Status: "fail", Details: "auth.json is a symlink; expected profile-local file"}
	}
	if info.IsDir() {
		return DoctorCheck{Name: name, Status: "fail", Details: "auth.json is a directory"}
	}
	if !info.Mode().IsRegular() {
		return DoctorCheck{Name: name, Status: "fail", Details: "auth.json is not a regular file"}
	}
	if fileHasMultipleLinks(info) {
		return DoctorCheck{Name: name, Status: "fail", Details: "auth.json has multiple hard links; expected profile-local file"}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{Name: name, Status: "fail", Details: fmt.Sprintf("auth.json read failed: %v", err)}
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		return DoctorCheck{Name: name, Status: "fail", Details: fmt.Sprintf("auth.json is invalid JSON: %v", err)}
	}

	tokensRaw, ok := parsed["tokens"]
	hasAPIKey := false
	if _, ok := parsed["OPENAI_API_KEY"]; ok {
		hasAPIKey = true
	}

	warnings := make([]string, 0, 4)
	if info.Mode().Perm()&0o077 != 0 {
		warnings = append(warnings, fmt.Sprintf("permissions are %o, recommend 600", info.Mode().Perm()))
	}

	if !ok {
		if hasAPIKey {
			warnings = append(warnings, "api-key auth detected. multisubs is designed for subscription account auth")
		} else {
			warnings = append(warnings, "auth.json parsed but token object is missing")
		}
	} else {
		tokens, ok := tokensRaw.(map[string]any)
		if !ok {
			return DoctorCheck{Name: name, Status: "fail", Details: "auth.json token object has unexpected shape"}
		}
		if _, ok := tokens["access_token"]; !ok {
			warnings = append(warnings, "auth.json token object is missing access_token")
		}
		if _, ok := tokens["refresh_token"]; !ok {
			warnings = append(warnings, "auth.json token object is missing refresh_token")
		}
		if _, ok := tokens["id_token"]; !ok {
			warnings = append(warnings, "auth.json token object is missing id_token")
		}
	}

	if len(warnings) > 0 {
		return DoctorCheck{Name: name, Status: "warn", Details: strings.Join(warnings, "; ")}
	}
	return DoctorCheck{Name: name, Status: "ok", Details: "auth.json present and structured as token auth"}
}

func printDoctorHuman(title string, report DoctorReport) {
	fmt.Println(title)
	fmt.Println()
	fails := 0
	warns := 0
	for _, c := range report.Checks {
		label := "ok"
		switch c.Status {
		case "fail":
			label = "fail"
			fails++
		case "warn":
			label = "warn"
			warns++
		}
		fmt.Printf("[%s] %s: %s\n", label, c.Name, c.Details)
	}
	fmt.Println()
	if fails > 0 {
		fmt.Printf("doctor result: FAIL (%d fail, %d warn)\n", fails, warns)
		return
	}
	if warns > 0 {
		fmt.Printf("doctor result: PASS (%d warn)\n", warns)
		return
	}
	fmt.Println("doctor result: PASS")
}

func printDoctorSection(name string, report DoctorReport) (int, int) {
	fmt.Printf("== %s ==\n", name)
	failures := 0
	warnings := 0
	for _, item := range report.Checks {
		if item.Status == "fail" {
			failures++
		}
		if item.Status == "warn" {
			warnings++
		}
		fmt.Printf("[%s] %s: %s\n", item.Status, item.Name, item.Details)
	}
	fmt.Println()
	return failures, warnings
}

func printAggregateDoctorHuman(report AggregateDoctorReport) {
	fmt.Println("multisubs doctor")
	fmt.Println()
	baseFailures, baseWarnings := printDoctorSection("shared/base", report.Base)
	codexFailures, codexWarnings := printDoctorSection("Codex", report.Codex)
	claudeFailures, claudeWarnings := printDoctorSection("Claude", report.Claude)
	failures := baseFailures + codexFailures + claudeFailures
	warnings := baseWarnings + codexWarnings + claudeWarnings
	if failures > 0 {
		fmt.Printf("doctor result: FAIL (%d fail, %d warn)\n", failures, warnings)
		return
	}
	if warnings > 0 {
		fmt.Printf("doctor result: PASS (%d warn)\n", warnings)
		return
	}
	fmt.Println("doctor result: PASS")
}
