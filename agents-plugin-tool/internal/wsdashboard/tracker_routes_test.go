package wsdashboard

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	wikiBlobs map[string][]byte // wiki images by wiki+"/"+oid
	maxBytes  int               // the fake store's attachment limit
	readErr   error             // fails every attachment and wiki image read, when set
	// wikis holds each fake wiki's pages by path; wikiRev counts its
	// saves (the commit); wikiErr fails every wiki call, wikiConflict
	// refuses every save, when set.
	wikis        map[string]map[string]string
	wikiRev      int
	wikiErr      error
	wikiConflict *TrackerWikiConflict
	wikiSaves    []TrackerWikiSave
}

func newFakeTracker() *fakeTracker {
	return &fakeTracker{
		stores:    []TrackerStore{{ID: "team", Label: "Team", Member: "ada", Remote: "https://***@example.invalid/x.git"}},
		blobs:     map[string][]byte{},
		wikiBlobs: map[string][]byte{},
		wikis:     map[string]map[string]string{},
		maxBytes:  1 << 20,
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

// Search answers every view item id containing query, and refuses a
// query starting with "bad:".
func (f *fakeTracker) Search(_ context.Context, id, query string) ([]string, error) {
	f.record("search " + id + " " + query)
	if strings.HasPrefix(query, "bad:") {
		return nil, &TrackerSearchError{Message: `search "` + query + `": bad takes nothing`}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.viewErr != nil {
		return nil, f.viewErr
	}
	var out []string
	for _, it := range []string{"ada.pc#1", "ada.pc#2"} {
		if strings.Contains(it, query) {
			out = append(out, it)
		}
	}
	return out, nil
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
	if req.Wiki != "" {
		out += "{wiki " + req.Wiki + "}"
	}
	return out, nil
}

func (f *fakeTracker) wikiCommit() string { return "c" + strconv.Itoa(f.wikiRev) }

func (f *fakeTracker) WikiList(_ context.Context, id string) ([]TrackerWiki, error) {
	f.record("wiki-list " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.wikiErr != nil {
		return nil, f.wikiErr
	}
	var out []TrackerWiki
	for name := range f.wikis {
		out = append(out, TrackerWiki{Name: name, Tip: f.wikiCommit()})
	}
	return out, nil
}

func (f *fakeTracker) WikiPages(_ context.Context, id, wiki string) (TrackerWikiPages, error) {
	f.record("wiki-pages " + id + " " + wiki)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.wikiErr != nil {
		return TrackerWikiPages{}, f.wikiErr
	}
	pages, ok := f.wikis[wiki]
	if !ok {
		return TrackerWikiPages{}, ErrTrackerWikiNotFound
	}
	out := TrackerWikiPages{Commit: f.wikiCommit()}
	for path := range pages {
		out.Pages = append(out.Pages, path)
	}
	return out, nil
}

func (f *fakeTracker) WikiRead(_ context.Context, id, wiki, path string) (TrackerWikiPage, error) {
	f.record("wiki-read " + id + " " + wiki + " " + path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.wikiErr != nil {
		return TrackerWikiPage{}, f.wikiErr
	}
	content, ok := f.wikis[wiki][path]
	if !ok {
		return TrackerWikiPage{}, ErrTrackerWikiNotFound
	}
	return TrackerWikiPage{Path: path, Content: content, Commit: f.wikiCommit(), HTML: "<p>{wiki " + wiki + "} " + content + "</p>"}, nil
}

func (f *fakeTracker) WikiSave(_ context.Context, id, wiki string, req TrackerWikiSave) (TrackerWikiSaveResult, error) {
	f.record("wiki-save " + id + " " + wiki)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wikiSaves = append(f.wikiSaves, req)
	if f.wikiErr != nil {
		return TrackerWikiSaveResult{}, f.wikiErr
	}
	if f.wikiConflict != nil {
		return TrackerWikiSaveResult{}, f.wikiConflict
	}
	_, exists := f.wikis[wiki]
	if req.NewWiki == exists || (!req.NewWiki && req.Base != f.wikiCommit()) {
		return TrackerWikiSaveResult{}, errors.Join(ErrTrackerWikiRefused, errors.New("fake: wrong new-wiki form or base"))
	}
	if req.Op != "write" || req.Path == "" {
		return TrackerWikiSaveResult{}, errors.Join(ErrTrackerWikiRefused, errors.New("fake: write only"))
	}
	if !exists {
		f.wikis[wiki] = map[string]string{}
	}
	f.wikis[wiki][req.Path] = req.Content
	f.wikiRev++
	return TrackerWikiSaveResult{Commit: f.wikiCommit(), Created: !exists}, nil
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
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.blobs[oid]
	if !ok {
		return nil, ErrTrackerAttachmentNotFound
	}
	return data, nil
}

func (f *fakeTracker) ReadWikiImage(_ context.Context, id, wiki, oid string) ([]byte, error) {
	f.record("wiki-image " + id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.wikiBlobs[wiki+"/"+oid]
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

	// The query reaches the host decoded; no match is [], not null; a
	// read does not bump the revision.
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=%232", "")
	if code != http.StatusOK || body != `{"ids":["ada.pc#2"]}`+"\n" {
		t.Fatalf("search = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=none", "")
	if code != http.StatusOK || body != `{"ids":[]}`+"\n" {
		t.Fatalf("search without a match = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=bad:x", "")
	if code != http.StatusBadRequest || body != `{"error":"search \"bad:x\": bad takes nothing"}`+"\n" {
		t.Fatalf("malformed search = %d %s", code, body)
	}
	if rev := trackerRev(t, d); rev != 0 {
		t.Fatalf("a search bumped the revision to %d", rev)
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
		{http.MethodPost, "/tracker/stores/team/search"},
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
		{http.MethodGet, "/tracker/stores/other/search"},
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
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/search?q=x", ""); code != http.StatusInternalServerError || !strings.Contains(body, `"error":"view broke"`) {
		t.Errorf("search error = %d %s", code, body)
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

func TestTrackerWikiImageRead(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	sum := sha256.Sum256(tinyPNG)
	oid := hex.EncodeToString(sum[:])
	textSum := sha256.Sum256([]byte("plain text"))
	textOID := hex.EncodeToString(textSum[:])
	fake.mu.Lock()
	fake.wikiBlobs["notes/"+oid] = tinyPNG
	fake.wikiBlobs["notes/"+textOID] = []byte("plain text")
	fake.mu.Unlock()
	read := "/tracker/stores/team/wiki/notes/attachments/" + oid

	rec := callRaw(d, http.MethodGet, read, nil, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), tinyPNG) {
		t.Fatalf("read = %d %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	for k, want := range map[string]string{
		"Content-Type":                 "image/png",
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      imageContentSecurityPolicy,
		"Cache-Control":                "private, max-age=31536000, immutable",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Length":               strconv.Itoa(len(tinyPNG)),
	} {
		if got := h.Get(k); got != want {
			t.Errorf("read %s = %q, want %q", k, got, want)
		}
	}
	for _, site := range []string{"none", "same-origin"} {
		if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": site}); rec.Code != http.StatusOK {
			t.Errorf("Sec-Fetch-Site %s read = %d, want 200", site, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusNotFound {
		t.Errorf("cross-site read = %d, want 404", rec.Code)
	}

	for name, path := range map[string]string{
		"non-image content": "/tracker/stores/team/wiki/notes/attachments/" + textOID,
		"unknown oid":       "/tracker/stores/team/wiki/notes/attachments/" + strings.Repeat("0", 64),
		"unknown wiki":      "/tracker/stores/team/wiki/other/attachments/" + oid,
		"uppercase wiki":    "/tracker/stores/team/wiki/Notes/attachments/" + oid,
		"leading hyphen":    "/tracker/stores/team/wiki/-notes/attachments/" + oid,
		"uppercase oid":     "/tracker/stores/team/wiki/notes/attachments/" + strings.ToUpper(oid),
		"malformed oid":     "/tracker/stores/team/wiki/notes/attachments/abc",
		"no attachments":    "/tracker/stores/team/wiki/notes/" + oid,
		"unknown store":     "/tracker/stores/other/wiki/notes/attachments/" + oid,
	} {
		if rec := callRaw(d, http.MethodGet, path, nil, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s read = %d, want 404", name, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodPost, read, tinyPNG, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST read path = %d, want 405", rec.Code)
	}
}

// TestTrackerBusyReadsAnswerRetryAfter: a wiki JSON read or an image read
// the host answers ErrTrackerBusy (the store stayed held by a fetch or
// push) is a 503 with Retry-After, so the page reads it again shortly.
func TestTrackerBusyReadsAnswerRetryAfter(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	busy := fmt.Errorf("%w: team", ErrTrackerBusy)
	fake.mu.Lock()
	fake.wikiErr, fake.readErr = busy, busy
	fake.mu.Unlock()
	oid := strings.Repeat("a", 64)
	for _, path := range []string{
		"/tracker/stores/team/wiki",
		"/tracker/stores/team/wiki/notes/pages",
		"/tracker/stores/team/wiki/notes/page?path=home",
		"/tracker/stores/team/attachments/" + oid,
		"/tracker/stores/team/wiki/notes/attachments/" + oid,
	} {
		rec := callRaw(d, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != trackerBusyRetry || !strings.Contains(rec.Body.String(), "store busy") {
			t.Errorf("busy %s = %d Retry-After %q %s, want 503 with Retry-After %s", path, rec.Code, rec.Header().Get("Retry-After"), rec.Body, trackerBusyRetry)
		}
	}
}

// TestTrackerWikiRoutes: the wiki list, page list, page read, and save
// reach the host with the wiki name and answer its results; a save bumps
// the revision; an unknown wiki or page is 404, a refused save 422, a
// conflict 409 with both texts, a host failure 500; the render request's
// wiki mode reaches the host; a malformed wiki name, a bare wiki path, a
// wrong method, a cross-origin save, and an unknown store are refused
// before the host.
func TestTrackerWikiRoutes(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)

	code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki", "")
	if code != http.StatusOK || body != `{"wikis":[]}`+"\n" {
		t.Fatalf("wiki list = %d %s", code, body)
	}
	for name, path := range map[string]string{
		"pages of an unknown wiki": "/tracker/stores/team/wiki/notes/pages",
		"page of an unknown wiki":  "/tracker/stores/team/wiki/notes/page?path=home",
	} {
		if code, body := call(d, http.MethodGet, path, ""); code != http.StatusNotFound || !strings.Contains(body, "not found") {
			t.Errorf("%s = %d %s, want 404", name, code, body)
		}
	}
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page", ""); code != http.StatusBadRequest || !strings.Contains(body, "path required") {
		t.Errorf("page without a path = %d %s, want 400", code, body)
	}

	rev := trackerRev(t, d)
	save := `{"base":"","newWiki":true,"op":"write","path":"guide/home","content":"# Home\n","attachments":[{"oid":"abc","name":"a.png","size":3,"mime":"image/png"}]}`
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", save)
	if code != http.StatusOK || body != `{"commit":"c1","merged":false,"created":true,"warnings":[]}`+"\n" {
		t.Fatalf("save = %d %s", code, body)
	}
	if got := trackerRev(t, d); got != rev+1 {
		t.Errorf("revision after a save = %d, want %d", got, rev+1)
	}
	if n := len(fake.wikiSaves); n != 1 || fake.wikiSaves[0].Path != "guide/home" || len(fake.wikiSaves[0].Attachments) != 1 || fake.wikiSaves[0].Attachments[0].OID != "abc" {
		t.Fatalf("the save reached the host as %+v", fake.wikiSaves)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki", "")
	if code != http.StatusOK || body != `{"wikis":[{"name":"notes","tip":"c1","unpushed":false}]}`+"\n" {
		t.Fatalf("wiki list after a save = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/pages", "")
	if code != http.StatusOK || body != `{"pages":["guide/home"],"commit":"c1"}`+"\n" {
		t.Fatalf("pages = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=guide%2Fhome", "")
	if code != http.StatusOK || body != `{"path":"guide/home","content":"# Home\n","commit":"c1","html":"\u003cp\u003e{wiki notes} # Home\n\u003c/p\u003e"}`+"\n" {
		t.Fatalf("page = %d %s", code, body)
	}
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=nope", ""); code != http.StatusNotFound {
		t.Errorf("unknown page = %d %s, want 404", code, body)
	}

	// A refusal, a conflict, and a failure, none bumping the revision.
	rev = trackerRev(t, d)
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":"stale","op":"write","path":"guide/home","content":"x"}`)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "wiki save refused") {
		t.Errorf("refused save = %d %s, want 422", code, body)
	}
	fake.wikiConflict = &TrackerWikiConflict{Path: "guide/home", Kind: "text", Reason: "lines overlap", Base: "c1", Theirs: "c2", OursText: "mine", TheirsText: "theirs"}
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":"c1","op":"write","path":"guide/home","content":"mine"}`)
	want := `{"conflict":{"path":"guide/home","kind":"text","reason":"lines overlap","base":"c1","theirs":"c2","oursText":"mine","theirsText":"theirs"},"error":"wiki text refused on guide/home: lines overlap"}` + "\n"
	if code != http.StatusConflict || body != want {
		t.Errorf("conflicting save = %d %s, want 409 %s", code, body, want)
	}
	fake.wikiConflict = nil
	fake.wikiErr = errors.New("wiki broke")
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/tracker/stores/team/wiki"},
		{http.MethodGet, "/tracker/stores/team/wiki/notes/pages"},
		{http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=guide/home"},
		{http.MethodPost, "/tracker/stores/team/wiki/notes/save"},
	} {
		if code, body := call(d, r.method, r.path, `{"base":"c1","op":"write","path":"p","content":"x"}`); code != http.StatusInternalServerError || !strings.Contains(body, "wiki broke") {
			t.Errorf("%s %s with a failing host = %d %s, want 500", r.method, r.path, code, body)
		}
	}
	fake.wikiErr = nil
	if got := trackerRev(t, d); got != rev {
		t.Errorf("revision after refused saves = %d, want %d", got, rev)
	}
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":`); code != http.StatusBadRequest || !strings.Contains(body, "malformed") {
		t.Errorf("malformed save = %d %s, want 400", code, body)
	}

	// The render request's wiki mode reaches the host.
	code, body = call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"x","wiki":"notes"}`)
	if code != http.StatusOK || body != `{"html":"\u003cp\u003ex\u003c/p\u003e{wiki notes}"}`+"\n" {
		t.Fatalf("render in wiki mode = %d %q", code, body)
	}

	before := len(fake.recorded())
	for name, r := range map[string]struct {
		method, path string
		want         int
	}{
		"uppercase wiki":   {http.MethodGet, "/tracker/stores/team/wiki/Notes/pages", http.StatusNotFound},
		"leading hyphen":   {http.MethodGet, "/tracker/stores/team/wiki/-notes/pages", http.StatusNotFound},
		"bare wiki":        {http.MethodGet, "/tracker/stores/team/wiki/notes", http.StatusNotFound},
		"op as the wiki":   {http.MethodGet, "/tracker/stores/team/wiki/pages", http.StatusNotFound},
		"unknown wiki op":  {http.MethodGet, "/tracker/stores/team/wiki/notes/history", http.StatusNotFound},
		"POST list":        {http.MethodPost, "/tracker/stores/team/wiki", http.StatusMethodNotAllowed},
		"POST pages":       {http.MethodPost, "/tracker/stores/team/wiki/notes/pages", http.StatusMethodNotAllowed},
		"GET save":         {http.MethodGet, "/tracker/stores/team/wiki/notes/save", http.StatusMethodNotAllowed},
		"unknown store":    {http.MethodGet, "/tracker/stores/other/wiki", http.StatusNotFound},
		"unknown store op": {http.MethodPost, "/tracker/stores/other/wiki/notes/save", http.StatusNotFound},
	} {
		if code, body := call(d, r.method, r.path, "{}"); code != r.want {
			t.Errorf("%s: %s %s = %d %s, want %d", name, r.method, r.path, code, body, r.want)
		}
	}
	if rec := callRaw(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", []byte(save), map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin save = %d, want 403", rec.Code)
	}
	if got := fake.recorded(); len(got) != before {
		t.Errorf("refused requests reached the host: %v", got[before:])
	}
}
