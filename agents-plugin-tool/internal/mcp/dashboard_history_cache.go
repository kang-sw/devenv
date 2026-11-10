package mcp

import (
	"errors"
	"io/fs"
	"sync"
)

// mailboxHistoryCache holds each served session's parsed mailbox history
// entries by file name, for DashboardSessionSource: an entry file is never
// rewritten, so a page read parses only the entries it has not seen. A
// session keeps only the names its latest read listed (the ring holds
// mailboxHistoryCap), and its entries go when the daemon's index drops the
// session (ForgetSession). The mailbox.history tool reads through the
// uncached readMailboxHistory.
type mailboxHistoryCache struct {
	mu       sync.Mutex
	sessions map[string]*historyEntries // by reply-id
	// parsed counts the entry files read from disk (a test seam).
	parsed int
}

type historyEntries struct {
	// key is the session key the entries were read under; a reply-id that
	// maps to another key since starts over.
	key    string
	byName map[string]mailboxHistoryRecord
}

// read is readMailboxHistory of session key key (reply-id replyID), with
// every entry already read taken from the cache.
func (c *mailboxHistoryCache) read(replyID, key string) ([]mailboxHistoryRecord, error) {
	dir, err := mailboxHistoryDir("", key)
	if err != nil {
		return nil, err
	}
	names, err := mailboxHistoryEntryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		names, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	var known map[string]mailboxHistoryRecord
	if s := c.sessions[replyID]; s != nil && s.key == key {
		known = s.byName
	}
	c.mu.Unlock()
	next := make(map[string]mailboxHistoryRecord, len(names))
	records := make([]mailboxHistoryRecord, 0, len(names))
	read := 0
	for _, name := range names {
		rec, ok := known[name]
		if !ok {
			read++
			if rec, ok = readMailboxHistoryEntry(dir, name); !ok {
				continue
			}
		}
		next[name] = rec
		records = append(records, rec)
	}
	c.mu.Lock()
	c.parsed += read
	if c.sessions == nil {
		c.sessions = map[string]*historyEntries{}
	}
	c.sessions[replyID] = &historyEntries{key: key, byName: next}
	c.mu.Unlock()
	return records, nil
}

// forget drops a session's entries.
func (c *mailboxHistoryCache) forget(replyID string) {
	c.mu.Lock()
	delete(c.sessions, replyID)
	c.mu.Unlock()
}
