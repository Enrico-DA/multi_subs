package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/buildinfo"
)

const (
	chatGPTOAuthUsageEndpoint = "https://chatgpt.com/backend-api/wham/usage"
)

var errOAuthAuthFileUnavailable = errors.New("oauth auth file unavailable")

type oauthAuthFileUnavailableError struct {
	err error
}

func (e *oauthAuthFileUnavailableError) Error() string {
	return e.err.Error()
}

func (e *oauthAuthFileUnavailableError) Unwrap() error {
	return e.err
}

func (e *oauthAuthFileUnavailableError) Is(target error) bool {
	return target == errOAuthAuthFileUnavailable || errors.Is(e.err, target)
}

type OAuthSource struct {
	httpClient *http.Client
	codexHome  string
}

func NewOAuthSourceForHome(codexHome string) *OAuthSource {
	return &OAuthSource{
		httpClient: &http.Client{Timeout: 8 * time.Second},
		codexHome:  strings.TrimSpace(codexHome),
	}
}

func (s *OAuthSource) Name() string {
	return "oauth"
}

func (s *OAuthSource) Fetch(ctx context.Context) (*Summary, error) {
	token, err := s.accessTokenFromAuthFile()
	if err != nil {
		return nil, err
	}
	req, err := newOAuthUsageRequest(ctx, token)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", clientName+"/"+buildinfo.Version)
	return s.fetchRequest(req)
}

func (s *OAuthSource) accessTokenFromAuthFile() (string, error) {
	authPath, err := findAuthJSONPathForHome(s.codexHome)
	if err != nil {
		return "", &oauthAuthFileUnavailableError{err: err}
	}
	token, err := readAccessToken(authPath)
	if err != nil {
		return "", &oauthAuthFileUnavailableError{err: err}
	}
	return token, nil
}

func (s *OAuthSource) fetchWithAccessToken(ctx context.Context, token string) (*Summary, error) {
	req, err := newOAuthUsageRequest(ctx, token)
	if err != nil {
		return nil, err
	}
	userAgent := clientName + "/" + buildinfo.Version
	req.Header.Set("User-Agent", userAgent)
	return s.fetchRequest(req)
}

func newOAuthUsageRequest(ctx context.Context, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chatGPTOAuthUsageEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build oauth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (s *OAuthSource) fetchRequest(req *http.Request) (*Summary, error) {
	res, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth request failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1_000_000))
	if err != nil {
		return nil, fmt.Errorf("read oauth response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, safeProviderHTTPError("oauth endpoint", res.StatusCode, body)
	}

	var payload oauthUsagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode oauth response: %w", err)
	}
	if payload.RateLimit == nil {
		return nil, errors.New("oauth response missing rate_limit")
	}
	if payload.RateLimit.PrimaryWindow == nil {
		return nil, errors.New("oauth response missing primary_window")
	}

	snapshot := rateLimitSnapshotRaw{
		LimitID:  "codex",
		PlanType: payload.PlanType,
		Primary:  oauthRateLimitWindow(payload.RateLimit, payload.RateLimit.PrimaryWindow),
	}
	if payload.RateLimit.SecondaryWindow != nil {
		snapshot.Secondary = oauthRateLimitWindow(payload.RateLimit, payload.RateLimit.SecondaryWindow)
	}
	rateLimitsByLimitID := map[string]rateLimitSnapshotRaw{
		snapshot.LimitID: snapshot,
	}
	for id, window := range buildRateLimitWindowsFromOAuthAdditionalLimits(payload.AdditionalRateLimits) {
		rateLimitsByLimitID[id] = window
	}

	return normalizeSummary(
		s.Name(),
		snapshot,
		rateLimitsByLimitID,
		len(rateLimitsByLimitID)-1,
		&identityInfo{
			Email:     strings.TrimSpace(payload.Email),
			AccountID: strings.TrimSpace(payload.AccountID),
			UserID:    strings.TrimSpace(payload.UserID),
		},
		nil,
	)
}

func (s *OAuthSource) Close() error {
	return nil
}

type oauthUsagePayload struct {
	Email                string                     `json:"email"`
	AccountID            string                     `json:"account_id"`
	UserID               string                     `json:"user_id"`
	PlanType             string                     `json:"plan_type"`
	RateLimit            *oauthRateLimitDetails     `json:"rate_limit"`
	AdditionalRateLimits []oauthAdditionalRateLimit `json:"additional_rate_limits"`
}

