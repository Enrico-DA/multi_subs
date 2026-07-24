package codexstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ManagedConfigPathDetails describes the filesystem form of a managed Codex
// config. RawLinkTarget is retained for diagnostics even when later symlink
// validation fails.
type ManagedConfigPathDetails struct {
	IsSymlink     bool
	RawLinkTarget string
}

// ValidateManagedConfigPath accepts the two supported managed config forms:
// a regular file with exactly one hard link, or a symlink whose resolved path
// is exactly the resolved default Codex config path.
func ValidateManagedConfigPath(
	profileConfigPath string,
	defaultConfigPath string,
) (ManagedConfigPathDetails, error) {
	var details ManagedConfigPathDetails
	if strings.TrimSpace(profileConfigPath) == "" {
		return details, fmt.Errorf("managed profile config path is empty")
	}
	if strings.TrimSpace(defaultConfigPath) == "" {
		return details, fmt.Errorf("default Codex config path is empty")
	}

	profileInfo, err := os.Lstat(profileConfigPath)
	if err != nil {
		return details, fmt.Errorf("inspect managed profile config: %w", err)
	}
	if profileInfo.Mode()&os.ModeSymlink == 0 {
		if !profileInfo.Mode().IsRegular() {
			return details, fmt.Errorf("managed profile config is not a regular file")
		}
		stat, ok := profileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return details, fmt.Errorf("managed profile config hard-link count is unavailable")
		}
		if stat.Nlink != 1 {
			return details, fmt.Errorf("managed profile config has multiple hard links")
		}
		return details, nil
	}

	details.IsSymlink = true
	rawTarget, err := os.Readlink(profileConfigPath)
	if err != nil {
		return details, fmt.Errorf("read managed profile config symlink: %w", err)
	}
	details.RawLinkTarget = rawTarget

	absoluteProfilePath, err := filepath.Abs(filepath.Clean(profileConfigPath))
	if err != nil {
		return details, fmt.Errorf("make managed profile config path absolute: %w", err)
	}
	absoluteDefaultPath, err := filepath.Abs(filepath.Clean(defaultConfigPath))
	if err != nil {
		return details, fmt.Errorf("make default Codex config path absolute: %w", err)
	}
	resolvedProfilePath, err := filepath.EvalSymlinks(absoluteProfilePath)
	if err != nil {
		return details, fmt.Errorf("resolve managed profile config symlink: %w", err)
	}
	resolvedDefaultPath, err := filepath.EvalSymlinks(absoluteDefaultPath)
	if err != nil {
		return details, fmt.Errorf("resolve default Codex config path: %w", err)
	}
	resolvedProfilePath = filepath.Clean(resolvedProfilePath)
	resolvedDefaultPath = filepath.Clean(resolvedDefaultPath)
	if resolvedProfilePath != resolvedDefaultPath {
		return details, fmt.Errorf("managed profile config symlink must point to default Codex config")
	}

	targetInfo, err := os.Stat(resolvedProfilePath)
	if err != nil {
		return details, fmt.Errorf("inspect managed profile config symlink target: %w", err)
	}
	if !targetInfo.Mode().IsRegular() {
		return details, fmt.Errorf("managed profile config symlink target is not a regular file")
	}
	return details, nil
}
