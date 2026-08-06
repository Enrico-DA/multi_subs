package processprobe

import (
	"bytes"
	"errors"
	"os/exec"
	"sync"
	"time"
)

const (
	OutputLimit = 1_000_000
	WaitDelay   = 500 * time.Millisecond
)

var ErrOutputLimit = errors.New("provider probe output exceeded safe limit")

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  *sharedLimit
}

type sharedLimit struct {
	mu        sync.Mutex
	remaining int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	b.limit.mu.Lock()
	defer b.limit.mu.Unlock()
	remaining := b.limit.remaining
	if remaining > 0 {
		keep := len(data)
		if keep > remaining {
			keep = remaining
		}
		_, _ = b.buffer.Write(data[:keep])
		b.limit.remaining -= keep
	}
	if len(data) > remaining {
		b.limit.truncated = true
	}
	return len(data), nil
}

func CombinedOutput(command *exec.Cmd) ([]byte, bool, error) {
	limit := &sharedLimit{remaining: OutputLimit}
	output := limitedBuffer{limit: limit}
	command.Stdout = &output
	command.Stderr = &output
	command.WaitDelay = WaitDelay
	err := command.Run()
	return output.buffer.Bytes(), limit.truncated, err
}

func SeparateOutput(command *exec.Cmd) ([]byte, []byte, bool, error) {
	limit := &sharedLimit{remaining: OutputLimit}
	stdout := limitedBuffer{limit: limit}
	stderr := limitedBuffer{limit: limit}
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.WaitDelay = WaitDelay
	err := command.Run()
	return stdout.buffer.Bytes(), stderr.buffer.Bytes(), limit.truncated, err
}
