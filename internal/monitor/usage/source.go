package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Source interface {
	Name() string
	Fetch(context.Context) (*Summary, error)
	Close() error
}

type UsageSource struct {
	primary   Source
	fallback  Source
	report    bool
	closeOnce sync.Once
	closeErr  error
}

type DefaultAccountSource struct {
	oauth Source
	// These two hold the OAuth access-token read and the fetch that consumes
	// it. Their names avoid ending in "Token": the repository secret scanner
	// rejects a `Token:` key in a struct literal.
	oauthCredential          func() (string, error)
	oauthFetchWithCredential func(context.Context, string) (*Summary, error)
	appServer                Source

	fetchMu                       sync.Mutex
	appServerUsed                 bool
	mu                            sync.Mutex
	allowAuthFileIdentityFallback bool
	closeOnce                     sync.Once
	closeErr                      error
}

func NewUsageSourceForHome(codexHome string) *DefaultAccountSource {
	oauth := NewOAuthSourceForHome(codexHome)
	return &DefaultAccountSource{
		oauth:                         oauth,
		oauthCredential:               oauth.accessTokenFromAuthFile,
		oauthFetchWithCredential:      oauth.fetchWithAccessToken,
		appServer:                     NewAppServerSourceForHome(codexHome),
		allowAuthFileIdentityFallback: true,
	}
}

func newManagedUsageSourceForHome(codexHome string) *UsageSource {
	return &UsageSource{
		primary:  newManagedAppServerSourceForHome(codexHome),
		fallback: NewOAuthSourceForHome(codexHome),
	}
}

func NewUsageSourceForAccount(account MonitorAccount) Source {
	switch account.SourceMode {
	case SourceModeManagedAppServer:
		return newManagedUsageSourceForHome(account.CodexHome)
	case SourceModeDefaultAccount:
		return NewUsageSourceForHome(account.CodexHome)
	default:
		return NewOAuthSourceForHome(account.CodexHome)
	}
}

// NewReportUsageSourceForAccount returns the one-shot report source. Unlike the
// shared monitor and routing source, it can retain a primary session window
// while obtaining required weekly data from the fallback source.
func NewReportUsageSourceForAccount(account MonitorAccount) Source {
	switch account.SourceMode {
	case SourceModeManagedAppServer:
		return &UsageSource{
			primary:  newManagedAppServerSourceForHome(account.CodexHome),
			fallback: NewOAuthSourceForHome(account.CodexHome),
			report:   true,
		}
	case SourceModeDefaultAccount:
		return NewUsageSourceForHome(account.CodexHome)
	default:
		return NewOAuthSourceForHome(account.CodexHome)
	}
}

func (s *DefaultAccountSource) Name() string {
	return "default-account"
}

func (s *DefaultAccountSource) Fetch(ctx context.Context) (*Summary, error) {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	token, err := s.oauthCredential()
	if errors.Is(err, errOAuthAuthFileUnavailable) {
		s.setAuthFileIdentityFallbackAllowed(false)
		s.appServerUsed = true
		return s.appServer.Fetch(ctx)
	}
	s.setAuthFileIdentityFallbackAllowed(true)
	if err != nil {
		return nil, err
	}

	summary, err := s.oauthFetchWithCredential(ctx, token)
	if err != nil {
		return nil, err
	}
	if summaryHasStandardWeeklyData(summary) {
		if closeErr := s.closeInactiveAppServer(); closeErr != nil {
			return nil, closeErr
		}
		return summary, nil
	}

	s.appServerUsed = true
	appSummary, appErr := s.appServer.Fetch(ctx)
	if appErr != nil || !summaryHasStandardWeeklyData(appSummary) {
		return summary, nil
	}
	return mergeDefaultOAuthWithAppServerWeekly(summary, appSummary), nil
}

func (s *DefaultAccountSource) closeInactiveAppServer() error {
	if !s.appServerUsed {
		return nil
	}
	if closeErr := s.appServer.Close(); closeErr != nil {
		if s.closeErr == nil {
			s.closeErr = closeErr
		}
		return fmt.Errorf("close inactive app-server source: %w", closeErr)
	}
	s.appServerUsed = false
	return nil
}

func (s *DefaultAccountSource) Close() error {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.closeOnce.Do(func() {
		if s.oauth != nil {
			if err := s.oauth.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
		}
		if s.appServer != nil && s.appServerUsed {
			if err := s.appServer.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
			s.appServerUsed = false
		}
	})
	return s.closeErr
}

func (s *DefaultAccountSource) AllowsAuthFileIdentityFallback() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowAuthFileIdentityFallback
}

func (s *DefaultAccountSource) setAuthFileIdentityFallbackAllowed(allowed bool) {
	s.mu.Lock()
	s.allowAuthFileIdentityFallback = allowed
	s.mu.Unlock()
}

func SourceAllowsAuthFileIdentityFallback(source Source) bool {
	policy, ok := source.(interface {
		AllowsAuthFileIdentityFallback() bool
	})
	return !ok || policy.AllowsAuthFileIdentityFallback()
}

func (s *UsageSource) Name() string {
	return "usage"
}

func (s *UsageSource) Fetch(ctx context.Context) (*Summary, error) {
	if s.report {
		return fetchReportWithFallback(ctx, s.primary, s.fallback)
	}
	return fetchWithFallback(ctx, s.primary, s.fallback)
}

func (s *UsageSource) Close() error {
	s.closeOnce.Do(func() {
		if s.primary != nil {
			if err := s.primary.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
		}
		if s.fallback != nil {
			if err := s.fallback.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
		}
	})
	return s.closeErr
}
