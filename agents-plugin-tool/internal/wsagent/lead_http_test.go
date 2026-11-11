package wsagent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// viewer is who a request to the runner's routes comes from.
type viewer struct {
	// remote is the peer address, host the name the daemon was addressed
	// by, and origin the Origin header ("" sends none).
	remote, host, origin string
}

var (
	// localBrowser is a page of the daemon's own, open on this machine.
	localBrowser = viewer{remote: "127.0.0.1:50000", host: "127.0.0.1:8123", origin: "http://127.0.0.1:8123"}
	// lanBrowser is the daemon's page on another device (LAN mode): the
	// daemon's own origin, but no loopback peer.
	lanBrowser = viewer{remote: "192.168.0.7:50000", host: "192.168.0.2:8123", origin: "http://192.168.0.2:8123"}
	// localClient is a non-browser client on this machine: no Origin.
	localClient = viewer{remote: "127.0.0.1:50001", host: "127.0.0.1:8123"}
	// reboundBrowser is a loopback peer that addressed the daemon by a
	// name that is not this machine's (a rebound DNS name).
	reboundBrowser = viewer{remote: "127.0.0.1:50002", host: "rebound.example:8123", origin: "http://rebound.example:8123"}
)

// route sends one request to the runner's handler as v.
func (h *leadHarness) route(v viewer, method, path string, body any) reply {
	h.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, "http://"+v.host+path, bytes.NewReader(raw))
	req.RemoteAddr, req.Host = v.remote, v.host
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if v.origin != "" {
		req.Header.Set("Origin", v.origin)
	}
	rec := httptest.NewRecorder()
	h.runner.Handler().ServeHTTP(rec, req)
	return reply{rec.Code, rec.Body.String()}
}

// leadControlWrites are the writes that decide what a lead session may do,
// against the session id with the prompt req-1 waiting.
func leadControlWrites(root, id string) map[string]struct {
	path string
	body any
} {
	return map[string]struct {
		path string
		body any
	}{
		"spawn":           {"/agents/leads", map[string]any{"root": root, "harness": BackendClaude}},
		"permission mode": {"/agents/" + id + "/settings", map[string]any{"permission_mode": "bypassPermissions"}},
		"model":           {"/agents/" + id + "/settings", map[string]any{"model": "opus"}},
		"effort":          {"/agents/" + id + "/settings", map[string]any{"effort": "max"}},
		"stop":            {"/agents/" + id + "/stop", nil},
		"restart":         {"/agents/" + id + "/restart", nil},
		"close":           {"/agents/" + id + "/close", nil},
		"approval":        {"/agents/" + id + "/approvals/req-1", map[string]any{"behavior": "allow"}},
	}
}

// waitingLead is a lead whose turn waits on the prompt req-1.
func (h *leadHarness) waitingLead() (id, holder string) {
	h.t.Helper()
	id, holder = h.runLead("go")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine("u1", "toolu_1"), stateLine(HarnessWaiting), canUseToolLine("req-1", "toolu_1"))
	eventually(h.t, "the prompt", func() bool { return len(h.state(id).Approvals) == 1 })
	return id, holder
}

// untouched fails the test unless the lead still runs its process with its
// prompt waiting and its settings as created, and no second conversation
// exists.
func (h *leadHarness) untouched(id, holder, after string) {
	h.t.Helper()
	st := h.state(id)
	if !st.Process || st.Status != StatusRunning || len(st.Approvals) != 1 || st.Model != "" || st.Effort != "" || st.PermissionMode == "bypassPermissions" {
		h.t.Fatalf("%s: the session changed: %+v", after, st)
	}
	if n := len(h.runner.List()); n != 1 {
		h.t.Fatalf("%s: %d conversations", after, n)
	}
	if h.holders.opCount("stop") != 0 || h.holders.opCount("launch") != 1 || len(h.holders.inputs(holder, "control_response")) != 0 || len(h.holders.inputs(holder, "control_request")) != 0 {
		h.t.Fatalf("%s: the process was reached: holder calls %v, input %q", after, h.holders.opList(), h.holders.inputs(holder, ""))
	}
}

