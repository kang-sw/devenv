package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// AgentHost is the daemon's agent runner (internal/wsagent), which hosts
// headless agent conversations as dashboard sessions. The daemon mounts its
// routes, starts it once the daemon holds its lock, tells it when a session
// page wrote to an agent conversation's inbox, and closes it on shutdown.
type AgentHost interface {
	// Handler serves AgentRoutePrefix and everything under it.
	Handler() http.Handler
	// Start loads the conversations; notify re-reads one session.
	Start(notify func(replyID, project string))
	// InboxChanged follows a session page's inbox write on a conversation.
	InboxChanged(replyID string)
	// Close ends every conversation's process.
	Close()
}

// AgentEndpointHost is an AgentHost whose conversations call back on the
// daemon's routes: the daemon gives it its loopback base URL
// ("http://127.0.0.1:<port>") before Start.
type AgentEndpointHost interface {
	SetEndpoint(baseURL string)
}

// AgentRoutePrefix is where the daemon mounts AgentHost.Handler.
const AgentRoutePrefix = "/agents"

// SupportRoutePrefix is where the daemon mounts DaemonOptions.Support.
const SupportRoutePrefix = "/support"

// agentSessions is the daemon's session source: agent registrations are
// read from their live stores alone (no ws-mcp serves them and they have no
// session record), and every other one from the configured source.
type agentSessions struct {
	inner SessionSource
	dir   string
	now   func() time.Time
	// snaps is the daemon's snapshot cache; nil reads every time.
	snaps *SnapshotCache
}

func (s agentSessions) ReadSession(reg Registration) (SessionData, bool) {
	if !reg.Agent {
		return s.inner.ReadSession(reg)
	}
	if _, err := os.Stat(SessionDir(s.dir, reg.ReplyID)); err != nil {
		return SessionData{}, false
	}
	h := sha256.New()
	updatedAt := s.snaps.HashLiveStores(h, s.dir, reg.ReplyID, s.now())
	return SessionData{Fingerprint: hex.EncodeToString(h.Sum(nil)[:16]), SnapshotUpdatedAt: updatedAt}, true
}

func (s agentSessions) ReadPage(reg Registration) (PageContent, bool) {
	return s.ReadPageLang(reg, "")
}

func (s agentSessions) ReadPageLang(reg Registration, lang string) (PageContent, bool) {
	if reg.Agent {
		return PageContent{}, true
	}
	if ls, ok := s.inner.(LangPageSource); ok {
		return ls.ReadPageLang(reg, lang)
	}
	if ps, ok := s.inner.(PageSource); ok {
		return ps.ReadPage(reg)
	}
	return PageContent{}, true
}

// ReadContacts forwards to the configured source; an agent conversation
// has no contacts.
func (s agentSessions) ReadContacts(reg Registration) ([]Contact, bool) {
	if reg.Agent {
		return nil, true
	}
	if cs, ok := s.inner.(ContactSource); ok {
		return cs.ReadContacts(reg)
	}
	return nil, false
}

// BoardItems forwards to the configured source (BoardItemsSource); an agent
// conversation has no board.
func (s agentSessions) BoardItems(reg Registration) (BoardItems, bool) {
	if reg.Agent {
		return BoardItems{}, true
	}
	if bs, ok := s.inner.(BoardItemsSource); ok {
		return bs.BoardItems(reg)
	}
	return BoardItems{}, false
}

// readsBoardItems reports whether the configured source reads board items
// (BoardItemsSource), so the answer check skips a session it could never
// judge before reading anything else of it.
func (s agentSessions) readsBoardItems() bool {
	_, ok := s.inner.(BoardItemsSource)
	return ok
}

// MailPrint forwards to the configured source (MailPrintSource); an agent
// conversation has no mailbox.
func (s agentSessions) MailPrint(reg Registration) (string, bool) {
	if reg.Agent {
		return "", true
	}
	if ms, ok := s.inner.(MailPrintSource); ok {
		return ms.MailPrint(reg)
	}
	return "", false
}

