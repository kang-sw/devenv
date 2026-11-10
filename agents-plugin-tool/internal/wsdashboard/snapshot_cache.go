package wsdashboard

import (
	"encoding/json"
	"os"

	"github.com/kang-sw/devenv/internal/wslive"
)

// SnapshotCache holds the parsed live snapshots of sessions (FileCache): a
// page render, its refresh stamp, and a session re-read (HashLiveStores)
// each read the snapshot of every session they name, and an unchanged one
// then costs a stat. The zero value is ready; a nil cache reads every time.
type SnapshotCache struct {
	files FileCache[parsedSnapshot]
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
	return c.files.Load(path, loadSnapshot)
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

// Forget drops the cached snapshot of session replyID under stateDir.
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
