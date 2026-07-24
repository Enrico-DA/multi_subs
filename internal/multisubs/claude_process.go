package multisubs

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
	RunWithReservation(context.Context, []string, []string, *os.File) error
	RunInteractive([]string, []string) error
}

type osClaudeCommandRunner struct{}

func (osClaudeCommandRunner) Capture(ctx context.Context, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	cmd.Dir = "/"
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (osClaudeCommandRunner) Run(ctx context.Context, args, env []string) error {
	return runClaudeCommand(ctx, args, env, nil)
}

func (osClaudeCommandRunner) RunWithReservation(ctx context.Context, args, env []string, reservation *os.File) error {
	return runClaudeCommand(ctx, args, env, reservation)
}

func runClaudeCommand(ctx context.Context, args, env []string, reservation *os.File) error {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if reservation != nil {
		cmd.ExtraFiles = []*os.File{reservation}
	}
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

var claudeDeniedEnvPrefixes = []string{
	"CLAUDE_CODE_OAUTH_",
	"CLAUDE_CODE_SKIP_",
}

var claudeDeniedEnvKeys = map[string]struct{}{
	"CLAUDE_CONFIG_DIR":                          {},
	"ANTHROPIC_API_KEY":                          {},
	"ANTHROPIC_AUTH_TOKEN":                       {},
	"ANTHROPIC_BASE_URL":                         {},
	"ANTHROPIC_CONFIG_DIR":                       {},
	"ANTHROPIC_PROFILE":                          {},
	"ANTHROPIC_ORGANIZATION_ID":                  {},
	"ANTHROPIC_WORKSPACE_ID":                     {},
	"ANTHROPIC_SERVICE_ACCOUNT_ID":               {},
	"ANTHROPIC_FEDERATION_RULE_ID":               {},
	"ANTHROPIC_ENVIRONMENT_KEY":                  {},
	"ANTHROPIC_CUSTOM_HEADERS":                   {},
	"ANTHROPIC_AWS_API_KEY":                      {},
	"ANTHROPIC_AWS_BASE_URL":                     {},
	"ANTHROPIC_BEDROCK_BASE_URL":                 {},
	"ANTHROPIC_BEDROCK_MANTLE_BASE_URL":          {},
	"ANTHROPIC_FOUNDRY_API_KEY":                  {},
	"ANTHROPIC_FOUNDRY_AUTH_TOKEN":               {},
	"ANTHROPIC_FOUNDRY_BASE_URL":                 {},
	"ANTHROPIC_GOOGLE_CLOUD_BASE_URL":            {},
	"ANTHROPIC_IDENTITY_TOKEN":                   {},
	"ANTHROPIC_IDENTITY_TOKEN_FILE":              {},
	"ANTHROPIC_VERTEX_BASE_URL":                  {},
	"ANTHROPIC_VERTEX_BASE_URL6":                 {},
	"CLAUDE_CODE_API_BASE_URL":                   {},
	"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR":        {},
	"CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL":    {},
	"CLAUDE_CODE_CLIENT_KEY":                     {},
	"CLAUDE_CODE_CLIENT_KEY_PASSPHRASE":          {},
	"CLAUDE_CODE_CUSTOM_OAUTH_URL":               {},
	"CLAUDE_CODE_DESIGN_OAUTH_CLIENT_ID":         {},
	"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": {},
	"CLAUDE_CODE_ENABLE_PROXY_AUTH_HELPER":       {},
	"CLAUDE_CODE_HOST_AUTH_ENV_VAR":              {},
	"CLAUDE_CODE_HOST_CREDS_FILE":                {},
	"CLAUDE_CODE_HFI_BEARER_TOKEN":               {},
	"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST":       {},
	"CLAUDE_CODE_PROXY_AUTHENTICATE":             {},
	"CLAUDE_CODE_SDK_HAS_HOST_AUTH_REFRESH":      {},
	"CLAUDE_CODE_SDK_HAS_OAUTH_REFRESH":          {},
	"CLAUDE_CODE_SESSION_ACCESS_TOKEN":           {},
	"CLAUDE_CODE_USE_BEDROCK":                    {},
	"CLAUDE_CODE_USE_VERTEX":                     {},
	"CLAUDE_CODE_USE_FOUNDRY":                    {},
	"CLAUDE_CODE_USE_ANTHROPIC_AWS":              {},
	"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD":     {},
	"CLAUDE_CODE_USE_GATEWAY":                    {},
	"CLAUDE_CODE_USE_MANTLE":                     {},
	"CLAUDE_CODE_WEBSOCKET_AUTH_FILE_DESCRIPTOR": {},
	"CLAUDE_SECURESTORAGE_CONFIG_DIR":            {},
	"CLAUDE_SESSION_INGRESS_TOKEN_FILE":          {},
	"CLAUDE_TRUSTED_DEVICE_TOKEN":                {},
	"CODEX_USAGE_MONITOR_ACCOUNTS_FILE":          {},
}

func claudeEnvVarShouldBeStripped(key string) bool {
	if strings.HasPrefix(key, "MULTISUBS_") || strings.HasPrefix(key, "MULTICODEX_") {
		return true
	}
	if _, denied := claudeDeniedEnvKeys[key]; denied {
		return true
	}
	for _, prefix := range claudeDeniedEnvPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
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

func claudeProbeFailure(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timed out"
	}
	var processExit *exec.ExitError
	if errors.As(err, &processExit) {
		return fmt.Sprintf("exit %d", processExit.ExitCode())
	}
	var execError *exec.Error
	var pathError *os.PathError
	if errors.As(err, &execError) || errors.As(err, &pathError) {
		return "launch failure"
	}
	return "unknown failure"
}
