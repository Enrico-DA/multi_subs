package multisubs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var guidanceNames = []string{"AGENTS.md", "AGENTS.override.md"}

// ProfileResources controls the optional resources multisubs manages in each profile.
type ProfileResources struct {
	Guidance *GuidanceResources `json:"guidance,omitempty"`
	Skills   *SkillResources    `json:"skills,omitempty"`
}

type GuidanceResources struct {
	Inherit *bool  `json:"inherit"`
	Source  string `json:"source,omitempty"`
}

type SkillResources struct {
	Inherit *bool     `json:"inherit"`
	Sources *[]string `json:"sources,omitempty"`
}

type ResourceChange struct {
	Action    string
	Path      string
	OldTarget string
	NewTarget string
}

func (c ResourceChange) String() string {
	parts := []string{fmt.Sprintf("%s %s", c.Action, c.Path)}
	if c.OldTarget != "" {
		parts = append(parts, "old target: "+c.OldTarget)
	}
	if c.NewTarget != "" {
		parts = append(parts, "new target: "+c.NewTarget)
	}
	return strings.Join(parts, "; ")
}

func printResourceChanges(changes []ResourceChange) {
	printResourceChangesTo(os.Stdout, changes)
}

func printResourceChangesToStderr(changes []ResourceChange) {
	printResourceChangesTo(os.Stderr, changes)
}

func printResourceChangesTo(writer io.Writer, changes []ResourceChange) {
	for _, change := range changes {
		fmt.Fprintln(writer, "profile resource:", change.String())
	}
}

func (r *ProfileResources) UnmarshalJSON(data []byte) error {
	type raw ProfileResources
	var decoded raw
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return fmt.Errorf("profile_resources: %w", err)
	}
	*r = ProfileResources(decoded)
	return nil
}

func (r *GuidanceResources) UnmarshalJSON(data []byte) error {
	type raw GuidanceResources
	var decoded raw
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return fmt.Errorf("guidance: %w", err)
	}
	if decoded.Inherit == nil {
		return errors.New("guidance: required field inherit is missing")
	}
	if !*decoded.Inherit && strings.TrimSpace(decoded.Source) != "" {
		return errors.New("guidance: source cannot be set when inherit is false")
	}
	*r = GuidanceResources(decoded)
	return nil
}

func (r *SkillResources) UnmarshalJSON(data []byte) error {
	type raw SkillResources
	var decoded raw
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return fmt.Errorf("skills: %w", err)
	}
	if decoded.Inherit == nil {
		return errors.New("skills: required field inherit is missing")
	}
	if !*decoded.Inherit && decoded.Sources != nil && len(*decoded.Sources) > 0 {
		return errors.New("skills: sources cannot be set when inherit is false")
	}
	if *decoded.Inherit && decoded.Sources != nil && len(*decoded.Sources) == 0 {
		return errors.New("skills: explicit sources must not be empty when inherit is true")
	}
	*r = SkillResources(decoded)
	return nil
}

func decodeStrictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

type resolvedProfileResources struct {
	guidance *resolvedGuidanceResources
	skills   *resolvedSkillResources
}

type resolvedGuidanceResources struct {
	inherit bool
	source  string
	desired map[string]string
}

type resolvedSkillResources struct {
	inherit bool
	sources []string
	desired map[string]string
}

// ResolveProfileResources validates configured resource paths without changing profile state.
func (s *Store) ResolveProfileResources(resources *ProfileResources) (*resolvedProfileResources, error) {
	resolved := &resolvedProfileResources{}
	if resources == nil {
		skills, err := s.resolveDefaultSkillResources()
		if err != nil {
			return nil, err
		}
		resolved.skills = skills
		return resolved, nil
	}
	if resources.Guidance != nil {
		guidance, err := s.resolveGuidanceResources(resources.Guidance)
		if err != nil {
			return nil, err
		}
		resolved.guidance = guidance
	}
	if resources.Skills != nil {
		skills, err := s.resolveSkillResources(resources.Skills)
		if err != nil {
			return nil, err
		}
		resolved.skills = skills
	} else {
		skills, err := s.resolveDefaultSkillResources()
		if err != nil {
			return nil, err
		}
		resolved.skills = skills
	}
	return resolved, nil
}

func (s *Store) resolveDefaultSkillResources() (*resolvedSkillResources, error) {
	inherit := true
	return s.resolveSkillResources(&SkillResources{Inherit: &inherit})
}

