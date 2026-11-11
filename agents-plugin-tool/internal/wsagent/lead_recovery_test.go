package wsagent

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

const wsServer = "plugin:ws:ws"

// mcpInitLine is a turn's init line reporting the ws server's status.
func mcpInitLine(status string) string {
	return jsonLine(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-1", "permissionMode": "auto",
		"mcp_servers": []map[string]any{{"name": wsServer, "status": status, "source": "plugin"}}})
}

// mcpStatusBody is an mcp_status answer's payload with the ws server's status.
func mcpStatusBody(status string) string {
	return fmt.Sprintf(`{"mcpServers":[{"name":%q,"status":%q,"scope":"dynamic"}]}`, wsServer, status)
}

// mcpTurn is a whole turn whose init reports the ws server's status.
func mcpTurn(uuid, status string) []string {
	return []string{stateLine(HarnessRunning), mcpInitLine(status), replyLine(uuid, "ok"), resultLine(), stateLine(HarnessIdle)}
}

// controlSubtypes are the subtypes of the control requests written to the
// holder's child, oldest first.
func (h *leadHarness) controlSubtypes(holder string) []string {
	var out []string
	for _, c := range h.controls(holder) {
		m, _ := c.(map[string]any)
		s, _ := m["subtype"].(string)
		out = append(out, s)
	}
	return out
}

// TestLeadMCPReconnectsInStream: a server the harness reports failed is
// reconnected with the mcp_reconnect control and checked with mcp_status;
// when it is back, nothing restarts and the session shows nothing broken.
func TestLeadMCPReconnectsInStream(t *testing.T) {
	h := newLeadHarness(t).start()
	h.holders.setControl("mcp_status", "", mcpStatusBody(MCPConnected))
	id, holder := h.runLead("one")
	h.holders.out(holder, mcpTurn("u1", MCPConnected)...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, mcpTurn("u2", MCPFailed)...)
	eventually(t, "the reconnect and the check", func() bool { return len(h.controlSubtypes(holder)) == 2 })
	h.sync(id, holder)
	if got := h.controlSubtypes(holder); !slices.Equal(got, []string{"mcp_reconnect", "mcp_status"}) {
		t.Fatalf("controls = %v", got)
	}
	if got := h.controls(holder)[0].(map[string]any)["serverName"]; got != wsServer {
		t.Fatalf("reconnected %v", got)
	}
	if st := h.state(id); len(st.MCPBroken) != 0 || !st.Process || h.holders.opCount("stop") != 0 || h.holders.opCount("launch") != 1 {
		t.Fatalf("a reconnect that worked restarts nothing: %+v, holder calls %v", st, h.holders.opList())
	}
	if host := h.snapshot(id).Host; host != nil && len(host.MCPBroken) != 0 {
		t.Fatalf("snapshot host = %+v", host)
	}
}

// TestLeadMCPRestartsAtTheFirstIdlePoint: a server the process had
// connected that does not come back (the reconnect answers an error, or
// the status still says failed) shows the session MCP-broken; while a
// background task runs the process keeps running, and it restarts on the
// same backend session at the first idle point with the list empty.
func TestLeadMCPRestartsAtTheFirstIdlePoint(t *testing.T) {
	for _, tc := range []struct{ name, refuse, status string }{
		{name: "error answer", refuse: "Failed to reconnect to " + wsServer, status: MCPFailed},
		{name: "still failed", status: MCPFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLeadHarness(t).start()
			h.holders.setControl("mcp_reconnect", tc.refuse, "")
			h.holders.setControl("mcp_status", "", mcpStatusBody(tc.status))
			id, holder := h.runLead("one")
			h.holders.out(holder, mcpTurn("u1", MCPConnected)...)
			h.idle(id)
			h.send(id, "two")
			h.holders.out(holder, stateLine(HarnessRunning), mcpInitLine(MCPFailed), tasksLine(1), replyLine("u2", "started"), resultLine(), stateLine(HarnessIdle))
			eventually(t, "the broken state", func() bool { return slices.Equal(h.state(id).MCPBroken, []string{wsServer}) })
			h.idle(id)
			eventually(t, "the broken state in the snapshot", func() bool {
				host := h.snapshot(id).Host
				return host != nil && slices.Equal(host.MCPBroken, []string{wsServer}) && host.BackgroundTasks == 1
			})
			time.Sleep(100 * time.Millisecond)
			if st := h.state(id); !st.Process || st.BackgroundTasks != 1 || h.holders.opCount("stop") != 0 {
				t.Fatalf("a background task keeps the process: %+v, holder calls %v", st, h.holders.opList())
			}
			sessionBefore := h.record(id).SessionID
			h.holders.out(holder, tasksLine(0))
			eventually(t, "the restart", func() bool { return h.holders.opCount("stop "+holder) == 1 && h.holders.opCount("launch") == 2 })
			eventually(t, "the new process", func() bool { st := h.state(id); return st.Process && h.holder(id) != holder })
			st := h.state(id)
			if len(st.MCPBroken) != 0 || st.Error != "" || st.Status != StatusIdle {
				t.Fatalf("after the restart: %+v", st)
			}
			if spec := h.holders.spawned(h.holder(id)); !slices.Contains(spec.Argv, "--resume") || !slices.Contains(spec.Argv, sessionBefore) {
				t.Fatalf("the restart resumes the session: %v", spec.Argv)
			}
		})
	}
}

