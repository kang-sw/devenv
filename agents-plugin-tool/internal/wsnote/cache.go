package wsnote

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// ReadPaths resolves note stores without Ensure's directory/metadata writes.
// Callers retain these paths rather than resolve the Git identity on each open.
func ReadPaths(root string, config wsconfig.Options, state wsstate.Options) (map[Layer]string, error) {
	paths := map[Layer]string{LayerRepo: RepoDir(root)}
	machine, err := MachinePath(config)
	if err != nil {
		return paths, err
	}
	paths[LayerMachine] = machine
	layout, _, _, err := wsstate.NewManager(state).Resolve(root)
	if err != nil {
		return paths, err
	}
	paths[LayerClone] = filepath.Join(layout.SharedDir, "notes.json")
	paths[LayerWorktree] = filepath.Join(layout.WorktreeDir, "notes.json")
	return paths, nil
}

// Cache is the daemon's memory-only presentation store. There is no timer:
// files load once, explicit refresh rereads, and only a repo drawer-open checks
// directory-entry fingerprints. Writers publish their already-read committed
// records under the same entry lock, never by rereading the note files.
type Cache struct {
	mu       sync.Mutex
	entries  map[string]*cacheEntry
	contexts map[string]readContext
	paths    map[string]map[Layer]string
	// Test seams count actual layer reads independently from metadata checks.
	loadFile func(string) (map[string]Record, error)
	loadRepo func(string) (map[string]Record, error)
}

type readContext struct {
	Root   string
	Config wsconfig.Options
	State  wsstate.Options
}

// Bind retains an authenticated session's caller environment in memory. The
// dashboard must not substitute the daemon's stores for a hosted caller's.
func (c *Cache) Bind(id, root string, config wsconfig.Options, state wsstate.Options) {
	if c == nil {
		return
	}
	context := readContext{root, config, state}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.contexts == nil {
		c.contexts = map[string]readContext{}
	}
	if old, ok := c.contexts[id]; ok && reflect.DeepEqual(old, context) {
		return
	}
	c.contexts[id] = context
	delete(c.paths, id)
}

// Paths resolves once per session store identity, outside the cache lock.
func (c *Cache) Paths(id, root string, refresh bool) (map[Layer]string, error) {
	c.mu.Lock()
	context, bound := c.contexts[id]
	if !bound || context.Root != root {
		context = readContext{Root: root}
		if c.contexts == nil {
			c.contexts = map[string]readContext{}
		}
		c.contexts[id] = context
		delete(c.paths, id)
	}
	if paths := c.paths[id]; paths != nil && !refresh {
		c.mu.Unlock()
		return paths, nil
	}
	c.mu.Unlock()
	paths, err := ReadPaths(root, context.Config, context.State)
	c.mu.Lock()
	if reflect.DeepEqual(c.contexts[id], context) {
		if c.paths == nil {
			c.paths = map[string]map[Layer]string{}
		}
		c.paths[id] = paths
	}
	c.mu.Unlock()
	return paths, err
}

type noteStamp struct {
	Size int64
	Mod  int64
}
type cacheEntry struct {
	mu      sync.Mutex
	ready   bool
	records map[string]Record
	stamps  map[string]noteStamp
}

func NewCache() *Cache { return &Cache{} }

func (c *Cache) entry(path string) *cacheEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*cacheEntry{}
	}
	if c.entries[path] == nil {
		c.entries[path] = &cacheEntry{}
	}
	return c.entries[path]
}

func lockCache(caches []*Cache, path string) (*cacheEntry, func()) {
	if len(caches) == 0 || caches[0] == nil {
		return nil, func() {}
	}
	e := caches[0].entry(path)
	e.mu.Lock()
	return e, e.mu.Unlock
}

func (e *cacheEntry) replace(records map[string]Record) {
	if e == nil {
		return
	}
	e.records, e.ready = records, true
}

func (e *cacheEntry) putRepo(dir string, rec Record) {
	if e == nil || !e.ready {
		return
	}
	e.records[rec.Key] = rec
	name := repoKeyFilename(rec.Key)
	if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
		e.stamps[name] = noteStamp{info.Size(), info.ModTime().UnixNano()}
	}
}

func (e *cacheEntry) eraseRepo(key string) {
	if e == nil || !e.ready {
		return
	}
	delete(e.records, key)
	delete(e.stamps, repoKeyFilename(key))
}

func repoFingerprint(dir string) (map[string]noteStamp, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]noteStamp{}, nil
	}
	if err != nil {
		return nil, err
	}
	stamps := map[string]noteStamp{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		stamps[entry.Name()] = noteStamp{info.Size(), info.ModTime().UnixNano()}
	}
	return stamps, nil
}

// Read runs off the interactive request path. For repo, even an in-place file
// edit changes its directory fingerprint; local-layer files are never statted
// on ordinary opens. force is only the explicit refresh action.
func (c *Cache) Read(path string, repo, force bool) ([]Record, error) {
	e := c.entry(path)
	e.mu.Lock()
	defer e.mu.Unlock()
	var stamps map[string]noteStamp
	if repo {
		var err error
		stamps, err = repoFingerprint(path)
		if err != nil {
			return nil, err
		}
	}
	if !e.ready || force || (repo && !reflect.DeepEqual(stamps, e.stamps)) {
		load := c.loadFile
		if load == nil {
			load = Load
		}
		if repo {
			load = c.loadRepo
			if load == nil {
				load = RepoLoad
			}
		}
		records, err := load(path)
		if err != nil {
			return nil, err
		}
		e.replace(records)
		e.stamps = stamps
	}
	out := make([]Record, 0, len(e.records))
	for _, rec := range e.records {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return CompareRecords(out[i], out[j]) })
	return out, nil
}
