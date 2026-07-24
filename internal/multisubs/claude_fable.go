package multisubs

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

const claudeSettingsFileLimit = 2 * 1024 * 1024

type fableApplicability int

const (
	fableNotApplicable fableApplicability = iota
	fableApplicable
	fablePossible
)

func (applicability fableApplicability) needsFable() bool {
	return applicability != fableNotApplicable
}

type claudeFableApplicabilityResolver interface {
	ParseIntent(args, environment []string) claudeCLIIntent
	Resolve(claudeCLIIntent, claudeTarget) fableApplicability
}

type claudeFieldState int

const (
	claudeFieldAbsent claudeFieldState = iota
	claudeFieldKnown
	claudeFieldUncertain
)

type claudeSettingField struct {
	State  claudeFieldState
	Values []string
}

func knownClaudeSetting(values ...string) claudeSettingField {
	return claudeSettingField{State: claudeFieldKnown, Values: append([]string(nil), values...)}
}

func uncertainClaudeSetting() claudeSettingField {
	return claudeSettingField{State: claudeFieldUncertain}
}

var claudeRelevantEnvironmentNames = []string{
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_DEFAULT_FABLE_MODEL",
}

type claudeRelevantSettings struct {
	Model         claudeSettingField
	FallbackModel claudeSettingField
	Environment   map[string]claudeSettingField
}

func emptyClaudeRelevantSettings() claudeRelevantSettings {
	settings := claudeRelevantSettings{
		Environment: make(map[string]claudeSettingField, len(claudeRelevantEnvironmentNames)),
	}
	for _, name := range claudeRelevantEnvironmentNames {
		settings.Environment[name] = claudeSettingField{}
	}
	return settings
}

func uncertainClaudeRelevantSettings() claudeRelevantSettings {
	settings := emptyClaudeRelevantSettings()
	settings.Model = uncertainClaudeSetting()
	settings.FallbackModel = uncertainClaudeSetting()
	for _, name := range claudeRelevantEnvironmentNames {
		settings.Environment[name] = uncertainClaudeSetting()
	}
	return settings
}

func overlayClaudeRelevantSettings(lower, higher claudeRelevantSettings) claudeRelevantSettings {
	result := emptyClaudeRelevantSettings()
	result.Model = overlayClaudeSettingField(lower.Model, higher.Model)
	result.FallbackModel = overlayClaudeSettingField(lower.FallbackModel, higher.FallbackModel)
	for _, name := range claudeRelevantEnvironmentNames {
		result.Environment[name] = overlayClaudeSettingField(lower.Environment[name], higher.Environment[name])
	}
	return result
}

func overlayClaudeSettingField(lower, higher claudeSettingField) claudeSettingField {
	if higher.State != claudeFieldAbsent {
		return higher
	}
	return lower
}

type claudePathIntent struct {
	Value string
	Known bool
}

type claudeSettingSourceSelection struct {
	Explicit  bool
	Uncertain bool
	User      bool
	Project   bool
	Local     bool
}

type claudeCLIIntent struct {
	Model            claudeSettingField
	FallbackModel    claudeSettingField
	ExplicitSettings claudeSettingField
	SettingSources   claudeSettingSourceSelection
	RestoresSession  bool
	Environment      claudeRelevantSettings
	WorkingDirectory claudePathIntent
	HomeDirectory    claudePathIntent
}

type claudeOpaqueSettings struct {
	Settings       claudeRelevantSettings
	AccountDefault claudeSettingField
}

type claudeSettingsFile interface {
	io.Reader
	io.Closer
	Stat() (os.FileInfo, error)
}

type claudeSettingsFileSystem interface {
	Lstat(string) (os.FileInfo, error)
	Open(string) (claudeSettingsFile, error)
	ReadDir(string) ([]os.DirEntry, error)
}

type osClaudeSettingsFileSystem struct{}

func (osClaudeSettingsFileSystem) Lstat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func (osClaudeSettingsFileSystem) Open(path string) (claudeSettingsFile, error) {
	return os.Open(path)
}

