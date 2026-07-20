package multicodex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type claudeAuthStatus struct {
	LoggedIn     bool
	Identity     string
	AuthMethod   string
	APIProvider  string
	Subscription string
}

func fetchClaudeAuthStatus(ctx context.Context, runner claudeCommandRunner, configDir string) (claudeAuthStatus, error) {
	stdout, stderr, err := runner.Capture(ctx, []string{"auth", "status", "--json"}, claudeEnv(os.Environ(), configDir))
	if err != nil {
		return claudeAuthStatus{}, fmt.Errorf("Claude auth status failed: %s", claudeProbeFailure(ctx, err, stderr))
	}
	status, err := parseClaudeAuthStatus(stdout)
	if err != nil {
		return claudeAuthStatus{}, err
	}
	return status, nil
}

func parseClaudeAuthStatus(raw []byte) (claudeAuthStatus, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return claudeAuthStatus{}, fmt.Errorf("parse Claude auth status JSON: %w", err)
	}
	if payload == nil {
		return claudeAuthStatus{}, errors.New("parse Claude auth status JSON: expected an object")
	}
	loggedInRaw, ok := firstClaudeJSONField(payload, "loggedIn", "logged_in")
	if !ok {
		return claudeAuthStatus{}, errors.New("parse Claude auth status JSON: missing loggedIn")
	}
	var loggedIn bool
	if err := json.Unmarshal(loggedInRaw, &loggedIn); err != nil {
		return claudeAuthStatus{}, errors.New("parse Claude auth status JSON: loggedIn must be a boolean")
	}

	status := claudeAuthStatus{
		LoggedIn:     loggedIn,
		Identity:     claudeJSONString(payload, "email", "identity", "accountEmail", "account_email"),
		AuthMethod:   claudeJSONString(payload, "authMethod", "auth_method"),
		APIProvider:  claudeJSONString(payload, "apiProvider", "api_provider"),
		Subscription: claudeJSONString(payload, "subscriptionType", "subscription_type", "subscription"),
	}
	if status.Identity == "" {
		var account map[string]json.RawMessage
		if accountRaw, ok := payload["account"]; ok && json.Unmarshal(accountRaw, &account) == nil {
			status.Identity = claudeJSONString(account, "email", "name", "id")
		}
	}
	return status, nil
}

func firstClaudeJSONField(payload map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, name := range names {
		if raw, ok := payload[name]; ok {
			return raw, true
		}
	}
	return nil, false
}

func claudeJSONString(payload map[string]json.RawMessage, names ...string) string {
	raw, ok := firstClaudeJSONField(payload, names...)
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