// TestLeadControlRoutesTakeTheTrustedGate: a lead session's spawn, settings
// change, stop, close, and approval answer are refused for a viewer that is
// not on this machine (404, as if no such route) and for one on this
// machine without the daemon's own Origin (403); the same writes from a
// trusted viewer reach the process. Send and interrupt take no such gate.
func TestLeadControlRoutesTakeTheTrustedGate(t *testing.T) {
	h := newLeadHarness(t)
	var local, own bool
	h.opts.Trusted = func(*http.Request) (bool, bool) { return local, own }
	h.start()
	id, holder := h.waitingLead()
	// The gate alone decides here; the requests themselves are alike.
	as := localBrowser

	for name, w := range leadControlWrites(h.root, id) {
		// A viewer with the LAN token: the daemon's own page elsewhere.
		local, own = false, true
		if r := h.route(as, http.MethodPost, w.path, w.body); r.status != http.StatusNotFound {
			t.Fatalf("%s from another device = %d %s, want 404", name, r.status, r.body)
		}
		h.untouched(id, holder, name+" from another device")
		local, own = true, false
		if r := h.route(as, http.MethodPost, w.path, w.body); r.status != http.StatusForbidden {
			t.Fatalf("%s without the daemon's origin = %d %s, want 403", name, r.status, r.body)
		}
		h.untouched(id, holder, name+" without the daemon's origin")
	}

	// A lead profile is not opened as an agent conversation, by anyone.
	for _, gate := range [][2]bool{{false, true}, {true, true}} {
		local, own = gate[0], gate[1]
		if r := h.route(as, http.MethodPost, "/agents", map[string]any{"profile": LeadProfileClaude, "root": h.root}); r.status != http.StatusForbidden {
			t.Fatalf("POST /agents with a lead profile (local %v) = %d %s, want 403", local, r.status, r.body)
		}
	}
	h.untouched(id, holder, "POST /agents with a lead profile")

	// Reading, sending, and interrupting are any viewer's; can_control
	// tells the page which viewer it serves.
	local, own = false, true
	r := h.route(as, http.MethodGet, "/agents/"+id, nil)
	if st := decodeState(t, r); r.status != http.StatusOK || !st.Lead || st.CanControl || len(st.Approvals) != 1 {
		t.Fatalf("GET from another device = %d %+v", r.status, st)
	}
	if r := h.route(as, http.MethodGet, "/agents", nil); r.status != http.StatusOK || strings.Contains(r.body, `"can_control":true`) {
		t.Fatalf("list from another device = %d %s", r.status, r.body)
	}
	r = h.route(as, http.MethodPost, "/agents/"+id+"/send", map[string]any{"text": "from the other room"})
	if st := decodeState(t, r); r.status != http.StatusOK || st.Held != 1 || st.CanControl {
		t.Fatalf("send from another device = %d %s", r.status, r.body)
	}
	r = h.route(as, http.MethodPost, "/agents/"+id+"/interrupt", nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "from the other room") {
		t.Fatalf("interrupt from another device = %d %s", r.status, r.body)
	}
	if got := h.holders.inputs(holder, "control_request"); len(got) != 1 || !strings.Contains(got[0], `"interrupt"`) {
		t.Fatalf("the interrupt should reach the process: %q", got)
	}

	// A trusted viewer's writes reach the process.
	local, own = true, true
	r = h.route(as, http.MethodGet, "/agents/"+id, nil)
	if st := decodeState(t, r); !st.CanControl {
		t.Fatalf("GET from this machine = %d %+v", r.status, st)
	}
	if r := h.route(as, http.MethodGet, "/agents", nil); !strings.Contains(r.body, `"can_control":true`) {
		t.Fatalf("list from this machine = %s", r.body)
	}
	if r := h.route(as, http.MethodPost, "/agents/"+id+"/approvals/req-1", map[string]any{"behavior": "maybe"}); r.status != http.StatusBadRequest {
		t.Fatalf("an answer that is neither allow nor deny = %d %s", r.status, r.body)
	}
	r = h.route(as, http.MethodPost, "/agents/"+id+"/approvals/req-1", map[string]any{"behavior": "deny", "message": "not this one"})
	if st := decodeState(t, r); r.status != http.StatusOK || len(st.Approvals) != 0 || !st.CanControl {
		t.Fatalf("approval answer = %d %s", r.status, r.body)
	}
	answers := h.holders.inputs(holder, "control_response")
	if len(answers) != 1 || !sameJSON(t, json.RawMessage(answers[0]), []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req-1","response":{"behavior":"deny","message":"not this one"}}}`)) {
		t.Fatalf("answers written = %q", answers)
	}
	if r := h.route(as, http.MethodPost, "/agents/"+id+"/approvals/req-1", map[string]any{"behavior": "allow"}); r.status != http.StatusConflict {
		t.Fatalf("a second answer = %d %s, want 409", r.status, r.body)
	}
	h.holders.out(holder, stateLine(HarnessRunning), replyLine("u2", "ok"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)

	r = h.route(as, http.MethodPost, "/agents/"+id+"/settings", map[string]any{"model": "opus", "effort": "max", "permission_mode": "plan"})
	if st := decodeState(t, r); r.status != http.StatusOK || st.Model != "opus" || st.Effort != "max" || st.PermissionMode != "plan" || !st.Process {
		t.Fatalf("settings = %d %s", r.status, r.body)
	}
	if got := h.controls(holder); len(got) != 4 {
		t.Fatalf("the changes go to the running process: %v", got)
	}
	if r := h.route(as, http.MethodPost, "/agents/"+id+"/settings", map[string]any{"permission_mode": "root"}); r.status != http.StatusBadRequest {
		t.Fatalf("an unknown permission mode = %d %s", r.status, r.body)
	}
	r = h.route(as, http.MethodPost, "/agents/"+id+"/stop", nil)
	if st := decodeState(t, r); r.status != http.StatusOK || st.Process || h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("stop = %d %s, holder calls %v", r.status, r.body, h.holders.opList())
	}
	r = h.route(as, http.MethodPost, "/agents/"+id+"/close", nil)
	if st := decodeState(t, r); r.status != http.StatusOK || st.Status != StatusClosed {
		t.Fatalf("close = %d %s", r.status, r.body)
	}
	// A closed lead's routes keep their gate.
	local, own = false, true
	if r := h.route(as, http.MethodPost, "/agents/"+id+"/stop", nil); r.status != http.StatusNotFound {
		t.Fatalf("stop of a closed lead from another device = %d %s, want 404", r.status, r.body)
	}

	local, own = true, true
	r = h.route(as, http.MethodPost, "/agents/leads", map[string]any{"root": h.root, "harness": BackendPi, "model": "provider/some-model", "effort": "high", "permission_mode": "plan"})
	if st := decodeState(t, r); r.status != http.StatusCreated || !st.Lead || st.Backend != BackendPi || st.Model != "provider/some-model" || st.Effort != "high" || st.PermissionMode != "" || !st.CanControl {
		t.Fatalf("spawn = %d %s", r.status, r.body)
	}
	if r := h.route(as, http.MethodPost, "/agents/leads", map[string]any{"root": h.root, "harness": "codex"}); r.status != http.StatusBadRequest {
		t.Fatalf("spawn on an unknown harness = %d %s", r.status, r.body)
	}
}

