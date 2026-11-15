package wswhiteboard

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FolderCache keeps read-only opens in process. Each hit still acquires the
// writer's flock and checks every authoritative file's identity, size, mode and
// mtime. A pending intent always misses, so recovery and schema guards run under
// the same lock as an uncached Open. The zero value is ready for use.
// No writer uses this cache. Opens share read-only backing data; each caller
// owns a small State header so mutation entry points can detach it safely.
type FolderCache struct {
	mu      sync.Mutex
	entries map[string]*folderCacheSlot
}

type folderCacheSlot struct {
	mu   sync.Mutex
	read folderCachedRead
}

type folderCachedRead struct {
	state *State
	files map[string]os.FileInfo
}

// unchanged needs no topic opens. Atomic replacements are detected even on a
// coarse timestamp filesystem by SameFile; in-place edits move size or mtime.
func (e folderCachedRead) unchanged(dir string) bool {
	if e.state == nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "pending.json")); !os.IsNotExist(err) {
		return false
	}
	for path, before := range e.files {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
			return false
		}
	}
	return true
}

func folderReadStamp(dir string) (map[string]os.FileInfo, error) {
	files := map[string]os.FileInfo{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// WalkDir does not follow linked directories. Keep such stores on
		// the uncached path rather than overlooking a linked topic's files.
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("whiteboard read cache: symbolic link %s", path)
		}
		if entry.IsDir() || (filepath.Ext(path) != ".json" && filepath.Ext(path) != ".jsonl") {
			return nil
		}
		info, err := os.Stat(path)
		if err == nil {
			files[path] = info
		}
		return err
	})
	return files, err
}

// Open is FolderStore.Open with an opt-in read cache, bounded to 64 boards.
// Absent boards and legacy conversion use Open unchanged and are not cached.
func (c *FolderCache) Open(f FolderStore, legacy *State) (*State, error) {
	if c == nil {
		return f.Open(legacy)
	}
	c.mu.Lock()
	slot := c.entries[f.Dir]
	if slot == nil {
		if c.entries == nil || len(c.entries) >= 64 {
			c.entries = map[string]*folderCacheSlot{}
		}
		slot = new(folderCacheSlot)
		c.entries[f.Dir] = slot
	}
	c.mu.Unlock()
	// A blocked board must not hold up reads of another session.
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if _, err := os.Stat(filepath.Join(f.Dir, "meta.json")); err != nil {
		slot.read = folderCachedRead{}
		return f.Open(legacy)
	}
	entry := slot.read
	var hit bool
	guard := func() error {
		hit = entry.unchanged(f.Dir)
		if hit {
			return nil
		}
		return f.guard()
	}
	var out *State
	err := f.withGuard(true, guard, func() error {
		if hit {
			out = entry.state.sharedRead()
			return nil
		}
		slot.read = folderCachedRead{}
		// Capture before reading: an out-of-band edit during load must not
		// be cached under the stamp of its later bytes. Pending replay changes
		// this stamp and is cached only on a subsequent clean open.
		files, stampErr := folderReadStamp(f.Dir)
		var err error
		out, _, err = f.load(legacy)
		if err == nil && stampErr == nil && out != nil {
			next := folderCachedRead{state: out, files: files}
			if next.unchanged(f.Dir) {
				out.shared = true
				slot.read = next
				out = out.sharedRead()
			}
		}
		return err
	})
	if err != nil {
		slot.read = folderCachedRead{}
	}
	return out, err
}

// sharedRead copies only the header. Returning the cache's actual pointer
// would make Apply's final assignment and BeginRender race with other readers.
func (s *State) sharedRead() *State {
	out := *s
	return &out
}

// Forget drops a board when its session leaves the dashboard's index.
func (c *FolderCache) Forget(dir string) {
	c.mu.Lock()
	delete(c.entries, dir)
	c.mu.Unlock()
}
