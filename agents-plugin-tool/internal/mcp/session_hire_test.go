package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func hireToolCall(t *testing.T, s *Server, name string, args map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.callTool(context.Background(), request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Params: raw})
	raw, err = json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestHiringMCPGatesAndStaticTools(t *testing.T) {
	f := newLiveFixture(t, true)
	s := f.server
	listed := func() string { raw, _ := json.Marshal(s.filteredTools()); return string(raw) }
	before := listed()
	for _, name := range []string{"session.request_hire", "session.roster", "session.children"} {
		if !strings.Contains(before, `"name":"`+name+`"`) {
			t.Fatalf("missing %s", name)
		}
	}
	base := map[string]any{"reason": "parallel task", "recovery_context": "recover request"}
	for _, test := range []struct {
		name   string
		role   toolRole
		parent string
	}{
		{"worker", roleLead, f.keyA}, {"delegate", roleDelegate, f.keyA}, {"leaf", roleLeaf, f.keyA},
	} {
		key, err := s.sessions.mint(f.root, test.role, test.parent)
		if err != nil {
			t.Fatal(err)
		}
		args := map[string]any{"session_key": key, "reason": base["reason"], "recovery_context": base["recovery_context"]}
		got := hireToolCall(t, s, "session.request_hire", args)
		if !strings.Contains(got, `"error"`) && !strings.Contains(got, `"isError":true`) {
			t.Fatalf("%s admitted: %s", test.name, got)
		}
	}
	for _, key := range []string{"", "not-a-key"} {
		got := hireToolCall(t, s, "session.request_hire", map[string]any{"session_key": key, "reason": "why", "recovery_context": "recover"})
		if !strings.Contains(got, `"isError":true`) {
			t.Fatalf("keyless admitted: %s", got)
		}
	}
	// The model cannot submit a flag mutation via a request argument or tool.
	got := hireToolCall(t, s, "session.request_hire", map[string]any{"session_key": f.keyA, "reason": "why", "recovery_context": "recover", "enabled": true})
	if !strings.Contains(got, `"isError":true`) || !strings.Contains(got, "enabled") {
		t.Fatalf("flag argument admitted: %s", got)
	}
	if strings.Contains(before, `"name":"session.set_hiring"`) {
		t.Fatal("model-facing flag writer")
	}
	// A clerk/support profile remains allowlist-gated even with a lead key.
	t.Setenv(envAllowedTools, "git.status")
	got = hireToolCall(t, s, "session.request_hire", map[string]any{"session_key": f.keyA, "reason": "why", "recovery_context": "recover"})
	if !strings.Contains(got, "tool not available") {
		t.Fatalf("clerk allowlist bypass: %s", got)
	}
	t.Setenv(envAllowedTools, "")
	if before != listed() {
		t.Fatal("flag eligibility changed the static tool list")
	}
}

func TestHiringMCPRequestOpensReplyAndRosterProxy(t *testing.T) {
	f := newLiveFixture(t, true)
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: f.stateDir, Sessions: NewDashboardSessionSource(), PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f.server.live.host = d
	// The login helper ends its stdio connection; revive the terminal rows to
	// model a connected caller without starting a real harness or MCP server.
	for _, id := range []string{f.replyA, f.replyB} {
		reg := wsdashboard.Registration{SchemaVersion: 1, ReplyID: id, Project: f.root, Harness: "pi", RegisteredAt: time.Now().UTC().Format(time.RFC3339), HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
		if err = wsdashboard.WriteRegistration(f.stateDir, reg); err != nil {
			t.Fatal(err)
		}
	}
	d.Index().Refresh()
	req := map[string]any{"session_key": f.keyA, "reason": "parallel task", "recovery_context": "resume after compaction"}
	got := hireToolCall(t, f.server, "session.request_hire", req)
	if !strings.Contains(got, "hiring is not enabled for this session") {
		t.Fatalf("unflagged tool: %s", got)
	}
	flag := httptest.NewRequest("POST", "http://127.0.0.1/s/"+f.replyA+"/hiring", strings.NewReader(`{"enabled":true}`))
	flag.RemoteAddr = "127.0.0.1:1234"
	flag.Header.Set("Origin", "http://127.0.0.1")
	flag.AddCookie(dashboardCookieForTest(t, d))
	res := httptest.NewRecorder()
	d.Handler().ServeHTTP(res, flag)
	if res.Code != 200 {
		t.Fatalf("enable: %d %s", res.Code, res.Body)
	}
	got = hireToolCall(t, f.server, "session.request_hire", req)
	if strings.Contains(got, `"isError":true`) || !strings.Contains(got, "outcome will arrive by mail") {
		t.Fatalf("request: %s", got)
	}
	var requestID string
	for _, line := range strings.Split(toolText(t, got), "\n") {
		if strings.HasPrefix(line, "request_id: ") {
			requestID = strings.TrimPrefix(line, "request_id: ")
		}
	}
	if requestID == "" {
		t.Fatalf("no request id: %s", got)
	}
	reject := httptest.NewRequest("POST", "http://127.0.0.1/session-hire/"+requestID+"/reject", strings.NewReader(`{"reason":"owner rejected"}`))
	reject.RemoteAddr = "127.0.0.1:1234"
	reject.Header.Set("Origin", "http://127.0.0.1")
	reject.AddCookie(dashboardCookieForTest(t, d))
	res = httptest.NewRecorder()
	d.Handler().ServeHTTP(res, reject)
	if res.Code != 200 {
		t.Fatalf("reject: %d %s", res.Code, res.Body)
	}
	badge := hireToolCall(t, f.server, "runtime.read", map[string]any{"session_key": f.keyA})
	if !strings.Contains(badge, "unread 1") {
		t.Fatalf("outcome not surfaced by piggyback: %s", badge)
	}
	received := hireToolCall(t, f.server, "mailbox.recv", map[string]any{"session_key": f.keyA})
	if !strings.Contains(received, "resume after compaction") || !strings.Contains(received, "rejected") {
		t.Fatalf("outcome not receivable: %s", received)
	}
	rec, ok := f.server.sessions.readState(f.keyA, partKey)
	if !ok || !rec.MailboxReplyOpened {
		t.Fatal("request did not open reply channel")
	}
	path, err := f.server.mailboxPaths().ReplyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsmailbox.LoadReplyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok = store.Entries[f.replyA]; !ok {
		t.Fatal("request did not publish env-less reply-id")
	}
	got = hireToolCall(t, f.server, "session.roster", map[string]any{"session_key": f.keyA, "format": "json"})
	if strings.Contains(got, `"isError":true`) || !strings.Contains(got, "[]") {
		t.Fatalf("roster proxy: %s", got)
	}
	// It is a read-only route: a POST cannot turn a roster read into control.
	post, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(d.Endpoint().Port)+"/session-hire/roster", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	post.Header.Set("Content-Type", "application/json")
	post.AddCookie(dashboardCookieForTest(t, d))
	resp, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("roster POST: %d", resp.StatusCode)
	}
	// Existing records remain ordinary typed records: no hiring flag, parent,
	// or grouping field was added to registration.json by these calls.
	raw, err := os.ReadFile(wsdashboard.RegistrationPath(f.stateDir, f.replyA))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"hiring", "hired_by"} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("metadata leaked: %s", raw)
		}
	}
}
