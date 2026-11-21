// tracker_test_helpers_test.go holds the tracker host fixture and stream helpers.
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
	unregErr  error // Unregister's error, when set; else the store leaves the list
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

// The search-hits method set the palette's tracker content source reads:
// pinned beside the fake so a signature change fails to compile.
var (
	_ TrackerHost                                                                    = (*fakeTracker)(nil)
	_ func(TrackerHost, context.Context, string, string) ([]TrackerSearchHit, error) = TrackerHost.SearchHits
)

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

// SearchHits answers the fake's two items as a real host would: ada.pc#1
// matched in its body and in a comment, ada.pc#2 on its title only.
func (f *fakeTracker) SearchHits(_ context.Context, id, query string) ([]TrackerSearchHit, error) {
	f.record("searchhits " + id + " " + query)
	if strings.HasPrefix(query, "bad:") {
		return nil, &TrackerSearchError{Message: `search "` + query + `": bad takes nothing`}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.viewErr != nil {
		return nil, f.viewErr
	}
	var out []TrackerSearchHit
	if strings.Contains("ada.pc#1", query) {
		out = append(out,
			TrackerSearchHit{ID: "ada.pc#1", Title: "One", Alias: "one", Kind: "task", Status: "doing", Field: "body", Text: "the …body… text"},
			TrackerSearchHit{ID: "ada.pc#1", Title: "One", Alias: "one", Kind: "task", Status: "doing", Field: "comment", Comment: "lena.pc#1", Text: "a comment"})
	}
	if strings.Contains("ada.pc#2", query) {
		out = append(out, TrackerSearchHit{ID: "ada.pc#2", Title: "Two", Kind: "topic", Hidden: true, Field: "item"})
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

func (f *fakeTracker) Unregister(_ context.Context, id, confirm string) error {
	f.record("unregister " + id + " " + confirm)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unregErr != nil {
		return f.unregErr
	}
	for i, st := range f.stores {
		if st.ID == id {
			f.stores = append(f.stores[:i:i], f.stores[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrTrackerUnknownStore, id)
}

// editStore rewrites the fake store id through edit and returns it.
func (f *fakeTracker) editStore(id string, edit func(*TrackerStore)) (TrackerStore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.stores {
		if f.stores[i].ID == id {
			edit(&f.stores[i])
			return f.stores[i], nil
		}
	}
	return TrackerStore{}, fmt.Errorf("%w: %s", ErrTrackerUnknownStore, id)
}

// Relabel refuses a label holding a newline, as the host refuses a
// control character.
func (f *fakeTracker) Relabel(id, label string) (TrackerStore, error) {
	f.record("label " + id + " " + label)
	if strings.Contains(label, "\n") {
		return TrackerStore{}, fmt.Errorf("%w: it holds a control character", ErrTrackerBadLabel)
	}
	return f.editStore(id, func(st *TrackerStore) { st.Label = label })
}

func (f *fakeTracker) SetHidden(id string, hidden bool) (TrackerStore, error) {
	f.record("hidden " + id + " " + strconv.FormatBool(hidden))
	return f.editStore(id, func(st *TrackerStore) { st.Hidden = hidden })
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
	return TrackerWikiPage{Path: path, Kind: "markdown", Content: content, Commit: f.wikiCommit(), HTML: "<p>{wiki " + wiki + "} " + content + "</p>"}, nil
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
	rec := serve(d, viewer{peer: "127.0.0.1:50000", host: host, localCookie: true}, method, path, body)
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
	localhostTestHandler(nil, d).ServeHTTP(rec, req)
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
	client := &http.Client{Transport: localhostTestHTTPTransport()}
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
