package multisubs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProfileResourcesJSONContract(t *testing.T) {
	t.Parallel()

	valid := []string{
		`{"guidance":{"inherit":true}}`,
		`{"guidance":{"inherit":false}}`,
		`{"skills":{"inherit":true}}`,
		`{"skills":{"inherit":true,"sources":["one"]}}`,
		`{"skills":{"inherit":false}}`,
		`{"skills":{"inherit":false,"sources":[]}}`,
	}
	for _, raw := range valid {
		var resources ProfileResources
		if err := json.Unmarshal([]byte(raw), &resources); err != nil {
			t.Errorf("valid settings %s: %v", raw, err)
		}
	}

	invalid := []string{
		`{"unknown":true}`,
		`{"guidance":{}}`,
		`{"guidance":{"inhert":true}}`,
		`{"guidance":{"inherit":false,"source":"custom"}}`,
		`{"guidance":{"inherit":true,"unknown":true}}`,
		`{"skills":{}}`,
		`{"skills":{"inhert":true}}`,
		`{"skills":{"inherit":true,"sources":[]}}`,
		`{"skills":{"inherit":false,"sources":["custom"]}}`,
		`{"skills":{"inherit":true,"unknown":true}}`,
	}
	for _, raw := range invalid {
		var resources ProfileResources
		if err := json.Unmarshal([]byte(raw), &resources); err == nil {
			t.Errorf("expected settings to fail: %s", raw)
		}
	}
}

func TestStoreProfileResourcesRoundTripAndOmission(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{
		MultisubsHome:    filepath.Join(root, "multi"),
		ConfigPath:       filepath.Join(root, "multi", "config.json"),
		ProfilesDir:      filepath.Join(root, "multi", "profiles"),
		DefaultCodexHome: filepath.Join(root, "codex"),
	}
	store := NewStore(paths)
	if err := store.Save(DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	plain, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "profile_resources") {
		t.Fatalf("omitted settings changed serialized config: %s", plain)
	}

	inherit := true
	sources := []string{"relative/skills", "~/shared-skills"}
	cfg := DefaultConfig()
	cfg.ProfileResources = &ProfileResources{
		Guidance: &GuidanceResources{Inherit: &inherit, Source: "relative/guidance"},
		Skills:   &SkillResources{Inherit: &inherit, Sources: &sources},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.ProfileResources, loaded.ProfileResources) {
		t.Fatalf("resource settings changed on round trip:\nwant %#v\ngot  %#v", cfg.ProfileResources, loaded.ProfileResources)
	}
}

