package wsagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// leadSessions is a session source with no lead sessions.
type leadSessions struct{}

func (leadSessions) ReadSession(wsdashboard.Registration) (wsdashboard.SessionData, bool) {
	return wsdashboard.SessionData{}, false
}

// daemonHarness serves a fresh runner from a real dashboard daemon.
type daemonHarness struct {
	*harness
	base string
}

func newDaemonHarness(t *testing.T, profiles map[string]string) *daemonHarness {
	t.Helper()
	h := newHarness(t, profiles)
	h.runner.Close()
	h.runner = NewRunner(h.config)
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: h.dir, Port: 0, Sessions: leadSessions{}, Agents: h.runner, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return &daemonHarness{harness: h, base: fmt.Sprintf("http://127.0.0.1:%d", d.Endpoint().Port)}
}

type reply struct {
	status int
	body   string
}

func (d *daemonHarness) do(method, path, origin, contentType string, body any) reply {
	d.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, d.base+path, rd)
	if err != nil {
		d.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, string(raw)}
}

func (d *daemonHarness) post(path string, body any) reply {
	return d.do(http.MethodPost, path, "", "application/json", body)
}

func decodeState(t *testing.T, r reply) State {
	t.Helper()
	var st State
	if err := json.Unmarshal([]byte(r.body), &st); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
	return st
}

func TestRESTRoutesEndToEnd(t *testing.T) {
	d := newDaemonHarness(t, map[string]string{"chat": chatProfile, "debug": `{"backend":"claude","list_in_index":true}`})

	r := d.post("/agents", map[string]string{"profile": "chat", "root": d.root})
	if r.status != http.StatusCreated {
		t.Fatalf("create: %d %s", r.status, r.body)
	}
	id := decodeState(t, r).ID
	if strings.Contains(r.body, "ws_session_key") || strings.Contains(r.body, "fake-agent-key") {
		t.Fatalf("no route may reveal the ws session key: %s", r.body)
	}
	listed := decodeState(t, d.post("/agents", map[string]string{"profile": "debug", "root": d.root})).ID

	// The index page lists only the profile that asks for it; both pages
	// are served.
	eventually(t, "index", func() bool {
		return strings.Contains(d.do(http.MethodGet, "/", "", "", nil).body, listed)
	})
	if strings.Contains(d.do(http.MethodGet, "/", "", "", nil).body, id) {
		t.Fatalf("an unlisted conversation must stay off the index page")
	}
	for _, page := range []string{id, listed} {
		if got := d.do(http.MethodGet, "/s/"+page, "", "", nil); got.status != http.StatusOK {
			t.Fatalf("page /s/%s: %d", page, got.status)
		}
	}

	if r := d.post("/agents/"+id+"/send", map[string]string{"text": "hello"}); r.status != http.StatusOK {
		t.Fatalf("send: %d %s", r.status, r.body)
	}
	eventually(t, "start", func() bool { return d.backend.startCount() == 1 })
	p := d.backend.proc(0)
	if got := p.nextSent(t); got != "hello" {
		t.Fatalf("turn = %q", got)
	}
	if st := decodeState(t, d.do(http.MethodGet, "/agents/"+id, "", "", nil)); st.Status != StatusRunning {
		t.Fatalf("state = %+v", st)
	}
	p.reply("hi there")
	eventually(t, "idle", func() bool {
		return decodeState(t, d.do(http.MethodGet, "/agents/"+id, "", "", nil)).Status == StatusIdle
	})

	// The session page's own composer reaches the agent too.
	page := d.do(http.MethodPost, "/s/"+id+"/message", d.base, "application/json", map[string]string{"text": "from the page"})
	if page.status != http.StatusOK {
		t.Fatalf("page message: %d %s", page.status, page.body)
	}
	if got := p.nextSent(t); got != "from the page" {
		t.Fatalf("page turn = %q", got)
	}

	if r := d.post("/agents/"+id+"/send", map[string]string{"text": "queued"}); decodeState(t, r).Held != 1 {
		t.Fatalf("a send during a turn is held: %s", r.body)
	}
	r = d.post("/agents/"+id+"/interrupt", nil)
	var ia struct {
		Returned []string `json:"returned"`
		Held     int      `json:"held"`
	}
	if err := json.Unmarshal([]byte(r.body), &ia); err != nil || r.status != http.StatusOK || len(ia.Returned) != 1 || ia.Returned[0] != "queued" || ia.Held != 0 {
		t.Fatalf("interrupt: %d %s", r.status, r.body)
	}
	p.events <- Event{TurnEnd: true}

	if r := d.post("/agents/"+id+"/settings", map[string]string{"model": "sonnet", "effort": "high"}); r.status != http.StatusOK || decodeState(t, r).Model != "sonnet" {
		t.Fatalf("settings: %d %s", r.status, r.body)
	}
	if r := d.post("/agents/"+id+"/stop", nil); r.status != http.StatusOK || decodeState(t, r).Process {
		t.Fatalf("stop: %d %s", r.status, r.body)
	}
	if r := d.post("/agents/"+id+"/close", nil); r.status != http.StatusOK || decodeState(t, r).Status != StatusClosed {
		t.Fatalf("close: %d %s", r.status, r.body)
	}
	if r := d.post("/agents/"+id+"/send", map[string]string{"text": "late"}); r.status != http.StatusConflict {
		t.Fatalf("send after close: %d %s", r.status, r.body)
	}
	var list struct {
		Conversations []State `json:"conversations"`
	}
	if err := json.Unmarshal([]byte(d.do(http.MethodGet, "/agents", "", "", nil).body), &list); err != nil || len(list.Conversations) != 2 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if r := d.do(http.MethodGet, "/agents/nosuch", "", "", nil); r.status != http.StatusNotFound {
		t.Fatalf("unknown id: %d", r.status)
	}
	if r := d.post("/agents", map[string]string{"profile": "absent", "root": d.root}); r.status != http.StatusBadRequest || !strings.Contains(r.body, "unknown agent profile") {
		t.Fatalf("unknown profile: %d %s", r.status, r.body)
	}
}

func TestRESTWritesRefuseOtherOrigins(t *testing.T) {
	d := newDaemonHarness(t, map[string]string{"chat": chatProfile})
	body := map[string]string{"profile": "chat", "root": d.root}
	for name, r := range map[string]reply{
		"foreign origin": d.do(http.MethodPost, "/agents", "http://evil.example", "application/json", body),
		"null origin":    d.do(http.MethodPost, "/agents", "null", "application/json", body),
		"text body":      d.do(http.MethodPost, "/agents", "", "text/plain", body),
		"no type":        d.do(http.MethodPost, "/agents", "", "", body),
	} {
		if r.status != http.StatusForbidden {
			t.Errorf("%s: %d %s, want 403", name, r.status, r.body)
		}
	}
	if r := d.do(http.MethodPost, "/agents", d.base, "application/json", body); r.status != http.StatusCreated {
		t.Fatalf("the dashboard's own origin may create: %d %s", r.status, r.body)
	}
	if r := d.do(http.MethodDelete, "/agents", "", "", nil); r.status != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE: %d", r.status)
	}
	req, _ := http.NewRequest(http.MethodGet, d.base+"/agents", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("the daemon's host guard should cover the agent routes")
	}
}