func (s *Store) resolveGuidanceResources(settings *GuidanceResources) (*resolvedGuidanceResources, error) {
	if settings.Inherit == nil {
		return nil, errors.New("profile_resources.guidance.inherit is required")
	}
	resolved := &resolvedGuidanceResources{inherit: *settings.Inherit, desired: map[string]string{}}
	if !resolved.inherit {
		if strings.TrimSpace(settings.Source) != "" {
			return nil, errors.New("profile_resources.guidance.source cannot be set when inherit is false")
		}
		return resolved, nil
	}
	source := strings.TrimSpace(settings.Source)
	if source == "" {
		resolved.source = s.paths.DefaultCodexHome
	} else {
		path, err := s.resolveResourcePath(source)
		if err != nil {
			return nil, fmt.Errorf("resolve profile_resources.guidance.source: %w", err)
		}
		resolved.source = path
	}
	if err := requireDirectory(resolved.source, "guidance source"); err != nil {
		return nil, err
	}
	for _, name := range guidanceNames {
		path := filepath.Join(resolved.source, name)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect guidance source file %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("guidance source is not a regular file: %s", path)
		}
		resolved.desired[name] = path
	}
	return resolved, nil
}

func (s *Store) resolveSkillResources(settings *SkillResources) (*resolvedSkillResources, error) {
	if settings.Inherit == nil {
		return nil, errors.New("profile_resources.skills.inherit is required")
	}
	resolved := &resolvedSkillResources{inherit: *settings.Inherit, desired: map[string]string{}}
	if !resolved.inherit {
		if settings.Sources != nil && len(*settings.Sources) > 0 {
			return nil, errors.New("profile_resources.skills.sources cannot be set when inherit is false")
		}
		return resolved, nil
	}
	defaultSkillsPath := filepath.Join(s.paths.DefaultCodexHome, "skills")
	sourcePaths := []string{defaultSkillsPath}
	sourceLabels := []string{defaultSkillsPath}
	allowMissingDefault := settings.Sources == nil
	if settings.Sources != nil {
		if len(*settings.Sources) == 0 {
			return nil, errors.New("profile_resources.skills.sources must not be empty")
		}
		sourcePaths = make([]string, 0, len(*settings.Sources))
		sourceLabels = make([]string, 0, len(*settings.Sources))
		for i, source := range *settings.Sources {
			path, err := s.resolveResourcePath(source)
			if err != nil {
				return nil, fmt.Errorf("resolve profile_resources.skills.sources[%d]: %w", i, err)
			}
			sourcePaths = append(sourcePaths, path)
			sourceLabels = append(sourceLabels, source)
		}
	}

	seen := map[string]bool{}
	for i, sourcePath := range sourcePaths {
		canonicalSource, err := resolveExistingPath(sourcePath)
		if errors.Is(err, os.ErrNotExist) && allowMissingDefault {
			continue
		}
		if err != nil {
			if settings.Sources == nil {
				return nil, fmt.Errorf("resolve default skill source %s: %w", sourcePath, err)
			}
			return nil, fmt.Errorf("resolve profile_resources.skills.sources[%d]: %w", i, err)
		}
		if err := requireDirectory(canonicalSource, "skill source"); err != nil {
			return nil, err
		}
		sourceUsesDefaultSkills := sameProfilePath(canonicalSource, defaultSkillsPath)
		if err := s.validateSkillSourcePath(canonicalSource, sourceUsesDefaultSkills); err != nil {
			if settings.Sources == nil {
				return nil, fmt.Errorf("validate default skill source: %w", err)
			}
			return nil, fmt.Errorf("validate profile_resources.skills.sources[%d]: %w", i, err)
		}
		if seen[canonicalSource] {
			return nil, fmt.Errorf("profile_resources.skills.sources[%d] duplicates an earlier source: %s", i, sourceLabels[i])
		}
		seen[canonicalSource] = true
		resolved.sources = append(resolved.sources, canonicalSource)
	}

	for _, source := range resolved.sources {
		sourceUsesDefaultSkills := sameProfilePath(source, defaultSkillsPath)
		entries, err := os.ReadDir(source)
		if err != nil {
			return nil, fmt.Errorf("read skill source %s: %w", source, err)
		}
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name())
			if !isInheritableSkillName(name) {
				continue
			}
			path := filepath.Join(source, name)
			canonicalEntry, err := resolveExistingPath(path)
			if err != nil {
				return nil, fmt.Errorf("resolve skill source entry %s: %w", path, err)
			}
			info, err := os.Stat(canonicalEntry)
			if err != nil {
				return nil, fmt.Errorf("inspect skill source entry %s: %w", canonicalEntry, err)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("skill source entry is not a directory: %s", canonicalEntry)
			}
			if err := s.validateSkillSourcePath(canonicalEntry, sourceUsesDefaultSkills); err != nil {
				return nil, fmt.Errorf("validate skill source entry %s: %w", canonicalEntry, err)
			}
			if _, exists := resolved.desired[name]; !exists {
				resolved.desired[name] = canonicalEntry
			}
		}
	}
	return resolved, nil
}

