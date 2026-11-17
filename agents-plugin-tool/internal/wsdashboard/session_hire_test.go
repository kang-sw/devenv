package wsdashboard

import (
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type hireFake struct {
	mu        sync.Mutex
	dir       string
	created   int
	launches  []HireLaunch
	root      string
	createErr error
	requests  map[string]string
	cancelled int
	admitted  map[string]bool
}

func (h *hireFake) AdmitsLead(name string) bool {
	if h.admitted != nil {
		return h.admitted[name]
	}
	return name == "claude" || name == "pi"
}

func hireRegistryFixture(o *DaemonOptions) { o.Agents = &hireFake{dir: o.Dir} }

func (h *hireFake) Handler() http.Handler      { return http.NotFoundHandler() }
func (h *hireFake) Start(func(string, string)) {}
func (h *hireFake) InboxChanged(string)        {}
func (h *hireFake) Close()                     {}
func (h *hireFake) CreateHireLead(requestID, root string, launch HireLaunch) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.createErr != nil {
		return "", h.createErr
	}
	if id := h.requests[requestID]; id != "" {
		return id, nil
	}
	if h.requests == nil {
		h.requests = map[string]string{}
	}
	h.created++
	h.launches = append(h.launches, launch)
	h.root = root
	id := "hiredsession"
	reg := Registration{SchemaVersion: 1, ReplyID: id, Project: root, Harness: launch.Harness, Hosted: true, HostStopped: true, RegisteredAt: time.Now().UTC().Format(time.RFC3339), HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
	if err := WriteRegistration(h.dir, reg); err != nil {
		return "", err
	}
	if err := MarkHired(h.dir, id, requestID, launch.Label); err != nil {
		return "", err
	}
	h.requests[requestID] = id
	return id, nil
}

func (h *hireFake) CancelHireLead(requestID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.requests[requestID]
	if id == "" {
		return nil
	}
	reg, err := ReadRegistration(h.dir, id)
	if err != nil {
		return err
	}
	if reg.EndedAt != "" {
		return nil
	}
	reg.EndedAt = time.Now().UTC().Format(time.RFC3339)
	if err = WriteRegistration(h.dir, reg); err != nil {
		return err
	}
	h.cancelled++
	return nil
}

type hireWakeCounter struct{ n atomic.Int64 }

func (w *hireWakeCounter) Wake()  { w.n.Add(1) }
func (w *hireWakeCounter) Nudge() {}

func hireHTTP(d *Daemon, path, body, origin, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, strings.NewReader(body))
	req.RemoteAddr = remote
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	localhostTestHandler(nil, d).ServeHTTP(out, req)
	return out
}
func hirePost(d *Daemon, path, body string) *httptest.ResponseRecorder {
	return hireHTTP(d, path, body, "http://127.0.0.1", "127.0.0.1:1234")
}
func enableHiring(t *testing.T, d *Daemon, id string) {
	t.Helper()
	// The fixture's background index load can still be running; enabling
	// requires a loaded index (503 otherwise), so load it synchronously.
	if d.index != nil && !d.index.Loaded() {
		d.index.Refresh()
	}
	if d.index == nil || !d.index.Loaded() {
		t.Fatal("enable: session index did not load")
	}
	res := hirePost(d, "/s/"+id+"/hiring", `{"enabled":true}`)
	if res.Code != 200 {
		t.Fatalf("enable: %d %s", res.Code, res.Body)
	}
}
func requestHiring(t *testing.T, d *Daemon, id string) HireProposal {
	t.Helper()
	p, err := d.RequestHire(id, HireRequest{Reason: "parallel investigation", RecoveryContext: "restore this exact context\nsecond line", Harness: "claude", Model: "suggested", Effort: "low", Label: "suggested label"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func hireMails(t *testing.T, d *Daemon, id string) []wsmailbox.Envelope {
	t.Helper()
	store, err := (wsmailbox.Paths{State: wsstate.Options{CacheHome: filepath.Dir(d.dir)}}).Store().LoadReply()
	if err != nil {
		t.Fatal(err)
	}
	return store.Queues[id]
}
func assertHireMail(t *testing.T, d *Daemon, p HireProposal, status string) {
	t.Helper()
	mails := hireMails(t, d, p.Requester)
	var found int
	for _, mail := range mails {
		if strings.Contains(mail.Content, p.ID) {
			found++
			if !strings.Contains(mail.Content, ": "+status+"\n") || !strings.Contains(mail.Content, p.RecoveryContext) || mail.From != "" || mail.ReplyTo != "" {
				t.Fatalf("mail = %+v", mail)
			}
		}
	}
	if found != 1 {
		t.Fatalf("outcomes for %s = %d, want one: %+v", p.ID, found, mails)
	}
}

func TestHiringFlagAndTrust(t *testing.T) {
	f := startPageFixture(t)
	waitFor(t, "hiring session index to load", f.d.Index().Loaded)
	req := HireRequest{Reason: "why", RecoveryContext: "recover"}
	if _, err := f.d.RequestHire("livesession", req); err == nil || !strings.Contains(err.Error(), "hiring is not enabled") {
		t.Fatalf("unflagged: %v", err)
	}
	for _, test := range []struct {
		origin, remote string
		code           int
	}{
		{"", "127.0.0.1:1", 403}, {"http://evil.test", "127.0.0.1:1", 403}, {"http://127.0.0.1", "192.0.2.1:1", 401},
	} {
		res := hireHTTP(f.d, "/s/livesession/hiring", `{"enabled":true}`, test.origin, test.remote)
		if res.Code != test.code {
			t.Fatalf("trusted gate %d: %s", res.Code, res.Body)
		}
	}
	enableHiring(t, f.d, "livesession")
	// A typed legacy registration rewrite leaves the dashboard-only sidecar.
	reg, err := ReadRegistration(f.dir, "livesession")
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	if _, err = f.d.RequestHire("livesession", req); err != nil {
		t.Fatal(err)
	}
	res := hirePost(f.d, railLayoutRoute, `{"op":"attach","rid":"livesession","parent":"boardsession"}`)
	if res.Code != 409 || !strings.Contains(res.Body.String(), `"hiring"`) {
		t.Fatalf("attach enabled root: %d %s", res.Code, res.Body)
	}
	if res = hirePost(f.d, "/s/livesession/hiring", `{"enabled":false}`); res.Code != 200 {
		t.Fatal(res.Body)
	}
	if res = hirePost(f.d, railLayoutRoute, `{"op":"attach","rid":"livesession","parent":"boardsession"}`); res.Code != 200 {
		t.Fatalf("attach: %d %s", res.Code, res.Body)
	}
	if res = hirePost(f.d, "/s/livesession/hiring", `{"enabled":true}`); res.Code != 409 {
		t.Fatalf("nested flag: %d", res.Code)
	}
	if err = MarkHired(f.dir, "boardsession", "req-id", ""); err != nil {
		t.Fatal(err)
	}
	if res = hirePost(f.d, "/s/boardsession/hiring", `{"enabled":true}`); res.Code != 409 {
		t.Fatalf("hired flag: %d", res.Code)
	}
	// Even a corrupt/manual enabled+hired combination is not an entitlement.
	if err = writeHiringMetadata(f.dir, "boardsession", HiringMetadata{Enabled: true, Hired: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.d.RequestHire("boardsession", req); err == nil || !strings.Contains(err.Error(), "hired") {
		t.Fatalf("hired request: %v", err)
	}
}

func TestHiringHTTPBodyBoundaries(t *testing.T) {
	f := startPageFixture(t, hireRegistryFixture)
	enableHiring(t, f.d, "livesession")
	for _, body := range []string{`{}`, `null`, `{"enabled":"true"}`, `{"enabled":true}{"enabled":false}`} {
		res := hirePost(f.d, "/s/livesession/hiring", body)
		if res.Code != 400 {
			t.Fatalf("flag body %q: %d", body, res.Code)
		}
	}
	res := hirePost(f.d, "/s/livesession/hiring", `{"enabled":false}`+strings.Repeat(" ", 4096))
	if res.Code != 413 {
		t.Fatalf("flag body limit: %d", res.Code)
	}
	meta, err := ReadHiringMetadata(f.dir, "livesession")
	if err != nil || !meta.Enabled {
		t.Fatalf("malformed request changed flag: %+v %v", meta, err)
	}
	p := requestHiring(t, f.d, "livesession")
	res = hirePost(f.d, hireRoute+"/"+p.ID+"/reject", `{} {}`)
	if res.Code != 400 {
		t.Fatalf("trailing approval body: %d", res.Code)
	}
	res = hirePost(f.d, hireRoute+"/"+p.ID+"/reject", `{}`+strings.Repeat(" ", 64<<10))
	if res.Code != 413 {
		t.Fatalf("approval body limit: %d", res.Code)
	}
	queue, err := f.d.pendingHires()
	if err != nil || len(queue) != 1 {
		t.Fatalf("malformed decision changed proposal: %+v %v", queue, err)
	}
}

func TestHiringRequestValidation(t *testing.T) {
	f := startPageFixture(t)
	enableHiring(t, f.d, "livesession")
	for _, req := range []HireRequest{
		{RecoveryContext: "recover"}, {Reason: "why"}, {Reason: "why", RecoveryContext: strings.Repeat("x", (16<<10)+1)},
		{Reason: "why", RecoveryContext: "recover", Harness: "codex"}, {Reason: "why", RecoveryContext: "recover", Label: "a\nb"},
	} {
		if _, err := f.d.RequestHire("livesession", req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	if _, err := f.d.RequestHire("livesession", HireRequest{Reason: "why", RecoveryContext: strings.Repeat("x", 16<<10)}); err != nil {
		t.Fatal(err)
	}
}

func TestHiringLifecycle(t *testing.T) {
	wake := &hireWakeCounter{}
	creator := &hireFake{}
	f := startPageFixture(t, func(o *DaemonOptions) { creator.dir = o.Dir; o.Agents = creator; o.MailWake = wake })
	f.src.setPage("hiredsession", PageContent{HasBoard: true})
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	if creator.created != 0 || p.Status != "pending" {
		t.Fatalf("request created a session")
	}
	if len(hireMails(t, f.d, p.Requester)) != 0 {
		t.Fatal("request mailed before decision")
	}
	for _, remote := range []struct {
		origin, addr string
		code         int
	}{{"", "127.0.0.1:1", 403}, {"http://127.0.0.1", "192.0.2.1:1", 401}} {
		res := hireHTTP(f.d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi"}`, remote.origin, remote.addr)
		if res.Code != remote.code {
			t.Fatalf("approval gate %d %s", res.Code, res.Body)
		}
	}
	res := hirePost(f.d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi","model":"owner-model","effort":"high","label":"owner label","root":"/must/not/be/used"}`)
	if res.Code != 200 {
		t.Fatalf("approve: %d %s", res.Code, res.Body)
	}
	if creator.created != 1 || creator.root != f.project || creator.launches[0].Model != "owner-model" || creator.launches[0].Harness != "pi" {
		t.Fatalf("creation: %+v", creator)
	}
	if f.d.hireParent("hiredsession") != "livesession" {
		t.Fatal("not nested under requester")
	}
	assertHireMail(t, f.d, p, "approved")
	res = hirePost(f.d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi"}`)
	if res.Code != 409 || creator.created != 1 {
		t.Fatalf("duplicate approval: %d %+v", res.Code, creator)
	}
	f.d.settleHires()
	assertHireMail(t, f.d, p, "approved")
	for _, reason := range []string{"", "not needed"} {
		p = requestHiring(t, f.d, "livesession")
		body, _ := json.Marshal(map[string]string{"reason": reason})
		res = hirePost(f.d, hireRoute+"/"+p.ID+"/reject", string(body))
		if res.Code != 200 {
			t.Fatal(res.Body)
		}
		assertHireMail(t, f.d, p, "rejected")
	}
	if wake.n.Load() != 3 {
		t.Fatalf("wake count = %d", wake.n.Load())
	}
	reg, err := ReadRegistration(f.dir, "livesession")
	if err != nil {
		t.Fatal(err)
	}
	reg.EndedAt = time.Now().UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	f.d.settleHires()
	creator.mu.Lock()
	cancelled := creator.cancelled
	creator.mu.Unlock()
	if cancelled != 0 {
		t.Fatal("requester end closed an already approved peer")
	}
}

func TestHiringExpiryAndRestart(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixMilli())
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	f := startPageFixture(t, hireRegistryFixture, func(o *DaemonOptions) { o.hireNow = now })
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	f.d.Close()
	// Keep the requester live while the daemon was down for over 30 minutes.
	clock.Add(int64((31 * time.Minute) / time.Millisecond))
	reg, err := ReadRegistration(f.dir, p.Requester)
	if err != nil {
		t.Fatal(err)
	}
	reg.HeartbeatAt = now().UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	d, err := StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, noGitBadge: true, hireNow: now, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	waitFor(t, "restart expiry mail", func() bool { return len(hireMails(t, d, p.Requester)) == 1 })
	assertHireMail(t, d, p, "expired")
	d.settleHires()
	assertHireMail(t, d, p, "expired")
}

func TestHiringPendingSurvivesRestartAndExpiryClock(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixMilli())
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	f := startPageFixture(t, hireRegistryFixture, func(o *DaemonOptions) { o.hireNow = now })
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	f.d.Close()
	d, err := StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, noGitBadge: true, hireNow: now, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	queue, err := d.pendingHires()
	if err != nil || len(queue) != 1 || queue[0].ID != p.ID || !queue[0].ExpiresAt.Equal(p.ExpiresAt) {
		t.Fatalf("restored: %+v %v", queue, err)
	}
	advanced := now().Add(hireLifetime)
	reg, err := ReadRegistration(f.dir, p.Requester)
	if err != nil {
		t.Fatal(err)
	}
	// Publish the future heartbeat and advance the fixture clock as one step
	// under hireMu. Ordering alone is not enough: a background hireLoop pass
	// could read the old registration before the write and the clock after
	// the jump, and then withdraw the request as "requester session has
	// ended". settleHires holds hireMu across its read and its verdict, so
	// every pass sees both values old or both new.
	d.hireMu.Lock()
	reg.HeartbeatAt = advanced.UTC().Format(time.RFC3339)
	err = WriteRegistration(f.dir, reg)
	if err == nil {
		clock.Store(advanced.UnixMilli())
	}
	d.hireMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	d.settleHires()
	assertHireMail(t, d, p, "expired")
}

func TestHiringWithdrawals(t *testing.T) {
	for _, edge := range []string{"index", "startup", "approval", "flag"} {
		t.Run(edge, func(t *testing.T) {
			f := startPageFixture(t, hireRegistryFixture)
			enableHiring(t, f.d, "livesession")
			p := requestHiring(t, f.d, "livesession")
			reg, err := ReadRegistration(f.dir, p.Requester)
			if err != nil {
				t.Fatal(err)
			}
			if edge == "flag" {
				res := hirePost(f.d, "/s/"+p.Requester+"/hiring", `{"enabled":false}`)
				if res.Code != 200 {
					t.Fatal(res.Body)
				}
			} else {
				if edge == "startup" {
					f.d.Close()
				}
				reg.EndedAt = time.Now().UTC().Format(time.RFC3339)
				if err = WriteRegistration(f.dir, reg); err != nil {
					t.Fatal(err)
				}
			}
			d := f.d
			switch edge {
			case "index":
				d.index.Refresh()
				d.pokeHires()
			case "startup":
				d, err = StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, noGitBadge: true, PollInterval: time.Hour})
				if err != nil {
					t.Fatal(err)
				}
				defer d.Close()
			case "approval", "flag":
				res := hirePost(d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi"}`)
				if res.Code != 200 && res.Code != 409 {
					t.Fatalf("recheck: %d %s", res.Code, res.Body)
				}
			}
			waitFor(t, "withdrawn mail", func() bool { return len(hireMails(t, d, p.Requester)) == 1 })
			assertHireMail(t, d, p, "withdrawn")
			reg.EndedAt = ""
			reg.HeartbeatAt = time.Now().UTC().Format(time.RFC3339)
			if err = WriteRegistration(f.dir, reg); err != nil {
				t.Fatal(err)
			}
			d.index.Refresh()
			d.settleHires()
			queue, err := d.pendingHires()
			if err != nil || len(queue) != 0 {
				t.Fatalf("revived cancelled proposal: %+v %v", queue, err)
			}
			assertHireMail(t, d, p, "withdrawn")
		})
	}
}

func TestHiringProcessStoppedKeepsRequest(t *testing.T) {
	f := startPageFixture(t, hireRegistryFixture)
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	reg, err := ReadRegistration(f.dir, p.Requester)
	if err != nil {
		t.Fatal(err)
	}
	reg.Hosted = true
	reg.HostStopped = true
	reg.HostPID = 0
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	f.d.settleHires()
	queue, err := f.d.pendingHires()
	if err != nil || len(queue) != 1 {
		t.Fatalf("stopped: %+v %v", queue, err)
	}
	if len(hireMails(t, f.d, p.Requester)) != 0 {
		t.Fatal("stopped requester withdrawn")
	}
}

func TestHiringCreationFailureStaysPending(t *testing.T) {
	creator := &hireFake{createErr: errors.New("fixture creation refused")}
	f := startPageFixture(t, func(o *DaemonOptions) { creator.dir = o.Dir; o.Agents = creator })
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	res := hirePost(f.d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi"}`)
	if res.Code != 409 {
		t.Fatalf("creation failure: %d %s", res.Code, res.Body)
	}
	queue, err := f.d.pendingHires()
	if err != nil || len(queue) != 1 || queue[0].Status != "pending" || queue[0].CreationError != "fixture creation refused" {
		t.Fatalf("failed request: %+v %v", queue, err)
	}
	if len(hireMails(t, f.d, p.Requester)) != 0 {
		t.Fatal("creation failure invented a terminal outcome")
	}
	creator.mu.Lock()
	creator.createErr = nil
	creator.mu.Unlock()
	f.src.setPage("hiredsession", PageContent{HasBoard: true})
	res = hirePost(f.d, hireRoute+"/"+p.ID+"/approve", `{"harness":"pi"}`)
	if res.Code != 200 {
		t.Fatalf("retry: %d %s", res.Code, res.Body)
	}
	assertHireMail(t, f.d, p, "approved")
}

func TestHiringApprovalIntentRecoversOnStartup(t *testing.T) {
	creator := &hireFake{}
	f := startPageFixture(t, func(o *DaemonOptions) { creator.dir = o.Dir; o.Agents = creator })
	f.src.setPage("hiredsession", PageContent{HasBoard: true})
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	f.d.Close()
	proposals, err := f.d.readHires()
	if err != nil {
		t.Fatal(err)
	}
	proposals[0].Launch = &HireLaunch{Harness: "pi", Model: "owner-model"}
	proposals[0].CreationAttempted = true
	if err = f.d.saveHires(proposals); err != nil {
		t.Fatal(err)
	}
	// Simulate the creation having committed before the daemon stopped, but
	// before the proposal's terminal outcome was checkpointed.
	id, err := creator.CreateHireLead(p.ID, f.project, *proposals[0].Launch)
	if err != nil {
		t.Fatal(err)
	}
	d, err := StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, Agents: creator, noGitBadge: true, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	waitFor(t, "approval intent recovery", func() bool { return len(hireMails(t, d, p.Requester)) == 1 })
	assertHireMail(t, d, p, "approved")
	creator.mu.Lock()
	count := creator.created
	creator.mu.Unlock()
	if count != 1 || d.hireParent(id) != p.Requester {
		t.Fatalf("recovered creation count %d, parent %s", count, d.hireParent(id))
	}
}

func TestHiringEndedRequesterCancelsInterruptedCreation(t *testing.T) {
	creator := &hireFake{}
	f := startPageFixture(t, func(o *DaemonOptions) { creator.dir = o.Dir; o.Agents = creator })
	f.src.setPage("hiredsession", PageContent{HasBoard: true})
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	f.d.Close()
	proposals, err := f.d.readHires()
	if err != nil {
		t.Fatal(err)
	}
	proposals[0].Launch = &HireLaunch{Harness: "pi"}
	proposals[0].CreationAttempted = true
	if err = f.d.saveHires(proposals); err != nil {
		t.Fatal(err)
	}
	id, err := creator.CreateHireLead(p.ID, f.project, *proposals[0].Launch)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ReadRegistration(f.dir, p.Requester)
	if err != nil {
		t.Fatal(err)
	}
	reg.EndedAt = time.Now().UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	d, err := StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, Agents: creator, noGitBadge: true, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	waitFor(t, "interrupted approval withdrawal", func() bool { return len(hireMails(t, d, p.Requester)) == 1 })
	assertHireMail(t, d, p, "withdrawn")
	child, err := ReadRegistration(f.dir, id)
	if err != nil || child.EndedAt == "" {
		t.Fatalf("provisional creation survived withdrawal: %+v %v", child, err)
	}
	creator.mu.Lock()
	created, cancelled := creator.created, creator.cancelled
	creator.mu.Unlock()
	if created != 1 || cancelled != 1 {
		t.Fatalf("created=%d cancelled=%d", created, cancelled)
	}
	reg.EndedAt = ""
	reg.HeartbeatAt = time.Now().UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	d.index.Refresh()
	d.settleHires()
	queue, err := d.pendingHires()
	if err != nil || len(queue) != 0 {
		t.Fatalf("cancelled creation revived: %+v %v", queue, err)
	}
	assertHireMail(t, d, p, "withdrawn")
}

func TestHiringOutcomeWriteFailureRetriesFromDisk(t *testing.T) {
	f := startPageFixture(t, hireRegistryFixture)
	enableHiring(t, f.d, "livesession")
	p := requestHiring(t, f.d, "livesession")
	// The mail write fails while another process holds the mailbox ring.
	cache, err := wsstate.CacheRoot(wsstate.Options{CacheHome: filepath.Dir(f.dir)})
	if err != nil {
		t.Fatal(err)
	}
	ringDir := filepath.Join(cache, wsmailbox.RingDirName)
	if err = os.MkdirAll(ringDir, 0o755); err != nil {
		t.Fatal(err)
	}
	holder := flock.New(filepath.Join(ringDir, "lock"))
	if locked, err := holder.TryLock(); err != nil || !locked {
		t.Fatalf("hold the mailbox ring: %v %v", locked, err)
	}
	res := hirePost(f.d, hireRoute+"/"+p.ID+"/reject", `{"reason":"no longer needed"}`)
	if res.Code != 500 {
		t.Fatalf("mail failure: %d %s", res.Code, res.Body)
	}
	f.d.Close()
	proposals, err := f.d.readHires()
	if err != nil {
		t.Fatal(err)
	}
	if proposals[0].Status != "rejected" || proposals[0].Notified {
		t.Fatalf("lost outcome: %+v", proposals)
	}
	if err = holder.Unlock(); err != nil {
		t.Fatal(err)
	}
	d, err := StartDaemon(DaemonOptions{Dir: f.dir, Sessions: f.src, noGitBadge: true, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	waitFor(t, "retained outcome mail", func() bool { return len(hireMails(t, d, p.Requester)) == 1 })
	assertHireMail(t, d, p, "rejected")
}

func TestHiringRegistryAdmission(t *testing.T) {
	for _, tt := range []struct {
		name, harness string
		admitted      map[string]bool
		want          bool
	}{
		{"registered extension", "fixture", map[string]bool{"fixture": true}, true},
		{"registered builtin", "claude", map[string]bool{"claude": true}, true},
		{"unregistered builtin", "pi", map[string]bool{"fixture": true}, false},
		{"worker only", "worker", map[string]bool{"worker": false}, false},
		{"unknown", "unknown", map[string]bool{}, false},
		{"unspecified", "", map[string]bool{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creator := &hireFake{admitted: tt.admitted}
			f := startPageFixture(t, func(o *DaemonOptions) { creator.dir, o.Agents = o.Dir, creator })
			enableHiring(t, f.d, "livesession")
			p, err := f.d.RequestHire("livesession", HireRequest{Reason: "why", RecoveryContext: "recover", Harness: tt.harness})
			if tt.want {
				if err != nil || p.Harness != tt.harness {
					t.Fatalf("request = %+v, %v", p, err)
				}
			} else if err == nil || err.Error() != "harness must be claude or pi" {
				t.Fatalf("refusal text changed: %v", err)
			}
			queue, err := f.d.pendingHires()
			if err != nil || len(queue) != map[bool]int{true: 1, false: 0}[tt.want] || creator.created != 0 {
				t.Fatalf("admission side effects = %+v, %v; created=%d", queue, err, creator.created)
			}
		})
	}
}

func TestHiringHostWithoutLeadAdmission(t *testing.T) {
	creator := &hireFake{}
	f := startPageFixture(t, func(o *DaemonOptions) {
		creator.dir = o.Dir
		// Expose only AgentHost, deliberately hiding optional lead admission.
		o.Agents = struct{ AgentHost }{creator}
	})
	enableHiring(t, f.d, "livesession")
	for _, harness := range []string{"claude", "pi", "unknown"} {
		_, err := f.d.RequestHire("livesession", HireRequest{Reason: "why", RecoveryContext: "recover", Harness: harness})
		if err == nil || err.Error() != "this daemon's agent host cannot admit a lead harness" {
			t.Fatalf("%s: inaccurate host refusal: %v", harness, err)
		}
	}
	queue, err := f.d.pendingHires()
	if err != nil || len(queue) != 0 || creator.created != 0 {
		t.Fatalf("refusal side effects = %+v, %v; created=%d", queue, err, creator.created)
	}
	// An omitted preference still bypasses admission, as before.
	if _, err := f.d.RequestHire("livesession", HireRequest{Reason: "why", RecoveryContext: "recover"}); err != nil {
		t.Fatalf("unspecified harness refused: %v", err)
	}
}

func TestHiringRoster(t *testing.T) {
	f := startPageFixture(t)
	res := hirePost(f.d, railLayoutRoute, `{"op":"attach","rid":"boardsession","parent":"livesession"}`)
	if res.Code != 200 {
		t.Fatal(res.Body)
	}
	if err := writeSessionLabel(SessionDir(f.dir, "boardsession"), "roster peer"); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	now := time.Now()
	snap := &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead:   wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying, Batches: []wslive.BatchView{{CallCount: 1, EndedAt: now.UnixMilli() - 500}}}, Observed: &wslive.ObservedModel{Current: wslive.ModelObservation{Model: "observed-model", At: now.UnixMilli()}}},
		Agents: []wslive.Agent{{ID: "direct", Status: "running"}, {ID: "deep", ParentID: "direct", Status: "running"}}}
	writeSnapshot(t, f.dir, "boardsession", snap)
	rows, err := f.d.sessionRoster("livesession")
	if err != nil || len(rows) != 1 {
		t.Fatalf("roster: %+v %v", rows, err)
	}
	row := rows[0]
	if row.Mailbox != "id:boardsession" || row.Label != "roster peer" || row.Harness != "claude" || row.State != "busy" || row.RunningSubagents != 1 || row.Model == nil || *row.Model != "observed-model" || row.LastActivity == nil || *row.LastActivity != now.UnixMilli()-500 {
		t.Fatalf("roster = %+v", row)
	}
	raw, _ := json.Marshal(row)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if len(fields) != 7 {
		t.Fatalf("fields = %s", raw)
	}
	// Permission waits are idle, stale models are unknown, and no adapter
	// can borrow launch settings or recursive running-agent counts.
	snap.Lead.Phase = wslive.PhasePermission
	writeSnapshot(t, f.dir, "boardsession", snap)
	rows, _ = f.d.sessionRoster("livesession")
	if rows[0].State != "idle" {
		t.Fatal(rows)
	}
	snap.UpdatedAt = now.Add(-10 * time.Minute).UnixMilli()
	writeSnapshot(t, f.dir, "boardsession", snap)
	rows, _ = f.d.sessionRoster("livesession")
	if rows[0].Model != nil || rows[0].RunningSubagents != 0 || rows[0].State != "idle" {
		t.Fatal(rows)
	}
	reg, err := ReadRegistration(f.dir, "boardsession")
	if err != nil {
		t.Fatal(err)
	}
	reg.EndedAt = now.UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	snap.UpdatedAt = now.UnixMilli()
	writeSnapshot(t, f.dir, "boardsession", snap)
	rows, _ = f.d.sessionRoster("livesession")
	if rows[0].State != "idle" || rows[0].RunningSubagents != 0 || rows[0].Model != nil {
		t.Fatalf("ended roster: %+v", rows)
	}
	reg.EndedAt = ""
	reg.HeartbeatAt = now.UTC().Format(time.RFC3339)
	if err = WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	if err = os.Remove(filepath.Join(SessionDir(f.dir, "boardsession"), "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	rows, _ = f.d.sessionRoster("livesession")
	if rows[0].Model != nil || rows[0].LastActivity != nil {
		t.Fatal(rows)
	}
	res = hirePost(f.d, railLayoutRoute, `{"op":"detach","rid":"boardsession"}`)
	if res.Code != 200 {
		t.Fatal(res.Body)
	}
	rows, _ = f.d.sessionRoster("livesession")
	if len(rows) != 0 {
		t.Fatalf("detached still listed: %+v", rows)
	}
}
