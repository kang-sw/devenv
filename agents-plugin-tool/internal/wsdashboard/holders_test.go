package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// fakeHolders is a HolderHost that records what the daemon asks of it.
type fakeHolders struct {
	mu      sync.Mutex
	listing wsholder.Listing
	stopErr map[string]error
	stopped []string
	runs    int
	closes  int
	running chan struct{}
}

func newFakeHolders() *fakeHolders {
	return &fakeHolders{stopErr: map[string]error{}, running: make(chan struct{}, 1)}
}

func (f *fakeHolders) Run() {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	select {
	case f.running <- struct{}{}:
	default:
	}
}

func (f *fakeHolders) Close() {
	f.mu.Lock()
	f.closes++
	f.mu.Unlock()
}

func (f *fakeHolders) List() wsholder.Listing {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listing
}

// Stop ends a listed holder unless the test set an error for it.
func (f *fakeHolders) Stop(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.stopErr[id]; err != nil {
		return err
	}
	for i, h := range f.listing.Holders {
		if h.ID == id {
			f.listing.Holders = append(append([]wsholder.Info{}, f.listing.Holders[:i]...), f.listing.Holders[i+1:]...)
			f.stopped = append(f.stopped, id)
			return nil
		}
	}
	return wsholder.ErrUnknownHolder
}

func (f *fakeHolders) stops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopped...)
}

func withHolders(h HolderHost) func(*DaemonOptions) {
	return func(o *DaemonOptions) { o.Holders = h }
}

// holderFixtureListing is two unattached holders, one attached, and one
// start failure.
func holderFixtureListing() wsholder.Listing {
	return wsholder.Listing{
		Holders: []wsholder.Info{
			{ID: "hlost", PID: 4100, Cause: wsholder.CauseNoAnswer, Protocol: 1, StartedAt: 1000, RestartSafe: true, Session: json.RawMessage(`{"label":"lost shell"}`)},
			{ID: "hjob", PID: 4200, Cause: wsholder.CauseProtocol, Protocol: 1, StartedAt: 2000, RestartSafe: false},
			{ID: "hlive", PID: 4300, Attached: true, Protocol: 1, StartedAt: 3000, RestartSafe: true, Child: &wsholder.ChildStatus{State: "running", PID: 4301, Mode: "pty"}},
		},
		Failures: []wsholder.StartFailure{{At: 5000, Message: "no interpreter for the session holder"}},
	}
}

// postFrom posts to path as v with the given Origin header ("" sends none).
func postFrom(d *Daemon, v viewer, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
	req.RemoteAddr = v.peer
	req.Host = v.host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if v.cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
	}
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeHolderList(t *testing.T, rec *httptest.ResponseRecorder) holderList {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /holders = %d (%s)", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	var list holderList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("GET /holders body: %v\n%s", err, rec.Body.String())
	}
	return list
}

// TestHoldersRouteListsHoldersAndStartFailures pins the holders route: it
// answers every holder with its attach state, cause, and restart-safe
// mark, the start failures, and that a local viewer may stop one; it never
// carries a holder's token.
func TestHoldersRouteListsHoldersAndStartFailures(t *testing.T) {
	holders := newFakeHolders()
	holders.listing = holderFixtureListing()
	f := startPageFixture(t, withHolders(holders))
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)}

	rec := serve(f.d, local, http.MethodGet, "/holders", "")
	list := decodeHolderList(t, rec)
	if !list.CanStop {
		t.Errorf("can_stop = false for a local viewer")
	}
	if len(list.Holders) != 3 || list.Holders[0].ID != "hlost" || list.Holders[0].Attached || list.Holders[0].Cause != wsholder.CauseNoAnswer ||
		list.Holders[1].RestartSafe || !list.Holders[2].Attached || list.Holders[2].Child == nil {
		t.Errorf("holders = %+v", list.Holders)
	}
	if len(list.Failures) != 1 || !strings.Contains(list.Failures[0].Message, "no interpreter") {
		t.Errorf("failures = %+v, want the start failure", list.Failures)
	}
	if strings.Contains(rec.Body.String(), "token") {
		t.Errorf("the listing mentions a token: %s", rec.Body.String())
	}
	if rec := serve(f.d, local, http.MethodPost, "/holders", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /holders = %d, want 405", rec.Code)
	}

	// Without a holder host the daemon serves neither route.
	bare := startPageFixture(t)
	bareLocal := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(bare.d.Endpoint().Port)}
	if rec := serve(bare.d, bareLocal, http.MethodGet, "/holders", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET /holders without a holder host = %d, want 404", rec.Code)
	}
	if rec := serve(bare.d, bareLocal, http.MethodPost, "/holders/hlost/stop", ""); rec.Code != http.StatusNotFound {
		t.Errorf("POST stop without a holder host = %d, want 404", rec.Code)
	}
}