// TestLeadMCPServerBackRestartsNothing: a reconnect that failed while a
// background task ran defers the restart; when the server is reported
// connected again before the task list empties, the empty list restarts
// nothing.
func TestLeadMCPServerBackRestartsNothing(t *testing.T) {
	h := newLeadHarness(t).start()
	h.holders.setControl("mcp_reconnect", "Failed to reconnect", "")
	id, holder := h.runLead("one")
	h.holders.out(holder, mcpTurn("u1", MCPConnected)...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), mcpInitLine(MCPFailed), tasksLine(1), replyLine("u2", "started"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the broken state", func() bool { return slices.Equal(h.state(id).MCPBroken, []string{wsServer}) })
	h.idle(id)
	h.send(id, "three")
	h.holders.out(holder, mcpTurn("u3", MCPConnected)...)
	eventually(t, "the server back", func() bool { return len(h.state(id).MCPBroken) == 0 })
	h.idle(id)
	h.holders.out(holder, tasksLine(0))
	eventually(t, "the empty task list", func() bool { return h.state(id).BackgroundTasks == 0 })
	h.sync(id, holder)
	if st := h.state(id); !st.Process || h.holders.opCount("stop") != 0 || h.holders.opCount("launch") != 1 {
		t.Fatalf("a server that came back restarts nothing: %+v, holder calls %v", st, h.holders.opList())
	}
}

// TestLeadMCPRestartsOnceForAServerThatNeverConnects: a server that failed
// from the process's start restarts it once at an idle point; when it
// fails again in the new process without having connected, nothing
// restarts on its own (a restart loop), the session shows it broken, and
// a restart from this machine starts a new process on the same session,
// which a viewer on another device cannot do.
func TestLeadMCPRestartsOnceForAServerThatNeverConnects(t *testing.T) {
	h := newLeadHarness(t)
	var local, own bool
	h.opts.Trusted = func(*http.Request) (bool, bool) { return local, own }
	h.start()
	h.holders.setControl("mcp_reconnect", "Failed to reconnect", "")
	id, holder := h.runLead("one")
	h.holders.out(holder, mcpTurn("u1", MCPFailed)...)
	eventually(t, "the automatic restart", func() bool { return h.holders.opCount("stop "+holder) == 1 && h.holders.opCount("launch") == 2 })
	eventually(t, "the new process", func() bool { return h.state(id).Process && h.holder(id) != holder })
	second := h.holder(id)
	h.send(id, "two")
	h.holders.out(second, mcpTurn("u2", MCPFailed)...)
	eventually(t, "the broken state", func() bool { return slices.Equal(h.state(id).MCPBroken, []string{wsServer}) })
	h.idle(id)
	h.sync(id, second)
	time.Sleep(100 * time.Millisecond)
	if st := h.state(id); !st.Process || h.holders.opCount("stop") != 1 {
		t.Fatalf("a server that failed again without connecting restarts nothing more: %+v, holder calls %v", st, h.holders.opList())
	}
	local, own = false, true
	if r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/restart", nil); r.status != http.StatusNotFound || h.holders.opCount("stop") != 1 {
		t.Fatalf("restart from another device = %d %s, holder calls %v", r.status, r.body, h.holders.opList())
	}
	local, own = true, true
	r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/restart", nil)
	st := decodeState(t, r)
	if r.status != http.StatusOK || !st.Process || len(st.MCPBroken) != 0 || h.holders.opCount("stop "+second) != 1 || h.holders.opCount("launch") != 3 {
		t.Fatalf("restart = %d %s, holder calls %v", r.status, r.body, h.holders.opList())
	}
}

// TestLeadFailureState: a typed failure is a notice line and the session's
// failure state, in its state and its snapshot, until a turn ends with no
// failure; a retry shows until the turn's output resumes.
func TestLeadFailureState(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	retry := jsonLine(map[string]any{"type": "system", "subtype": "api_retry", "attempt": 1, "max_retries": 10, "error_status": 529, "error": "overloaded"})
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), retry)
	eventually(t, "the retry in the snapshot", func() bool {
		host := h.snapshot(id).Host
		return host != nil && host.Retry != nil && host.Retry.Attempt == 1 && host.Retry.Status == 529
	})
	login := failureLine(map[string]any{"error": "authentication_failed"}, "Not logged in · Please run /login")
	h.holders.out(holder, login, jsonLine(map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": "Not logged in · Please run /login"}), stateLine(HarnessIdle))
	h.idle(id)
	st := h.state(id)
	if st.Failure == nil || st.Failure.Kind != wslive.NoticeLogin {
		t.Fatalf("failure state = %+v", st)
	}
	host := h.snapshot(id).Host
	if host == nil || host.Failure == nil || host.Failure.Kind != wslive.NoticeLogin || host.Retry != nil {
		t.Fatalf("snapshot host = %+v", host)
	}
	var notices []wslive.Notice
	for _, e := range h.ring(id) {
		if e.Kind == wslive.RingNotice {
			notices = append(notices, *e.Notice)
		}
		if e.Kind == wslive.RingReply && strings.Contains(e.Text, "/login") {
			t.Fatalf("the failure's text is no reply: %+v", e)
		}
	}
	if len(notices) != 1 || notices[0].Text != "Not logged in · Please run /login" {
		t.Fatalf("notice lines = %+v", notices)
	}
	h.send(id, "two")
	h.holders.out(holder, turnLines("sess-1", "u2", "fine")...)
	eventually(t, "the failure to clear", func() bool { return h.state(id).Failure == nil })
	eventually(t, "the snapshot's failure to clear", func() bool { host := h.snapshot(id).Host; return host == nil || host.Failure == nil })
}