func (osClaudeSettingsFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

type localClaudeFableApplicabilityResolver struct {
	fileSystem                  claudeSettingsFileSystem
	workingDirectory            func() (string, error)
	homeDirectory               func() (string, error)
	managedSettingsDirectory    string
	inspectLocalManagedSettings bool
	opaqueSettings              func(claudeTarget) claudeOpaqueSettings
}

func newLocalClaudeFableApplicabilityResolver() *localClaudeFableApplicabilityResolver {
	return &localClaudeFableApplicabilityResolver{
		fileSystem:                  osClaudeSettingsFileSystem{},
		workingDirectory:            os.Getwd,
		homeDirectory:               os.UserHomeDir,
		managedSettingsDirectory:    filepath.Join(string(filepath.Separator), "Library", "Application Support", "ClaudeCode"),
		inspectLocalManagedSettings: runtime.GOOS == "darwin",
		opaqueSettings: func(claudeTarget) claudeOpaqueSettings {
			return claudeOpaqueSettings{
				Settings:       uncertainClaudeRelevantSettings(),
				AccountDefault: uncertainClaudeSetting(),
			}
		},
	}
}

func (resolver *localClaudeFableApplicabilityResolver) ParseIntent(args, environment []string) claudeCLIIntent {
	intent := claudeCLIIntent{
		Environment: emptyClaudeRelevantSettings(),
	}
	if workingDirectory, err := resolver.workingDirectory(); err == nil && filepath.IsAbs(workingDirectory) {
		intent.WorkingDirectory = claudePathIntent{Value: filepath.Clean(workingDirectory), Known: true}
	}
	if homeDirectory, err := resolver.homeDirectory(); err == nil && filepath.IsAbs(homeDirectory) {
		intent.HomeDirectory = claudePathIntent{Value: filepath.Clean(homeDirectory), Known: true}
	}
	intent.Environment = parseClaudeRelevantEnvironment(environment)

	var settingSourcesField claudeSettingField
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "--" {
			break
		}
		switch {
		case arg == "--model" || arg == "-m":
			value, found := nextClaudeOptionValue(args, index)
			setClaudeCLIField(&intent.Model, value, found)
			if found {
				index++
			}
		case strings.HasPrefix(arg, "--model="):
			setClaudeCLIField(&intent.Model, strings.TrimPrefix(arg, "--model="), true)
		case strings.HasPrefix(arg, "-m="):
			setClaudeCLIField(&intent.Model, strings.TrimPrefix(arg, "-m="), true)
		case strings.HasPrefix(arg, "--model") || (strings.HasPrefix(arg, "-m") && arg != "-m"):
			setClaudeCLIField(&intent.Model, "", false)
		case arg == "--fallback-model":
			value, found := nextClaudeOptionValue(args, index)
			setClaudeCLIField(&intent.FallbackModel, value, found)
			if found {
				index++
			}
		case strings.HasPrefix(arg, "--fallback-model="):
			setClaudeCLIField(&intent.FallbackModel, strings.TrimPrefix(arg, "--fallback-model="), true)
		case strings.HasPrefix(arg, "--fallback-model"):
			setClaudeCLIField(&intent.FallbackModel, "", false)
		case arg == "--settings":
			value, found := nextClaudeOptionValue(args, index)
			setClaudeCLIField(&intent.ExplicitSettings, value, found && strings.TrimSpace(value) != "")
			if found {
				index++
			}
		case strings.HasPrefix(arg, "--settings="):
			value := strings.TrimPrefix(arg, "--settings=")
			setClaudeCLIField(&intent.ExplicitSettings, value, strings.TrimSpace(value) != "")
		case strings.HasPrefix(arg, "--settings"):
			setClaudeCLIField(&intent.ExplicitSettings, "", false)
		case arg == "--setting-sources":
			value, found := nextClaudeOptionValue(args, index)
			setClaudeCLIField(&settingSourcesField, value, found)
			if found {
				index++
			}
		case strings.HasPrefix(arg, "--setting-sources="):
			setClaudeCLIField(&settingSourcesField, strings.TrimPrefix(arg, "--setting-sources="), true)
		case strings.HasPrefix(arg, "--setting-sources"):
			setClaudeCLIField(&settingSourcesField, "", false)
		case claudeSessionRestorationArgument(arg):
			intent.RestoresSession = true
		}
	}
	intent.SettingSources = parseClaudeSettingSourceSelection(settingSourcesField)
	return intent
}

func nextClaudeOptionValue(args []string, index int) (string, bool) {
	if index+1 >= len(args) {
		return "", false
	}
	value := strings.TrimSpace(args[index+1])
	if value == "--" || strings.HasPrefix(value, "-") {
		return "", false
	}
	return args[index+1], true
}

