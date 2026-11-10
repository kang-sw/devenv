package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FileCache holds values parsed from small files that other processes
// replace whole (an atomic temp-and-rename write), keyed by path, so a read
// of an unchanged file costs one stat instead of a read and a parse. A
// cached value is served while the file stats as it did before the read
// that produced it: the same size and modification time, compared by
// equality (concurrent replaces can move the time backwards). An edit that
// keeps both within the filesystem's timestamp resolution would be missed,
// so a file modified within fileCacheRacyWindow of the read is not kept: it
// reads again until it is older than any timestamp granule.
//
// The stat comes before the read, so a write landing between the two is
// cached under the older stamp and read again at the next check. A failed
// read or parse is never cached, and a file that does not stat is passed to
// the loader uncached (its answer for a missing file stands).
//
// A value is shared by every caller: callers treat it as read-only. The zero
// value is ready; a nil cache reads every time.
type FileCache[T any] struct {
	mu      sync.Mutex
	entries map[string]fileCacheEntry[T]
	// hits and misses count the loads served from the cache and the ones
	// that read the file; a load of a file that does not stat is neither.
	hits, misses atomic.Int64
	// now is the clock the racy window is judged by; nil is time.Now.
	now func() time.Time
}

type fileCacheEntry[T any] struct {
	size, mod int64
	val       T
}

// fileCacheRacyWindow is how recently a file may have been modified and
// still be cached. It is wider than any timestamp granule a supported
// filesystem has (FAT's 2 s is the coarsest), so a later edit within the
// same granule cannot hide behind an unchanged stamp.
const fileCacheRacyWindow = 2 * time.Second

// Load returns the value load reads from path, from the cache while path
// stats as it did before that read.
func (c *FileCache[T]) Load(path string, load func(path string) (T, error)) (T, error) {
	if c == nil {
		return load(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		// Counted neither way: a file that is not there reads no bytes.
		c.forget(path)
		return load(path)
	}
	size, mod := info.Size(), info.ModTime().UnixNano()
	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && e.size == size && e.mod == mod {
		c.hits.Add(1)
		return e.val, nil
	}
	c.misses.Add(1)
	v, err := load(path)
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil || now().Sub(info.ModTime()) < fileCacheRacyWindow {
		delete(c.entries, path)
		return v, err
	}
	if c.entries == nil {
		c.entries = map[string]fileCacheEntry[T]{}
	}
	c.entries[path] = fileCacheEntry[T]{size: size, mod: mod, val: v}
	return v, nil
}

func (c *FileCache[T]) forget(path string) {
	c.mu.Lock()
	delete(c.entries, path)
	c.mu.Unlock()
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
	for p := range c.entries {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(c.entries, p)
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
