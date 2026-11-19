package wsdashboard

import (
	"sync"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// entryFiles holds each session's parsed drained board-answer records by file
// name, so a page read parses only the drained records it has not seen.
// Conversation ring entries for active hosted sessions are served directly
// from wslive's pinned daemon RAM ring cache (no per-render directory listing
// or entry-file reads); closed/non-hosted rings read on demand without pinning.
// The daemon drops a session's entries when it leaves the index (Daemon.sessionChanged).
// Held inbox entries and the snapshot change, and are read on every render.
type entryFiles struct {
	mu       sync.Mutex
	sessions map[string]*sessionFiles
	// parsed counts the drained entry files read from disk; ringLists,
	// drainedLists, and heldReads count the ring reads, drained
	// listings, and held-entry reads (test seams).
	parsed                             int
	ringLists, drainedLists, heldReads int
}

type sessionFiles struct {
	drained map[string]wsinbox.Record
}

func newEntryFiles() *entryFiles { return &entryFiles{sessions: map[string]*sessionFiles{}} }

// readRing reads dir, the conversation ring of session replyID. For active
// hosted sessions wslive serves this directly from pinned RAM with zero disk ops.
func (c *entryFiles) readRing(replyID, dir string) ([]wslive.RingRecord, error) {
	if c != nil {
		c.mu.Lock()
		c.ringLists++
		c.mu.Unlock()
	}
	return wslive.ReadRing(dir)
}

// readDrained is wsinbox.Drained of the inbox at dir, session replyID's,
// with every record already read taken from the cache. A nil cache reads
// every record.
func (c *entryFiles) readDrained(replyID, dir string) ([]wsinbox.Record, error) {
	if c == nil {
		return wsinbox.Drained(dir)
	}
	c.mu.Lock()
	c.drainedLists++
	c.mu.Unlock()
	names, err := wsinbox.DrainedNames(dir)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	var known map[string]wsinbox.Record
	if s := c.sessions[replyID]; s != nil {
		known = s.drained
	}
	c.mu.Unlock()
	next := make(map[string]wsinbox.Record, len(names))
	out := make([]wsinbox.Record, 0, len(names))
	read := 0
	for _, name := range names {
		rec, ok := known[name]
		if !ok {
			read++
			r, err := wsinbox.ReadDrained(dir, name)
			if err != nil {
				continue
			}
			rec = r
		}
		next[name] = rec
		out = append(out, rec)
	}
	c.mu.Lock()
	c.parsed += read
	c.session(replyID).drained = next
	c.mu.Unlock()
	return out, nil
}

// readHeld is wsinbox.Held of the inbox at dir. Held entries change in
// place (a merge, a steer), so they are read whole every time; the cache
// only counts the reads.
func (c *entryFiles) readHeld(dir string) ([]wsinbox.Record, error) {
	if c != nil {
		c.mu.Lock()
		c.heldReads++
		c.mu.Unlock()
	}
	return wsinbox.Held(dir)
}

// session returns replyID's entries, creating them; the caller holds mu.
func (c *entryFiles) session(replyID string) *sessionFiles {
	s := c.sessions[replyID]
	if s == nil {
		s = &sessionFiles{}
		c.sessions[replyID] = s
	}
	return s
}

// drop forgets a session's entries.
func (c *entryFiles) drop(replyID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, replyID)
	c.mu.Unlock()
}

// parsedCount reports the entry files read so far (a test seam).
func (c *entryFiles) parsedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.parsed
}

// cached reports whether a session holds any entries (a test seam).
func (c *entryFiles) cached(replyID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[replyID] != nil
}
