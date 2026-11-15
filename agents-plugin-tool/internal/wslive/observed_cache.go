package wslive

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// observedResweep bounds how long a load may trust the evidence
	// directory's own stat: past it, the next load stats every record again.
	observedResweep = 15 * time.Second
	// observedRacyWindow is how recent a directory mtime may be, relative to
	// the sweep that recorded it, and still be distrusted: an entry added on a
	// filesystem with coarse timestamps can keep the directory mtime equal.
	observedRacyWindow = 2 * time.Second
)

// ObservedCache retains immutable decoded evidence and its derived model
// histories. Evidence files are write-once (AppendObserved links a synced temp
// file into place and skips an existing name), so every legitimate change
// adds or removes a directory entry and moves the directory's mtime. A load
// therefore trusts the directory's own stat while it is unchanged and its
// mtime is older than the racy window (observedRacyWindow) of the stat that
// recorded it. Otherwise it lists the directory, reuses each known name
// without a stat, and stats and reads only new names; a vanished name drops.
// A full sweep, which stats every record (following symlinks) and re-reads
// one whose SameFile, size, mode or mtime moved, runs at least every
// observedResweep and whenever the directory stat is unusable: it bounds the
// staleness of an in-place child edit, a same-name replacement, or a
// coarse-mtime collision. The prior per-load full sweep cost one stat per
// record on every snapshot read, which a long session's thousands of records
// made the dashboard's dominant cost. The zero value is ready.
type ObservedCache struct {
	mu       sync.Mutex
	sessions map[string]*observedSlot
	opens    atomic.Int64
	// open, stat and now are instance-local test seams; set before the first
	// Load.
	open func(string) (*os.File, error)
	stat func(string) (os.FileInfo, error)
	now  func() time.Time
}

type observedSlot struct {
	mu       sync.Mutex
	files    map[string]observedFile
	evidence *ObservedEvidence
	// dirInfo is the evidence directory's stat taken before the last listing,
	// dirAt the clock reading before that stat, and sweptAt the clock reading
	// before the last full sweep.
	dirInfo        os.FileInfo
	dirAt, sweptAt time.Time
}

// sweepDue reports that a load at clock now must stat every record.
func (s *observedSlot) sweepDue(now time.Time) bool {
	d := now.Sub(s.sweptAt)
	return s.evidence == nil || s.dirInfo == nil || d < 0 || d >= observedResweep
}

// trusted reports that the slot's last listing still describes the directory
// now stat'ed as info at clock now.
func (s *observedSlot) trusted(info os.FileInfo, now time.Time) bool {
	prev := s.dirInfo
	if s.sweepDue(now) || info == nil || !info.IsDir() || info.ModTime().IsZero() {
		return false
	}
	if !info.ModTime().Before(s.dirAt.Add(-observedRacyWindow)) {
		return false
	}
	return os.SameFile(prev, info) && prev.Size() == info.Size() && prev.Mode() == info.Mode() && prev.ModTime().Equal(info.ModTime())
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
	stat := c.stat
	if stat == nil {
		stat = os.Stat
	}
	clock := time.Now
	if c.now != nil {
		clock = c.now
	}
	evidenceDir := filepath.Join(dir, ObservedDirName)
	now := clock()
	dirInfo, dirErr := os.Stat(evidenceDir)
	if dirErr == nil && slot.trusted(dirInfo, now) {
		return slot.evidence
	}
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		slot.files = nil
		slot.evidence = nil
		slot.dirInfo = nil
		return nil
	}
	full := slot.sweepDue(now) || dirErr != nil
	// An unusable directory stat leaves nothing to trust: every load sweeps.
	slot.dirInfo, slot.dirAt = nil, now
	if dirErr == nil {
		slot.dirInfo = dirInfo
	}
	if full {
		slot.sweptAt = now
	}
	files := make(map[string]observedFile, len(entries))
	changed := slot.evidence == nil
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if prev, ok := slot.files[entry.Name()]; ok && !full {
			files[entry.Name()] = prev
			continue
		}
		path := filepath.Join(evidenceDir, entry.Name())
		info, err := stat(path)
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