func setClaudeCLIField(field *claudeSettingField, value string, valid bool) {
	if field.State != claudeFieldAbsent || !valid {
		field.State = claudeFieldUncertain
		field.Values = nil
		return
	}
	*field = knownClaudeSetting(strings.TrimSpace(value))
}

func parseClaudeSettingSourceSelection(field claudeSettingField) claudeSettingSourceSelection {
	if field.State == claudeFieldAbsent {
		return claudeSettingSourceSelection{User: true, Project: true, Local: true}
	}
	if field.State == claudeFieldUncertain || len(field.Values) != 1 {
		return claudeSettingSourceSelection{Explicit: true, Uncertain: true}
	}
	selection := claudeSettingSourceSelection{Explicit: true}
	value := strings.TrimSpace(field.Values[0])
	if value == "" {
		return selection
	}
	seen := make(map[string]struct{}, 3)
	for _, item := range strings.Split(value, ",") {
		source := strings.ToLower(strings.TrimSpace(item))
		if _, duplicate := seen[source]; duplicate {
			selection.Uncertain = true
			continue
		}
		seen[source] = struct{}{}
		switch source {
		case "user":
			selection.User = true
		case "project":
			selection.Project = true
		case "local":
			selection.Local = true
		default:
			selection.Uncertain = true
		}
	}
	if selection.Uncertain {
		selection.User = false
		selection.Project = false
		selection.Local = false
	}
	return selection
}

func claudeSessionRestorationArgument(arg string) bool {
	switch arg {
	case "--continue", "-c", "--resume", "-r":
		return true
	}
	return strings.HasPrefix(arg, "--continue=") ||
		strings.HasPrefix(arg, "--resume=") ||
		(strings.HasPrefix(arg, "-c") && arg != "-c") ||
		(strings.HasPrefix(arg, "-r") && arg != "-r")
}

func parseClaudeRelevantEnvironment(environment []string) claudeRelevantSettings {
	settings := emptyClaudeRelevantSettings()
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if !found || !isClaudeRelevantEnvironmentName(name) {
			continue
		}
		field := settings.Environment[name]
		if field.State != claudeFieldAbsent {
			settings.Environment[name] = uncertainClaudeSetting()
			continue
		}
		settings.Environment[name] = knownClaudeSetting(value)
	}
	return settings
}

func isClaudeRelevantEnvironmentName(name string) bool {
	for _, relevantName := range claudeRelevantEnvironmentNames {
		if name == relevantName {
			return true
		}
	}
	return false
}

func (resolver *localClaudeFableApplicabilityResolver) Resolve(intent claudeCLIIntent, target claudeTarget) fableApplicability {
	settings := emptyClaudeRelevantSettings()
	selection := intent.SettingSources
	if selection.Uncertain {
		settings = overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
	} else {
		if selection.User {
			settings = overlayClaudeRelevantSettings(settings, resolver.resolveClaudeUserSettings(intent, target))
		}
		if selection.Project || selection.Local {
			projectRoot, rootState := resolver.resolveClaudeProjectRoot(intent.WorkingDirectory)
			if rootState == claudeProjectRootUncertain {
				if selection.Project {
					settings = overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
				}
				if selection.Local {
					settings = overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
				}
			} else {
				if selection.Project {
					settings = overlayClaudeRelevantSettings(
						settings,
						resolver.readClaudeSettingsFile(filepath.Join(projectRoot, ".claude", "settings.json"), "project settings", true),
					)
				}
				if selection.Local {
					settings = overlayClaudeRelevantSettings(
						settings,
						resolver.readClaudeSettingsFile(filepath.Join(projectRoot, ".claude", "settings.local.json"), "local settings", true),
					)
				}
			}
		}
	}
	settings = overlayClaudeRelevantSettings(settings, resolver.resolveExplicitClaudeSettings(intent))
	settings = overlayClaudeRelevantSettings(settings, resolver.resolveLocalManagedClaudeSettings())

	opaque := claudeOpaqueSettings{
		Settings:       uncertainClaudeRelevantSettings(),
		AccountDefault: uncertainClaudeSetting(),
	}
	if resolver.opaqueSettings != nil {
		opaque = resolver.opaqueSettings(target)
	}
	settings = overlayClaudeRelevantSettings(settings, opaque.Settings)

	effectiveEnvironment := emptyClaudeRelevantSettings()
	effectiveEnvironment.Environment = intent.Environment.Environment
	effectiveEnvironment = overlayClaudeRelevantSettings(effectiveEnvironment, claudeRelevantSettings{
		Environment: settings.Environment,
	})

	context := claudeModelClassificationContext{
		Environment:    effectiveEnvironment.Environment,
		AccountDefault: opaque.AccountDefault,
	}
	primary := resolveClaudePrimaryApplicability(intent.Model, effectiveEnvironment.Environment["ANTHROPIC_MODEL"], settings.Model, context)
	fallback := resolveClaudeFallbackApplicability(intent.FallbackModel, settings.FallbackModel, context)
	applicability := combineFableApplicability(primary, fallback)
	if intent.RestoresSession && !claudeCLIPrimaryIsConclusive(intent.Model, context) {
		applicability = combineFableApplicability(applicability, fablePossible)
	}
	return applicability
}

