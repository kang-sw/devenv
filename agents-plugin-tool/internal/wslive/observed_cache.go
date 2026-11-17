package wslive

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// observedResweep bounds how long a load may trust the evidence
	// directory's own stat: past it, the next load stats every record again.
	observedResweep = 15 * time.Second
	// observedValidationInterval shares directory checks across concurrent loads.
	observedValidationInterval = time.Second
	// observedRacyWindow is how recent a directory mtime may be, relative to
	// the sweep that recorded it, and still be distrusted: an entry added on a
	// filesystem with coarse timestamps can keep the directory mtime equal.
	observedRacyWindow = 2 * time.Second
)

// ObservedCache retains immutable decoded evidence and its derived model
// histories. Evidence files are write-once (AppendObserved links a synced temp
// file into place and skips an existing name), so every legitimate change
// adds or removes a directory entry and moves the directory's mtime. Loads
// validate at most once per second per session; Invalidate and Forget bypass
// that delay.
// Validation trusts the directory's own stat while it is unchanged and its
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
	// winners records lexical first-write precedence for normalized producer
	// identities. It is private to the slot, never part of a published view.
	winners map[string]string
	// dirInfo is the evidence directory's stat taken before the last listing,
	// dirAt the clock reading before that stat, and sweptAt the clock reading
	// before the last full sweep.
	dirInfo        os.FileInfo
	dirAt, sweptAt time.Time
	checkedAt      time.Time // last directory validation, including failures
	checked        bool
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

// Load reuses each unchanged record and extends immutable histories for new,
// chronologically trailing evidence. Deletions, replacements, earlier duplicate
// winners and late arrivals rebuild from retained records. Failure to list is
// retried at the next validation. All loads share validation for one second.
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
	if elapsed := now.Sub(slot.checkedAt); slot.checked && elapsed >= 0 && elapsed < observedValidationInterval {
		return slot.evidence
	}
	slot.checkedAt, slot.checked = now, true
	dirInfo, dirErr := stat(evidenceDir)
	if dirErr == nil && slot.trusted(dirInfo, now) {
		return slot.evidence
	}
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		slot.files = nil
		slot.evidence = nil
		slot.winners = nil
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
	appendOnly := slot.evidence != nil
	var added []string
	cleaner := NewState("")
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
			appendOnly = false
			continue
		}
		prev, ok := slot.files[entry.Name()]
		if ok && os.SameFile(prev.info, info) && prev.info.Size() == info.Size() && prev.info.Mode() == info.Mode() && prev.info.ModTime().Equal(info.ModTime()) {
			files[entry.Name()] = prev
			continue
		}
		changed = true
		if ok {
			appendOnly = false
		} else {
			added = append(added, entry.Name())
		}
		open := c.open
		if open == nil {
			open = os.Open
		}
		c.opens.Add(1)
		r, valid, err := readObservedRecord(path, open)
		if err == nil {
			if valid {
				o := cleaner.cleanObservation(&r.ModelObservation)
				valid = o != nil && (r.AgentID == "" || agentIDPattern.MatchString(r.AgentID))
				if valid {
					r.ModelObservation = *o
				}
			}
			files[entry.Name()] = observedFile{info: info, record: r, valid: valid}
		}
	}
	if len(files) != len(slot.files) {
		changed = true
	}
	// A deletion can accompany additions without changing the file count.
	if appendOnly {
		for name := range slot.files {
			if _, ok := files[name]; !ok {
				appendOnly = false
				break
			}
		}
	}
	if changed {
		if !appendOnly || !slot.extend(files, added) {
			// ReadObserved consumes files in lexical order. Normalize only on
			// file intake, but retain its first-write-wins duplicate precedence.
			records := make(map[string]ObservedRecord, len(files))
			slot.winners = make(map[string]string, len(files))
			for _, entry := range entries {
				if f, ok := files[entry.Name()]; ok && f.valid {
					key := observationKey(f.record)
					if _, exists := records[key]; !exists {
						records[key], slot.winners[key] = f.record, entry.Name()
					}
				}
			}
			slot.evidence = &ObservedEvidence{models: summarizeObserved(records)}
		}
		slot.files = files
	}
	return slot.evidence
}

// extend handles the ordinary append path without revalidating or sorting the
// retained records. Nothing published is modified: touched models own new
// histories, and untouched producers keep their existing immutable projections.
// Less common changes fall back to the canonical retained-record rebuild.
func (s *observedSlot) extend(files map[string]observedFile, names []string) bool {
	var additions []ObservedRecord
	winners := make(map[string]string, len(names))
	for _, name := range names { // ReadDir's lexical order
		f := files[name]
		if !f.valid {
			continue
		}
		r := f.record
		key := observationKey(r)
		if prev, ok := s.winners[key]; ok {
			if name < prev {
				return false
			}
			continue
		}
		if _, ok := winners[key]; ok {
			continue
		}
		if model := s.evidence.models[r.AgentID]; model != nil {
			last := ObservedRecord{AgentID: r.AgentID, ModelObservation: model.Current}
			if r.At < last.At || (r.At == last.At && key < observationKey(last)) {
				return false
			}
		}
		winners[key] = name
		additions = append(additions, r)
	}
	if len(additions) == 0 {
		return true
	}
	sort.Slice(additions, func(i, j int) bool {
		if additions[i].At != additions[j].At {
			return additions[i].At < additions[j].At
		}
		return observationKey(additions[i]) < observationKey(additions[j])
	})
	models := make(map[string]*ObservedModel, len(s.evidence.models)+1)
	for id, model := range s.evidence.models {
		models[id] = model
	}
	touched := map[string]bool{}
	for _, r := range additions {
		model := models[r.AgentID]
		if !touched[r.AgentID] {
			if model == nil {
				model = &ObservedModel{}
			} else {
				model = model.clone()
			}
			models[r.AgentID], touched[r.AgentID] = model, true
		}
		if model.Current.Model != r.Model {
			model.History = append(model.History, ModelChange{Previous: model.Current.Model, Model: r.Model, At: r.At, Source: r.Source})
		}
		model.Current = r.ModelObservation
	}
	for key, name := range winners {
		s.winners[key] = name
	}
	s.evidence = &ObservedEvidence{models: models}
	return true
}

// Invalidate expires directory validation after a change signal while retaining
// decoded records and immutable histories for reuse by the next load.
func (c *ObservedCache) Invalidate(dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	slot := c.sessions[dir]
	c.mu.Unlock()
	if slot != nil {
		slot.mu.Lock()
		slot.checked = false
		slot.mu.Unlock()
	}
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
