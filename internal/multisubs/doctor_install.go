package multisubs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type goInstallTargetInput struct {
	runningPath       string
	pathLookup        string
	goBin             string
	goPath            string
	home              string
	installFileExists func(string) bool
}

func checkGoInstallTarget() DoctorCheck {
	home, _ := os.UserHomeDir()
	return goInstallTargetCheck(goInstallTargetInput{
		runningPath:       currentExecutablePath(),
		pathLookup:        lookPathResolved("multisubs"),
		goBin:             os.Getenv("GOBIN"),
		goPath:            os.Getenv("GOPATH"),
		home:              home,
		installFileExists: regularFileExists,
	})
}

func goInstallTargetCheck(in goInstallTargetInput) DoctorCheck {
	const name = "go install target"
	running := cleanResolvedPath(in.runningPath)
	if running == "" {
		return DoctorCheck{Name: name, Status: "ok", Details: "running binary path was not available"}
	}
	if !isMultisubsExecutableName(filepath.Base(running)) {
		return DoctorCheck{Name: name, Status: "ok", Details: "skipped; this process is not the multisubs executable"}
	}

	exists := in.installFileExists
	if exists == nil {
		exists = regularFileExists
	}
	runningDir := filepath.Dir(running)
	installDir := goInstallBinDir(in.goBin, in.goPath, in.home)
	defaultDir := goInstallBinDir("", in.goPath, in.home)
	binaryName := goInstallBinaryName()

	var parts []string
	status := "ok"
	if installDir == "" {
		return DoctorCheck{Name: name, Status: "warn", Details: "could not determine the go install directory"}
	}
	if sameResolvedPath(installDir, runningDir) {
		parts = append(parts, "go install replaces the running binary in "+runningDir)
	} else {
		status = "warn"
		parts = append(parts, "go install writes to "+installDir+"; the running binary is "+running+". set GOBIN to "+runningDir+" before the next install")
	}

	leftovers := uniqueExistingLeftovers(exists, running, binaryName, installDir, defaultDir)
	if len(leftovers) > 0 {
		status = "warn"
		parts = append(parts, "a second binary is at "+strings.Join(leftovers, " and ")+"; remove it so only the running copy remains")
	}

	pathLookup := cleanResolvedPath(in.pathLookup)
	if pathLookup != "" && !sameResolvedPath(pathLookup, running) {
		status = "warn"
		parts = append(parts, "PATH finds "+pathLookup+", which is not this process")
	}

	return DoctorCheck{Name: name, Status: status, Details: strings.Join(parts, ". ")}
}

func uniqueExistingLeftovers(exists func(string) bool, running, binaryName string, dirs ...string) []string {
	seen := make(map[string]struct{})
	var leftovers []string
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, binaryName)
		if !exists(candidate) {
			continue
		}
		resolved := cleanResolvedPath(candidate)
		if resolved == "" {
			resolved = filepath.Clean(candidate)
		}
		if sameResolvedPath(resolved, running) {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		leftovers = append(leftovers, resolved)
	}
	return leftovers
}

func goInstallBinDir(gobin, gopath, home string) string {
	if dir := strings.TrimSpace(gobin); dir != "" {
		return filepath.Clean(dir)
	}
	gopath = strings.TrimSpace(gopath)
	if gopath != "" {
		first := gopath
		if index := strings.IndexByte(gopath, os.PathListSeparator); index >= 0 {
			first = strings.TrimSpace(gopath[:index])
		}
		if first != "" {
			return filepath.Join(filepath.Clean(first), "bin")
		}
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(filepath.Clean(home), "go", "bin")
}

func goInstallBinaryName() string {
	if runtime.GOOS == "windows" {
		return "multisubs.exe"
	}
	return "multisubs"
}

func isMultisubsExecutableName(base string) bool {
	switch strings.TrimSpace(base) {
	case "multisubs", "multisubs.exe":
		return true
	default:
		return false
	}
}

func lookPathResolved(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return cleanResolvedPath(path)
}

func cleanResolvedPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if trimmed := strings.TrimSpace(resolved); trimmed != "" {
			path = trimmed
		}
	}
	return filepath.Clean(path)
}

func sameResolvedPath(a, b string) bool {
	left := cleanResolvedPath(a)
	right := cleanResolvedPath(b)
	if left == "" || right == "" {
		return false
	}
	return left == right
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