func (resolver *localClaudeFableApplicabilityResolver) resolveClaudeUserSettings(intent claudeCLIIntent, target claudeTarget) claudeRelevantSettings {
	if target.Kind == "managed" {
		if strings.TrimSpace(target.ConfigDir) == "" {
			return uncertainClaudeRelevantSettings()
		}
		return resolver.readClaudeSettingsFile(filepath.Join(target.ConfigDir, "settings.json"), "user settings", true)
	}
	if !intent.HomeDirectory.Known {
		return uncertainClaudeRelevantSettings()
	}
	return resolver.readClaudeSettingsFile(
		filepath.Join(intent.HomeDirectory.Value, ".claude", "settings.json"),
		"user settings",
		true,
	)
}

func (resolver *localClaudeFableApplicabilityResolver) resolveExplicitClaudeSettings(intent claudeCLIIntent) claudeRelevantSettings {
	if intent.ExplicitSettings.State == claudeFieldAbsent {
		return emptyClaudeRelevantSettings()
	}
	if intent.ExplicitSettings.State == claudeFieldUncertain || len(intent.ExplicitSettings.Values) != 1 {
		return uncertainClaudeRelevantSettings()
	}
	value := strings.TrimSpace(intent.ExplicitSettings.Values[0])
	if strings.HasPrefix(value, "{") {
		if len(value) > claudeSettingsFileLimit {
			return uncertainClaudeRelevantSettings()
		}
		settings, err := decodeClaudeRelevantSettings(strings.NewReader(value))
		if err != nil {
			return uncertainClaudeRelevantSettings()
		}
		return settings
	}
	path := value
	if !filepath.IsAbs(path) {
		if !intent.WorkingDirectory.Known {
			return uncertainClaudeRelevantSettings()
		}
		path = filepath.Join(intent.WorkingDirectory.Value, path)
	}
	return resolver.readClaudeSettingsFile(filepath.Clean(path), "explicit settings", false)
}

func (resolver *localClaudeFableApplicabilityResolver) resolveLocalManagedClaudeSettings() claudeRelevantSettings {
	settings := emptyClaudeRelevantSettings()
	if !resolver.inspectLocalManagedSettings {
		return settings
	}
	settings = overlayClaudeRelevantSettings(
		settings,
		resolver.readClaudeSettingsFile(
			filepath.Join(resolver.managedSettingsDirectory, "managed-settings.json"),
			"managed settings",
			true,
		),
	)
	directory := filepath.Join(resolver.managedSettingsDirectory, "managed-settings.d")
	info, err := resolver.fileSystem.Lstat(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings
		}
		return overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
	}
	entries, err := resolver.fileSystem.ReadDir(directory)
	if err != nil {
		return overlayClaudeRelevantSettings(settings, uncertainClaudeRelevantSettings())
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		settings = overlayClaudeRelevantSettings(
			settings,
			resolver.readClaudeSettingsFile(filepath.Join(directory, name), "managed settings", false),
		)
	}
	return settings
}

type claudeProjectRootState int

const (
	claudeProjectRootFound claudeProjectRootState = iota
	claudeProjectRootOutsideRepository
	claudeProjectRootUncertain
)

