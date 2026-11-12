package mcp

import (
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wslive"
)

// pi_live.go feeds a lead's Pi subagents (pi_tools.go) into the agents
// panel of the lead's own live state: a row per child, marked with
// AgentKindPi and linked to the child's conversation page. The server
// subscribes to the daemon runner's spawned conversations and applies each
// change of a child whose parent is one of its live sessions; a live
// session it creates, and one an adapter reset, gets its open children's
// rows from the runner, so the rows come back after a reconnect. A session
// whose dashboard feed is off has no live session here, so no row.

// startPiFeed subscribes the server to the runner's spawned conversations;
// Server.Close ends it.
func (s *Server) startPiFeed() {
	if s.pi.agents == nil {
		return
	}
	s.pi.stopWatch = s.pi.agents.Watch(s.piChanged)
}

// piChanged applies one child's change to its parent's live sessions in
// this server. It runs on the child's conversation loop.
func (s *Server) piChanged(st wsagent.State) {
	if st.Parent == nil || st.Parent.Kind != wsagent.ParentLead {
		return
	}
	sessions := s.live.byReplyID(st.Parent.ReplyID)
	if len(sessions) == 0 {
		return
	}
	ev := s.piRow(st)
	for _, ls := range sessions {
		ls.applyAgentRows([]wslive.Event{ev})
	}
}

// piSeed gives a live session the rows of its open Pi children.
func (s *Server) piSeed(ls *liveSession) {
	if s.pi.agents == nil {
		return
	}
	var rows []wslive.Event
	for _, st := range s.pi.agents.Children(wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: ls.replyID}) {
		if st.Status != wsagent.StatusClosed {
			rows = append(rows, s.piRow(st))
		}
	}
	if len(rows) > 0 {
		ls.applyAgentRows(rows)
	}
}

// piAgentPrefix keeps a child's row id apart from the host's own agent ids.
const piAgentPrefix = "pi:"

// piRow is a child's agent event: running while a turn runs or a message
// waits for one, failed after a failure, else done (a closed child too).
func (s *Server) piRow(st wsagent.State) wslive.Event {
	status := wslive.StatusDone
	switch {
	case st.Status == wsagent.StatusRunning || (st.Status != wsagent.StatusClosed && st.Held > 0):
		status = wslive.StatusRunning
	case st.Error != "":
		status = wslive.StatusFailed
	}
	label := st.Label
	if label == "" {
		label = "Pi subagent " + st.ID
	}
	return wslive.Event{
		Kind:       wslive.KindAgent,
		AgentID:    piAgentPrefix + st.ID,
		AgentKind:  wslive.AgentKindPi,
		Label:      label,
		Status:     status,
		Model:      st.Model,
		Effort:     st.Effort,
		SessionKey: s.piChildKey(st.ID),
	}
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
