package multicodex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type claudeCommandRunner interface {
	Capture(context.Context, []string, []string) ([]byte, []byte, error)
	Run(context.Context, []string, []string) error
	RunInteractive([]string, []string) error
}

type osClaudeCommandRunner struct{}

func (osClaudeCommandRunner) Capture(ctx context.Context, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (osClaudeCommandRunner) Run(ctx context.Context, args, env []string) error {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (osClaudeCommandRunner) RunInteractive(args, env []string) error {
	if isInteractiveTerminalAttached() {
		path, err := execLookPath("claude")
		if err != nil {
			return fmt.Errorf("find command claude: %w", err)
		}
		return syscallExec(path, append([]string{"claude"}, args...), env)
	}
	return osClaudeCommandRunner{}.Run(context.Background(), args, env)
}

func claudeEnv(base []string, configDir string) []string {
	env := make([]string, 0, len(base)+1)
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok || claudeEnvVarShouldBeStripped(key) {
			continue
		}
		env = append(env, item)
	}
	if configDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+configDir)
	}
	return env
}

func claudeEnvVarShouldBeStripped(key string) bool {
	switch key {
	case "CLAUDE_CONFIG_DIR",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"MULTICODEX_CLAUDE_PROFILE",
		"MULTICODEX_CLAUDE_CONFIG_DIR",
		"MULTICODEX_CLAUDE_TARGET",
		"MULTICODEX_CLAUDE_ACTIVE_PROFILE",
		"MULTICODEX_CLAUDE_SELECTED_PROFILE",
		"MULTICODEX_ACTIVE_CLAUDE_PROFILE",
		"MULTICODEX_SELECTED_CLAUDE_PROFILE",
		"MULTICODEX_ACTIVE_PROVIDER",
		"MULTICODEX_SELECTED_PROVIDER":
		return true
	default:
		return false
	}
}

func claudeChildError(err error, message string) error {
	if err == nil {
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Message == "" {
			return &ExitError{Code: exitErr.Code, Message: message}
		}
		return exitErr
	}
	var processExit *exec.ExitError
	if errors.As(err, &processExit) {
		return &ExitError{Code: processExit.ExitCode(), Message: message}
	}
	return fmt.Errorf("%s: %w", message, err)
}

func claudeProbeFailure(ctx context.Context, err error, stderr []byte) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timed out"
	}
	var processExit *exec.ExitError
	if errors.As(err, &processExit) {
		if detail := safeClaudeDiagnostic(stderr); detail != "" {
			return fmt.Sprintf("exit %d: %s", processExit.ExitCode(), detail)
		}
		return fmt.Sprintf("exit %d", processExit.ExitCode())
	}
	if detail := safeClaudeDiagnostic(stderr); detail != "" {
		return detail
	}
	if err == nil {
		return "unknown failure"
	}
	return err.Error()
}

func safeClaudeDiagnostic(raw []byte) string {
	detail := firstLineOrDash(strings.TrimSpace(string(raw)))
	if detail == "-" {
		return ""
	}
	return truncate(detail, 160)
}
