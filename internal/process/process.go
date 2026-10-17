package process

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Buffer bounds memory use without blocking a noisy subprocess.
type Buffer struct {
	mu        sync.Mutex
	data      []byte
	Limit     int
	truncated bool
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.Limit
	if limit <= 0 {
		limit = 1024 * 1024
	}
	n := len(p)
	remain := limit - len(b.data)
	if remain > 0 {
		if remain > n {
			remain = n
		}
		b.data = append(b.data, p[:remain]...)
	}
	if remain < n {
		b.truncated = true
	}
	return n, nil
}
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := string(b.data)
	if b.truncated {
		s += "\n[output truncated]"
	}
	return s
}

func (b *Buffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// Run owns a process group/job until exit. Cancel and normal parent exit kill descendants.
func Run(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := prepare(cmd)
	if err != nil {
		return err
	}
	defer job.close()
	cmd.Cancel = func() error { return job.kill(cmd) }
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := job.attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

var _ io.Writer = (*Buffer)(nil)
