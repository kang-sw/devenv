package wsdashboard

import (
	"container/list"
	"html/template"
	"strconv"
	"sync"

	"github.com/kang-sw/devenv/internal/wslive"
)

// entryCacheCap bounds one session's cached entries: the live view plus a
// few recently fetched history pages. Past it the least recently used entry
// is evicted.
const entryCacheCap = 5 * wslive.RingCap

// entryCache holds each conversation entry's rendered HTML, per session, so
// markdown and the entry template run once per entry instead of once per
// region render: a ring entry file never changes once written. The daemon
// drops a session's entries when the session leaves the index.
type entryCache struct {
	mu       sync.Mutex
	sessions map[string]*sessionEntries
	// builds counts cache misses (a test seam).
	builds int
}

type sessionEntries struct {
	order *list.List // of *cachedEntry, most recently used first
	byKey map[string]*list.Element
}

type cachedEntry struct {
	key  string
	html template.HTML
}

func newEntryCache() *entryCache { return &entryCache{sessions: map[string]*sessionEntries{}} }

// entryCacheKey is the entry's name plus every other input its HTML depends
// on. A mail line's HTML also depends on whether its body is still in the
// mailbox history and whether this daemon serves its peer, so a body leaving
// that ring or a peer leaving the index renders anew instead of freezing the
// old line into every later render.
func entryCacheKey(e ConvEntry) string {
	return e.Key + "\x00" + strconv.FormatBool(e.HasBody) + "\x00" + strconv.FormatBool(e.PeerLinked) + "\x00" + e.PeerReplyID
}

// render returns the entry's cached HTML, building and caching it on a miss.
// A nil cache always builds. The build runs outside the lock; two streams
// missing the same entry at once both build it, and the result is the same.
func (c *entryCache) render(replyID string, e ConvEntry, build func() (template.HTML, error)) (template.HTML, error) {
	if c == nil {
		return build()
	}
	key := entryCacheKey(e)
	c.mu.Lock()
	if s := c.sessions[replyID]; s != nil {
		if el, ok := s.byKey[key]; ok {
			s.order.MoveToFront(el)
			html := el.Value.(*cachedEntry).html
			c.mu.Unlock()
			return html, nil
		}
	}
	c.builds++
	c.mu.Unlock()

	html, err := build()
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sessions[replyID]
	if s == nil {
		s = &sessionEntries{order: list.New(), byKey: map[string]*list.Element{}}
		c.sessions[replyID] = s
	}
	if el, ok := s.byKey[key]; ok {
		s.order.MoveToFront(el)
		return html, nil
	}
	s.byKey[key] = s.order.PushFront(&cachedEntry{key: key, html: html})
	for s.order.Len() > entryCacheCap {
		oldest := s.order.Back()
		s.order.Remove(oldest)
		delete(s.byKey, oldest.Value.(*cachedEntry).key)
	}
	return html, nil
}

// drop forgets a session's cached entries.
func (c *entryCache) drop(replyID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, replyID)
	c.mu.Unlock()
}

// size reports how many entries a session has cached (a test seam).
func (c *entryCache) size(replyID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.sessions[replyID]; s != nil {
		return s.order.Len()
	}
	return 0
}

// buildCount reports the cache misses so far (a test seam).
func (c *entryCache) buildCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.builds
}
