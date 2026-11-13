package mcp

import (
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wslive"
)

// pi_live.go feeds a lead's Pi subagents (pi_tools.go) into the agents
// panel of the lead's own live state: a row per child, marked with
// AgentKindPi and linked to the child's conversation page. The server
// subscribes to the daemon runner's spawned conversations and applies each
// change of a descendant owned by one of its live sessions; a live session
// it creates, and one an adapter reset, gets its open spawn tree from the
// runner, so the rows come back after a reconnect. Hosted leads use their
// runner-owned writer instead; their intake remains disabled.

// startPiFeed subscribes the server to the runner's spawned conversations;
// Server.Close ends it.
func (s *Server) startPiFeed() {
	if s.pi.agents == nil {
		return
	}
	s.pi.stopWatch = s.pi.agents.Watch(s.piChanged)
}

// piChanged applies one descendant's change to its owning lead's live sessions
// in this server. It runs on the child's conversation loop.
func (s *Server) piChanged(st wsagent.State) {
	lead := wsagent.SpawnLead(st, s.pi.agents.Get)
	if lead == "" {
		return
	}
	sessions := s.live.byReplyID(lead)
	if len(sessions) == 0 {
		return
	}
	ev := s.piRow(st)
	for _, ls := range sessions {
		ls.applyAgentRows([]wslive.Event{ev})
	}
}

// piSeed gives a live session the rows of its open Pi spawn tree.
func (s *Server) piSeed(ls *liveSession) {
	if s.pi.agents == nil {
		return
	}
	var rows []wslive.Event
	for _, st := range wsagent.SpawnTree(wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: ls.replyID}, s.pi.agents.Children) {
		if st.Status != wsagent.StatusClosed {
			rows = append(rows, s.piRow(st))
		}
	}
	if len(rows) > 0 {
		ls.applyAgentRows(rows)
	}
}

// piRow is a child's agent event: running while a turn runs or a message
// waits for one, failed after a failure, else done (a closed child too).
func (s *Server) piRow(st wsagent.State) wslive.Event {
	return wsagent.SpawnAgentEvent(st, s.piChildKey(st.ID))
}

// piChildKey is a child's ws session key, which labels its row with the
// child's reply-id (its conversation page); "" when its record cannot be
// read, which leaves the row unlinked.
func (s *Server) piChildKey(id string) string {
	s.pi.keyMu.Lock()
	key, ok := s.pi.keys[id]
	s.pi.keyMu.Unlock()
	if ok {
		return key
	}
	dir, err := s.dashboard.stateDir()
	if err != nil {
		return ""
	}
	rec, err := wsagent.ReadRecord(dir, id)
	if err != nil || rec.WSKey == "" {
		return ""
	}
	s.pi.keyMu.Lock()
	if s.pi.keys == nil {
		s.pi.keys = map[string]string{}
	}
	s.pi.keys[id] = rec.WSKey
	s.pi.keyMu.Unlock()
	return rec.WSKey
}

// byReplyID is the hub's running live sessions of reply-id replyID.
func (h *liveHub) byReplyID(replyID string) []*liveSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*liveSession
	for _, ls := range h.sessions {
		if ls.replyID == replyID {
			out = append(out, ls)
		}
	}
	return out
}

// applyAgentRows applies agent events the server derived itself and
// schedules the snapshot write as a subagent's change.
func (ls *liveSession) applyAgentRows(events []wslive.Event) {
	seams := currentLiveSeams()
	now := seams.now().UnixMilli()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.stopped {
		return
	}
	changed := false
	for _, ev := range events {
		res, err := ls.state.Apply(ev, now)
		if err != nil {
			ls.debug("dashboard.pi_row_error", map[string]any{"agent": ev.AgentID, "error": err.Error()})
			continue
		}
		changed = changed || res.Changed
	}
	if changed {
		ls.dirty = true
		ls.scheduleLocked(seams.debounceAgent)
	}
}