func (resolver *localClaudeFableApplicabilityResolver) resolveClaudeProjectRoot(workingDirectory claudePathIntent) (string, claudeProjectRootState) {
	if !workingDirectory.Known {
		return "", claudeProjectRootUncertain
	}
	current := filepath.Clean(workingDirectory.Value)
	for {
		gitPath := filepath.Join(current, ".git")
		info, err := resolver.fileSystem.Lstat(gitPath)
		if err == nil {
			if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return current, claudeProjectRootFound
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !resolver.validClaudeWorktreeGitFile(gitPath, current) {
				return "", claudeProjectRootUncertain
			}
			return current, claudeProjectRootFound
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", claudeProjectRootUncertain
		}
		parent := filepath.Dir(current)
		if parent == current {
			return workingDirectory.Value, claudeProjectRootOutsideRepository
		}
		current = parent
	}
}

func (resolver *localClaudeFableApplicabilityResolver) validClaudeWorktreeGitFile(path, root string) bool {
	file, err := resolver.fileSystem.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) == 0 || len(raw) > 4096 {
		return false
	}
	line := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	prefix, value, found := strings.Cut(line, "gitdir:")
	if !found || strings.TrimSpace(prefix) != "" || strings.TrimSpace(value) == "" {
		return false
	}
	gitDirectory := strings.TrimSpace(value)
	if !filepath.IsAbs(gitDirectory) {
		gitDirectory = filepath.Join(root, gitDirectory)
	}
	info, err := resolver.fileSystem.Lstat(filepath.Clean(gitDirectory))
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

type claudeSettingsReadError string

func (readError claudeSettingsReadError) Error() string {
	return string(readError) + " could not be read safely"
}

func (resolver *localClaudeFableApplicabilityResolver) readClaudeSettingsFile(path, category string, missingAllowed bool) claudeRelevantSettings {
	info, err := resolver.fileSystem.Lstat(path)
	if err != nil {
		if missingAllowed && errors.Is(err, os.ErrNotExist) {
			return emptyClaudeRelevantSettings()
		}
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > claudeSettingsFileLimit {
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	file, err := resolver.fileSystem.Open(path)
	if err != nil {
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() < 0 || openedInfo.Size() > claudeSettingsFileLimit {
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	settings, err := decodeClaudeRelevantSettings(io.LimitReader(file, claudeSettingsFileLimit+1))
	if err != nil {
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	finalInfo, err := file.Stat()
	if err != nil || !finalInfo.Mode().IsRegular() || finalInfo.Size() < 0 || finalInfo.Size() > claudeSettingsFileLimit {
		_ = claudeSettingsReadError(category)
		return uncertainClaudeRelevantSettings()
	}
	return settings
}

func decodeClaudeRelevantSettings(reader io.Reader) (claudeRelevantSettings, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return claudeRelevantSettings{}, errors.New("settings JSON must be one object")
	}
	settings := emptyClaudeRelevantSettings()
	seen := make(map[string]bool, 3)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
		}
		key, ok := keyToken.(string)
		if !ok {
			return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
		}
		switch key {
		case "model":
			if seen[key] {
				settings.Model = uncertainClaudeSetting()
				if err := skipClaudeJSONValue(decoder); err != nil {
					return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
				}
				continue
			}
			seen[key] = true
			settings.Model = decodeClaudeStringField(decoder)
		case "fallbackModel":
			if seen[key] {
				settings.FallbackModel = uncertainClaudeSetting()
				if err := skipClaudeJSONValue(decoder); err != nil {
					return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
				}
				continue
			}
			seen[key] = true
			settings.FallbackModel = decodeClaudeFallbackField(decoder)
		case "env":
			if seen[key] {
				for _, name := range claudeRelevantEnvironmentNames {
					settings.Environment[name] = uncertainClaudeSetting()
				}
				if err := skipClaudeJSONValue(decoder); err != nil {
					return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
				}
				continue
			}
			seen[key] = true
			if err := decodeClaudeEnvironmentFields(decoder, &settings); err != nil {
				return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
			}
		default:
			if err := skipClaudeJSONValue(decoder); err != nil {
				return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
			}
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return claudeRelevantSettings{}, errors.New("settings JSON is malformed")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return claudeRelevantSettings{}, errors.New("settings JSON has trailing data")
	}
	return settings, nil
}

func decodeClaudeStringField(decoder *json.Decoder) claudeSettingField {
	token, err := decoder.Token()
	if err != nil {
		return uncertainClaudeSetting()
	}
	value, ok := token.(string)
	if !ok {
		if delimiter, isDelimiter := token.(json.Delim); isDelimiter {
			_ = skipClaudeJSONAfterToken(decoder, delimiter)
		}
		return uncertainClaudeSetting()
	}
	return knownClaudeSetting(value)
}

func decodeClaudeFallbackField(decoder *json.Decoder) claudeSettingField {
	token, err := decoder.Token()
	if err != nil {
		return uncertainClaudeSetting()
	}
	if value, ok := token.(string); ok {
		return knownClaudeSetting(value)
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		if ok {
			_ = skipClaudeJSONAfterToken(decoder, delimiter)
		}
		return uncertainClaudeSetting()
	}
	values := make([]string, 0, 3)
	uncertain := false
	for decoder.More() {
		item, err := decoder.Token()
		if err != nil {
			return uncertainClaudeSetting()
		}
		value, ok := item.(string)
		if ok {
			values = append(values, value)
			continue
		}
		uncertain = true
		if nested, isDelimiter := item.(json.Delim); isDelimiter {
			if err := skipClaudeJSONAfterToken(decoder, nested); err != nil {
				return uncertainClaudeSetting()
			}
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') || uncertain {
		return uncertainClaudeSetting()
	}
	return knownClaudeSetting(values...)
}

func decodeClaudeEnvironmentFields(decoder *json.Decoder, settings *claudeRelevantSettings) error {
	start, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := start.(json.Delim)
	if !ok || delimiter != '{' {
		if ok {
			if err := skipClaudeJSONAfterToken(decoder, delimiter); err != nil {
				return err
			}
		}
		for _, name := range claudeRelevantEnvironmentNames {
			settings.Environment[name] = uncertainClaudeSetting()
		}
		return nil
	}
	seen := make(map[string]bool, len(claudeRelevantEnvironmentNames))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("environment key is not a string")
		}
		if !isClaudeRelevantEnvironmentName(key) {
			if err := skipClaudeJSONValue(decoder); err != nil {
				return err
			}
			continue
		}
		if seen[key] {
			settings.Environment[key] = uncertainClaudeSetting()
			if err := skipClaudeJSONValue(decoder); err != nil {
				return err
			}
			continue
		}
		seen[key] = true
		settings.Environment[key] = decodeClaudeStringField(decoder)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return errors.New("environment object is malformed")
	}
	return nil
}

func skipClaudeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		return skipClaudeJSONAfterToken(decoder, delimiter)
	}
	return nil
}

