package wsdashboard

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTracker is a TrackerHost that records the calls it gets.
type fakeTracker struct {
	mu        sync.Mutex
	calls     []string
	stores    []TrackerStore
	storesErr error
	viewErr   error
	dispatch  error // Dispatch's error, when set
	checkErr  error
	saveErr   error
	regErr    error
	renderErr error
	events    []TrackerEvent
	seen      TrackerSeen
	profiles  []TrackerProfile
	blobs     map[string][]byte // staged attachments by oid
	maxBytes  int               // the fake store's attachment limit
}

func newFakeTracker() *fakeTracker {
	return &fakeTracker{
		stores:   []TrackerStore{{ID: "team", Label: "Team", Member: "ada", Remote: "https://***@example.invalid/x.git"}},
		blobs:    map[string][]byte{},
		maxBytes: 1 << 20,
	}
}

func withTracker(h TrackerHost) func(*DaemonOptions) {
	return func(o *DaemonOptions) { o.Tracker = h }
}

func (f *fakeTracker) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeTracker) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeTracker) Stores() ([]TrackerStore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stores, f.storesErr
}

func (f *fakeTracker) View(_ context.Context, id string) (TrackerView, error) {
	f.record("view " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.viewErr != nil {
		return TrackerView{}, f.viewErr
	}
	return TrackerView{View: json.RawMessage(`{"store":{"name":"` + id + `"}}`), Seen: f.seen}, nil
}

func (f *fakeTracker) Dispatch(_ context.Context, id string, events []TrackerEvent) ([]string, error) {
	f.record("dispatch " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dispatch != nil {
		return nil, f.dispatch
	}
	f.events = append(f.events, events...)
	ids := make([]string, len(events))
	for i := range events {
		ids[i] = "ada@pc:" + strconv.Itoa(len(f.events)-len(events)+i+1)
	}
	return ids, nil
}

func (f *fakeTracker) Check(_ context.Context, id string) (TrackerSyncReport, error) {
	f.record("check " + id)
	if f.checkErr != nil {
		return TrackerSyncReport{}, f.checkErr
	}
	return TrackerSyncReport{Offline: true, Unpushed: 2}, nil
}

func (f *fakeTracker) Sync(_ context.Context, id string) (TrackerSyncReport, error) {
	f.record("sync " + id)
	if f.checkErr != nil {
		return TrackerSyncReport{}, f.checkErr
	}
	return TrackerSyncReport{Pushed: true, Received: 3}, nil
}

func (f *fakeTracker) MarkSeen(id string, add TrackerSeen) error {
	f.record("seen " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen.Badge = append(f.seen.Badge, add.Badge...)
	f.seen.Read = append(f.seen.Read, add.Read...)
	return nil
}

func (f *fakeTracker) Profiles() (TrackerProfiles, error) {
	f.record("profiles")
	f.mu.Lock()
	defer f.mu.Unlock()
	return TrackerProfiles{Last: len(f.profiles) - 1, Profiles: f.profiles}, nil
}

func (f *fakeTracker) SaveProfile(p TrackerProfile) error {
	f.record("save " + p.Given)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.profiles = append(f.profiles, p)
	return nil
}

func (f *fakeTracker) Register(_ context.Context, req TrackerRegisterRequest) (TrackerRegisterResult, error) {
	f.record("register " + req.ID)
	if f.regErr != nil {
		return TrackerRegisterResult{}, f.regErr
	}
	return TrackerRegisterResult{Store: req.ID, Creator: true, Profile: TrackerProfile{Given: "Ada"}}, nil
}

func (f *fakeTracker) Render(_ context.Context, id string, req TrackerRenderRequest) (string, error) {
	f.record("render " + id)
	if f.renderErr != nil {
		return "", f.renderErr
	}
	out := "<p>" + req.Text + "</p>"
	for _, a := range req.Attachments {
		out += "[" + a.OID + " " + a.Name + "]"
	}
	return out, nil
}

func (f *fakeTracker) StageAttachment(_ context.Context, id, name string, data []byte) (TrackerAttachment, error) {
	f.record("stage " + id)
	mime := http.DetectContentType(data)
	if !trackerAttachmentTypes[mime] {
		return TrackerAttachment{}, ErrTrackerNotImage
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(data) > f.maxBytes {
		return TrackerAttachment{}, ErrTrackerAttachmentTooLarge
	}
	sum := sha256.Sum256(data)
	oid := hex.EncodeToString(sum[:])
	f.blobs[oid] = append([]byte(nil), data...)
	return TrackerAttachment{OID: oid, Name: name, Size: int64(len(data)), Mime: mime}, nil
}

func (f *fakeTracker) ReadAttachment(_ context.Context, id, oid string) ([]byte, error) {
	f.record("read " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.blobs[oid]
	if !ok {
		return nil, ErrTrackerAttachmentNotFound
	}
	return data, nil
}

// startTrackerDaemon starts a daemon over an empty session source with
// tracker as its tracker host.
func startTrackerDaemon(t *testing.T, tracker TrackerHost) *Daemon {
	t.Helper()
	return startIndexedDaemon(t, t.TempDir(), newFakeSource(), time.Hour, withTracker(tracker))
}

// call runs one request through d's handler as a viewer on this machine,
// same-origin for a POST, and returns the status and body.
func call(d *Daemon, method, path, body string) (int, string) {
	host := "127.0.0.1:" + strconv.Itoa(d.Endpoint().Port)
	rec := serve(d, viewer{peer: "127.0.0.1:50000", host: host}, method, path, body)
	return rec.Code, rec.Body.String()
}

// callRaw runs one request with a raw body and the given headers through
// d's handler as a viewer on this machine; a POST without an Origin header
// in headers gets the daemon's own.
func callRaw(d *Daemon, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	host := "127.0.0.1:" + strconv.Itoa(d.Endpoint().Port)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:50000"
	req.Host = host
	if method == http.MethodPost {
		req.Header.Set("Origin", "http://"+host)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	return rec
}

// trackerRev reads the revision the stores route reports.
func trackerRev(t *testing.T, d *Daemon) int64 {
	t.Helper()
	code, body := call(d, http.MethodGet, "/tracker/stores", "")
	var got struct {
		Rev int64 `json:"rev"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &got) != nil {
		t.Fatalf("GET /tracker/stores = %d %s", code, body)
	}
	return got.Rev
}

// trackerEvents reads events until quiet passes with none and returns the
// tracker events' data.
func trackerEvents(events <-chan sseEvent, quiet time.Duration) []string {
	var got []string
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return got
			}
			if ev.name == "tracker" {
				got = append(got, ev.data)
			}
		case <-time.After(quiet):
			return got
		}
	}
}

func openStream(t *testing.T, url string) <-chan sseEvent {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream %s = %d", url, resp.StatusCode)
	}
	events := make(chan sseEvent, 64)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	return events
}

func TestNoTrackerHostServesNoTrackerRoutes(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/tracker/stores", "/tracker/profiles", "/tracker/stores/team/view"} {
		if code, _ := get(t, f.base+path); code != http.StatusNotFound {
			t.Errorf("GET %s without a tracker host = %d, want 404", path, code)
		}
	}
	for _, path := range []string{"/s/livesession/events", "/events"} {
		events := openStream(t, f.base+path)
		f.d.TrackerChanged() // harmless without a host
		if got := trackerEvents(events, 600*time.Millisecond); len(got) != 0 {
			t.Errorf("%s sent tracker events %v without a tracker host", path, got)
		}
	}
}

func TestTrackerRoutesAnswer(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)

	code, body := call(d, http.MethodGet, "/tracker/stores", "")
	if code != http.StatusOK || body != `{"rev":0,"stores":[{"id":"team","label":"Team","member":"ada","remote":"https://***@example.invalid/x.git"}],"support":false}`+"\n" {
		t.Fatalf("stores = %d %s", code, body)
	}

	code, body = call(d, http.MethodGet, "/tracker/stores/team/view", "")
	if code != http.StatusOK || body != `{"view":{"store":{"name":"team"}},"seen":{"badge":[],"read":[]}}`+"\n" {
		t.Fatalf("view = %d %s", code, body)
	}

	code, body = call(d, http.MethodPost, "/tracker/stores/team/dispatch", `{"events":[{"type":"item.create","payload":{"title":"x"}},{"type":"comment.add","payload":{"item":"$1"}}]}`)
	if code != http.StatusOK || body != `{"ids":["ada@pc:1","ada@pc:2"]}`+"\n" {
		t.Fatalf("dispatch = %d %s", code, body)
	}

	code, body = call(d, http.MethodPost, "/tracker/stores/team/check", "")
	if code != http.StatusOK || body != `{"noRemote":false,"offline":true,"closed":false,"pushed":false,"received":0,"unpushed":2}`+"\n" {
		t.Fatalf("check = %d %s", code, body)
	}
	code, body = call(d, http.MethodPost, "/tracker/stores/team/sync", "")
	if code != http.StatusOK || body != `{"noRemote":false,"offline":false,"closed":false,"pushed":true,"received":3,"unpushed":0}`+"\n" {
		t.Fatalf("sync = %d %s", code, body)
	}

	code, body = call(d, http.MethodPost, "/tracker/stores/team/seen", `{"badge":["b1"],"read":["r1"]}`)
	if code != http.StatusNoContent {
		t.Fatalf("seen = %d %s", code, body)
	}
	if _, body = call(d, http.MethodGet, "/tracker/stores/team/view", ""); !strings.Contains(body, `"seen":{"badge":["b1"],"read":["r1"]}`) {
		t.Fatalf("view after seen = %s", body)
	}

	code, body = call(d, http.MethodGet, "/tracker/profiles", "")
	if code != http.StatusOK || body != `{"last":-1,"profiles":[]}`+"\n" {
		t.Fatalf("profiles = %d %s", code, body)
	}
	code, body = call(d, http.MethodPost, "/tracker/profiles", `{"org":"Acme","family":"Lovelace","given":"Ada"}`)
	if code != http.StatusOK || body != `{"last":0,"profiles":[{"org":"Acme","family":"Lovelace","given":"Ada"}]}`+"\n" {
		t.Fatalf("profile save = %d %s", code, body)
	}

	code, body = call(d, http.MethodPost, "/tracker/register", `{"id":"new","label":"New","remote":"","member":"ada","profile":{"given":"Ada"}}`)
	if code != http.StatusOK || body != `{"store":"new","creator":true,"profile":{"org":"","family":"","given":"Ada"}}`+"\n" {
		t.Fatalf("register = %d %s", code, body)
	}

	code, body = call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"**md**"}`)
	if code != http.StatusOK || body != `{"html":"\u003cp\u003e**md**\u003c/p\u003e"}`+"\n" {
		t.Fatalf("render = %d %s", code, body)
	}
	// The pending attachment list reaches the host as sent.
	code, body = call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"x","attachments":[{"oid":"abc","name":"a.png","size":3,"mime":"image/png"}]}`)
	if code != http.StatusOK || body != `{"html":"\u003cp\u003ex\u003c/p\u003e[abc a.png]"}`+"\n" {
		t.Fatalf("render with attachments = %d %s", code, body)
	}

	for _, wrong := range []struct{ method, path string }{
		{http.MethodPost, "/tracker/stores"},
		{http.MethodPost, "/tracker/stores/team/view"},
		{http.MethodGet, "/tracker/stores/team/dispatch"},
		{http.MethodGet, "/tracker/stores/team/sync"},
		{http.MethodGet, "/tracker/stores/team/render"},
		{http.MethodGet, "/tracker/register"},
		{http.MethodDelete, "/tracker/profiles"},
	} {
		if code, _ := call(d, wrong.method, wrong.path, "{}"); code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", wrong.method, wrong.path, code)
		}
	}
	for _, path := range []string{"/tracker/", "/tracker/nope", "/tracker/stores/team", "/tracker/stores/team/nope"} {
		if code, _ := call(d, http.MethodGet, path, ""); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
}

func TestTrackerRouteErrors(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)

	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/tracker/stores/other/view"},
		{http.MethodPost, "/tracker/stores/other/dispatch"},
		{http.MethodPost, "/tracker/stores/other/seen"},
		{http.MethodPost, "/tracker/stores/other/render"},
	} {
		code, body := call(d, r.method, r.path, `{"events":[{"type":"x"}]}`)
		if code != http.StatusNotFound || body != `{"error":"unknown store"}`+"\n" {
			t.Errorf("%s %s = %d %s, want 404 unknown store", r.method, r.path, code, body)
		}
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("an unknown store reached the host: %v", got)
	}

	fake.viewErr = errors.New("view broke")
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/view", ""); code != http.StatusInternalServerError || !strings.Contains(body, `"error":"view broke"`) {
		t.Errorf("view error = %d %s", code, body)
	}
	fake.checkErr = errors.New("check broke")
	for _, op := range []string{"check", "sync"} {
		if code, body := call(d, http.MethodPost, "/tracker/stores/team/"+op, ""); code != http.StatusInternalServerError || !strings.Contains(body, "check broke") {
			t.Errorf("%s error = %d %s", op, code, body)
		}
	}
	fake.renderErr = errors.New("render broke")
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"x"}`); code != http.StatusInternalServerError || !strings.Contains(body, "render broke") {
		t.Errorf("render error = %d %s", code, body)
	}
	fake.saveErr = errors.New("bad profile")
	if code, body := call(d, http.MethodPost, "/tracker/profiles", `{"given":""}`); code != http.StatusBadRequest || !strings.Contains(body, "bad profile") {
		t.Errorf("profile save error = %d %s", code, body)
	}
	fake.regErr = errors.New("clone failed")
	if code, body := call(d, http.MethodPost, "/tracker/register", `{"id":"x"}`); code != http.StatusUnprocessableEntity || !strings.Contains(body, "clone failed") {
		t.Errorf("register error = %d %s", code, body)
	}

	fake.storesErr = errors.New("tracker host unavailable: no home")
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/tracker/stores"},
		{http.MethodGet, "/tracker/stores/team/view"},
		{http.MethodPost, "/tracker/stores/team/sync"},
		{http.MethodPost, "/tracker/stores/team/render"},
		{http.MethodGet, "/tracker/profiles"},
		{http.MethodPost, "/tracker/register"},
	} {
		code, body := call(d, r.method, r.path, `{"id":"x"}`)
		if code != http.StatusServiceUnavailable || body != `{"error":"tracker host unavailable: no home"}`+"\n" {
			t.Errorf("%s %s with a failed host = %d %s, want 503 with its error", r.method, r.path, code, body)
		}
	}
}

func TestTrackerDispatchForcesHumanOriginAndRefuses(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)

	if code, body := call(d, http.MethodPost, "/tracker/stores/team/dispatch", `{"events":[{"type":"item.create","payload":{},"origin":"agent"},{"type":"item.create","payload":{}}]}`); code != http.StatusOK {
		t.Fatalf("dispatch = %d %s", code, body)
	}
	for i, ev := range fake.events {
		if ev.Origin != "human" {
			t.Errorf("event %d reached the host with origin %q, want human", i, ev.Origin)
		}
	}

	fake.dispatch = &TrackerRefusal{Index: 1, Type: "comment.add", Reason: "unknown item"}
	code, body := call(d, http.MethodPost, "/tracker/stores/team/dispatch", `{"events":[{"type":"comment.add","payload":{}}]}`)
	if code != http.StatusConflict || body != `{"error":"event 1 (comment.add) would be flagged: unknown item","refusal":{"index":1,"type":"comment.add","reason":"unknown item"}}`+"\n" {
		t.Fatalf("refusal = %d %s", code, body)
	}
	fake.dispatch = errors.New("disk full")
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/dispatch", `{"events":[{"type":"x"}]}`); code != http.StatusInternalServerError || !strings.Contains(body, "disk full") {
		t.Fatalf("dispatch error = %d %s", code, body)
	}

	before := len(fake.recorded())
	oversize := `{"events":[{"type":"x","payload":"` + strings.Repeat("a", trackerBodyLimit) + `"}]}`
	for name, body := range map[string]string{
		"malformed": `{"events":`,
		"oversize":  oversize,
		"empty":     `{"events":[]}`,
		"absent":    `{}`,
	} {
		if code, _ := call(d, http.MethodPost, "/tracker/stores/team/dispatch", body); code != http.StatusBadRequest {
			t.Errorf("%s body = %d, want 400", name, code)
		}
	}
	for _, path := range []string{"/tracker/stores/team/seen", "/tracker/stores/team/render", "/tracker/profiles", "/tracker/register"} {
		if code, _ := call(d, http.MethodPost, path, "not json"); code != http.StatusBadRequest {
			t.Errorf("malformed body to %s = %d, want 400", path, code)
		}
	}
	// The render route takes the write body limit too.
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"`+strings.Repeat("a", trackerBodyLimit)+`"}`); code != http.StatusBadRequest || !strings.Contains(body, "too large") {
		t.Errorf("oversize render body = %d %s, want 400 too large", code, body)
	}
	if got := fake.recorded(); len(got) != before {
		t.Fatalf("a refused body reached the host: %v", got[before:])
	}
}

// trackerPosts is every tracker POST route with a body it accepts.
func trackerPosts() []struct{ path, body string } {
	return []struct{ path, body string }{
		{"/tracker/stores/team/dispatch", `{"events":[{"type":"item.create","payload":{}}]}`},
		{"/tracker/stores/team/check", ``},
		{"/tracker/stores/team/sync", ``},
		{"/tracker/stores/team/seen", `{"badge":["b"]}`},
		{"/tracker/profiles", `{"given":"Ada"}`},
		{"/tracker/register", `{"id":"new"}`},
	}
}

func TestTrackerPostsRequireTheDaemonsOwnOrigin(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	base := endpointURL(d.Endpoint(), "")
	// The render route is a POST that writes nothing; it takes the same rule.
	posts := append(trackerPosts(), struct{ path, body string }{"/tracker/stores/team/render", `{"text":"x"}`})
	for _, route := range posts {
		for _, origin := range []string{"", "null", "http://evil.example"} {
			code, header, _ := postJSON(t, base+route.path, origin, route.body)
			if code != http.StatusForbidden {
				t.Errorf("%s with Origin %q = %d, want 403", route.path, origin, code)
			}
			assertNoCORS(t, header)
		}
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("a cross-origin post reached the host: %v", got)
	}
	if rev := trackerRev(t, d); rev != 0 {
		t.Fatalf("a refused post bumped the revision to %d", rev)
	}
	// The same routes from the daemon's own origin pass.
	for _, route := range posts {
		if code, _, body := postJSON(t, base+route.path, base, route.body); code >= 300 {
			t.Errorf("same-origin %s = %d %s", route.path, code, body)
		}
	}
}

func TestTrackerWritesBumpTheRevision(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	last := trackerRev(t, d)
	if last != 0 {
		t.Fatalf("initial revision = %d", last)
	}
	for _, route := range trackerPosts() {
		if code, body := call(d, http.MethodPost, route.path, route.body); code >= 300 {
			t.Fatalf("%s = %d %s", route.path, code, body)
		}
		if rev := trackerRev(t, d); rev <= last {
			t.Errorf("revision after %s = %d, want above %d", route.path, rev, last)
		} else {
			last = rev
		}
	}
	// A failed check is recorded by the host, so it bumps too.
	fake.checkErr = errors.New("offline")
	call(d, http.MethodPost, "/tracker/stores/team/check", "")
	if rev := trackerRev(t, d); rev <= last {
		t.Errorf("revision after a failed check = %d, want above %d", rev, last)
	}
	// Reads do not.
	last = trackerRev(t, d)
	call(d, http.MethodGet, "/tracker/stores/team/view", "")
	call(d, http.MethodGet, "/tracker/profiles", "")
	call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"x"}`)
	if rev := trackerRev(t, d); rev != last {
		t.Errorf("a read moved the revision from %d to %d", last, rev)
	}
}

