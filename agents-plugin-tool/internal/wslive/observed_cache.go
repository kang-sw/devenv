package wslive

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// ObservedCache retains immutable decoded evidence and its derived model
// histories. Each load lists the directory and stats each record, following
// symlinks. SameFile, size, mode and mtime detect replacements and child edits
// even when the directory's own stat is unchanged. The zero value is ready.
// Like FolderCache, in-place writes preserving the entire identity are outside
// this cache's contract; no TTL or recent-write exclusion is used.
type ObservedCache struct {
	mu       sync.Mutex
	sessions map[string]*observedSlot
	opens    atomic.Int64
	// open is an instance-local test seam; set before the first Load.
	open func(string) (*os.File, error)
}

type observedSlot struct {
	mu       sync.Mutex
	files    map[string]observedFile
	evidence *ObservedEvidence
}
type observedFile struct {
	info   os.FileInfo
	record ObservedRecord
	valid  bool
}

// ObservedEvidence is a shared read-only projection of retained observations.
// Its backing maps and histories are never mutated after publication.
type ObservedEvidence struct{ models map[string]*ObservedModel }

// Overlay adds this evidence without changing snapshot activity or borrowing
// another producer's evidence. The snapshot's newer live evidence still wins.
func (e *ObservedEvidence) Overlay(snap *Snapshot) {
	if e != nil && len(e.models) > 0 {
		overlayObserved(snap, e.models)
	}
}

// Load reuses each unchanged record and rebuilds histories only when the set
// or identity of its files changes. Failure to list is retried on the next load.
func (c *ObservedCache) Load(dir string) *ObservedEvidence {
	if c == nil {
		records, _ := ReadObserved(dir)
		st := NewState("")
		st.SeedObserved(records)
		return &ObservedEvidence{models: st.observed}
	}
	c.mu.Lock()
	slot := c.sessions[dir]
	if slot == nil {
		if c.sessions == nil || len(c.sessions) >= 256 {
			c.sessions = map[string]*observedSlot{}
		}
		slot = &observedSlot{}
		c.sessions[dir] = slot
	}
	c.mu.Unlock()
	slot.mu.Lock()
	defer slot.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(dir, ObservedDirName))
	if err != nil {
		slot.files = nil
		slot.evidence = nil
		return nil
	}
	files := make(map[string]observedFile, len(entries))
	changed := slot.evidence == nil
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, ObservedDirName, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			changed = true
			continue
		}
		prev, ok := slot.files[entry.Name()]
		if ok && os.SameFile(prev.info, info) && prev.info.Size() == info.Size() && prev.info.Mode() == info.Mode() && prev.info.ModTime().Equal(info.ModTime()) {
			files[entry.Name()] = prev
			continue
		}
		changed = true
		open := c.open
		if open == nil {
			open = os.Open
		}
		c.opens.Add(1)
		r, valid, err := readObservedRecord(path, open)
		if err == nil {
			files[entry.Name()] = observedFile{info: info, record: r, valid: valid}
		}
	}
	if len(files) != len(slot.files) {
		changed = true
	}
	if changed {
		// ReadObserved consumes files in lexical order, so duplicate producer IDs
		// retain the same first-write-wins precedence here as on an uncached read.
		records := make([]ObservedRecord, 0, len(files))
		for _, entry := range entries {
			if f, ok := files[entry.Name()]; ok && f.valid {
				records = append(records, f.record)
			}
		}
		st := NewState("")
		st.SeedObserved(records)
		slot.evidence = &ObservedEvidence{models: st.observed}
		slot.files = files
	}
	return slot.evidence
}

// Forget drops session-local evidence when it leaves the dashboard index.
func (c *ObservedCache) Forget(dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, dir)
	c.mu.Unlock()
}

// Opens reports record-file open attempts, excluding directory listings.
func (c *ObservedCache) Opens() int64 {
	if c == nil {
		return 0
	}
	return c.opens.Load()
}
