package productidentity

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	expectedModule     = "github.com/Enrico-DA/multi_subs"
	expectedCommandDir = "cmd/multisubs"
)

type repository struct {
	root         string
	trackedPaths []string
	tracked      map[string]struct{}
	files        map[string][]byte
	readFailed   map[string]bool
	errors       []string
}

func trackedPathsFromGit(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "-z")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("enumerate tracked paths with git ls-files -z: %w", err)
	}
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, fmt.Errorf("enumerate tracked paths with git ls-files -z: output is not NUL-terminated")
	}

	parts := bytes.Split(output[:len(output)-1], []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		paths = append(paths, string(part))
	}
	return paths, nil
}

func validateRepository(root string, trackedPaths []string) []string {
	repo := &repository{
		root:       root,
		tracked:    make(map[string]struct{}, len(trackedPaths)),
		files:      make(map[string][]byte),
		readFailed: make(map[string]bool),
	}
	for _, trackedPath := range trackedPaths {
		repo.addTrackedPath(trackedPath)
	}

	repo.checkTrackedPathNames()
	repo.checkCommandDirectory()
	repo.checkModule()
	repo.checkMainCommand()
	repo.checkApplicationName()
	repo.checkMonitorIdentity()
	repo.checkOAuthUserAgent()
	repo.checkTUIIdentity()
	repo.checkReleaseWorkflow()
	repo.checkForbiddenLegacyEnvironmentReads()
	repo.checkLegacyOccurrences()
	return repo.errors
}

func (repo *repository) addTrackedPath(trackedPath string) {
	if trackedPath == "" {
		repo.errors = append(repo.errors, "tracked path manifest contains an empty path")
		return
	}
	if filepath.IsAbs(trackedPath) || path.Clean(trackedPath) != trackedPath || trackedPath == "." || strings.HasPrefix(trackedPath, "../") {
		repo.errors = append(repo.errors, fmt.Sprintf("%q: tracked path must be a clean repository-relative path", trackedPath))
		return
	}
	if _, exists := repo.tracked[trackedPath]; exists {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: tracked path is listed more than once", trackedPath))
		return
	}
	repo.tracked[trackedPath] = struct{}{}
	repo.trackedPaths = append(repo.trackedPaths, trackedPath)
}

func (repo *repository) readFile(relativePath string) ([]byte, bool) {
	if data, ok := repo.files[relativePath]; ok {
		return data, true
	}
	if repo.readFailed[relativePath] {
		return nil, false
	}
	if _, ok := repo.tracked[relativePath]; !ok {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: required identity source is not Git-tracked", relativePath))
		repo.readFailed[relativePath] = true
		return nil, false
	}

	data, err := os.ReadFile(filepath.Join(repo.root, filepath.FromSlash(relativePath)))
	if err != nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: cannot read tracked identity source: %v", relativePath, err))
		repo.readFailed[relativePath] = true
		return nil, false
	}
	repo.files[relativePath] = data
	return data, true
}

func (repo *repository) parseGoFile(relativePath string) (*ast.File, bool) {
	source, ok := repo.readFile(relativePath)
	if !ok {
		return nil, false
	}
	file, err := parser.ParseFile(token.NewFileSet(), relativePath, source, 0)
	if err != nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: cannot parse active Go source: %v", relativePath, err))
		return nil, false
	}
	return file, true
}

func (repo *repository) checkTrackedPathNames() {
	legacyName := strings.ToLower(legacyProduct)
	for _, trackedPath := range repo.trackedPaths {
		if strings.Contains(strings.ToLower(trackedPath), legacyName) {
			repo.errors = append(repo.errors, fmt.Sprintf("%s: legacy product text is prohibited in Git-tracked paths", trackedPath))
		}
	}
}

func (repo *repository) checkCommandDirectory() {
	commandDirectories := make(map[string]struct{})
	for _, trackedPath := range repo.trackedPaths {
		if !strings.HasPrefix(trackedPath, "cmd/") {
			continue
		}
		remainder := strings.TrimPrefix(trackedPath, "cmd/")
		directory, _, _ := strings.Cut(remainder, "/")
		if directory != "" {
			commandDirectories[directory] = struct{}{}
		}
	}
	if len(commandDirectories) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("cmd: tracked paths must contain only %s; found %s", expectedCommandDir, joinedMapKeys(commandDirectories)))
		return
	}
	if _, ok := commandDirectories["multisubs"]; !ok {
		repo.errors = append(repo.errors, fmt.Sprintf("cmd: tracked paths must contain only %s; found %s", expectedCommandDir, joinedMapKeys(commandDirectories)))
	}
}