// TestLeadControlRoutesThroughTheDaemonsGate runs the same routes through
// the gate a runner has when none is injected: the dashboard's own, which
// takes a loopback peer that addressed the daemon by a loopback name and
// sent the daemon's own Origin.
func TestLeadControlRoutesThroughTheDaemonsGate(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.waitingLead()

	for name, w := range leadControlWrites(h.root, id) {
		for _, v := range []viewer{lanBrowser, reboundBrowser} {
			if r := h.route(v, http.MethodPost, w.path, w.body); r.status != http.StatusNotFound {
				t.Fatalf("%s from %s = %d %s, want 404", name, v.host, r.status, r.body)
			}
			h.untouched(id, holder, name+" from "+v.host)
		}
		if r := h.route(localClient, http.MethodPost, w.path, w.body); r.status != http.StatusForbidden {
			t.Fatalf("%s with no Origin = %d %s, want 403", name, r.status, r.body)
		}
		h.untouched(id, holder, name+" with no Origin")
		other := localBrowser
		other.origin = "http://other.example"
		if r := h.route(other, http.MethodPost, w.path, w.body); r.status != http.StatusForbidden {
			t.Fatalf("%s from another page = %d %s, want 403", name, r.status, r.body)
		}
		h.untouched(id, holder, name+" from another page")
	}

	if st := decodeState(t, h.route(lanBrowser, http.MethodGet, "/agents/"+id, nil)); st.CanControl || !st.Lead {
		t.Fatalf("GET from another device: %+v", st)
	}
	if st := decodeState(t, h.route(localBrowser, http.MethodGet, "/agents/"+id, nil)); !st.CanControl {
		t.Fatalf("GET from this machine: %+v", st)
	}
	if r := h.route(lanBrowser, http.MethodPost, "/agents/"+id+"/send", map[string]any{"text": "from the other room"}); r.status != http.StatusOK {
		t.Fatalf("send from another device = %d %s", r.status, r.body)
	}

	r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/approvals/req-1", map[string]any{"behavior": "allow"})
	if st := decodeState(t, r); r.status != http.StatusOK || len(st.Approvals) != 0 {
		t.Fatalf("approval answer from this machine = %d %s", r.status, r.body)
	}
	if got := h.holders.inputs(holder, "control_response"); len(got) != 1 {
		t.Fatalf("answers written = %q", got)
	}
	r = h.route(localBrowser, http.MethodPost, "/agents/leads", map[string]any{"root": h.root, "harness": BackendClaude, "permission_mode": "plan"})
	if st := decodeState(t, r); r.status != http.StatusCreated || !st.Lead || st.PermissionMode != "plan" || !st.CanControl {
		t.Fatalf("spawn from this machine = %d %s", r.status, r.body)
	}
	r = h.route(localBrowser, http.MethodPost, "/agents/"+id+"/stop", nil)
	if st := decodeState(t, r); r.status != http.StatusOK || st.Process || h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("stop from this machine = %d %s", r.status, r.body)
	}
}
