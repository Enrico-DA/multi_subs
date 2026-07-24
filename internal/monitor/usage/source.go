package usage

import (
	"context"
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

func NewUsageSourceForHome(codexHome string) *UsageSource {
	return &UsageSource{
		primary:  NewAppServerSourceForHome(codexHome),
		fallback: NewOAuthSourceForHome(codexHome),
	}
}

func newManagedUsageSourceForHome(codexHome string) *UsageSource {
	return &UsageSource{
		primary:  newManagedAppServerSourceForHome(codexHome),
		fallback: NewOAuthSourceForHome(codexHome),
	}
}

func NewUsageSourceForAccount(account MonitorAccount) Source {
	if account.UseAppServer {
		return newManagedUsageSourceForHome(account.CodexHome)
	}
	return NewOAuthSourceForHome(account.CodexHome)
}

// NewReportUsageSourceForAccount returns the one-shot report source. Unlike the
// shared monitor and routing source, it can retain a primary session window
// while obtaining required weekly data from the fallback source.
func NewReportUsageSourceForAccount(account MonitorAccount) Source {
	if !account.UseAppServer {
		return NewOAuthSourceForHome(account.CodexHome)
	}
	return &UsageSource{
		primary:  newManagedAppServerSourceForHome(account.CodexHome),
		fallback: NewOAuthSourceForHome(account.CodexHome),
		report:   true,
	}
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
