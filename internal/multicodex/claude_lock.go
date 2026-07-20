package multicodex

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type claudeReservation struct {
	file *os.File
}

func claudeReservationTargetForOrg(orgID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(orgID)))
	return fmt.Sprintf("org-%x", sum[:16])
}

func (s *claudeStore) acquireReservation(target string) (*claudeReservation, bool, error) {
	if target != claudeDefaultTarget {
		if err := validateClaudeProfileName(target); err != nil {
			return nil, false, err
		}
	}
	reservationsDir := filepath.Join(s.paths.ClaudeRunDir, "reservations")
	for _, path := range []string{
		s.paths.MulticodexHome,
		filepath.Dir(s.paths.ClaudeProviderDir),
		s.paths.ClaudeProviderDir,
		s.paths.ClaudeRunDir,
		reservationsDir,
	} {
		if err := ensureClaudePrivateDir(path, true); err != nil {
			return nil, false, err
		}
	}
	if err := ensurePathPrefixesBelowRootNotSymlinks(s.paths.MulticodexHome, reservationsDir); err != nil {
		return nil, false, fmt.Errorf("unsafe Claude reservation directory: %w", err)
	}

	lockPath := filepath.Join(reservationsDir, "claude-"+target+".lock")
	if err := ensurePrivateRegularFileForWrite(lockPath, "Claude reservation lock"); err != nil {
		return nil, false, err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open Claude reservation lock: %w", err)
	}
	closeWithUnlock := func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}
	if err := lockFile.Chmod(0o600); err != nil {
		_ = lockFile.Close()
		return nil, false, fmt.Errorf("secure Claude reservation lock: %w", err)
	}
	info, err := lockFile.Stat()
	if err != nil {
		_ = lockFile.Close()
		return nil, false, fmt.Errorf("inspect Claude reservation lock: %w", err)
	}
	if err := validatePrivateRegularFileInfo(lockPath, "Claude reservation lock", info); err != nil {
		_ = lockFile.Close()
		return nil, false, err
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("acquire Claude reservation lock: %w", err)
	}
	info, err = lockFile.Stat()
	if err != nil {
		closeWithUnlock()
		return nil, false, fmt.Errorf("inspect opened Claude reservation lock: %w", err)
	}
	if err := validatePrivateRegularFileInfo(lockPath, "Claude reservation lock", info); err != nil {
		closeWithUnlock()
		return nil, false, err
	}
	if err := lockFile.Truncate(0); err != nil {
		closeWithUnlock()
		return nil, false, fmt.Errorf("truncate Claude reservation lock: %w", err)
	}
	if _, err := lockFile.WriteString(fmt.Sprintf("%d\n", os.Getpid())); err != nil {
		closeWithUnlock()
		return nil, false, fmt.Errorf("write Claude reservation lock: %w", err)
	}
	return &claudeReservation{file: lockFile}, true, nil
}

func (r *claudeReservation) Release() {
	if r == nil || r.file == nil {
		return
	}
	_ = syscall.Flock(int(r.file.Fd()), syscall.LOCK_UN)
	_ = r.file.Close()
	r.file = nil
}
