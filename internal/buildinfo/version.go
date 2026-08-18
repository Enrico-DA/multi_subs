package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version is the compile-time product version. Release builds override this
// with -ldflags. Local go install leaves the default; DisplayVersion then uses
// the module version or VCS revision so `multisubs version` can tell binaries apart.
var Version = "0.1.0-dev"

const fallbackDevVersion = "0.1.0-dev"

type buildInfoReader func() (*debug.BuildInfo, bool)

func readBuildInfo() (*debug.BuildInfo, bool) {
	return debug.ReadBuildInfo()
}

// DisplayVersion is the version shown to users. A release override wins. A
// go-install module version or short Git revision is next. The compile-time
// default stays only when nothing more specific is available.
func DisplayVersion() string {
	return displayVersion(Version, readBuildInfo)
}

func displayVersion(version string, readInfo buildInfoReader) string {
	if trimmed := strings.TrimSpace(version); trimmed != "" && trimmed != fallbackDevVersion {
		return trimmed
	}
	if info, ok := readInfo(); ok && info != nil {
		if moduleVersion := strings.TrimSpace(info.Main.Version); moduleVersion != "" &&
			moduleVersion != "(devel)" &&
			moduleVersion != fallbackDevVersion {
			return moduleVersion
		}
		revision := ""
		modified := ""
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = strings.TrimSpace(setting.Value)
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if revision != "" {
			if len(revision) > 12 {
				revision = revision[:12]
			}
			if modified == "true" {
				return revision + "-dirty"
			}
			return revision
		}
	}
	if trimmed := strings.TrimSpace(version); trimmed != "" {
		return trimmed
	}
	return fallbackDevVersion
}

// HasIdentifyingVersion reports whether DisplayVersion can tell this binary
// apart from another development build that still uses the compile-time default.
func HasIdentifyingVersion() bool {
	return DisplayVersion() != fallbackDevVersion
}
