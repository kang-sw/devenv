package mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// dashboardPoolTimeout bounds one pool read's git commands together.
const dashboardPoolTimeout = 10 * time.Second

// ReadWorktrees reads the worktree pool of the project reg's session is
// rooted at, for the dashboard's pooled-worktree card: the worktree_pool
// value as the session resolves it, and the pooled worktrees under the
// roots worktree.list owns. A lease's keys go out as their reply-ids, since
// the answer reaches a browser.
func (src *DashboardSessionSource) ReadWorktrees(reg wsdashboard.Registration) (wsdashboard.WorktreePool, error) {
	key, _, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.WorktreePool{}, errors.New("no session record")
	}
	adapter := sessionConfigAdapter{s: src.store}
	resolver := wsconfig.NewResolver(wsconfig.Options{RepoRoot: record.Root}, builtinConfigDefaults(), adapter, adapter)
	poolRV, err := resolver.Get(key, wsconfig.ItemWorktreePool)
	if err != nil {
		return wsdashboard.WorktreePool{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dashboardPoolTimeout)
	defer cancel()
	return readDashboardPool(ctx, wsgit.ExecRunner{}, record.Root, poolRV.Value)
}

// readDashboardPool lists root's pooled worktrees as listPoolWorktrees
// selects them, with the facts the dashboard card shows. A fact that cannot
// be read is left empty rather than failing the read.
func readDashboardPool(ctx context.Context, runner wsgit.Runner, root, poolConfigValue string) (wsdashboard.WorktreePool, error) {
	pool := wsdashboard.WorktreePool{Worktrees: []wsdashboard.PooledWorktree{}}
	entries, err := listWorktrees(ctx, runner, root)
	if err != nil {
		return pool, err
	}
	if len(entries) == 0 {
		return pool, fmt.Errorf("could not resolve the primary worktree for %q", root)
	}
	mainRoot := entries[0].Path
	roots := ownedPoolRoots(poolConfigValue, mainRoot)
	pool.Pool = roots[0]
	for i, e := range entries {
		if i == 0 || e.Path == mainRoot || e.Prunable || !underAnyPool(roots, e.Path) {
			continue
		}
		wt := wsdashboard.PooledWorktree{Name: filepath.Base(e.Path), Path: e.Path, Detached: e.Detached || e.Branch == ""}
		if !wt.Detached {
			wt.Branch = strings.TrimPrefix(e.Branch, "refs/heads/")
		}
		if marker, err := readWorktreeMarker(ctx, runner, e.Path); err == nil {
			if marker == nil {
				wt.Profile = worktreeLegacyLabel
			} else {
				wt.Profile, wt.Quarantined = marker.Profile, marker.Quarantine != nil
			}
		}
		lease, _ := readWorktreeLease(ctx, runner, e.Path)
		if lease != nil {
			wt.Leased = true
			wt.Holder, wt.Parent = keyReplyID(lease.WorkerKey), keyReplyID(lease.ParentKey)
			if at, err := time.Parse(time.RFC3339, lease.AcquiredAt); err == nil {
				wt.AcquiredAt = at.UnixMilli()
			}
		}
		if activity, err := worktreeActivity(ctx, runner, e.Path, lease); err == nil {
			wt.ActiveAt = activity.UnixMilli()
		}
		pool.Worktrees = append(pool.Worktrees, wt)
	}
	sort.Slice(pool.Worktrees, func(i, j int) bool { return pool.Worktrees[i].Path < pool.Worktrees[j].Path })
	return pool, nil
}

// keyReplyID is key's reply-id, empty for an empty key.
func keyReplyID(key string) string {
	if key == "" {
		return ""
	}
	return wsmailbox.ReplyID(key)
}
