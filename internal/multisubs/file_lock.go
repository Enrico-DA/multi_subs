package multisubs

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const (
	defaultAccountConfigLockTimeout = 5 * time.Second
	configLockRetryInterval         = 25 * time.Millisecond
)

var errFileLockTimeout = errors.New("file lock wait timed out")

func accountConfigLockTimeout(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return defaultAccountConfigLockTimeout
}

func lockFileWithTimeout(file *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errFileLockTimeout
		}
		wait := configLockRetryInterval
		if remaining < wait {
			wait = remaining
		}
		time.Sleep(wait)
	}
}