type oauthAdditionalRateLimit struct {
	LimitName string                 `json:"limit_name"`
	RateLimit *oauthRateLimitDetails `json:"rate_limit"`
}

type oauthRateLimitDetails struct {
	Allowed         *bool                `json:"allowed"`
	LimitReached    *bool                `json:"limit_reached"`
	PrimaryWindow   *oauthWindowSnapshot `json:"primary_window"`
	SecondaryWindow *oauthWindowSnapshot `json:"secondary_window"`
}

type oauthWindowSnapshot struct {
	UsedPercent        int `json:"used_percent"`
	LimitWindowSeconds int `json:"limit_window_seconds"`
	ResetAfterSeconds  int `json:"reset_after_seconds"`
	ResetAt            int `json:"reset_at"`
}

type authFilePayload struct {
	Email  string `json:"email"`
	Tokens struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	} `json:"tokens"`
}

func findAuthJSONPathForHome(codexHome string) (string, error) {
	if strings.TrimSpace(codexHome) != "" {
		p := filepath.Join(codexHome, "auth.json")
		ok, err := usableAuthFile(p)
		if err != nil {
			return "", err
		}
		if ok {
			return p, nil
		}
	}

	return "", fmt.Errorf("auth.json not found in %s", filepath.Join(codexHome, "auth.json"))
}

func readAccessToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read auth file: %w", err)
	}

	var payload authFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode auth file: %w", err)
	}
	token := strings.TrimSpace(payload.Tokens.AccessToken)
	if token == "" {
		return "", errors.New("auth.json missing tokens.access_token")
	}
	return token, nil
}

func fileExists(path string) bool {
	ok, err := usableAuthFile(path)
	return err == nil && ok
}

func usableAuthFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("auth.json is a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("auth.json is not a regular file: %s", path)
	}
	if monitorFileHasMultipleLinks(info) {
		return false, fmt.Errorf("auth.json has multiple hard links: %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return false, fmt.Errorf("auth.json permissions are %o, expected 600: %s", info.Mode().Perm(), path)
	}
	return true, nil
}

func toMins(seconds int) *int {
	if seconds <= 0 {
		return nil
	}
	v := seconds / 60
	return &v
}

func toInt64Ptr(v int) *int64 {
	if v <= 0 {
		return nil
	}
	out := int64(v)
	return &out
}

func oauthRateLimitWindow(details *oauthRateLimitDetails, window *oauthWindowSnapshot) *rateLimitWindowRaw {
	if details == nil || window == nil {
		return nil
	}
	usedPercent := window.UsedPercent
	if details.Allowed != nil && !*details.Allowed {
		usedPercent = unavailableUsedPercent
	}
	return &rateLimitWindowRaw{
		UsedPercent:        usedPercent,
		WindowDurationMins: toMins(window.LimitWindowSeconds),
		ResetsAt:           toInt64Ptr(window.ResetAt),
		exhausted:          details.LimitReached != nil && *details.LimitReached,
	}
}

func buildRateLimitWindowsFromOAuthAdditionalLimits(additionalLimits []oauthAdditionalRateLimit) map[string]rateLimitSnapshotRaw {
	if len(additionalLimits) == 0 {
		return nil
	}

	windowByLimit := map[string]rateLimitSnapshotRaw{}
	for i, additional := range additionalLimits {
		if additional.RateLimit == nil || additional.RateLimit.PrimaryWindow == nil {
			continue
		}

		limitName := strings.TrimSpace(additional.LimitName)
		if limitName == "" {
			limitName = "additional-" + strconv.Itoa(i)
		}
		windowByLimit[limitName] = rateLimitSnapshotRaw{
			LimitID:   limitName,
			LimitName: &additional.LimitName,
			Primary:   oauthRateLimitWindow(additional.RateLimit, additional.RateLimit.PrimaryWindow),
		}
		if additional.RateLimit.SecondaryWindow != nil {
			window := windowByLimit[limitName]
			window.Secondary = oauthRateLimitWindow(additional.RateLimit, additional.RateLimit.SecondaryWindow)
			windowByLimit[limitName] = window
		}
	}
	if len(windowByLimit) == 0 {
		return nil
	}
	return windowByLimit
}
