package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// AgentPageInputHost distinguishes an explicit composer send from passive
// inbox notifications. Archive restoration must not be inferred from a nudge.
type AgentPageInputHost interface {
	PageInput(replyID string) error
}

// AgentEndpointHost is an AgentHost whose conversations call back on the
// daemon's routes: the daemon gives it its loopback base URL
// ("http://127.0.0.1:<port>") before Start.
type AgentEndpointHost interface {
	SetEndpoint(baseURL string)
}

// AgentLiveHost is an optional AgentHost whose native lead snapshots are
// delivered in RAM. The daemon binds it before Start; no wire protocol changes.
type AgentLiveHost interface {
	SetLiveHost(LiveHost)
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

func (s agentSessions) ReadSession(reg Registration) (SessionData, bool, error) {
	// This is an authoritative index read, already coalesced under its lane,
	// not a presentation read. Refresh the daemon's snapshot owner even for
	// leads whose fingerprint uses the inner source's separate cache: one
	// nudge must not disappear behind either owner's presentation TTL.
	s.snaps.Invalidate(s.dir, reg.ReplyID)
	if !reg.Agent {
		s.snaps.snapshotWithoutObserved(s.dir, reg.ReplyID)
		return s.inner.ReadSession(reg)
	}
	if _, err := os.Stat(SessionDir(s.dir, reg.ReplyID)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionData{}, false, nil
		}
		return SessionData{}, false, err
	}
	h := sha256.New()
	stores, updatedAt, staleAt := s.snaps.ReadLiveStoresClock(h, s.dir, reg.ReplyID, s.now(), LiveStoreFingerprint{}, AllLiveStores)
	return SessionData{Fingerprint: hex.EncodeToString(h.Sum(nil)[:16]), LiveStores: stores, SnapshotUpdatedAt: updatedAt, SnapshotStaleAt: staleAt}, true, nil
}

// ReadLiveSession bypasses the inner source's non-live stores for runner
// writes. Legacy/custom sources retain their ordinary read-through behavior.
func (s agentSessions) ReadLiveSession(reg Registration, previous SessionData, stores LiveStores) (SessionData, bool, error) {
	s.snaps.Invalidate(s.dir, reg.ReplyID)
	if !reg.Agent {
		s.snaps.snapshotWithoutObserved(s.dir, reg.ReplyID)
		if live, ok := s.inner.(LiveSessionSource); ok {
			return live.ReadLiveSession(reg, previous, stores)
		}
		return s.inner.ReadSession(reg)
	}
	h := sha256.New()
	previous.LiveStores, previous.SnapshotUpdatedAt, previous.SnapshotStaleAt = s.snaps.ReadLiveStoresClock(h, s.dir, reg.ReplyID, s.now(), previous.LiveStores, stores)
	previous.Fingerprint = hex.EncodeToString(h.Sum(nil)[:16])
	return previous, true, nil
}

func (s agentSessions) ReadPage(reg Registration) (PageContent, bool, error) {
	return s.ReadPageLang(reg, "")
}

