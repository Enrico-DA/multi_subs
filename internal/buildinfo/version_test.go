package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestDisplayVersionUsesReleaseOverride(t *testing.T) {
	t.Parallel()

	got := displayVersion("1.2.3", func() (*debug.BuildInfo, bool) {
		t.Fatal("release override should not read build info")
		return nil, false
	})
	if got != "1.2.3" {
		t.Fatalf("displayVersion() = %q, want 1.2.3", got)
	}
}

func TestDisplayVersionUsesModuleVersion(t *testing.T) {
	t.Parallel()

	got := displayVersion(fallbackDevVersion, func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260818154100-b7a4e5e12e81"}}, true
	})
	if got != "v0.0.0-20260818154100-b7a4e5e12e81" {
		t.Fatalf("displayVersion() = %q", got)
	}
}

func TestDisplayVersionUsesShortRevision(t *testing.T) {
	t.Parallel()

	got := displayVersion(fallbackDevVersion, func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "b7a4e5e12e81553c86d215af4dfb531673c5299e"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	})
	if got != "b7a4e5e12e81-dirty" {
		t.Fatalf("displayVersion() = %q, want short dirty revision", got)
	}
}

func TestDisplayVersionNeverEmpty(t *testing.T) {
	t.Parallel()

	got := displayVersion(fallbackDevVersion, func() (*debug.BuildInfo, bool) {
		return nil, false
	})
	if got != fallbackDevVersion {
		t.Fatalf("displayVersion() = %q, want %q", got, fallbackDevVersion)
	}
	if DisplayVersion() == "" {
		t.Fatal("DisplayVersion() was empty")
	}
}