func TestResolveProfileResourcesPathsAndValidation(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	configDir := filepath.Join(root, "state")
	defaultHome := filepath.Join(root, "default")
	for _, path := range []string{
		filepath.Join(home, "guidance"),
		filepath.Join(configDir, "relative-skills"),
		filepath.Join(root, "absolute-skills"),
		defaultHome,
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(Paths{ConfigPath: filepath.Join(configDir, "config.json"), DefaultCodexHome: defaultHome})
	inherit := true
	sources := []string{"relative-skills", filepath.Join(root, "absolute-skills")}
	resolved, err := store.ResolveProfileResources(&ProfileResources{
		Guidance: &GuidanceResources{Inherit: &inherit, Source: "~/guidance"},
		Skills:   &SkillResources{Inherit: &inherit, Sources: &sources},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.guidance.source != filepath.Join(home, "guidance") {
		t.Fatalf("unexpected home-relative guidance path: %s", resolved.guidance.source)
	}
	wantSources := []string{
		mustResolveExistingPath(t, filepath.Join(configDir, "relative-skills")),
		mustResolveExistingPath(t, filepath.Join(root, "absolute-skills")),
	}
	if !reflect.DeepEqual(resolved.skills.sources, wantSources) {
		t.Fatalf("unexpected skill paths: want %v, got %v", wantSources, resolved.skills.sources)
	}

	cases := []struct {
		name      string
		resources *ProfileResources
		want      string
	}{
		{"blank", &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: stringSlicePointer([]string{" "})}}, "blank"},
		{"unsupported tilde", &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: stringSlicePointer([]string{"~someone/skills"})}}, "unsupported home path"},
		{"missing", &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: stringSlicePointer([]string{"missing"})}}, "resolve profile_resources.skills.sources[0]: lstat"},
		{"duplicate", &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: stringSlicePointer([]string{"relative-skills", "./relative-skills"})}}, "duplicates"},
	}
	notDirectory := filepath.Join(configDir, "file")
	if err := os.WriteFile(notDirectory, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases = append(cases, struct {
		name      string
		resources *ProfileResources
		want      string
	}{"not directory", &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: stringSlicePointer([]string{"file"})}}, "not a directory"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.ResolveProfileResources(tc.resources)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestExplicitSkillSourcesRejectProtectedStateBoundaries(t *testing.T) {
	store, _ := newResourceTestStore(t)
	inherit := true
	otherProfileHome := filepath.Join(store.paths.ProfilesDir, "other", "codex-home")
	if err := os.MkdirAll(filepath.Join(otherProfileHome, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "another profile home", source: otherProfileHome, want: "overlaps multisubs-owned state"},
		{name: "ancestor containing protected state", source: filepath.Dir(store.paths.MultisubsHome), want: "overlaps multisubs-owned state"},
		{name: "default Codex home", source: store.paths.DefaultCodexHome, want: "overlaps default Codex home"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sources := []string{test.source}
			_, err := store.ResolveProfileResources(&ProfileResources{
				Skills: &SkillResources{Inherit: &inherit, Sources: &sources},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected overlap error containing %q, got %v", test.want, err)
			}
		})
	}

	externalSource := t.TempDir()
	if err := os.Symlink(filepath.Join(otherProfileHome, "shared"), filepath.Join(externalSource, "linked-profile-skill")); err != nil {
		t.Fatal(err)
	}
	sources := []string{externalSource}
	_, err := store.ResolveProfileResources(&ProfileResources{
		Skills: &SkillResources{Inherit: &inherit, Sources: &sources},
	})
	if err == nil || !strings.Contains(err.Error(), "overlaps multisubs-owned state") {
		t.Fatalf("expected protected symlink target error, got %v", err)
	}
}

func TestExplicitSkillSourcesRequireSkillDirectories(t *testing.T) {
	store, _ := newResourceTestStore(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "auth.json"), []byte(`{"token":"synthetic"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{source}
	_, err := store.ResolveProfileResources(&ProfileResources{
		Skills: &SkillResources{Inherit: &inherit, Sources: &sources},
	})
	if err == nil || !strings.Contains(err.Error(), "skill source entry is not a directory") {
		t.Fatalf("expected regular-file skill entry error, got %v", err)
	}
}

func TestExplicitSkillSourcesAllowCanonicalDefaultAndExternalDirectories(t *testing.T) {
	store, _ := newResourceTestStore(t)
	defaultSkills := filepath.Join(store.paths.DefaultCodexHome, "skills")
	externalSkills := t.TempDir()
	linkedSkillTarget := t.TempDir()
	for _, path := range []string{
		filepath.Join(defaultSkills, "default-skill"),
		filepath.Join(externalSkills, "external-skill"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(linkedSkillTarget, filepath.Join(externalSkills, "linked-skill")); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{defaultSkills, externalSkills}
	resolved, err := store.ResolveProfileResources(&ProfileResources{
		Skills: &SkillResources{Inherit: &inherit, Sources: &sources},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"default-skill":  mustResolveExistingPath(t, filepath.Join(defaultSkills, "default-skill")),
		"external-skill": mustResolveExistingPath(t, filepath.Join(externalSkills, "external-skill")),
		"linked-skill":   mustResolveExistingPath(t, linkedSkillTarget),
	} {
		if got := resolved.skills.desired[name]; got != want {
			t.Fatalf("skill %s target: got %q want %q", name, got, want)
		}
	}
}

func TestExplicitGuidanceReconciliation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	sourceOne := filepath.Join(t.TempDir(), "guidance-one")
	sourceTwo := filepath.Join(t.TempDir(), "guidance-two")
	for _, source := range []string{sourceOne, sourceTwo} {
		if err := os.MkdirAll(source, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range guidanceNames {
			if err := os.WriteFile(filepath.Join(source, name), []byte(source+name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	inherit := true
	policy := &ProfileResources{Guidance: &GuidanceResources{Inherit: &inherit, Source: sourceOne}}
	changes, err := store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected two guidance links, got %#v", changes)
	}
	for _, name := range guidanceNames {
		assertLinkTarget(t, filepath.Join(profile.CodexHome, name), filepath.Join(sourceOne, name))
	}

	policy.Guidance.Source = sourceTwo
	changes, err = store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].OldTarget == "" || changes[1].OldTarget == "" {
		t.Fatalf("expected retarget reports with old targets, got %#v", changes)
	}

	localPath := filepath.Join(profile.CodexHome, "AGENTS.md")
	if err := os.Remove(localPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	changes, err = store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if string(mustReadFile(t, localPath)) != "local" {
		t.Fatal("regular local guidance override changed")
	}
	if _, err := os.Lstat(filepath.Join(profile.CodexHome, "AGENTS.override.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected other inherited guidance link removed, got %v", err)
	}
	if len(changes) != 1 || changes[0].OldTarget == "" {
		t.Fatalf("expected removed link with old target, got %#v", changes)
	}
}

func TestGuidanceDefaultSourceAllowsOneMissingFile(t *testing.T) {
	store, profile := newResourceTestStore(t)
	source := store.paths.DefaultCodexHome
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("default guidance"), 0o600); err != nil {
		t.Fatal(err)
	}
	inherit := true
	changes, err := store.EnsureProfileDir(profile, &ProfileResources{Guidance: &GuidanceResources{Inherit: &inherit}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected one available guidance file to be linked, got %#v", changes)
	}
	assertLinkTarget(t, filepath.Join(profile.CodexHome, "AGENTS.md"), filepath.Join(source, "AGENTS.md"))
	if _, err := os.Lstat(filepath.Join(profile.CodexHome, "AGENTS.override.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source guidance file should stay absent: %v", err)
	}
}

func TestExplicitGuidanceIsolationOwnsSymlinksAndPreservesFiles(t *testing.T) {
	store, profile := newResourceTestStore(t)
	if err := os.MkdirAll(profile.CodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignTarget := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreignTarget, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(profile.CodexHome, "AGENTS.md")
	if err := os.Symlink(foreignTarget, link); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(profile.CodexHome, "AGENTS.override.md")
	if err := os.WriteFile(regular, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	inherit := false
	changes, err := store.EnsureProfileDir(profile, &ProfileResources{Guidance: &GuidanceResources{Inherit: &inherit}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].OldTarget != foreignTarget {
		t.Fatalf("expected owned foreign link removal, got %#v", changes)
	}
	if string(mustReadFile(t, regular)) != "local" {
		t.Fatal("regular guidance override changed")
	}
}

func TestExplicitSkillSourcesOrderingOverridesAndIsolation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(dir, "shared"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(first, "first-only"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(second, "second-only"), 0o700); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{first, second}
	policy := &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}
	changes, err := store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("expected three skill links, got %#v", changes)
	}
	assertLinkTarget(t, filepath.Join(profile.CodexHome, "skills", "shared"), filepath.Join(first, "shared"))
	assertLinkTarget(t, filepath.Join(profile.CodexHome, "skills", "second-only"), filepath.Join(second, "second-only"))

	sources = []string{second, first}
	changes, err = store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "retargeted" || changes[0].OldTarget == "" {
		t.Fatalf("expected shared skill retarget report, got %#v", changes)
	}
	assertLinkTarget(t, filepath.Join(profile.CodexHome, "skills", "shared"), filepath.Join(second, "shared"))

	local := filepath.Join(profile.CodexHome, "skills", "first-only")
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(local, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignTarget := filepath.Join(t.TempDir(), "foreign")
	if err := os.Mkdir(foreignTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignLink := filepath.Join(profile.CodexHome, "skills", "foreign")
	if err := os.Symlink(foreignTarget, foreignLink); err != nil {
		t.Fatal(err)
	}
	inherit = false
	changes, err = store.EnsureProfileDir(profile, &ProfileResources{Skills: &SkillResources{Inherit: &inherit}})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(local); err != nil || !info.IsDir() {
		t.Fatalf("regular local skill override changed: %v", err)
	}
	for _, name := range []string{"shared", "second-only", "foreign"} {
		if _, err := os.Lstat(filepath.Join(profile.CodexHome, "skills", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected inherited symlink %s removed, got %v", name, err)
		}
	}
	if len(changes) != 3 {
		t.Fatalf("expected three removed symlinks, got %#v", changes)
	}
}

func TestExplicitSkillReconciliationKeepsSystemSkillsProfileLocal(t *testing.T) {
	for _, inherit := range []bool{false, true} {
		for _, entryKind := range []string{"local directory", "inherited link", "external link", "broken link"} {
			t.Run(fmt.Sprintf("inherit=%t/%s", inherit, entryKind), func(t *testing.T) {
				store, profile := newResourceTestStore(t)
				defaultSystemPath := filepath.Join(store.paths.DefaultCodexHome, "skills", ".system")
				if err := os.MkdirAll(defaultSystemPath, 0o700); err != nil {
					t.Fatal(err)
				}
				skillsPath := filepath.Join(profile.CodexHome, "skills")
				if err := os.MkdirAll(skillsPath, 0o700); err != nil {
					t.Fatal(err)
				}
				systemPath := filepath.Join(skillsPath, ".system")
				rawTarget := ""
				switch entryKind {
				case "local directory":
					if err := os.Mkdir(systemPath, 0o700); err != nil {
						t.Fatal(err)
					}
				case "inherited link":
					rawTarget = defaultSystemPath
				case "external link":
					rawTarget = filepath.Join(t.TempDir(), ".system")
					if err := os.Mkdir(rawTarget, 0o700); err != nil {
						t.Fatal(err)
					}
				case "broken link":
					rawTarget = filepath.Join(t.TempDir(), "missing-system")
				default:
					t.Fatalf("unknown entry kind %q", entryKind)
				}
				if rawTarget != "" {
					if err := os.Symlink(rawTarget, systemPath); err != nil {
						t.Fatal(err)
					}
				}

				changes, err := store.EnsureProfileDir(profile, &ProfileResources{
					Skills: &SkillResources{Inherit: &inherit},
				})
				switch entryKind {
				case "local directory":
					if err != nil {
						t.Fatal(err)
					}
					if len(changes) != 0 {
						t.Fatalf("local .system directory produced changes: %#v", changes)
					}
					info, statErr := os.Lstat(systemPath)
					if statErr != nil || !info.IsDir() {
						t.Fatalf("local .system directory changed: info=%v err=%v", info, statErr)
					}
				case "inherited link":
					if err != nil {
						t.Fatal(err)
					}
					if len(changes) != 1 || changes[0].Action != "removed" || changes[0].OldTarget != rawTarget {
						t.Fatalf("unexpected inherited .system change: %#v", changes)
					}
					if _, statErr := os.Lstat(systemPath); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("inherited .system link was not removed: %v", statErr)
					}
				default:
					if err == nil {
						t.Fatal("expected unsafe .system link to fail")
					}
					gotTarget, readErr := os.Readlink(systemPath)
					if readErr != nil {
						t.Fatalf("unsafe .system link was changed: %v", readErr)
					}
					if gotTarget != rawTarget {
						t.Fatalf("unsafe .system target changed: got=%q want=%q", gotTarget, rawTarget)
					}
				}
			})
		}
	}
}

func TestExplicitDefaultSkillSourceAndForeignBrokenLinks(t *testing.T) {
	store, profile := newResourceTestStore(t)
	defaultSkills := filepath.Join(store.paths.DefaultCodexHome, "skills")
	if err := os.MkdirAll(filepath.Join(defaultSkills, "default-skill"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profile.CodexHome, "skills"), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(profile.CodexHome, "skills", "default-skill")
	if err := os.Symlink("../../../../outside/missing", foreign); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(profile.CodexHome, "skills", "stale")
	if err := os.Symlink("../../../../outside/also-missing", stale); err != nil {
		t.Fatal(err)
	}
	inherit := true
	changes, err := store.EnsureProfileDir(profile, &ProfileResources{Skills: &SkillResources{Inherit: &inherit}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected retarget and stale removal, got %#v", changes)
	}
	assertLinkTarget(t, foreign, filepath.Join(defaultSkills, "default-skill"))
	if _, err := os.Lstat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected broken stale link removed: %v", err)
	}
	if changes[0].OldTarget == "" || changes[1].OldTarget == "" {
		t.Fatalf("expected traversal-shaped old targets to be reported: %#v", changes)
	}
}

func TestSymlinkedSkillSourceDirectoryIsSupported(t *testing.T) {
	store, profile := newResourceTestStore(t)
	realSource := filepath.Join(t.TempDir(), "real-skills")
	if err := os.MkdirAll(filepath.Join(realSource, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	linkedSource := filepath.Join(t.TempDir(), "linked-skills")
	if err := os.Symlink(realSource, linkedSource); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{linkedSource}
	if _, err := store.EnsureProfileDir(profile, &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}); err != nil {
		t.Fatal(err)
	}
	assertExactLinkTarget(t, filepath.Join(profile.CodexHome, "skills", "shared"), mustResolveExistingPath(t, filepath.Join(realSource, "shared")))
}

func TestExplicitSkillLinksPinCanonicalSourceAndEntryTargets(t *testing.T) {
	store, profile := newResourceTestStore(t)
	realSource := filepath.Join(t.TempDir(), "real-skills")
	realEntry := filepath.Join(realSource, "source-skill")
	linkedEntryTarget := filepath.Join(t.TempDir(), "linked-entry-target")
	for _, path := range []string{realEntry, linkedEntryTarget} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(linkedEntryTarget, filepath.Join(realSource, "entry-skill")); err != nil {
		t.Fatal(err)
	}
	sourceAlias := filepath.Join(t.TempDir(), "skills-alias")
	if err := os.Symlink(realSource, sourceAlias); err != nil {
		t.Fatal(err)
	}
	profileSkills := filepath.Join(profile.CodexHome, "skills")
	if err := os.MkdirAll(profileSkills, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sourceAlias, "source-skill"), filepath.Join(profileSkills, "source-skill")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(realSource, "entry-skill"), filepath.Join(profileSkills, "entry-skill")); err != nil {
		t.Fatal(err)
	}

	inherit := true
	sources := []string{sourceAlias}
	policy := &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}
	resolved, err := store.ResolveProfileResources(policy)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRealSource := mustResolveExistingPath(t, realSource)
	if got := resolved.skills.sources; !reflect.DeepEqual(got, []string{canonicalRealSource}) {
		t.Fatalf("canonical sources: got %#v want %#v", got, []string{canonicalRealSource})
	}
	canonicalRealEntry := mustResolveExistingPath(t, realEntry)
	canonicalLinkedEntryTarget := mustResolveExistingPath(t, linkedEntryTarget)
	for name, want := range map[string]string{
		"source-skill": canonicalRealEntry,
		"entry-skill":  canonicalLinkedEntryTarget,
	} {
		if got := resolved.skills.desired[name]; got != want {
			t.Fatalf("canonical desired target for %s: got %q want %q", name, got, want)
		}
	}

	changes, err := store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Action != "retargeted" || changes[1].Action != "retargeted" {
		t.Fatalf("expected raw alias links to be retargeted once, got %#v", changes)
	}
	assertExactLinkTarget(t, filepath.Join(profileSkills, "source-skill"), canonicalRealEntry)
	assertExactLinkTarget(t, filepath.Join(profileSkills, "entry-skill"), canonicalLinkedEntryTarget)
}

func TestExplicitSkillSourceAliasRetargetRequiresReconciliation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	firstSource := filepath.Join(t.TempDir(), "first-source")
	secondSource := filepath.Join(t.TempDir(), "second-source")
	for _, source := range []string{firstSource, secondSource} {
		if err := os.MkdirAll(filepath.Join(source, "shared"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sourceAlias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(firstSource, sourceAlias); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{sourceAlias}
	policy := &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}
	profileLink := filepath.Join(profile.CodexHome, "skills", "shared")

	if _, err := store.EnsureProfileDir(profile, policy); err != nil {
		t.Fatal(err)
	}
	firstPinnedTarget := mustResolveExistingPath(t, filepath.Join(firstSource, "shared"))
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)

	if err := os.Remove(sourceAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondSource, sourceAlias); err != nil {
		t.Fatal(err)
	}
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)

	changes, err := store.EnsureProfileDir(profile, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "retargeted" {
		t.Fatalf("expected one source-alias retarget, got %#v", changes)
	}
	secondPinnedTarget := mustResolveExistingPath(t, filepath.Join(secondSource, "shared"))
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)

	protectedSource := filepath.Join(store.paths.ProfilesDir, "other", "codex-home", "skills")
	if err := os.MkdirAll(filepath.Join(protectedSource, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourceAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(protectedSource, sourceAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureProfileDir(profile, policy); err == nil || !strings.Contains(err.Error(), "overlaps multisubs-owned state") {
		t.Fatalf("expected protected source target to fail, got %v", err)
	}
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)
}

func TestExplicitSkillEntryAliasRetargetRequiresReconciliation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	source := filepath.Join(t.TempDir(), "source")
	firstTarget := filepath.Join(t.TempDir(), "first-entry")
	secondTarget := filepath.Join(t.TempDir(), "second-entry")
	for _, path := range []string{source, firstTarget, secondTarget} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	entryAlias := filepath.Join(source, "shared")
	if err := os.Symlink(firstTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	inherit := true
	sources := []string{source}
	policy := &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}}
	profileLink := filepath.Join(profile.CodexHome, "skills", "shared")

	if _, err := store.EnsureProfileDir(profile, policy); err != nil {
		t.Fatal(err)
	}
	firstPinnedTarget := mustResolveExistingPath(t, firstTarget)
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)

	if err := os.Remove(entryAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)
	if _, err := store.EnsureProfileDir(profile, policy); err != nil {
		t.Fatal(err)
	}
	secondPinnedTarget := mustResolveExistingPath(t, secondTarget)
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)

	protectedTarget := filepath.Join(store.paths.ProfilesDir, "other", "codex-home", "skills", "shared")
	if err := os.MkdirAll(protectedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entryAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(protectedTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureProfileDir(profile, policy); err == nil || !strings.Contains(err.Error(), "overlaps multisubs-owned state") {
		t.Fatalf("expected protected entry target to fail, got %v", err)
	}
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)
}

func TestDefaultSkillSourceInheritancePinsCanonicalTargets(t *testing.T) {
	policies := []struct {
		name   string
		policy func() *ProfileResources
	}{
		{name: "legacy omitted policy", policy: func() *ProfileResources { return nil }},
		{
			name: "guidance-only legacy path",
			policy: func() *ProfileResources {
				inherit := false
				return &ProfileResources{Guidance: &GuidanceResources{Inherit: &inherit}}
			},
		},
		{
			name: "explicit omitted sources",
			policy: func() *ProfileResources {
				inherit := true
				return &ProfileResources{Skills: &SkillResources{Inherit: &inherit}}
			},
		},
	}

	for _, test := range policies {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store, profile := newResourceTestStore(t)
			firstSource := filepath.Join(t.TempDir(), "first-default-skills")
			secondSource := filepath.Join(t.TempDir(), "second-default-skills")
			for _, source := range []string{firstSource, secondSource} {
				if err := os.MkdirAll(filepath.Join(source, "shared"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			defaultSourceAlias := filepath.Join(store.paths.DefaultCodexHome, "skills")
			if err := os.Symlink(firstSource, defaultSourceAlias); err != nil {
				t.Fatal(err)
			}
			profileLink := filepath.Join(profile.CodexHome, "skills", "shared")
			policy := test.policy()

			if _, err := store.EnsureProfileDir(profile, policy); err != nil {
				t.Fatal(err)
			}
			firstPinnedTarget := mustResolveExistingPath(t, filepath.Join(firstSource, "shared"))
			assertExactLinkTarget(t, profileLink, firstPinnedTarget)

			if err := os.Remove(defaultSourceAlias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(secondSource, defaultSourceAlias); err != nil {
				t.Fatal(err)
			}
			assertExactLinkTarget(t, profileLink, firstPinnedTarget)
			if _, err := store.EnsureProfileDir(profile, policy); err != nil {
				t.Fatal(err)
			}
			secondPinnedTarget := mustResolveExistingPath(t, filepath.Join(secondSource, "shared"))
			assertExactLinkTarget(t, profileLink, secondPinnedTarget)

			protectedSource := filepath.Join(store.paths.ProfilesDir, "other", "codex-home", "skills")
			if err := os.MkdirAll(filepath.Join(protectedSource, "shared"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(defaultSourceAlias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(protectedSource, defaultSourceAlias); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EnsureProfileDir(profile, policy); err == nil || !strings.Contains(err.Error(), "overlaps multisubs-owned state") {
				t.Fatalf("expected protected default source target to fail, got %v", err)
			}
			assertExactLinkTarget(t, profileLink, secondPinnedTarget)
		})
	}
}

func TestDefaultSkillEntryInheritancePinsCanonicalTarget(t *testing.T) {
	store, profile := newResourceTestStore(t)
	defaultSkills := filepath.Join(store.paths.DefaultCodexHome, "skills")
	firstTarget := filepath.Join(t.TempDir(), "first-default-entry")
	secondTarget := filepath.Join(t.TempDir(), "second-default-entry")
	for _, path := range []string{defaultSkills, firstTarget, secondTarget} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	entryAlias := filepath.Join(defaultSkills, "shared")
	if err := os.Symlink(firstTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	profileLink := filepath.Join(profile.CodexHome, "skills", "shared")

	if _, err := store.EnsureProfileDir(profile, nil); err != nil {
		t.Fatal(err)
	}
	firstPinnedTarget := mustResolveExistingPath(t, firstTarget)
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)

	if err := os.Remove(entryAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	assertExactLinkTarget(t, profileLink, firstPinnedTarget)
	if _, err := store.EnsureProfileDir(profile, nil); err != nil {
		t.Fatal(err)
	}
	secondPinnedTarget := mustResolveExistingPath(t, secondTarget)
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)

	protectedTarget := filepath.Join(store.paths.ProfilesDir, "other", "codex-home", "skills", "shared")
	if err := os.MkdirAll(protectedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entryAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(protectedTarget, entryAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureProfileDir(profile, nil); err == nil || !strings.Contains(err.Error(), "overlaps multisubs-owned state") {
		t.Fatalf("expected protected default entry target to fail, got %v", err)
	}
	assertExactLinkTarget(t, profileLink, secondPinnedTarget)
}

func TestResourceChangeOutputIncludesOldTarget(t *testing.T) {
	var output bytes.Buffer
	printResourceChangesTo(&output, []ResourceChange{{Action: "retargeted", Path: "/profile/skills/x", OldTarget: "/old/x", NewTarget: "/new/x"}})
	for _, want := range []string{"retargeted", "/profile/skills/x", "old target: /old/x", "new target: /new/x"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in output %q", want, output.String())
		}
	}
}

func TestResourceValidationPrecedesProfileMutation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	inherit := true
	sources := []string{filepath.Join(t.TempDir(), "missing")}
	_, err := store.EnsureProfileDir(profile, &ProfileResources{Skills: &SkillResources{Inherit: &inherit, Sources: &sources}})
	if err == nil {
		t.Fatal("expected missing source error")
	}
	if _, statErr := os.Lstat(profile.CodexHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid source mutated profile home: %v", statErr)
	}
}

func TestResourceDestinationValidationPrecedesProfileMutation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	if err := os.MkdirAll(profile.CodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	oldTarget := filepath.Join(t.TempDir(), "old-guidance")
	if err := os.WriteFile(oldTarget, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	guidancePath := filepath.Join(profile.CodexHome, "AGENTS.md")
	if err := os.Symlink(oldTarget, guidancePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile.CodexHome, "skills"), []byte("local override"), 0o600); err != nil {
		t.Fatal(err)
	}

	guidanceSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(guidanceSource, "AGENTS.md"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillSource := t.TempDir()
	inherit := true
	sources := []string{skillSource}
	policy := &ProfileResources{
		Guidance: &GuidanceResources{Inherit: &inherit, Source: guidanceSource},
		Skills:   &SkillResources{Inherit: &inherit, Sources: &sources},
	}

	_, err := store.EnsureProfileDir(profile, policy)
	if err == nil || !strings.Contains(err.Error(), "profile skills path is not a directory") {
		t.Fatalf("expected destination error, got %v", err)
	}
	assertLinkTarget(t, guidancePath, oldTarget)
	if _, err := os.Lstat(filepath.Join(profile.CodexHome, "config.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination validation mutated profile config: %v", err)
	}
}

func TestLegacySkillDestinationValidationPrecedesGuidanceMutation(t *testing.T) {
	store, profile := newResourceTestStore(t)
	if err := os.MkdirAll(filepath.Join(profile.CodexHome, "skills"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldTarget := filepath.Join(t.TempDir(), "old-guidance")
	if err := os.WriteFile(oldTarget, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	guidancePath := filepath.Join(profile.CodexHome, "AGENTS.md")
	if err := os.Symlink(oldTarget, guidancePath); err != nil {
		t.Fatal(err)
	}
	foreignSkill := filepath.Join(t.TempDir(), "foreign-skill")
	if err := os.Mkdir(foreignSkill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignSkill, filepath.Join(profile.CodexHome, "skills", "foreign")); err != nil {
		t.Fatal(err)
	}
	guidanceSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(guidanceSource, "AGENTS.md"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	inherit := true
	policy := &ProfileResources{Guidance: &GuidanceResources{Inherit: &inherit, Source: guidanceSource}}

	_, err := store.EnsureProfileDir(profile, policy)
	if err == nil || !strings.Contains(err.Error(), "must point under default skills directory") {
		t.Fatalf("expected legacy skill destination error, got %v", err)
	}
	assertLinkTarget(t, guidancePath, oldTarget)
}

func TestOmittedResourcePolicyLeavesGuidanceUntouched(t *testing.T) {
	store, profile := newResourceTestStore(t)
	if err := os.MkdirAll(profile.CodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "guidance")
	if err := os.WriteFile(target, []byte("guidance"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(profile.CodexHome, "AGENTS.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureProfileDir(profile, nil); err != nil {
		t.Fatal(err)
	}
	assertLinkTarget(t, link, target)
}

func newResourceTestStore(t *testing.T) (*Store, Profile) {
	t.Helper()
	root := t.TempDir()
	paths := Paths{
		MultisubsHome:    filepath.Join(root, "multi"),
		ConfigPath:       filepath.Join(root, "multi", "config.json"),
		ProfilesDir:      filepath.Join(root, "multi", "profiles"),
		DefaultCodexHome: filepath.Join(root, "default-codex"),
	}
	if err := os.MkdirAll(paths.DefaultCodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.DefaultCodexHome, "config.toml"), []byte(generatedProfileConfigContent), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := Profile{Name: "work", CodexHome: filepath.Join(paths.ProfilesDir, "work", "codex-home")}
	return NewStore(paths), profile
}

func stringSlicePointer(value []string) *[]string {
	return &value
}

func assertLinkTarget(t *testing.T, path, want string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink: %s", path)
	}
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalProfilePath(got) != canonicalProfilePath(want) {
		t.Fatalf("unexpected target for %s: want %s, got %s", path, want, got)
	}
}

func assertExactLinkTarget(t *testing.T, path, want string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink: %s", path)
	}
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("unexpected exact target for %s: want %s, got %s", path, want, got)
	}
}

func mustResolveExistingPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := resolveExistingPath(path)
	if err != nil {
		t.Fatalf("resolve existing path %s: %v", path, err)
	}
	return resolved
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