func joinedMapKeys(values map[string]struct{}) string {
	if len(values) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func (repo *repository) checkModule() {
	const relativePath = "go.mod"
	source, ok := repo.readFile(relativePath)
	if !ok {
		return
	}
	lines := strings.Split(string(source), "\n")
	expected := "module " + expectedModule
	if len(lines) == 0 || lines[0] != expected {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: module declaration must be the exact first line %q", relativePath, expected))
	}
	count := 0
	for _, line := range lines {
		if line == expected {
			count++
		}
	}
	if count != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: module declaration must occur exactly once; found %d", relativePath, count))
	}
}

func (repo *repository) checkMainCommand() {
	const relativePath = "cmd/multisubs/main.go"
	file, ok := repo.parseGoFile(relativePath)
	if !ok {
		return
	}
	importName, ok := importedPackageName(file, expectedModule+"/internal/multisubs")
	if !ok {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: command must import %s/internal/multisubs", relativePath, expectedModule))
		return
	}
	if importName == "." || importName == "_" {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: multisubs command import must have an addressable package name", relativePath))
		return
	}

	mainFunction := findFunction(file, "main")
	if mainFunction == nil || mainFunction.Body == nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: command must define main", relativePath))
		return
	}
	foundRunCLI := false
	ast.Inspect(mainFunction.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "RunCLI" {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if ok && packageName.Name == importName {
			foundRunCLI = true
		}
		return true
	})
	if !foundRunCLI {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: main must call RunCLI from the imported internal/multisubs package", relativePath))
	}
}

func (repo *repository) checkApplicationName() {
	const relativePath = "internal/multisubs/version.go"
	file, ok := repo.parseGoFile(relativePath)
	if !ok {
		return
	}
	values := topLevelConstantValues(file, "appName")
	if len(values) != 1 || stringConstant(values[0]) != "multisubs" {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active appName constant must be exactly %q", relativePath, "multisubs"))
	}
}

func (repo *repository) checkMonitorIdentity() {
	const relativePath = "internal/monitor/usage/appserver.go"
	file, ok := repo.parseGoFile(relativePath)
	if !ok {
		return
	}
	values := topLevelConstantValues(file, "clientName")
	if len(values) != 1 || stringConstant(values[0]) != "multisubs-monitor" {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active clientName constant must be exactly %q", relativePath, "multisubs-monitor"))
	}

	buildinfoName, imported := importedPackageName(file, expectedModule+"/internal/buildinfo")
	if !imported || buildinfoName == "." || buildinfoName == "_" {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: monitor must import %s/internal/buildinfo", relativePath, expectedModule))
		return
	}
	initialize := findFunction(file, "ensureInitialized")
	if initialize == nil || initialize.Body == nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: monitor must define ensureInitialized", relativePath))
		return
	}

	foundClientInfo := false
	ast.Inspect(initialize.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isIdentifier(literal.Type, "clientInfo") {
			return true
		}
		nameValue, hasName := keyedValue(literal, "Name")
		versionValue, hasVersion := keyedValue(literal, "Version")
		if hasName && hasVersion &&
			isIdentifier(nameValue, "clientName") &&
			isSelector(versionValue, buildinfoName, "Version") {
			foundClientInfo = true
		}
		return true
	})
	if !foundClientInfo {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: ensureInitialized must construct clientInfo{Name: clientName, Version: buildinfo.Version}", relativePath))
	}
}

func (repo *repository) checkOAuthUserAgent() {
	const relativePath = "internal/monitor/usage/oauth.go"
	file, ok := repo.parseGoFile(relativePath)
	if !ok {
		return
	}
	buildinfoName, imported := importedPackageName(file, expectedModule+"/internal/buildinfo")
	if !imported || buildinfoName == "." || buildinfoName == "_" {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: OAuth monitor must import %s/internal/buildinfo", relativePath, expectedModule))
		return
	}
	fetch := findFunction(file, "Fetch")
	if fetch == nil || fetch.Body == nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: OAuth monitor must define Fetch", relativePath))
		return
	}

	foundUserAgent := false
	ast.Inspect(fetch.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 || selectorChain(call.Fun) != "req.Header.Set" {
			return true
		}
		if stringConstant(call.Args[0]) == "User-Agent" && isClientVersionExpression(call.Args[1], buildinfoName) {
			foundUserAgent = true
		}
		return true
	})
	if !foundUserAgent {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: OAuth User-Agent must actively use clientName+%q+buildinfo.Version", relativePath, "/"))
	}
}