func (s *Store) validateSkillSourcePath(path string, allowDefaultSkillsTree bool) error {
	protectedPaths := []string{
		s.paths.MultisubsHome,
		s.paths.ConfigPath,
		s.paths.ProfilesDir,
		s.paths.ClaudeProviderDir,
		s.paths.ClaudeConfigPath,
		s.paths.ClaudeProfilesDir,
		s.paths.ClaudeRunDir,
	}
	for _, protectedPath := range protectedPaths {
		if strings.TrimSpace(protectedPath) == "" {
			continue
		}
		if pathIsInsideRoot(protectedPath, path) || pathIsInsideRoot(path, protectedPath) {
			return fmt.Errorf("skill source overlaps multisubs-owned state: %s", path)
		}
	}
	if strings.TrimSpace(s.paths.DefaultCodexHome) == "" {
		return nil
	}
	if !pathIsInsideRoot(s.paths.DefaultCodexHome, path) && !pathIsInsideRoot(path, s.paths.DefaultCodexHome) {
		return nil
	}
	defaultSkillsPath := filepath.Join(s.paths.DefaultCodexHome, "skills")
	if allowDefaultSkillsTree && pathIsInsideRoot(defaultSkillsPath, path) {
		return nil
	}
	return fmt.Errorf("skill source overlaps default Codex home: %s", path)
}

func isInheritableSkillName(name string) bool {
	return name != "" && name != "." && name != ".." && name != ".system"
}

func inspectProfileSystemSkill(defaultSkillsPath, systemPath string) (string, bool, error) {
	info, err := os.Lstat(systemPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect profile system skills path: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if !info.IsDir() {
			return "", false, fmt.Errorf("profile system skills path is not a directory: %s", systemPath)
		}
		return "", false, nil
	}

	rawTarget, err := os.Readlink(systemPath)
	if err != nil {
		return "", false, fmt.Errorf("read profile system skills symlink: %w", err)
	}
	if containsParentPathSegment(rawTarget) {
		return "", false, fmt.Errorf("profile system skills symlink contains parent directory traversal: %s", systemPath)
	}
	resolvedTarget, err := resolveExistingSymlinkTarget(systemPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("profile system skills symlink is broken: %s", systemPath)
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve profile system skills symlink: %w", err)
	}
	resolvedDefaultSkillsPath, err := resolveExistingPath(defaultSkillsPath)
	if err != nil {
		return "", false, fmt.Errorf("resolve default skills directory: %w", err)
	}
	if !pathIsInsideRoot(resolvedDefaultSkillsPath, resolvedTarget) {
		return "", false, fmt.Errorf("profile system skills symlink must point under default skills directory: %s", systemPath)
	}
	return rawTarget, true, nil
}

