package usage

import "testing"

func TestUsageSourceForManagedAccountUsesManagedAppServerWithOAuthFallback(t *testing.T) {
	t.Parallel()

	source := NewUsageSourceForAccount(MonitorAccount{
		Label:        "managed",
		CodexHome:    "/managed",
		UseAppServer: true,
	})
	usageSource, ok := source.(*UsageSource)
	if !ok {
		t.Fatalf("managed account source type: got %T want *UsageSource", source)
	}
	appServer, ok := usageSource.primary.(*AppServerSource)
	if !ok {
		t.Fatalf("managed primary source type: got %T want *AppServerSource", usageSource.primary)
	}
	if !appServer.managedProfile {
		t.Fatal("managed account primary app-server is not managed")
	}
	if _, ok := usageSource.fallback.(*OAuthSource); !ok {
		t.Fatalf("managed fallback source type: got %T want *OAuthSource", usageSource.fallback)
	}
}

func TestUsageSourcesWithoutManagedProofRemainUnforced(t *testing.T) {
	t.Parallel()

	unverified := NewUsageSourceForAccount(MonitorAccount{
		Label:     "unverified",
		CodexHome: "/unverified",
	})
	if _, ok := unverified.(*OAuthSource); !ok {
		t.Fatalf("unverified account source type: got %T want *OAuthSource", unverified)
	}

	raw := NewUsageSourceForHome("/raw")
	appServer, ok := raw.primary.(*AppServerSource)
	if !ok {
		t.Fatalf("raw primary source type: got %T want *AppServerSource", raw.primary)
	}
	if appServer.managedProfile {
		t.Fatal("raw app-server source was forced into managed mode")
	}
	if _, ok := raw.fallback.(*OAuthSource); !ok {
		t.Fatalf("raw fallback source type: got %T want *OAuthSource", raw.fallback)
	}
}

func TestReportUsageSourceForManagedAccountUsesReportFallbackMode(t *testing.T) {
	t.Parallel()

	source := NewReportUsageSourceForAccount(MonitorAccount{
		Label:        "managed",
		CodexHome:    "/managed",
		UseAppServer: true,
	})
	reportSource, ok := source.(*UsageSource)
	if !ok {
		t.Fatalf("managed report source type: got %T want *UsageSource", source)
	}
	if !reportSource.report {
		t.Fatal("managed report source did not enable report fallback semantics")
	}
}

func TestUsageSourceCloseIsIdempotent(t *testing.T) {
	primary := &fakeSource{name: "primary"}
	fallback := &fakeSource{name: "fallback"}
	source := &UsageSource{primary: primary, fallback: fallback}

	if err := source.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if primary.closeCount != 1 || fallback.closeCount != 1 {
		t.Fatalf("source close counts: primary=%d fallback=%d", primary.closeCount, fallback.closeCount)
	}
}
