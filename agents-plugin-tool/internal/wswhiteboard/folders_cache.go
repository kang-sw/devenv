package wswhiteboard

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// FolderCache keeps read-only opens in process. Each hit still acquires the
// writer's flock and checks only the root meta/committed file stamps and pending
// presence. Every store commit replaces both roots before removing pending; root
// metadata also catches the external board-takeover edit without a new generation.
// A pending intent always misses, so recovery and schema guards run under the same
// lock as an uncached Open. The zero value is ready for use.
// Leaf-only edits bypass the commit protocol: callers making them must Forget
// the cached board (or use an uncached Open); they cannot invalidate a root stamp.
// No writer uses this cache. Opens share read-only backing data; each caller
// owns a small State header so mutation entry points can detach it safely.
type FolderCache struct {
	mu      sync.Mutex
	entries map[string]*folderCacheSlot
	stat    func(string) (os.FileInfo, error) // optional validation counter seam
}

// SharedFolderCache serves dashboard, MCP queries and nudge decisions. Writers
// remain uncached; commit root stamps preserve read-your-writes across callers.
var SharedFolderCache FolderCache

type folderCacheSlot struct {
	mu   sync.Mutex
	read folderCachedRead
}

type folderCachedRead struct {
	state    *State
	meta     *folderSession
	stamp    folderRootStamp
	revision uint64
}

// Revisions identify immutable reads across all cache instances in this process.
// They are not persisted and must not be carried by mutable State headers.
var folderReadRevision atomic.Uint64

func newFolderReadRevision(state *State) uint64 {
	if state == nil {
		return 0
	}
	return folderReadRevision.Add(1)
}

type folderRootStamp struct {
	meta      os.FileInfo
	committed os.FileInfo
}

// unchanged needs no topic probes. Identity metadata catches ordinary atomic
// replacements, even at an unchanged generation/mtime; equal metadata cannot
// prove unchanged bytes after an in-place edit that restores all metadata.
func (e folderCachedRead) unchanged(dir string, stat func(string) (os.FileInfo, error)) bool {
	if e.state == nil {
		return false
	}
	if _, err := stat(filepath.Join(dir, "pending.json")); !os.IsNotExist(err) {
		return false
	}
	after, err := folderReadStamp(dir, stat)
	return err == nil && wsstate.SameFileMetadata(e.stamp.meta, after.meta) &&
		wsstate.SameFileMetadata(e.stamp.committed, after.committed)
}

func folderReadStamp(dir string, stat func(string) (os.FileInfo, error)) (folderRootStamp, error) {
	meta, err := stat(filepath.Join(dir, "meta.json"))
	if err != nil {
		return folderRootStamp{}, err
	}
	committed, err := stat(filepath.Join(dir, "committed.json"))
	return folderRootStamp{meta: meta, committed: committed}, err
}

// Open is FolderStore.Open with an opt-in read cache, bounded to 512 boards.
// Absent boards and legacy conversion use Open unchanged and are not cached.
func (c *FolderCache) Open(f FolderStore, legacy *State) (*State, error) {
	state, _, err := c.OpenRevision(f, legacy)
	return state, err
}

// OpenRevision returns Open's state and a process-local read identity. A cache
// hit shares its immutable read's revision; every miss or uncached nonnil result
// receives a fresh revision. The state header and revision are captured together
// under the cache slot lock, without additional filesystem probes.
func (c *FolderCache) OpenRevision(f FolderStore, legacy *State) (*State, uint64, error) {
	return c.openRevision(f, legacy, nil)
}

// visit runs under the board lock, after the same root validation as reads.
// Keeping slot -> board lock ordering lets Notify reuse decoded authority
// without opening the board again or holding the cache map lock during I/O.
func (c *FolderCache) openRevision(f FolderStore, legacy *State, visit func(*State, *folderSession) error) (*State, uint64, error) {
	uncached := func() (*State, uint64, error) {
		if visit != nil {
			var state *State
			err := f.withLock(false, func() error {
				var meta *folderSession
				var err error
				state, meta, err = f.load(legacy)
				if err != nil {
					return err
				}
				return visit(state, meta)
			})
			return state, newFolderReadRevision(state), err
		}
		state, err := f.Open(legacy)
		return state, newFolderReadRevision(state), err
	}
	if c == nil {
		return uncached()
	}
	c.mu.Lock()
	slot := c.entries[f.Dir]
	if slot == nil {
		if c.entries == nil || len(c.entries) >= 512 {
			c.entries = map[string]*folderCacheSlot{}
		}
		slot = new(folderCacheSlot)
		c.entries[f.Dir] = slot
	}
	c.mu.Unlock()
	// A blocked board must not hold up reads of another session.
	slot.mu.Lock()
	defer slot.mu.Unlock()
	stat := c.stat
	if stat == nil {
		stat = wsstate.Stat
	}
	if _, err := stat(filepath.Join(f.Dir, "meta.json")); err != nil {
		slot.read = folderCachedRead{}
		return uncached()
	}
	entry := slot.read
	var hit bool
	guard := func() error {
		hit = entry.unchanged(f.Dir, stat)
		if hit {
			return nil
		}
		return f.guard()
	}
	var out *State
	var revision uint64
	err := f.withGuard(true, guard, func() error {
		if hit {
			out = entry.state.sharedRead()
			revision = entry.revision
			if visit != nil {
				return visit(out, entry.meta)
			}
			return nil
		}
		slot.read = folderCachedRead{}
		// Capture before reading: an out-of-band edit during load must not
		// be cached under the stamp of its later bytes. Pending replay changes
		// this stamp and is cached only on a subsequent clean open.
		stamp, stampErr := folderReadStamp(f.Dir, stat)
		var err error
		var meta *folderSession
		out, meta, err = f.load(legacy)
		revision = newFolderReadRevision(out)
		if err == nil && stampErr == nil && out != nil {
			next := folderCachedRead{state: out, meta: meta, stamp: stamp, revision: revision}
			if next.unchanged(f.Dir, stat) {
				out.shared = true
				slot.read = next
				out = out.sharedRead()
			}
		}
		if err == nil && visit != nil {
			return visit(out, meta)
		}
		return err
	})
	if err != nil {
		slot.read = folderCachedRead{}
	}
	return out, revision, err
}

// sharedRead copies only the header. Returning the cache's actual pointer
// would make Apply's final assignment and BeginRender race with other readers.
func (s *State) sharedRead() *State {
	out := *s
	return &out
}

// Forget drops a board when its session leaves the dashboard's index, along
// with the process-wide topic memo under its directory (folders_memo.go).
func (c *FolderCache) Forget(dir string) {
	c.mu.Lock()
	delete(c.entries, dir)
	c.mu.Unlock()
	forgetFolderMemo(dir)
}