func TestStreamsSendTheTrackerRevision(t *testing.T) {
	f := startPageFixture(t, withTracker(newFakeTracker()))
	for i, path := range []string{"/s/livesession/events", "/events"} {
		events := openStream(t, f.base+path)
		want := strconv.Itoa(i)
		if got := trackerEvents(events, 600*time.Millisecond); len(got) != 1 || got[0] != `{"rev":`+want+`}` {
			t.Fatalf("%s on connect sent tracker events %v, want one rev %s", path, got, want)
		}
		f.d.TrackerChanged()
		want = strconv.Itoa(i + 1)
		if got := trackerEvents(events, 600*time.Millisecond); len(got) != 1 || got[0] != `{"rev":`+want+`}` {
			t.Fatalf("%s after TrackerChanged sent %v, want one rev %s", path, got, want)
		}
	}
}

// tinyPNG is enough of a PNG for http.DetectContentType.
var tinyPNG = append([]byte("\x89PNG\r\n\x1a\n"), 0, 0, 0, 13, 'I', 'H', 'D', 'R')

func TestTrackerAttachmentUpload(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	const upload = "/tracker/stores/team/attachments"

	rec := callRaw(d, http.MethodPost, upload+"?name=shot.png", tinyPNG, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	var got TrackerAttachment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tinyPNG)
	want := TrackerAttachment{OID: hex.EncodeToString(sum[:]), Name: "shot.png", Size: int64(len(tinyPNG)), Mime: "image/png"}
	if got != want {
		t.Fatalf("upload answered %+v, want %+v", got, want)
	}

	if rec := callRaw(d, http.MethodPost, upload+"?name=a.txt", []byte("just text"), nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("non-image upload = %d %s, want 415", rec.Code, rec.Body)
	}
	big := append(append([]byte(nil), tinyPNG...), make([]byte, fake.maxBytes)...)
	if rec := callRaw(d, http.MethodPost, upload+"?name=big.png", big, nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize upload = %d %s, want 413", rec.Code, rec.Body)
	}
	before := len(fake.recorded())
	if rec := callRaw(d, http.MethodPost, upload, tinyPNG, map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin upload = %d %s, want 403", rec.Code, rec.Body)
	}
	if got := fake.recorded(); len(got) != before {
		t.Errorf("a cross-origin upload reached the host: %v", got[before:])
	}
	if rec := callRaw(d, http.MethodGet, upload, nil, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET upload path = %d, want 405", rec.Code)
	}
	if rec := callRaw(d, http.MethodPost, "/tracker/stores/other/attachments", tinyPNG, nil); rec.Code != http.StatusNotFound {
		t.Errorf("upload to an unknown store = %d, want 404", rec.Code)
	}
}

