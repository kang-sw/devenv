package wsdashboard

import (
	"sync"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// entryFiles holds each session's parsed conversation ring entries and
// drained board-answer records by file name, so a page read parses only the
// entries it has not seen: an entry file is never rewritten, its name is
// its identity (as HashLiveStores relies on). A session keeps only the
// names its latest read listed, the newest wslive.RingCap ring entries and
// the drained records the page shows, so memory follows the page's window,
// not the history; the daemon drops a session's entries when it leaves the
// index (Daemon.sessionChanged). Held inbox entries and the snapshot change,
// and are read on every render.
type entryFiles struct {
	mu       sync.Mutex
	sessions map[string]*sessionFiles
	// parsed counts the entry files read from disk (a test seam).
	parsed int
}

type sessionFiles struct {
	ring    map[string]wslive.RingRecord
	drained map[string]wsinbox.Record
}

func newEntryFiles() *entryFiles { return &entryFiles{sessions: map[string]*sessionFiles{}} }

// readRing is wslive.ReadRing of dir, the ring of session replyID, with
// every entry already read taken from the cache. A nil cache reads every
// entry.
func (c *entryFiles) readRing(replyID, dir string) ([]wslive.RingRecord, error) {
	if c == nil {
		return wslive.ReadRing(dir)
	}
	names, err := wslive.RingNames(dir)
	if err != nil {
		return nil, err
	}
	if len(names) > wslive.RingCap {
		names = names[len(names)-wslive.RingCap:]
	}
	c.mu.Lock()
	var known map[string]wslive.RingRecord
	if s := c.sessions[replyID]; s != nil {
		known = s.ring
	}
	c.mu.Unlock()
	next := make(map[string]wslive.RingRecord, len(names))
	out := make([]wslive.RingRecord, 0, len(names))
	read := 0
	for _, name := range names {
		rec, ok := known[name]
		if !ok {
			read++
			// An entry that does not read is skipped, as ReadRing skips it,
			// and tried again next time.
			if rec, ok = wslive.ReadRingEntry(dir, name); !ok {
				continue
			}
		}
		next[name] = rec
		out = append(out, rec)
	}
	c.mu.Lock()
	c.parsed += read
	c.session(replyID).ring = next
	c.mu.Unlock()
	return out, nil
}

// readDrained is wsinbox.Drained of the inbox at dir, session replyID's,
// with every record already read taken from the cache. A nil cache reads
// every record.
func (c *entryFiles) readDrained(replyID, dir string) ([]wsinbox.Record, error) {
	if c == nil {
		return wsinbox.Drained(dir)
	}
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
