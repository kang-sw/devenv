package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// These fakes implement transport only; mailbox dispatch, claims, runner loops,
// registration ownership, and HTTP origin gates remain production paths.
type lifecycleBackend struct {
	mu     sync.Mutex
	starts int
	sends  []string
}

func (b *lifecycleBackend) Start(context.Context, wsagent.StartSpec) (wsagent.Process, error) {
	b.mu.Lock()
	b.starts++
	n := b.starts
	b.mu.Unlock()
	return &lifecycleProcess{backend: b, id: fmt.Sprintf("fake-holder-%d", n), events: make(chan wsagent.Event, 8)}, nil
}
func (b *lifecycleBackend) counts() (int, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts, append([]string(nil), b.sends...)
}

type lifecycleProcess struct {
	backend *lifecycleBackend
	id      string
	events  chan wsagent.Event
	once    sync.Once
}

func (p *lifecycleProcess) Events() <-chan wsagent.Event { return p.events }
func (p *lifecycleProcess) Send(text string) error {
	p.backend.mu.Lock()
	p.backend.sends = append(p.backend.sends, text)
	p.backend.mu.Unlock()
	return nil
}
func (p *lifecycleProcess) Interrupt() error                { return nil }
func (p *lifecycleProcess) SetModel(string) error           { return nil }
func (p *lifecycleProcess) SetEffort(string) error          { return nil }
func (p *lifecycleProcess) Close() error                    { p.once.Do(func() { close(p.events) }); return nil }
func (p *lifecycleProcess) HolderInfo() (string, int, bool) { return p.id, 4242, true }
func (p *lifecycleProcess) ReplayUntil(uint64, uint64)      {}
func (p *lifecycleProcess) StopError() error                { return nil }
func (p *lifecycleProcess) Detach() bool                    { _ = p.Close(); return true }

type lifecycleHolders struct{}

func (lifecycleHolders) AddSink(wsholder.Sink)  {}
func (lifecycleHolders) Scan()                  {}
func (lifecycleHolders) List() wsholder.Listing { return wsholder.Listing{} }
func (lifecycleHolders) LaunchWith(wsholder.LaunchSpec) (string, error) {
	return "", fmt.Errorf("unexpected real holder launch")
}
func (lifecycleHolders) Spawn(string, wsholder.SpawnSpec) (wsholder.ChildStatus, error) {
	return wsholder.ChildStatus{}, fmt.Errorf("unexpected holder spawn")
}
func (lifecycleHolders) Input(string, []byte, bool) error {
	return fmt.Errorf("unexpected holder input")
}
func (lifecycleHolders) Status(string) (wsholder.AttachInfo, error) {
	return wsholder.AttachInfo{}, wsholder.ErrUnknownHolder
}
func (lifecycleHolders) Stop(string) error { return nil }

type lifecycleObservedMail struct {
	*LeadMailbox
	mu      sync.Mutex
	signals map[string]int
}

func (m *lifecycleObservedMail) Watch(root, key string, changed func()) func() {
	return m.LeadMailbox.Watch(root, key, func() {
		changed()
		m.mu.Lock()
		m.signals[key]++
		m.mu.Unlock()
	})
}

func (m *lifecycleObservedMail) signalled(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.signals[key] > 0
}

