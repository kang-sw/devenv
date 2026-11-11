package wsdashboard

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"

	"github.com/kang-sw/devenv/internal/wslive"
)

// SnapshotCache holds the parsed live snapshots of sessions (FileCache): a
// page render, its refresh stamp, and a session re-read (HashLiveStores)
// each read the snapshot of every session they name, and an unchanged one
// then costs a stat. A hosted session's in-memory copy (SetLive) is served
// in place of its file, with no stat, until ClearLive. The zero value is
// ready; a nil cache reads every time.
type SnapshotCache struct {
	files FileCache[parsedSnapshot]

	liveMu sync.Mutex
	live   map[string]parsedSnapshot // by snapshot path
}

// parsedSnapshot is one snapshot file read: the snapshot, and the document
// without updatedAt that HashLiveStores folds (docOK false when the file is
// no JSON object).
type parsedSnapshot struct {
	snap      wslive.Snapshot
	canonical []byte
	docOK     bool
}

// loadSnapshot reads and parses a snapshot file; an error when it does not
// read or its snapshot does not parse.
func loadSnapshot(path string) (parsedSnapshot, error) {
	raw, err := os.ReadFile(path)
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
	return c.files.Load(path, loadSnapshot)
}

// SetLive serves snap as session replyID's snapshot under stateDir in place
// of its file, until ClearLive, and reports whether it differs from the
// copy it replaces beyond updatedAt (a first copy always does). The cache
// keeps a copy of its own: the caller may reuse snap.
func (c *SnapshotCache) SetLive(stateDir, replyID string, snap wslive.Snapshot) bool {
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
	c.liveMu.Unlock()
	c.files.forget(path)
}

// Snapshot reads the live snapshot of session replyID under stateDir; nil
// when it has none. The snapshot is shared: the caller only reads it.
func (c *SnapshotCache) Snapshot(stateDir, replyID string) *wslive.Snapshot {
	p, err := c.read(wslive.SnapshotPath(SessionDir(stateDir, replyID)))
	if err != nil {
		return nil
	}
	return &p.snap
}

// Forget drops the cached snapshot file of session replyID under stateDir;
// an in-memory copy stays until ClearLive (its slot owns it).
func (c *SnapshotCache) Forget(stateDir, replyID string) {
	if c == nil {
		return
	}
	c.files.ForgetUnder(SessionDir(stateDir, replyID))
}

// Counts reports the snapshot reads served from the cache and the ones that
// read the file.
func (c *SnapshotCache) Counts() (hits, misses int64) {
	if c == nil {
		return 0, 0
	}
	return c.files.Counts()
}