// validateProfileResourceDestinations checks profile-owned positions before any
// profile setup or resource reconciliation changes the filesystem.
func (s *Store) validateProfileResourceDestinations(codexHome string, policy *ProfileResources, resolved *resolvedProfileResources) error {
	if policy != nil && policy.Guidance != nil {
		if err := validateOwnedLinkPositions(codexHome, guidanceNames, "profile guidance"); err != nil {
			return err
		}
	}
	defaultSkillsPath := filepath.Join(s.paths.DefaultCodexHome, "skills")
	if policy == nil || policy.Skills == nil {
		if resolved != nil && resolved.skills != nil && len(resolved.skills.sources) > 0 {
			defaultSkillsPath = resolved.skills.sources[0]
		}
	}

	profileSkillsPath := filepath.Join(codexHome, "skills")
	if err := ensurePathNotSymlinkIfExists(profileSkillsPath); err != nil {
		return err
	}
	info, err := os.Lstat(profileSkillsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect profile skills path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("profile skills path is not a directory: %s", profileSkillsPath)
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(codexHome, profileSkillsPath); err != nil {
		return err
	}
	entries, err := os.ReadDir(profileSkillsPath)
	if err != nil {
		return fmt.Errorf("read profile skills dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if policy != nil && policy.Skills != nil &&
			entry.Name() != ".system" &&
			!isInheritableSkillName(strings.TrimSpace(entry.Name())) {
			continue
		}
		names = append(names, entry.Name())
	}
	if err := validateOwnedLinkPositions(profileSkillsPath, names, "profile skill"); err != nil {
		return err
	}
	systemPath := filepath.Join(profileSkillsPath, ".system")
	if _, _, err := inspectProfileSystemSkill(defaultSkillsPath, systemPath); err != nil {
		return err
	}
	if policy != nil && policy.Skills != nil {
		return nil
	}
	for _, name := range names {
		path := filepath.Join(profileSkillsPath, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if name == ".system" {
			if _, _, err := inspectProfileSystemSkill(defaultSkillsPath, path); err != nil {
				return err
			}
			continue
		}
		target, err := resolveExistingSymlinkTarget(path)
		if errors.Is(err, os.ErrNotExist) {
			target, err = resolveBrokenManagedSymlinkTarget(path)
		}
		if err != nil {
			return fmt.Errorf("resolve profile skill symlink %s: %w", path, err)
		}
		if !pathIsInsideRoot(defaultSkillsPath, target) {
			_, wanted := resolved.skills.desired[name]
			if !wanted {
				return fmt.Errorf("profile skill symlink must point under default skills directory: %s", path)
			}
			allowDefaultSkillsTree := pathIsInsideRoot(defaultSkillsPath, target)
			if err := s.validateSkillSourcePath(target, allowDefaultSkillsTree); err != nil {
				return fmt.Errorf("validate existing profile skill symlink %s: %w", path, err)
			}
			if err := requireDirectory(target, "existing profile skill target"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOwnedLinkPositions(root string, names []string, label string) error {
	for _, name := range names {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s %s: %w", label, path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, err := os.Readlink(path); err != nil {
			return fmt.Errorf("read %s symlink %s: %w", label, path, err)
		}
	}
	return nil
}

func (s *Store) resolveResourcePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("path is blank")
	}
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if value == "~" {
			value = home
		} else if strings.HasPrefix(value, "~/") {
			value = filepath.Join(home, value[2:])
		} else {
			return "", fmt.Errorf("unsupported home path %q; use ~ or ~/path", value)
		}
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(filepath.Dir(s.paths.ConfigPath), value)
	}
	return filepath.Clean(value), nil
}

func requireDirectory(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %s: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func (s *Store) reconcileProfileResources(codexHome string, policy *ProfileResources, resolved *resolvedProfileResources) ([]ResourceChange, error) {
	if policy == nil {
		return nil, s.ensureProfileSkills(codexHome, resolved.skills)
	}
	var changes []ResourceChange
	if resolved.guidance != nil {
		guidanceChanges, err := reconcileGuidance(codexHome, resolved.guidance)
		if err != nil {
			return nil, err
		}
		changes = append(changes, guidanceChanges...)
	}
	if policy.Skills == nil {
		if err := s.ensureProfileSkills(codexHome, resolved.skills); err != nil {
			return nil, err
		}
	} else {
		skillChanges, err := reconcileExplicitSkills(
			codexHome,
			filepath.Join(s.paths.DefaultCodexHome, "skills"),
			resolved.skills,
		)
		if err != nil {
			return nil, err
		}
		changes = append(changes, skillChanges...)
	}
	return changes, nil
}

func reconcileGuidance(codexHome string, resolved *resolvedGuidanceResources) ([]ResourceChange, error) {
	localOverride := false
	for _, name := range guidanceNames {
		info, err := os.Lstat(filepath.Join(codexHome, name))
		if err == nil && info.Mode()&os.ModeSymlink == 0 {
			localOverride = true
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect profile guidance %s: %w", name, err)
		}
	}
	desired := resolved.desired
	if !resolved.inherit || localOverride {
		desired = map[string]string{}
	}
	return reconcileOwnedLinks(codexHome, guidanceNames, desired, "profile guidance")
}

func reconcileExplicitSkills(codexHome, defaultSkillsPath string, resolved *resolvedSkillResources) ([]ResourceChange, error) {
	profileSkillsPath := filepath.Join(codexHome, "skills")
	if err := ensurePathNotSymlinkIfExists(profileSkillsPath); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(profileSkillsPath); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("profile skills path is not a directory: %s", profileSkillsPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect profile skills path: %w", err)
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(codexHome, profileSkillsPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profileSkillsPath, 0o700); err != nil {
		return nil, fmt.Errorf("create profile skills dir: %w", err)
	}
	if err := os.Chmod(profileSkillsPath, 0o700); err != nil {
		return nil, fmt.Errorf("secure profile skills dir permissions: %w", err)
	}
	systemChanges, err := reconcileProfileSystemSkill(defaultSkillsPath, profileSkillsPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(profileSkillsPath)
	if err != nil {
		return nil, fmt.Errorf("read profile skills dir: %w", err)
	}
	names := make([]string, 0, len(entries)+len(resolved.desired))
	seen := map[string]bool{}
	for _, entry := range entries {
		if !isInheritableSkillName(strings.TrimSpace(entry.Name())) {
			continue
		}
		names = append(names, entry.Name())
		seen[entry.Name()] = true
	}
	for name := range resolved.desired {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	desired := resolved.desired
	if !resolved.inherit {
		desired = map[string]string{}
	}
	skillChanges, err := reconcileSkillLinks(profileSkillsPath, names, desired, "profile skill")
	if err != nil {
		return nil, err
	}
	return append(systemChanges, skillChanges...), nil
}

func reconcileProfileSystemSkill(defaultSkillsPath, profileSkillsPath string) ([]ResourceChange, error) {
	systemPath := filepath.Join(profileSkillsPath, ".system")
	oldTarget, inherited, err := inspectProfileSystemSkill(defaultSkillsPath, systemPath)
	if err != nil {
		return nil, err
	}
	if !inherited {
		return nil, nil
	}
	if err := os.Remove(systemPath); err != nil {
		return nil, fmt.Errorf("remove inherited profile system skills symlink: %w", err)
	}
	return []ResourceChange{{
		Action:    "removed",
		Path:      systemPath,
		OldTarget: oldTarget,
	}}, nil
}

func reconcileOwnedLinks(root string, names []string, desired map[string]string, label string) ([]ResourceChange, error) {
	return reconcileOwnedLinksWithTargetMatch(root, names, desired, label, false)
}

func reconcileSkillLinks(root string, names []string, desired map[string]string, label string) ([]ResourceChange, error) {
	return reconcileOwnedLinksWithTargetMatch(root, names, desired, label, true)
}

func reconcileOwnedLinksWithTargetMatch(root string, names []string, desired map[string]string, label string, exactTarget bool) ([]ResourceChange, error) {
	var changes []ResourceChange
	for _, name := range names {
		path := filepath.Join(root, name)
		want, wanted := desired[name]
		info, err := os.Lstat(path)
		if err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			oldTarget, err := os.Readlink(path)
			if err != nil {
				return nil, fmt.Errorf("read %s symlink %s: %w", label, path, err)
			}
			if wanted {
				targetMatches := oldTarget == want
				if !exactTarget {
					resolvedOld := resolveLinkTarget(path, oldTarget)
					targetMatches = canonicalProfilePath(resolvedOld) == canonicalProfilePath(want)
				}
				if targetMatches {
					continue
				}
			}
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("remove %s symlink %s: %w", label, path, err)
			}
			action := "removed"
			if wanted {
				action = "retargeted"
			}
			change := ResourceChange{Action: action, Path: path, OldTarget: oldTarget}
			if wanted {
				if err := os.Symlink(want, path); err != nil {
					return nil, fmt.Errorf("link %s %s: %w", label, path, err)
				}
				change.NewTarget = want
			}
			changes = append(changes, change)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect %s %s: %w", label, path, err)
		}
		if !wanted {
			continue
		}
		if err := os.Symlink(want, path); err != nil {
			return nil, fmt.Errorf("link %s %s: %w", label, path, err)
		}
		changes = append(changes, ResourceChange{Action: "linked", Path: path, NewTarget: want})
	}
	return changes, nil
}

func resolveLinkTarget(linkPath, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
}
