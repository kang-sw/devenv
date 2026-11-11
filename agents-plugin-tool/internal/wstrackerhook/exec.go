package wstrackerhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
)

// stderrLimit is how much of a failed run's stderr its log line keeps.
const stderrLimit = 1024

// RunCommand runs argv without a shell, in dir, with stdin, under the
// caller's environment, and waits at most timeout: then the command's
// process tree is killed. A non-zero exit is an error carrying the end of
// its stderr. Stdout is discarded.
func RunCommand(ctx context.Context, argv []string, dir string, stdin []byte, timeout time.Duration) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("empty command")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	stderr := &tailBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	configureCommand(cmd)
	cmd.Cancel = func() error { return execjob.KillTree(cmd.Process.Pid) }
	// A process the command left holding stderr keeps Wait waiting; the
	// kill above ends the tree, and this bounds what is left.
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("timed out after %s; its process tree was killed", timeout)
	}
	if err != nil {
		if tail := strings.TrimSpace(stderr.String()); tail != "" {
			return fmt.Errorf("%w: %s", err, tail)
		}
		return err
	}
	return nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.limit; over > 0 {
		b.buf = append([]byte(nil), b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