type lifecycleFixture struct {
	t         *testing.T
	root, dir string
	runner    *wsagent.Runner
	backend   *lifecycleBackend
	daemon    *wsdashboard.Daemon
	keys      *AgentKeyStore
	mail      *lifecycleObservedMail
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	keys := NewAgentKeyStore()
	backend := &lifecycleBackend{}
	mail := &lifecycleObservedMail{LeadMailbox: NewLeadMailbox(dir), signals: map[string]int{}}
	runner := wsagent.NewRunner(wsagent.Options{Dir: dir, Keys: keys, LeadKeys: keys, Holders: lifecycleHolders{}, Mail: mail, Backends: map[string]wsagent.BackendRegistration{wsagent.BackendClaude: {Backend: backend, LeadProfile: wsagent.LeadProfileClaude, PermissionModes: wsagent.LeadPermissionModes}}})
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: dir, Agents: runner, Sessions: NewDashboardSessionSource(), PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(daemon.Close)
	return &lifecycleFixture{t, root, dir, runner, backend, daemon, keys, mail}
}
func (f *lifecycleFixture) lead() wsagent.State {
	f.t.Helper()
	st, err := f.runner.CreateLead(wsagent.BackendClaude, f.root, wsagent.LeadOptions{})
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}
func (f *lifecycleFixture) state(id string) wsagent.State {
	f.t.Helper()
	st, err := f.runner.Get(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}
func (f *lifecycleFixture) hidden(id string, v bool) {
	f.t.Helper()
	if _, err := f.runner.SetHidden(id, v); err != nil {
		f.t.Fatal(err)
	}
}
func lifecycleEventually(t *testing.T, what string, fn func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for !fn() {
		if time.Now().After(until) {
			t.Fatal("timed out: " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (f *lifecycleFixture) post(path, body, origin string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(dashboardCookieForTest(f.t, f.daemon))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	out := httptest.NewRecorder()
	f.daemon.Handler().ServeHTTP(out, req)
	return out
}
func lifecycleQueued(t *testing.T, s *Server, id string) int {
	t.Helper()
	path, err := s.mailboxPaths().ReplyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	if err := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error { n = len(store.Queues[id]); return nil }); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSessionLifecycleRealMailboxArchiveDeliveryContract(t *testing.T) {
	f := newLifecycleFixture(t)
	lead := f.lead()
	f.hidden(lead.ID, true)
	sender := NewServer(f.root, "test")
	defer sender.Close()
	key := mailboxLogin(t, sender, 1, f.root)
	mustSend(t, sender, 2, key, "id:"+lead.ID, "archived-mail-contract")
	// Inspect the actual dispatch queue without claiming it. A runner notification
	// is also forced so the no-wake assertion does not depend on watch scheduling.
	f.runner.InboxChanged(lead.ID)
	time.Sleep(200 * time.Millisecond)
	if n := lifecycleQueued(t, sender, lead.ID); n != 1 {
		t.Fatalf("hidden mail queue = %d, want unclaimed 1", n)
	}
	// CreateLead canonicalizes temporary roots (notably /var on macOS).
	session, ok := f.keys.LeadSession(lead.Root, lead.ID)
	if !ok {
		t.Fatal("missing lead key")
	}
	lifecycleEventually(t, "real mailbox watch signals while archived", func() bool { return f.mail.signalled(session.Key) })
	history, err := readMailboxHistory("", session.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("hidden mail was claimed: %+v", history)
	}
	if starts, sends := f.backend.counts(); starts != 0 || len(sends) != 0 {
		t.Fatalf("hidden starts=%d sends=%q", starts, sends)
	}
	if st := f.state(lead.ID); !st.Hidden || st.Process || st.Lifecycle != wsdashboard.SessionArchived {
		t.Fatalf("hidden state = %+v", st)
	}
	f.hidden(lead.ID, false)
	lifecycleEventually(t, "unhide delivers queued mail", func() bool { _, sends := f.backend.counts(); return len(sends) == 1 })
	starts, sends := f.backend.counts()
	if starts != 1 || !strings.Contains(sends[0], "archived-mail-contract") {
		t.Fatalf("delivery starts=%d sends=%q", starts, sends)
	}
	if n := lifecycleQueued(t, sender, lead.ID); n != 0 {
		t.Fatalf("claimed queue=%d", n)
	}
	history, err = readMailboxHistory("", session.Key)
	if err != nil || len(history) != 1 || history[0].Direction != mailboxHistoryRecv {
		t.Fatalf("claim history=%+v err=%v", history, err)
	}
	f.runner.InboxChanged(lead.ID)
	f.hidden(lead.ID, false)
	time.Sleep(100 * time.Millisecond)
	if starts, sends := f.backend.counts(); starts != 1 || len(sends) != 1 {
		t.Fatalf("duplicate delivery starts=%d sends=%q", starts, sends)
	}
	// A distinct visible, processless lead still wakes through real mailbox.send.
	visible := f.lead()
	if st := f.state(visible.ID); st.Hidden || st.Process || st.Lifecycle != wsdashboard.SessionAsleep {
		t.Fatalf("visible asleep=%+v", st)
	}
	mustSend(t, sender, 3, key, "id:"+visible.ID, "visible-mail-contract")
	lifecycleEventually(t, "visible asleep mail wakes", func() bool { _, sends := f.backend.counts(); return len(sends) == 2 })
	if starts, sends := f.backend.counts(); starts != 2 || !strings.Contains(sends[1], "visible-mail-contract") {
		t.Fatalf("visible delivery starts=%d sends=%q", starts, sends)
	}
}

func TestSessionLifecycleHostedControlGateContract(t *testing.T) {
	f := newLifecycleFixture(t)
	lead := f.lead()
	path := "/agents/" + lead.ID
	for _, origin := range []string{"", "http://other.invalid"} {
		out := f.post(path+"/hide", `{}`, origin)
		if out.Code != http.StatusForbidden {
			t.Fatalf("untrusted hide origin %q = %d: %s", origin, out.Code, out.Body.String())
		}
		if f.state(lead.ID).Hidden {
			t.Fatal("refused hide changed visibility")
		}
	}
	for _, action := range []string{"hide", "unhide"} {
		out := f.post(path+"/"+action, `{}`, "http://127.0.0.1")
		if out.Code != http.StatusOK {
			t.Fatalf("trusted %s = %d: %s", action, out.Code, out.Body.String())
		}
		if got := f.state(lead.ID).Hidden; got != (action == "hide") {
			t.Fatalf("%s hidden = %v", action, got)
		}
	}
	// The registration is the real ownership fence, not a fake Trusted hook.
	reg, err := wsdashboard.ReadRegistration(f.dir, lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	reg.Hosted = false
	if err := wsdashboard.WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	out := f.post(path+"/hide", `{}`, "http://127.0.0.1")
	if out.Code != http.StatusConflict {
		t.Fatalf("terminal-owned hide = %d: %s", out.Code, out.Body.String())
	}
	if hidden, err := wsdashboard.ReadSessionHidden(f.dir, lead.ID); err != nil || hidden {
		t.Fatalf("ownership refusal changed visibility: %v, %v", hidden, err)
	}
	if starts, sends := f.backend.counts(); starts != 0 || len(sends) != 0 {
		t.Fatalf("visibility controls launched: starts=%d sends=%q", starts, sends)
	}
}

func TestSessionLifecycleOwnPageRouteContract(t *testing.T) {
	f := newLifecycleFixture(t)
	lead := f.lead()
	f.hidden(lead.ID, true)
	f.daemon.Index().Refresh()
	path := "/s/" + lead.ID
	// Passive input is held, not an explicit unarchive operation.
	if _, err := f.runner.Send(lead.ID, "passive-contract"); err != nil {
		t.Fatal(err)
	}
	f.runner.InboxChanged(lead.ID)
	time.Sleep(100 * time.Millisecond)
	if st := f.state(lead.ID); !st.Hidden || st.Process {
		t.Fatalf("passive input woke=%+v", st)
	}
	// Remove that held message through the existing cancel route, not a direct helper.
	inbox := wsinbox.Dir(wsdashboard.SessionDir(f.dir, lead.ID))
	held, err := wsinbox.Held(inbox)
	if err != nil || len(held) != 1 {
		t.Fatalf("held=%+v err=%v", held, err)
	}
	for _, tc := range []struct {
		route, body, origin string
		status              int
	}{
		{"message", `{"text":"forbidden"}`, "http://other.invalid", 403},
		{"message", `{`, "http://127.0.0.1", 400},
		{"steer", fmt.Sprintf(`{"id":%q}`, held[0].ID()), "http://127.0.0.1", 200},
	} {
		out := f.post(path+"/"+tc.route, tc.body, tc.origin)
		if out.Code != tc.status {
			t.Fatalf("%s = %d: %s", tc.route, out.Code, out.Body.String())
		}
		if st := f.state(lead.ID); !st.Hidden || st.Process {
			t.Fatalf("%s unhid=%+v", tc.route, st)
		}
	}
	held, err = wsinbox.Held(inbox)
	if err != nil || len(held) != 1 {
		t.Fatalf("steered held=%+v err=%v", held, err)
	}
	out := f.post(path+"/cancel", fmt.Sprintf(`{"id":%q}`, held[0].ID()), "http://127.0.0.1")
	if out.Code != 200 {
		t.Fatalf("cancel=%d: %s", out.Code, out.Body.String())
	}
	if st := f.state(lead.ID); !st.Hidden || st.Process {
		t.Fatalf("cancel unhid=%+v", st)
	}
	// Empty manual unhide changes visibility only.
	f.hidden(lead.ID, false)
	time.Sleep(100 * time.Millisecond)
	if st := f.state(lead.ID); st.Hidden || st.Process || st.Lifecycle != wsdashboard.SessionAsleep {
		t.Fatalf("empty unhide=%+v", st)
	}
	f.hidden(lead.ID, true)
	out = f.post(path+"/message", `{"text":"own-page-contract"}`, "http://127.0.0.1")
	if out.Code != 200 {
		t.Fatalf("message=%d: %s", out.Code, out.Body.String())
	}
	lifecycleEventually(t, "own page input starts", func() bool { _, sends := f.backend.counts(); return len(sends) == 1 })
	if st := f.state(lead.ID); st.Hidden || !st.Process || st.Lifecycle != wsdashboard.SessionActiveOn {
		t.Fatalf("page input=%+v", st)
	}
	if starts, sends := f.backend.counts(); starts != 1 || !strings.Contains(sends[0], "own-page-contract") || strings.Contains(sends[0], "passive-contract") {
		t.Fatalf("page delivery starts=%d sends=%q", starts, sends)
	}
}