func (repo *repository) checkTUIIdentity() {
	const relativePath = "internal/monitor/tui/model.go"
	file, ok := repo.parseGoFile(relativePath)
	if !ok {
		return
	}
	renderHeader := findFunction(file, "renderHeader")
	if renderHeader == nil || renderHeader.Body == nil {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: TUI must define renderHeader", relativePath))
		return
	}

	foundTitle := false
	foundCompactName := false
	ast.Inspect(renderHeader.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		switch selectorChain(call.Fun) {
		case "m.styles.title.Render":
			foundTitle = stringConstant(call.Args[0]) == " multisubs codex monitor " || foundTitle
		case "m.styles.accent.Render":
			foundCompactName = stringConstant(call.Args[0]) == "multisubs" || foundCompactName
		}
		return true
	})
	if !foundTitle {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active monitor TUI title is incorrect", relativePath))
	}
	if !foundCompactName {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active compact monitor TUI product name is incorrect", relativePath))
	}
}

func (repo *repository) checkForbiddenLegacyEnvironmentReads() {
	for _, relativePath := range repo.trackedPaths {
		if !strings.HasSuffix(relativePath, ".go") || strings.HasSuffix(relativePath, "_test.go") {
			continue
		}
		file, ok := repo.parseGoFile(relativePath)
		if !ok {
			continue
		}
		osName, imported := importedPackageName(file, "os")
		if !imported || osName == "." || osName == "_" {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			functionName := ""
			switch {
			case isSelector(call.Fun, osName, "Getenv"):
				functionName = "Getenv"
			case isSelector(call.Fun, osName, "LookupEnv"):
				functionName = "LookupEnv"
			default:
				return true
			}
			value, constant := constantStringExpression(call.Args[0])
			if constant && strings.HasPrefix(value, legacyEnvironmentPrefix+"_") {
				repo.errors = append(repo.errors, fmt.Sprintf("%s: active os.%s read of the legacy environment namespace is prohibited", relativePath, functionName))
			}
			return true
		})
	}
}

func importedPackageName(file *ast.File, importPath string) (string, bool) {
	foundName := ""
	found := false
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil || value != importPath {
			continue
		}
		if found {
			return "", false
		}
		found = true
		if spec.Name != nil {
			foundName = spec.Name.Name
		} else {
			foundName = path.Base(importPath)
		}
	}
	return foundName, found
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}

func topLevelConstantValues(file *ast.File, name string) []ast.Expr {
	var values []ast.Expr
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, rawSpec := range general.Specs {
			spec, ok := rawSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, constantName := range spec.Names {
				if constantName.Name != name {
					continue
				}
				if index < len(spec.Values) {
					values = append(values, spec.Values[index])
				} else {
					values = append(values, nil)
				}
			}
		}
	}
	return values
}

func keyedValue(literal *ast.CompositeLit, key string) (ast.Expr, bool) {
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok || !isIdentifier(keyValue.Key, key) {
			continue
		}
		return keyValue.Value, true
	}
	return nil, false
}

func isIdentifier(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func isSelector(expression ast.Expr, packageName, memberName string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != memberName {
		return false
	}
	return isIdentifier(selector.X, packageName)
}

func selectorChain(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		parent := selectorChain(value.X)
		if parent == "" {
			return ""
		}
		return parent + "." + value.Sel.Name
	default:
		return ""
	}
}

func isClientVersionExpression(expression ast.Expr, buildinfoName string) bool {
	outer, ok := expression.(*ast.BinaryExpr)
	if !ok || outer.Op != token.ADD || !isSelector(outer.Y, buildinfoName, "Version") {
		return false
	}
	inner, ok := outer.X.(*ast.BinaryExpr)
	return ok &&
		inner.Op == token.ADD &&
		isIdentifier(inner.X, "clientName") &&
		stringConstant(inner.Y) == "/"
}

func stringConstant(expression ast.Expr) string {
	value, ok := constantStringExpression(expression)
	if !ok {
		return ""
	}
	return value
}

func constantStringExpression(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(value.Value)
		return unquoted, err == nil
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantStringExpression(value.X)
		right, rightOK := constantStringExpression(value.Y)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + right, true
	case *ast.ParenExpr:
		return constantStringExpression(value.X)
	default:
		return "", false
	}
}