func skipClaudeJSONAfterToken(decoder *json.Decoder, delimiter json.Delim) error {
	if delimiter != '{' && delimiter != '[' {
		return errors.New("unexpected closing delimiter")
	}
	stack := []json.Delim{delimiter}
	for len(stack) > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		next, ok := token.(json.Delim)
		if !ok {
			continue
		}
		switch next {
		case '{', '[':
			stack = append(stack, next)
		case '}', ']':
			open := stack[len(stack)-1]
			if (open == '{' && next != '}') || (open == '[' && next != ']') {
				return errors.New("mismatched JSON delimiter")
			}
			stack = stack[:len(stack)-1]
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	return nil
}

type claudeModelClassificationContext struct {
	Environment    map[string]claudeSettingField
	AccountDefault claudeSettingField
}

func resolveClaudePrimaryApplicability(cliModel, environmentModel, settingsModel claudeSettingField, context claudeModelClassificationContext) fableApplicability {
	for _, field := range []claudeSettingField{cliModel, environmentModel, settingsModel, context.AccountDefault} {
		switch field.State {
		case claudeFieldUncertain:
			return fablePossible
		case claudeFieldKnown:
			return classifyClaudeModelField(field, context)
		}
	}
	return fablePossible
}

func resolveClaudeFallbackApplicability(cliFallback, persistentFallback claudeSettingField, context claudeModelClassificationContext) fableApplicability {
	if cliFallback.State != claudeFieldAbsent {
		return classifyClaudeFallbackField(cliFallback, context)
	}
	if persistentFallback.State == claudeFieldAbsent {
		return fableNotApplicable
	}
	return classifyClaudeFallbackField(persistentFallback, context)
}

func classifyClaudeModelField(field claudeSettingField, context claudeModelClassificationContext) fableApplicability {
	if field.State != claudeFieldKnown || len(field.Values) != 1 {
		return fablePossible
	}
	return classifyClaudeModelValue(field.Values[0], context, make(map[string]bool))
}

func classifyClaudeFallbackField(field claudeSettingField, context claudeModelClassificationContext) fableApplicability {
	if field.State == claudeFieldUncertain {
		return fablePossible
	}
	if field.State == claudeFieldAbsent {
		return fableNotApplicable
	}
	models := splitClaudeFallbackModels(field.Values)
	if len(models) == 0 {
		return fableNotApplicable
	}
	applicability := fableNotApplicable
	for _, model := range models {
		applicability = combineFableApplicability(
			applicability,
			classifyClaudeModelValue(model, context, make(map[string]bool)),
		)
	}
	return applicability
}

func splitClaudeFallbackModels(values []string) []string {
	models := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			model := strings.TrimSpace(item)
			key := strings.ToLower(model)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			models = append(models, model)
			if len(models) == 3 {
				return models
			}
		}
	}
	return models
}

