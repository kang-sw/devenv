package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// AgentRoutePrefix is where the daemon mounts AgentHost.Handler.
const AgentRoutePrefix = "/agents"

// agentSessions is the daemon's session source: agent registrations are
// read from their live stores alone (no ws-mcp serves them and they have no
// session record), and every other one from the configured source.
type agentSessions struct {
	inner SessionSource
	dir   string
	now   func() time.Time
}

func (s agentSessions) ReadSession(reg Registration) (SessionData, bool) {
	if !reg.Agent {
		return s.inner.ReadSession(reg)
	}
	sessionDir := SessionDir(s.dir, reg.ReplyID)
	if _, err := os.Stat(sessionDir); err != nil {
		return SessionData{}, false
	}
	h := sha256.New()
	updatedAt := hashLiveStores(h, sessionDir, s.now())
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

// hashLiveStores folds a session's live stores into h, as ws-mcp's source
// does for a lead session: the snapshot without updatedAt but with its
// staleness, and the ring and inbox entry names (entry files are never
// rewritten). It returns the snapshot's updatedAt.
func hashLiveStores(h io.Writer, sessionDir string, now time.Time) time.Time {
	var updatedAt time.Time
	if raw, err := os.ReadFile(wslive.SnapshotPath(sessionDir)); err == nil {
		var doc map[string]any
		var snap wslive.Snapshot
		if json.Unmarshal(raw, &doc) == nil && json.Unmarshal(raw, &snap) == nil {
			delete(doc, "updatedAt")
			canonical, _ := json.Marshal(doc)
			fmt.Fprintf(h, "\x00snapshot stale=%t\x00%s", snap.Stale(now), canonical)
			if snap.UpdatedAt > 0 {
				updatedAt = time.UnixMilli(snap.UpdatedAt)
			}
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
