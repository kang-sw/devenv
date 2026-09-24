package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeLeaseFileName is the worktree lease record's file name inside a
// linked worktree's Git admin directory (.git/worktrees/<id>/). The ws- prefix
// keeps it from colliding with Git's own admin files. Living in the admin
// directory (not the working tree) means release's reset --hard / clean -ffdx
// never touch it, and Git deletes it with the admin directory on
// `git worktree remove`/`prune`, so the record's lifetime tracks the worktree's.
const worktreeLeaseFileName = "ws-worktree-lease.json"

const worktreeLeaseSchemaVersion = 1

// worktreeLease records which session currently holds a pooled worktree: the
// worker key worktree.acquire minted, the lead key that acquired it, and when.
// It is the worktree -> key pointer the session key records cannot provide
// (they point key -> worktree only, and several stale keys can share a root
// after pool reuse).
type worktreeLease struct {
	SchemaVersion int    `json:"schema_version"`
	WorkerKey     string `json:"worker_key"`
	ParentKey     string `json:"parent_key"`
	AcquiredAt    string `json:"acquired_at"`
}

// worktreeAdminDir resolves wtPath's Git admin directory. --absolute-git-dir
// is required: plain --git-dir can return a relative path.
func worktreeAdminDir(ctx context.Context, runner wsgit.Runner, wtPath string) (string, error) {
	dir, err := wtRun(ctx, runner, wtPath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("resolve git admin dir of %q: %v", wtPath, err)
	}
	return filepath.Clean(dir), nil
}

// writeWorktreeLease replaces wtPath's worktree lease atomically (temp file +
// rename), so a reacquired worktree always carries the fresh holder and a
// concurrent reader never sees a partial record.
func writeWorktreeLease(ctx context.Context, runner wsgit.Runner, wtPath string, lease worktreeLease) error {
	dir, err := worktreeAdminDir(ctx, runner, wtPath)
	if err != nil {
		return err
	}
	lease.SchemaVersion = worktreeLeaseSchemaVersion
	payload, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "ws-worktree-lease-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, worktreeLeaseFileName)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// removeWorktreeLease deletes wtPath's worktree lease. A missing lease (a
// worktree acquired before leases existed, or already released) is not an
// error.
func removeWorktreeLease(ctx context.Context, runner wsgit.Runner, wtPath string) error {
	dir, err := worktreeAdminDir(ctx, runner, wtPath)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, worktreeLeaseFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// readWorktreeLease returns wtPath's worktree lease, or nil when it has none.
func readWorktreeLease(ctx context.Context, runner wsgit.Runner, wtPath string) (*worktreeLease, error) {
	dir, err := worktreeAdminDir(ctx, runner, wtPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, worktreeLeaseFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lease worktreeLease
	if err := json.Unmarshal(data, &lease); err != nil {
		return nil, fmt.Errorf("parse worktree lease: %v", err)
	}
	return &lease, nil
}
