package wsdashboard

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// SnapshotCache holds the parsed live snapshots of sessions (FileCache): a
// page render, its refresh stamp, and a session re-read (HashLiveStores)
// each read the snapshot of every session they name. File validation is shared
// across those readers at most once per second, even for recent files. A hosted
// session's in-memory copy (SetLive) is served in place of its file, with no
// stat, until ClearLive. The zero value is
// ready; a nil cache reads every time.
//
// Retained model evidence (observed) overlays every snapshot Snapshot reads,
// except a hosted copy whose writer folded the evidence directory into its
// own state (setLive with folded): that copy already holds the fold the
// overlay would build, so its reads touch no evidence file. Another writer
// adding evidence beside the copy's (shareObserved) returns the session to
// the overlay until ClearLive.
type SnapshotCache struct {
	files       FileCache[parsedSnapshot]
	observed    wslive.ObservedCache
	outputRates wslive.OutputRateCache
	// load is an instance-local snapshot opener/decoder seam for tests.
	load func(string) (parsedSnapshot, error)

	liveMu sync.Mutex
	live   map[string]parsedSnapshot // by snapshot path
	// shared names the hosted sessions, by snapshot path, whose evidence
	// directory another writer may add to; under liveMu, cleared by ClearLive.
	shared map[string]bool
}

// parsedSnapshot is one snapshot file read: the snapshot, and the document
// without updatedAt that HashLiveStores folds (docOK false when the file is
// no JSON object).
type parsedSnapshot struct {
	snap      wslive.Snapshot
	canonical []byte
	docOK     bool
	// folded marks a hosted copy whose writer seeded its state from the
	// evidence directory before its slot opened (ObservedFoldSlot); never set
	// on a file read.
	folded bool
}

// loadSnapshot reads and parses a snapshot file; an error when it does not
// read or its snapshot does not parse.
func loadSnapshot(path string) (parsedSnapshot, error) {
	raw, err := wsstate.ReadFileOnce(path)
	if err != nil {
		return parsedSnapshot{}, err
	}
	return parseSnapshot(raw)
}

// parseSnapshot parses a snapshot document.
func parseSnapshot(raw []byte) (parsedSnapshot, error) {
	var p parsedSnapshot
	if err := json.Unmarshal(raw, &p.snap); err != nil {
		return parsedSnapshot{}, err
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) == nil {
		delete(doc, "updatedAt")
		p.canonical, _ = json.Marshal(doc)
		p.docOK = true
	}
	return p, nil
}

// read returns the parsed snapshot file at path.
func (c *SnapshotCache) read(path string) (parsedSnapshot, error) {
	if c == nil {
		return loadSnapshot(path)
	}
	c.liveMu.Lock()
	p, ok := c.live[path]
	c.liveMu.Unlock()
	if ok {
		return p, nil
	}
	load := c.load
	if load == nil {
		load = loadSnapshot
	}
	return c.files.LoadIdentity(path, load)
}

// FreshSnapshot reads current admission/capability evidence without trusting
// the presentation cache or loading retained history. Hosted RAM still takes
// precedence over disk. A disk read is not published to the presentation cache.
func (c *SnapshotCache) FreshSnapshot(stateDir, replyID string) *wslive.Snapshot {
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	load := loadSnapshot
	if c != nil {
		c.liveMu.Lock()
		p, ok := c.live[path]
		c.liveMu.Unlock()
		if ok {
			return &p.snap
		}
		if c.load != nil {
			load = c.load
		}
	}
	p, err := load(path)
	if err != nil {
		return nil
	}
	return &p.snap
}

// lastKnown returns snapshot evidence already read by the index or a page,
// without probing disk. The index polling backstop/nudges keep file evidence
// current; the clock still ages it on every stamp. Live copies take priority.
func (c *SnapshotCache) lastKnown(stateDir, replyID string) *wslive.Snapshot {
	if c == nil {
		// Uncached standalone readers retain their existing read-through behavior.
		return c.snapshotWithoutObserved(stateDir, replyID)
	}
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	c.liveMu.Lock()
	p, ok := c.live[path]
	c.liveMu.Unlock()
	if ok {
		return &p.snap
	}
	c.files.mu.Lock()
	e, ok := c.files.entries[path]
	c.files.mu.Unlock()
	if !ok {
		return nil
	}
	return &e.val.snap
}

func snapshotStaleAt(snap *wslive.Snapshot) time.Time {
	if snap == nil {
		return time.Time{}
	}
	beat := snap.HeartbeatMs
	if beat <= 0 {
		beat = wslive.HeartbeatInterval.Milliseconds()
	}
	// Snapshot.Stale uses a strict greater-than millisecond comparison.
	return time.UnixMilli(snap.UpdatedAt + wslive.StaleAfterBeats*beat + 1)
}

// SetLive serves snap as session replyID's snapshot under stateDir in place
// of its file, until ClearLive, and reports whether it differs from the
// copy it replaces beyond updatedAt (a first copy always does). The cache
// keeps a copy of its own: the caller may reuse snap.
func (c *SnapshotCache) SetLive(stateDir, replyID string, snap wslive.Snapshot) bool {
	return c.setLive(stateDir, replyID, snap, false)
}