// Counters forwards to the configured source (CounterSource).
func (s agentSessions) Counters() map[string]int64 {
	if cs, ok := s.inner.(CounterSource); ok {
		return cs.Counters()
	}
	return nil
}

// ForgetSession forwards to the configured source (SessionForgetter).
func (s agentSessions) ForgetSession(replyID string) {
	if f, ok := s.inner.(SessionForgetter); ok {
		f.ForgetSession(replyID)
	}
}

// SetLiveSnapshot forwards to the configured source (LiveSnapshotHolder).
func (s agentSessions) SetLiveSnapshot(stateDir, replyID string, snap wslive.Snapshot) {
	if h, ok := s.inner.(LiveSnapshotHolder); ok {
		h.SetLiveSnapshot(stateDir, replyID, snap)
	}
}

// ClearLiveSnapshot forwards to the configured source (LiveSnapshotHolder).
func (s agentSessions) ClearLiveSnapshot(stateDir, replyID string) {
	if h, ok := s.inner.(LiveSnapshotHolder); ok {
		h.ClearLiveSnapshot(stateDir, replyID)
	}
}

// ReadClosedTopics forwards to the configured source; an agent conversation
// has no board.
func (s agentSessions) ReadClosedTopics(reg Registration, lang string, from, n int) (template.HTML, bool) {
	if cs, ok := s.inner.(ClosedTopicsSource); ok && !reg.Agent {
		return cs.ReadClosedTopics(reg, lang, from, n)
	}
	return "", false
}

// ReadOutsideLinks forwards to the configured source; an agent conversation
// has no board, so its board links nothing.
func (s agentSessions) ReadOutsideLinks(reg Registration) ([]string, bool) {
	if ls, ok := s.inner.(OutsideLinkSource); ok && !reg.Agent {
		return ls.ReadOutsideLinks(reg)
	}
	return nil, false
}

// HashLiveStores folds the live stores of session replyID under stateDir
// into h and returns the snapshot's updatedAt, reading every file afresh
// (SnapshotCache.HashLiveStores with no cache).
func HashLiveStores(h io.Writer, stateDir, replyID string, now time.Time) time.Time {
	return (*SnapshotCache)(nil).HashLiveStores(h, stateDir, replyID, now)
}

// HashLiveStores folds the live stores of session replyID under stateDir
// into h and returns the snapshot's updatedAt, reading the snapshot through
// the cache. It is the change detector both session kinds share: ws-mcp's
// source for a lead session and agentSessions for an agent conversation.
// The snapshot is hashed without updatedAt but with its staleness at now,
// so a heartbeat rewrite is no change while a writer that stopped is one;
// the ring and the board-answer inbox (held and drained) are hashed by their
// entry names, since an entry file is never rewritten, so held and drained
// names tell a send (a new held name, or a merge replacing one) and a drain
// apart.
func (c *SnapshotCache) HashLiveStores(h io.Writer, stateDir, replyID string, now time.Time) time.Time {
	sessionDir := SessionDir(stateDir, replyID)
	var updatedAt time.Time
	if p, err := c.read(wslive.SnapshotPath(sessionDir)); err == nil && p.docOK {
		fmt.Fprintf(h, "\x00snapshot stale=%t\x00%s", p.snap.Stale(now), p.canonical)
		if p.snap.UpdatedAt > 0 {
			updatedAt = time.UnixMilli(p.snap.UpdatedAt)
		}
	}
	for _, list := range []struct {
		tag   string
		names func() ([]string, error)
	}{
		{"ring", func() ([]string, error) { return wslive.RingNames(wslive.RingDir(sessionDir)) }},
		{"inbox", func() ([]string, error) { return wsinbox.Names(wsinbox.Dir(sessionDir)) }},
		{"inbox-drained", func() ([]string, error) { return wsinbox.DrainedNames(wsinbox.Dir(sessionDir)) }},
	} {
		if names, err := list.names(); err == nil {
			io.WriteString(h, "\x00"+list.tag)
			for _, name := range names {
				io.WriteString(h, "\x00"+name)
			}
		}
	}
	return updatedAt
}