// TestHolderStopRouteEndsAHolder pins the stop route's answers for a local
// viewer: 204 once the holder ended, 404 for an unknown or malformed id,
// 409 with the reason when the guarded kill is refused, 405 for a read.
func TestHolderStopRouteEndsAHolder(t *testing.T) {
	holders := newFakeHolders()
	holders.listing = holderFixtureListing()
	holders.stopErr["hjob"] = wsholder.ErrStartTimeMismatch
	f := startPageFixture(t, withHolders(holders))
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)}

	if rec := serve(f.d, local, http.MethodPost, "/holders/hlost/stop", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("POST stop = %d (%s), want 204", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := holders.stops(); len(got) != 1 || got[0] != "hlost" {
		t.Fatalf("stopped = %v, want [hlost]", got)
	}
	if list := decodeHolderList(t, serve(f.d, local, http.MethodGet, "/holders", "")); len(list.Holders) != 2 {
		t.Errorf("holders after the stop = %+v, want the stopped one gone", list.Holders)
	}
	rec := serve(f.d, local, http.MethodPost, "/holders/hjob/stop", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "start time") {
		t.Errorf("POST stop of a holder whose kill is refused = %d (%s), want 409 with the reason", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	for _, path := range []string{"/holders/nobody/stop", "/holders/bin/stop", "/holders/a%2Fb/stop", "/holders/hlive", "/holders/hlive/kill"} {
		if rec := serve(f.d, local, http.MethodPost, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, rec.Code)
		}
	}
	if rec := serve(f.d, local, http.MethodGet, "/holders/hlive/stop", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET stop = %d, want 405", rec.Code)
	}
	if got := holders.stops(); len(got) != 1 {
		t.Errorf("stopped = %v, want only the first stop", got)
	}
}

// TestHolderStopIsForALocalSameOriginViewerOnly pins the stop route's gate:
// a holder may own a terminal, so only a browser on this machine that sends
// the daemon's own Origin stops one. A LAN viewer holding the token reads
// the list, told it cannot stop, and its stop request finds no route.
func TestHolderStopIsForALocalSameOriginViewerOnly(t *testing.T) {
	holders := newFakeHolders()
	holders.listing = holderFixtureListing()
	f := startPageFixture(t, lanMode, withHolders(holders))
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}
	lanHost := "192.168.7.20:" + port
	remote := []viewer{
		{name: "foreign peer with the token", peer: "192.168.7.30:50000", host: lanHost, cookie: token},
		{name: "loopback peer, foreign Host, token", peer: "127.0.0.1:50000", host: lanHost, cookie: token},
	}
	for _, v := range remote {
		list := decodeHolderList(t, serve(f.d, v, http.MethodGet, "/holders", ""))
		if list.CanStop || len(list.Holders) != 3 {
			t.Errorf("%s: can_stop = %v with %d holders, want the list without the stop", v.name, list.CanStop, len(list.Holders))
		}
		if rec := postFrom(f.d, v, "/holders/hlost/stop", "http://"+v.host); rec.Code != http.StatusNotFound {
			t.Errorf("%s: POST stop = %d, want 404", v.name, rec.Code)
		}
	}
	for _, origin := range []string{"", "null", "http://evil.example", "http://127.0.0.1:1", "https://127.0.0.1:" + port} {
		if rec := postFrom(f.d, local, "/holders/hlost/stop", origin); rec.Code != http.StatusForbidden {
			t.Errorf("local POST stop with Origin %q = %d, want 403", origin, rec.Code)
		}
	}
	if got := holders.stops(); len(got) != 0 {
		t.Fatalf("a refused request stopped %v", got)
	}
	if rec := postFrom(f.d, local, "/holders/hlost/stop", "http://"+local.host); rec.Code != http.StatusNoContent {
		t.Fatalf("local same-origin POST stop = %d (%s), want 204", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := holders.stops(); len(got) != 1 || got[0] != "hlost" {
		t.Fatalf("stopped = %v, want [hlost]", got)
	}
}

// TestDaemonRunsAndReleasesItsHolderHost pins the lifecycle: a started
// daemon runs the holder scans, and closing it only closes the host (which
// detaches), never stopping a holder.
func TestDaemonRunsAndReleasesItsHolderHost(t *testing.T) {
	holders := newFakeHolders()
	holders.listing = holderFixtureListing()
	f := startPageFixture(t, withHolders(holders))
	<-holders.running
	f.d.Close()
	holders.mu.Lock()
	runs, closes := holders.runs, holders.closes
	holders.mu.Unlock()
	if runs != 1 || closes != 1 {
		t.Fatalf("runs = %d, closes = %d, want one of each", runs, closes)
	}
	if got := holders.stops(); len(got) != 0 {
		t.Fatalf("closing the daemon stopped %v", got)
	}
}