// setLive is SetLive; folded reports that snap's writer folded the session's
// evidence directory into its state, so reads take the fold from snap.
func (c *SnapshotCache) setLive(stateDir, replyID string, snap wslive.Snapshot, folded bool) bool {
	if c == nil {
		return true
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return false
	}
	p, err := parseSnapshot(raw)
	if err != nil {
		return false
	}
	p.folded = folded
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	prev, had := c.live[path]
	if c.live == nil {
		c.live = map[string]parsedSnapshot{}
	}
	c.live[path] = p
	return !had || !prev.docOK || !p.docOK || !bytes.Equal(prev.canonical, p.canonical)
}

// ClearLive drops session replyID's in-memory copy under stateDir, so its
// file is read again.
func (c *SnapshotCache) ClearLive(stateDir, replyID string) {
	if c == nil {
		return
	}
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	c.liveMu.Lock()
	delete(c.live, path)
	delete(c.shared, path)
	c.liveMu.Unlock()
	c.files.forget(path)
}

// shareObserved records that a writer other than the hosted copy's may add
// evidence for session replyID under stateDir: until ClearLive, its reads
// overlay the evidence directory again, as a file-only session's do.
func (c *SnapshotCache) shareObserved(stateDir, replyID string) {
	if c == nil {
		return
	}
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	c.liveMu.Lock()
	if c.shared == nil {
		c.shared = map[string]bool{}
	}
	c.shared[path] = true
	c.liveMu.Unlock()
}

// snapshotWithoutObserved reads the shared live snapshot without loading
// retained history. Use Snapshot only when rendered output consumes history.
func (c *SnapshotCache) snapshotWithoutObserved(stateDir, replyID string) *wslive.Snapshot {
	p, err := c.read(wslive.SnapshotPath(SessionDir(stateDir, replyID)))
	if err != nil {
		return nil
	}
	return &p.snap
}

// Snapshot reads the live snapshot of session replyID under stateDir; nil
// when it has none. The snapshot is shared: the caller only reads it.
func (c *SnapshotCache) Snapshot(stateDir, replyID string) *wslive.Snapshot {
	snap, _ := c.snapshotResult(stateDir, replyID)
	return snap
}

// snapshotResult preserves read failures for incremental consumers that must
// retry transient errors rather than cache them as an authoritative absence.
func (c *SnapshotCache) snapshotResult(stateDir, replyID string) (*wslive.Snapshot, error) {
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	if p, ok := c.foldedCopy(path); ok {
		return &p.snap, nil
	}
	p, err := c.read(path)
	if err != nil {
		return nil, err
	}
	dir := SessionDir(stateDir, replyID)
	var observed *wslive.ObservedCache
	var outputRates *wslive.OutputRateCache
	if c != nil {
		observed = &c.observed
		outputRates = &c.outputRates
	}
	observed.Load(dir).Overlay(&p.snap)
	if p.snap.Lead.Observed == nil {
		wslive.OverlayOutputRate(&p.snap, outputRates.Load(dir))
	}
	return &p.snap, nil
}

// foldedCopy is the hosted copy at snapshot path when it stands for its
// session's evidence directory. Its writer seeded from the directory and
// applies each record it writes there itself, so the directory holds no
// record the copy lacks, and the overlay would rebuild the copy's own fold:
// the same Current and History per producer. No copy, an undeclared one, or a
// shared one (another writer) is overlaid instead.
func (c *SnapshotCache) foldedCopy(path string) (parsedSnapshot, bool) {
	if c == nil {
		return parsedSnapshot{}, false
	}
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	p, ok := c.live[path]
	return p, ok && p.folded && !c.shared[path]
}

// Invalidate expires disk snapshot and observed-evidence validation after a
// change signal without discarding last-known evidence, hosted RAM, or history.
func (c *SnapshotCache) Invalidate(stateDir, replyID string) {
	if c == nil {
		return
	}
	path := wslive.SnapshotPath(SessionDir(stateDir, replyID))
	c.files.InvalidateUnder(path)
	c.observed.Invalidate(SessionDir(stateDir, replyID))
	c.outputRates.Invalidate(SessionDir(stateDir, replyID))
}

// Forget drops the cached snapshot file of session replyID under stateDir;
// an in-memory copy stays until ClearLive (its slot owns it).
func (c *SnapshotCache) Forget(stateDir, replyID string) {
	if c == nil {
		return
	}
	c.files.ForgetUnder(SessionDir(stateDir, replyID))
	c.observed.Forget(SessionDir(stateDir, replyID))
	c.outputRates.Forget(SessionDir(stateDir, replyID))
}

// Counts reports the snapshot reads served from the cache and the ones that
// read the file.
func (c *SnapshotCache) Counts() (hits, misses int64) {
	if c == nil {
		return 0, 0
	}
	return c.files.Counts()
}
