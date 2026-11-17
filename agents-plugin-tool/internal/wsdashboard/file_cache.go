package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// FileCache holds values parsed from small files that other processes
// replace whole (an atomic temp-and-rename write), keyed by path, so a read
// of an unchanged file validates at most once per second instead of reading
// and parsing every time. Between validations the cached value is served;
// at validation it is kept while the file stats as it did before the read
// that produced it: the same size and modification time, compared by
// equality (concurrent replaces can move the time backwards). An edit that
// keeps both within the filesystem's timestamp resolution would be missed,
// so a file modified within fileCacheRacyWindow of the read is read again at
// each validation until it is older than any timestamp granule.
//
// The stat comes before the read, so a write landing between the two is
// cached under the older stamp and read again at the next check. A failed
// read or parse is never cached, and a file that does not stat is passed to
// the loader uncached (its answer for a missing file stands). Failed stat
// results are shared for the validation interval too, without caching the
// loader's answer.
//
// A value is shared by every caller: callers treat it as read-only. The zero
// value is ready; a nil cache reads every time.
type FileCache[T any] struct {
	mu      sync.Mutex
	entries map[string]fileCacheEntry[T]
	checks  map[string]fileCacheCheck // validation retained for uncached loads
	slots   map[string]*fileCacheSlot // per-path I/O singleflight
	// hits and misses count the loads served from the cache and the ones
	// that read the file; a load of a file that does not stat is neither.
	hits, misses atomic.Int64
	// stat and now are instance-local test seams, set before the first Load.
	// Nil uses os.Stat and time.Now.
	stat func(string) (os.FileInfo, error)
	now  func() time.Time
}

type fileCacheSlot struct {
	mu         sync.Mutex
	generation uint64 // guarded by FileCache.mu, not the I/O lock
}

type fileCacheEntry[T any] struct {
	info      os.FileInfo // retained for identity-checked reads
	size, mod int64
	val       T
	checkedAt time.Time
	racy      bool
}

type fileCacheCheck struct {
	info os.FileInfo // nil when stat failed; the loader still runs every time
	at   time.Time
}

// fileCacheRacyWindow is how recently a file may have been modified and
// still require a reload at the next validation. It is wider than any
// timestamp granule a supported filesystem has (FAT's 2 s is the coarsest),
// so a later edit within the same granule cannot hide behind an unchanged stamp.
const fileCacheRacyWindow = 2 * time.Second

const fileCacheValidationInterval = time.Second

// Load returns the value load reads from path, from the cache while path
// stats as it did before that read, with validation at most once per second.
func (c *FileCache[T]) Load(path string, load func(path string) (T, error)) (T, error) {
	return c.load(path, load, false)
}

// LoadIdentity checks file identity metadata, mode, size and mtime, including
// recently written files. Equal metadata is not proof of unchanged content;
// on Windows creation time is an identity hint, not a unique file ID.
// Dashboard freshness and contact stamps use this rather than a time window.
func (c *FileCache[T]) LoadIdentity(path string, load func(path string) (T, error)) (T, error) {
	return c.load(path, load, true)
}