func classifyClaudeModelValue(value string, context claudeModelClassificationContext, seen map[string]bool) fableApplicability {
	model := strings.ToLower(strings.TrimSpace(value))
	if model == "" {
		return fablePossible
	}
	if isClaudeFableModel(model) {
		return fableApplicable
	}
	fableMapping := context.Environment["ANTHROPIC_DEFAULT_FABLE_MODEL"]
	if fableMapping.State == claudeFieldKnown && len(fableMapping.Values) == 1 {
		mappedFable := strings.ToLower(strings.TrimSpace(fableMapping.Values[0]))
		if mappedFable != "" && model == mappedFable {
			return fableApplicable
		}
	}
	switch model {
	case "best":
		return fablePossible
	case "default":
		if context.AccountDefault.State == claudeFieldKnown &&
			len(context.AccountDefault.Values) == 1 &&
			strings.ToLower(strings.TrimSpace(context.AccountDefault.Values[0])) != "default" {
			return classifyClaudeModelValue(context.AccountDefault.Values[0], context, seen)
		}
		return fablePossible
	case "sonnet", "opus", "haiku":
		if seen[model] {
			return fablePossible
		}
		seen[model] = true
		mapping := context.Environment[claudeAliasEnvironmentName(model)]
		switch mapping.State {
		case claudeFieldUncertain:
			return fablePossible
		case claudeFieldKnown:
			if len(mapping.Values) != 1 {
				return fablePossible
			}
			return classifyClaudeModelValue(mapping.Values[0], context, seen)
		default:
			if fableMapping.State == claudeFieldUncertain ||
				(fableMapping.State == claudeFieldKnown && len(fableMapping.Values) != 1) {
				return fablePossible
			}
			return fableNotApplicable
		}
	}
	if isRecognizedClaudeNonFableModelID(model) {
		return fableNotApplicable
	}
	return fablePossible
}

func claudeAliasEnvironmentName(alias string) string {
	switch alias {
	case "opus":
		return "ANTHROPIC_DEFAULT_OPUS_MODEL"
	case "sonnet":
		return "ANTHROPIC_DEFAULT_SONNET_MODEL"
	case "haiku":
		return "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	default:
		return ""
	}
}

func isClaudeFableModel(model string) bool {
	if model == "fable" {
		return true
	}
	parts := strings.Split(model, "-")
	if len(parts) < 3 || parts[0] != "claude" {
		return false
	}
	for _, part := range parts[1:] {
		if part == "fable" {
			return true
		}
	}
	return false
}

func isRecognizedClaudeNonFableModelID(model string) bool {
	parts := strings.Split(model, "-")
	if len(parts) < 4 || parts[0] != "claude" {
		return false
	}
	hasFamily := false
	hasVersion := false
	for _, part := range parts[1:] {
		if part == "" {
			return false
		}
		switch part {
		case "sonnet", "opus", "haiku":
			hasFamily = true
		}
		for _, character := range part {
			if unicode.IsDigit(character) {
				hasVersion = true
			}
			if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
				return false
			}
		}
	}
	return hasFamily && hasVersion
}

func claudeCLIPrimaryIsConclusive(field claudeSettingField, context claudeModelClassificationContext) bool {
	if field.State != claudeFieldKnown || len(field.Values) != 1 {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(field.Values[0]))
	if isClaudeFableModel(model) || isRecognizedClaudeNonFableModelID(model) {
		return true
	}
	return classifyClaudeModelValue(model, context, make(map[string]bool)) == fableApplicable
}

func combineFableApplicability(left, right fableApplicability) fableApplicability {
	if left == fableApplicable || right == fableApplicable {
		return fableApplicable
	}
	if left == fablePossible || right == fablePossible {
		return fablePossible
	}
	return fableNotApplicable
}
