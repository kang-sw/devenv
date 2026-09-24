package wsindex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CloneID returns this clone's random id, generating and storing it in the
// clone's shared .git/config on first use. Every worktree of the clone shares
// it; separate clones get distinct ids. Generation is serialized by a lock
// file in the common dir so two worktrees racing the first call agree.
func (c *Client) CloneID(ctx context.Context) (string, error) {
	if id := c.readCloneID(ctx); id != "" {
		return id, nil
	}
	lock := filepath.Join(c.commonDir, stateDirName, "clone-id.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return "", err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		if info, statErr := os.Stat(lock); statErr == nil && time.Since(info.ModTime()) > 10*time.Second {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("generate clone id: lock %s is held", lock)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer os.Remove(lock)
	if id := c.readCloneID(ctx); id != "" {
		return id, nil
	}
	id := randomHex(8)
	if _, err := c.git(ctx, nil, "config", "--local", CloneIDConfigKey, id); err != nil {
		return "", fmt.Errorf("store clone id: %w", err)
	}
	return id, nil
}

func (c *Client) readCloneID(ctx context.Context) string {
	out, err := c.git(ctx, nil, "config", "--local", "--get", CloneIDConfigKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