func TestTrackerAttachmentRead(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	rec := callRaw(d, http.MethodPost, "/tracker/stores/team/attachments?name=shot.png", tinyPNG, nil)
	var att TrackerAttachment
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &att) != nil {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	read := "/tracker/stores/team/attachments/" + att.OID

	rec = callRaw(d, http.MethodGet, read, nil, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), tinyPNG) {
		t.Fatalf("read = %d %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	for k, want := range map[string]string{
		"Content-Type":                 "image/png",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Security-Policy":      imageContentSecurityPolicy,
		"Content-Length":               strconv.Itoa(len(tinyPNG)),
	} {
		if got := h.Get(k); got != want {
			t.Errorf("read %s = %q, want %q", k, got, want)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "same-origin"}); rec.Code != http.StatusOK {
		t.Errorf("same-origin read = %d, want 200", rec.Code)
	}

	for name, path := range map[string]string{
		"unknown oid":   "/tracker/stores/team/attachments/" + strings.Repeat("0", 64),
		"malformed oid": "/tracker/stores/team/attachments/abc",
		"uppercase oid": "/tracker/stores/team/attachments/" + strings.ToUpper(att.OID),
		"unknown store": "/tracker/stores/other/attachments/" + att.OID,
	} {
		if rec := callRaw(d, http.MethodGet, path, nil, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s read = %d, want 404", name, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusNotFound {
		t.Errorf("cross-site read = %d, want 404", rec.Code)
	}
	if rec := callRaw(d, http.MethodPost, read, tinyPNG, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST read path = %d, want 405", rec.Code)
	}

	// Content that does not sniff as an accepted image is never served.
	sum := sha256.Sum256([]byte("<svg/>"))
	oid := hex.EncodeToString(sum[:])
	fake.mu.Lock()
	fake.blobs[oid] = []byte("<svg/>")
	fake.mu.Unlock()
	if rec := callRaw(d, http.MethodGet, "/tracker/stores/team/attachments/"+oid, nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("non-image content read = %d, want 404", rec.Code)
	}
}
