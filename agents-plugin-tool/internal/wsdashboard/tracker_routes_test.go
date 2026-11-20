package wsdashboard

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

	// excerpts=1 adds the hits beside the distinct ids: a body hit and a
	// comment hit with its comment id, then an item hit. Any other value
	// leaves the answer as it was.
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=%23&excerpts=1", "")
	wantHits := `{"hits":[` +
		`{"id":"ada.pc#1","title":"One","alias":"one","kind":"task","status":"doing","field":"body","text":"the …body… text"},` +
		`{"id":"ada.pc#1","title":"One","alias":"one","kind":"task","status":"doing","field":"comment","comment":"lena.pc#1","text":"a comment"},` +
		`{"id":"ada.pc#2","title":"Two","alias":"","kind":"topic","status":"","hidden":true,"field":"item"}],` +
		`"ids":["ada.pc#1","ada.pc#2"]}` + "\n"
	if code != http.StatusOK || body != wantHits {
		t.Fatalf("search with excerpts = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=none&excerpts=1", "")
	if code != http.StatusOK || body != `{"hits":[],"ids":[]}`+"\n" {
		t.Fatalf("search with excerpts and no match = %d %s", code, body)
	}
	for _, q := range []string{"%232", "%232&excerpts=0", "%232&excerpts=true", "%232&excerpts="} {
		code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q="+q, "")
		if code != http.StatusOK || body != `{"ids":["ada.pc#2"]}`+"\n" {
			t.Fatalf("search ?q=%s = %d %s", q, code, body)
		}
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/search?q=bad:x&excerpts=1", "")
	if code != http.StatusBadRequest || body != `{"error":"search \"bad:x\": bad takes nothing"}`+"\n" {
		t.Fatalf("malformed search with excerpts = %d %s", code, body)
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
		{"/tracker/stores/team/label", `{"label":"Team"}`},
		{"/tracker/stores/team/hidden", `{"hidden":false}`},
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
