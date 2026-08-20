package multisubs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	installModulePath      = "github.com/Enrico-DA/multi_subs/cmd/multisubs"
	installPrivateModule   = "github.com/Enrico-DA/multi_subs"
	installShellBlockBegin = "# BEGIN MULTISUBS INSTALL PATH"
	installShellBlockEnd   = "# END MULTISUBS INSTALL PATH"
	installUsage           = "usage: multisubs install [ref]"
)

var (
	installExecutablePath = currentExecutablePath
	installRunGo          = runGoModuleInstall
)

func (a *App) cmdInstall(args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return printCommandHelp([]string{"install"})
	}
	ref, err := parseInstallRef(args)
	if err != nil {
		return err
	}
	running := installExecutablePath()
	if running == "" || !filepath.IsAbs(running) || !isMultisubsExecutableName(filepath.Base(running)) {
		return &ExitError{Code: 1, Message: "could not determine the running multisubs binary"}
	}
	gobin := filepath.Dir(running)
	if _, err := quoteInstallShellValue(gobin); err != nil {
		return err
	}

	if err := installRunGo(gobin, ref); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	rcPath, fish := defaultInstallShellRC(home, os.Getenv("SHELL"))
	wroteRC, err := persistInstallShellBlock(rcPath, gobin, fish)
	if err != nil {
		return err
	}

	if err := a.store.EnsureBaseDirs(); err != nil {
		return err
	}
	if err := writeInstallEnvFile(a.store.paths.MultisubsHome, gobin); err != nil {
		return err
	}

	removed, err := removeInstallLeftovers(running, os.Getenv("GOBIN"), os.Getenv("GOPATH"), home)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "installed %s@%s to %s\n", installModulePath, ref, running)
	if wroteRC {
		fmt.Fprintf(os.Stdout, "set GOBIN in %s\n", rcPath)
	} else {
		fmt.Fprintf(os.Stdout, "GOBIN already set in %s\n", rcPath)
	}
	for _, path := range removed {
		fmt.Fprintf(os.Stdout, "removed leftover binary %s\n", path)
	}
	return nil
}

func parseInstallRef(args []string) (string, error) {
	if len(args) == 0 {
		return "latest", nil
	}
	if len(args) != 1 {
		return "", &ExitError{Code: 2, Message: installUsage}
	}
	ref := strings.TrimSpace(args[0])
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", &ExitError{Code: 2, Message: installUsage}
	}
	if strings.ContainsAny(ref, "/@\\") {
		return "", &ExitError{Code: 2, Message: "install ref must be latest, a release tag, or a commit hash; branch names that contain / are not valid"}
	}
	return ref, nil
}

func writeInstallEnvFile(multisubsHome, gobin string) error {
	path := filepath.Join(multisubsHome, "install.env")
	body := "GOPRIVATE=" + installPrivateModule + "\nGOBIN=" + gobin + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write install env: %w", err)
	}
	return nil
}

func defaultInstallShellRC(home, shell string) (string, bool) {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(shell)))
	switch {
	case strings.Contains(base, "fish"):
		return filepath.Join(home, ".config", "fish", "config.fish"), true
	case strings.Contains(base, "bash"):
		bashrc := filepath.Join(home, ".bashrc")
		if regularFileExists(bashrc) {
			return bashrc, false
		}
		return filepath.Join(home, ".bash_profile"), false
	default:
		return filepath.Join(home, ".zshrc"), false
	}
}

func persistInstallShellBlock(rcPath, gobin string, fish bool) (bool, error) {
	block, err := installShellBlock(gobin, fish)
	if err != nil {
		return false, err
	}
	current, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read shell rc: %w", err)
	}
	var next string
	changed := false
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(rcPath), 0o700); err != nil {
			return false, fmt.Errorf("create shell rc directory: %w", err)
		}
		next = block + "\n"
		changed = true
	} else {
		text := string(current)
		updated, ok, replaceErr := replaceInstallShellBlock(text, block)
		if replaceErr != nil {
			return false, replaceErr
		}
		if ok {
			next = updated
			changed = text != updated
		} else {
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			next = text + "\n" + block + "\n"
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(rcPath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(rcPath, []byte(next), mode); err != nil {
		return false, fmt.Errorf("write shell rc: %w", err)
	}
	return true, nil
}

func installShellBlock(gobin string, fish bool) (string, error) {
	private, err := quoteInstallShellValue(installPrivateModule)
	if err != nil {
		return "", err
	}
	quotedBin, err := quoteInstallShellValue(gobin)
	if err != nil {
		return "", err
	}
	var body string
	if fish {
		body = "set -gx GOPRIVATE " + private + "\nset -gx GOBIN " + quotedBin
	} else {
		body = "export GOPRIVATE=" + private + "\nexport GOBIN=" + quotedBin
	}
	return installShellBlockBegin + "\n" + body + "\n" + installShellBlockEnd, nil
}

func quoteInstallShellValue(value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\n\r'") {
		return "", &ExitError{Code: 1, Message: "install path is not safe to write to the shell profile"}
	}
	return "'" + value + "'", nil
}

func replaceInstallShellBlock(text, block string) (string, bool, error) {
	beginCount := strings.Count(text, installShellBlockBegin)
	endCount := strings.Count(text, installShellBlockEnd)
	if beginCount == 0 && endCount == 0 {
		return text, false, nil
	}
	if beginCount != 1 || endCount != 1 {
		return "", false, &ExitError{Code: 1, Message: "shell rc has a broken multisubs install block"}
	}
	begin := strings.Index(text, installShellBlockBegin)
	end := strings.Index(text[begin:], installShellBlockEnd)
	if end < 0 {
		return "", false, &ExitError{Code: 1, Message: "shell rc has a broken multisubs install block"}
	}
	end = begin + end + len(installShellBlockEnd)
	for end < len(text) && (text[end] == '\n' || text[end] == '\r') {
		end++
	}
	updated := text[:begin] + block + "\n" + text[end:]
	return updated, true, nil
}

func removeInstallLeftovers(running, goBin, goPath, home string) ([]string, error) {
	leftovers := uniqueExistingLeftovers(
		regularFileExists,
		running,
		goInstallBinaryName(),
		goInstallBinDir(goBin, goPath, home),
		goInstallBinDir("", goPath, home),
	)
	removed := make([]string, 0, len(leftovers))
	for _, path := range leftovers {
		if err := os.Remove(path); err != nil {
			return removed, fmt.Errorf("remove leftover binary: %w", err)
		}
		removed = append(removed, path)
	}
	return removed, nil
}

func runGoModuleInstall(gobin, ref string) error {
	command := exec.Command("go", "install", installModulePath+"@"+ref)
	command.Env = installCommandEnv(os.Environ(), gobin)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return &ExitError{Code: 1, Message: "go install failed"}
	}
	return nil
}

func installCommandEnv(env []string, gobin string) []string {
	out := make([]string, 0, len(env)+2)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch name {
		case "GOBIN", "GOPRIVATE":
			continue
		}
		out = append(out, entry)
	}
	out = append(out, "GOPRIVATE="+installPrivateModule)
	out = append(out, "GOBIN="+gobin)
	return out
}