func (s agentSessions) ReadPageLang(reg Registration, lang string) (PageContent, bool, error) {
	if reg.Agent {
		return PageContent{}, true, nil
	}
	if ls, ok := s.inner.(LangPageSource); ok {
		return ls.ReadPageLang(reg, lang)
	}
	if ps, ok := s.inner.(PageSource); ok {
		return ps.ReadPage(reg)
	}
	return PageContent{}, true, nil
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

// RecordDashboardAnswer forwards board-answer arrivals for lead sessions.
func (s agentSessions) RecordDashboardAnswer(reg Registration, id string, now time.Time) error {
	if reg.Agent {
		return nil
	}
	if source, ok := s.inner.(DashboardAnswerSource); ok {
		return source.RecordDashboardAnswer(reg, id, now)
	}
	return nil
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
// names tell a send (a new held name), a steer (a name replaced), and a
// drain apart.
func (c *SnapshotCache) HashLiveStores(h io.Writer, stateDir, replyID string, now time.Time) time.Time {
	updatedAt, _ := c.HashLiveStoresClock(h, stateDir, replyID, now)
	return updatedAt
}

// LiveStores names the directories a write can change. Snapshot evidence is
// always refreshed; it needs no directory listing. Unknown writes name all.
type LiveStores uint8

const (
	LiveRing LiveStores = 1 << iota
	LiveInbox
	LiveObserved
	AllLiveStores = LiveRing | LiveInbox | LiveObserved
)

// LiveStoreFingerprint retains directory-name prints, not entry contents.
// It belongs to one indexed session and is copied with its last full read.
type LiveStoreFingerprint struct {
	known LiveStores
	parts [4]string
}

// HashLiveStoresClock returns clock evidence from the same immutable snapshot
// used by the fingerprint, so a concurrent publish cannot mix generations.
func (c *SnapshotCache) HashLiveStoresClock(h io.Writer, stateDir, replyID string, now time.Time) (updatedAt, staleAt time.Time) {
	_, updatedAt, staleAt = c.ReadLiveStoresClock(h, stateDir, replyID, now, LiveStoreFingerprint{}, AllLiveStores)
	return updatedAt, staleAt
}

// ReadLiveStoresClock refreshes only named directories once initialized, folding
// retained prints for the rest. A cold read still initializes every directory.
func (c *SnapshotCache) ReadLiveStoresClock(h io.Writer, stateDir, replyID string, now time.Time, previous LiveStoreFingerprint, stores LiveStores) (LiveStoreFingerprint, time.Time, time.Time) {
	return c.readLiveStoresClock(h, stateDir, replyID, now, previous, stores, nil)
}

// list is an instance-local counting seam; nil performs the ordinary listing.
func (c *SnapshotCache) readLiveStoresClock(h io.Writer, stateDir, replyID string, now time.Time, previous LiveStoreFingerprint, stores LiveStores, list func(string, func() ([]string, error)) ([]string, error)) (LiveStoreFingerprint, time.Time, time.Time) {
	var updatedAt, staleAt time.Time
	sessionDir := SessionDir(stateDir, replyID)
	if p, err := c.read(wslive.SnapshotPath(sessionDir)); err == nil && p.docOK {
		staleAt = snapshotStaleAt(&p.snap)
		fmt.Fprintf(h, "\x00snapshot stale=%t\x00%s", p.snap.Stale(now), p.canonical)
		if p.snap.UpdatedAt > 0 {
			updatedAt = time.UnixMilli(p.snap.UpdatedAt)
		}
	}
	for i, store := range []struct {
		tag   string
		kind  LiveStores
		names func() ([]string, error)
	}{
		{"model-evidence", LiveObserved, func() ([]string, error) {
			entries, err := os.ReadDir(filepath.Join(sessionDir, wslive.ObservedDirName))
			var names []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					names = append(names, e.Name())
				}
			}
			return names, err
		}},
		{"ring", LiveRing, func() ([]string, error) { return wslive.RingNames(wslive.RingDir(sessionDir)) }},
		{"inbox", LiveInbox, func() ([]string, error) { return wsinbox.Names(wsinbox.Dir(sessionDir)) }},
		{"inbox-drained", LiveInbox, func() ([]string, error) { return wsinbox.DrainedNames(wsinbox.Dir(sessionDir)) }},
	} {
		if stores&store.kind != 0 || previous.known&store.kind == 0 {
			namesFn := store.names
			if list != nil {
				namesFn = func() ([]string, error) { return list(store.tag, store.names) }
			}
			previous.parts[i] = ""
			if names, err := namesFn(); err == nil {
				var print strings.Builder
				print.WriteString("\x00" + store.tag)
				for _, name := range names {
					print.WriteString("\x00" + name)
				}
				previous.parts[i] = print.String()
			}
		}
		io.WriteString(h, previous.parts[i])
	}
	previous.known = AllLiveStores
	return previous, updatedAt, staleAt
}