func (c *FileCache[T]) load(path string, load func(path string) (T, error), identity bool) (T, error) {
	if c == nil {
		return load(path)
	}
	// Only readers of this path wait for its I/O. Forget can detach a slot
	// while a reader waits, so retry slot selection before starting work.
	var slot *fileCacheSlot
	for {
		c.mu.Lock()
		slot = c.slots[path]
		if slot == nil {
			if c.slots == nil {
				c.slots = map[string]*fileCacheSlot{}
			}
			slot = &fileCacheSlot{}
			c.slots[path] = slot
		}
		c.mu.Unlock()
		slot.mu.Lock()
		c.mu.Lock()
		if c.slots[path] == slot {
			break
		}
		c.mu.Unlock()
		slot.mu.Unlock()
	}
	defer slot.mu.Unlock()
	generation := slot.generation
	e, ok := c.entries[path]
	check, checked := c.checks[path]
	clock := time.Now
	if c.now != nil {
		clock = c.now
	}
	stat := c.stat
	c.mu.Unlock()
	now := clock()
	if elapsed := now.Sub(e.checkedAt); ok && elapsed >= 0 && elapsed < fileCacheValidationInterval {
		c.hits.Add(1)
		return e.val, nil
	}
	if stat == nil {
		stat = wsstate.Stat
	}
	if elapsed := now.Sub(check.at); !checked || elapsed < 0 || elapsed >= fileCacheValidationInterval {
		check.info, _ = stat(path)
		check.at = now
	}
	info := check.info
	if info == nil {
		// Counted neither way. Missing/unreadable paths still reach the
		// loader on every call, but share the failed stat for one second.
		c.mu.Lock()
		if c.slots[path] == slot && slot.generation == generation {
			delete(c.entries, path)
			if c.checks == nil {
				c.checks = map[string]fileCacheCheck{}
			}
			c.checks[path] = check
		}
		c.mu.Unlock()
		return load(path)
	}
	size, mod := info.Size(), info.ModTime().UnixNano()
	if ok && (identity || !e.racy) && e.size == size && e.mod == mod && (!identity || wsstate.SameFileMetadata(e.info, info)) {
		e.checkedAt = now
		c.mu.Lock()
		if c.slots[path] == slot && slot.generation == generation {
			c.entries[path] = e
		}
		c.mu.Unlock()
		c.hits.Add(1)
		return e.val, nil
	}
	c.misses.Add(1)
	v, err := load(path)
	racy := clock().Sub(info.ModTime()) < fileCacheRacyWindow
	c.mu.Lock()
	defer c.mu.Unlock()
	// A change signal or Forget during I/O must not be undone by this old
	// result. Its caller may finish, but the next read still validates anew.
	if c.slots[path] != slot || slot.generation != generation {
		return v, err
	}
	if err != nil {
		delete(c.entries, path)
		if c.checks == nil {
			c.checks = map[string]fileCacheCheck{}
		}
		c.checks[path] = check
		return v, err
	}
	delete(c.checks, path)
	if c.entries == nil {
		c.entries = map[string]fileCacheEntry[T]{}
	}
	c.entries[path] = fileCacheEntry[T]{info: info, size: size, mod: mod, val: v, checkedAt: check.at, racy: racy}
	return v, nil
}

func (c *FileCache[T]) forget(path string) {
	c.mu.Lock()
	delete(c.entries, path)
	delete(c.checks, path)
	delete(c.slots, path)
	c.mu.Unlock()
}

// InvalidateUnder expires validation for path and its descendants without
// discarding parsed values. The next load validates authoritatively; unchanged
// stable files still reuse their parsed values. Explicit store-change signals
// use this under the index lane, never once per presentation render.
func (c *FileCache[T]) InvalidateUnder(path string) {
	if c == nil {
		return
	}
	prefix := strings.TrimSuffix(path, string(filepath.Separator)) + string(filepath.Separator)
	c.mu.Lock()
	defer c.mu.Unlock()
	for p, slot := range c.slots {
		if p == path || strings.HasPrefix(p, prefix) {
			slot.generation++
		}
	}
	for p, e := range c.entries {
		if p == path || strings.HasPrefix(p, prefix) {
			e.checkedAt = time.Time{}
			c.entries[p] = e
		}
	}
	for p := range c.checks {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(c.checks, p)
		}
	}
}

// ForgetUnder drops the cached file path and every file under it, when it
// is a directory.
func (c *FileCache[T]) ForgetUnder(path string) {
	if c == nil {
		return
	}
	prefix := strings.TrimSuffix(path, string(filepath.Separator)) + string(filepath.Separator)
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.slots {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(c.slots, p)
		}
	}
	for p := range c.entries {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(c.entries, p)
		}
	}
	for p := range c.checks {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(c.checks, p)
		}
	}
}

// Counts reports the loads served from the cache and the ones that read
// the file.
func (c *FileCache[T]) Counts() (hits, misses int64) {
	if c == nil {
		return 0, 0
	}
	return c.hits.Load(), c.misses.Load()
}

// cachedPaths reports the paths the cache holds (a test seam).
func (c *FileCache[T]) cachedPaths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.entries))
	for p := range c.entries {
		out = append(out, p)
	}
	return out
}
