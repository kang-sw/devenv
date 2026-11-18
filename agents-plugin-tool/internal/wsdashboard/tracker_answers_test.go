package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// countingTracker is fakeTracker counting the page reads' host calls, with
// a view whose content and tag the test sets. The daemon reads no file on
// these routes itself, so no host call is no file read.
type countingTracker struct {
	*fakeTracker
	storeReads, viewReads atomic.Int64
	viewMu                sync.Mutex
	raw, tag              string
}

func (c *countingTracker) Stores() ([]TrackerStore, error) {
	c.storeReads.Add(1)
	return c.fakeTracker.Stores()
}

func (c *countingTracker) View(ctx context.Context, id string) (TrackerView, error) {
	c.viewReads.Add(1)
	v, err := c.fakeTracker.View(ctx, id)
	c.viewMu.Lock()
	defer c.viewMu.Unlock()
	v.View, v.Tag = json.RawMessage(c.raw), c.tag
	return v, err
}

func (c *countingTracker) setView(raw, tag string) {
	c.viewMu.Lock()
	c.raw, c.tag = raw, tag
	c.viewMu.Unlock()
}

// TestTrackerPageReadsAnswerFromMemory pins the store list's and the view's
// cached answers (trackerAnswers): an unchanged repeat reads nothing from
// the host and answers a matching If-None-Match 304 with no body; a change
// the daemon learns of (TrackerChanged) reads the view again and answers
// the new one 200; a view read again unchanged, or changed only in its time
// of day (TrackerView.Tag), answers 304; and the seen marks reach the
// cached view without a view read, under a new ETag.
func TestTrackerPageReadsAnswerFromMemory(t *testing.T) {
	c := &countingTracker{fakeTracker: newFakeTracker(), raw: `{"items":1}`}
	d := startTrackerDaemon(t, c)
	const view = "/tracker/stores/team/view"
	get := func(path, etag string) *httptest.ResponseRecorder {
		t.Helper()
		h := map[string]string{}
		if etag != "" {
			h["If-None-Match"] = etag
		}
		return callRaw(d, http.MethodGet, path, nil, h)
	}
	reads := func() [2]int64 { return [2]int64{c.storeReads.Load(), c.viewReads.Load()} }
	want := func(what string, rec *httptest.ResponseRecorder, code int, body string) string {
		t.Helper()
		if rec.Code != code || (body != "" && rec.Body.String() != body) || (code == http.StatusNotModified && rec.Body.Len() != 0) {
			t.Fatalf("%s = %d %q, want %d %q", what, rec.Code, rec.Body.String(), code, body)
		}
		etag := rec.Header().Get("ETag")
		if !strings.HasPrefix(etag, `W/"`) {
			t.Fatalf("%s ETag = %q, want a weak one", what, etag)
		}
		return etag
	}

	etag := want("first view", get(view, ""), http.StatusOK, `{"view":{"items":1},"seen":{"badge":[],"read":[]}}`+"\n")
	if got := reads(); got[1] != 1 {
		t.Fatalf("first view read the view %d times, want 1", got[1])
	}
	listTag := want("first list", get("/tracker/stores", ""), http.StatusOK, "")
	before := reads()
	if e := want("unchanged view revalidated", get(view, etag), http.StatusNotModified, ""); e != etag {
		t.Fatalf("unchanged view ETag %q, want %q", e, etag)
	}
	want("unchanged view read plain", get(view, ""), http.StatusOK, `{"view":{"items":1},"seen":{"badge":[],"read":[]}}`+"\n")
	want("unchanged list revalidated", get("/tracker/stores", listTag), http.StatusNotModified, "")
	want("matching one of several tags", get(view, `W/"x", `+etag), http.StatusNotModified, "")
	if got := reads(); got != before {
		t.Fatalf("unchanged repeats read the host: store/view reads %v -> %v", before, got)
	}

	c.setView(`{"items":2}`, "")
	d.TrackerChanged()
	etag2 := want("changed view", get(view, etag), http.StatusOK, `{"view":{"items":2},"seen":{"badge":[],"read":[]}}`+"\n")
	if etag2 == etag {
		t.Fatal("a changed view kept its ETag")
	}
	want("list after a change", get("/tracker/stores", listTag), http.StatusOK, "")

	d.TrackerChanged()
	n := reads()[1]
	want("view read again unchanged", get(view, etag2), http.StatusNotModified, "")
	if got := reads()[1]; got != n+1 {
		t.Fatalf("a change read the view %d times, want 1", got-n)
	}

	c.setView(`{"items":2,"now":"2026-11-17T10:00:00Z"}`, "t")
	d.TrackerChanged()
	etag3 := want("tagged view", get(view, etag2), http.StatusOK, "")
	c.setView(`{"items":2,"now":"2026-11-17T10:05:00Z"}`, "t")
	d.TrackerChanged()
	want("same tag, another now", get(view, etag3), http.StatusNotModified, "")

	rev := trackerRev(t, d)
	n = reads()[1]
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/seen", `{"badge":["b1"],"read":["","r1"]}`); code != http.StatusNoContent {
		t.Fatalf("seen = %d %s", code, body)
	}
	if now := trackerRev(t, d); now <= rev {
		t.Fatalf("seen left the revision at %d", now)
	}
	rec := get(view, etag3)
	etag4 := want("view after seen", rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"seen":{"badge":["b1"],"read":["r1"]}`) || etag4 == etag3 {
		t.Fatalf("view after seen = %s (ETag %s), want the marks under a new ETag", rec.Body.String(), etag4)
	}
	if got := reads()[1]; got != n {
		t.Fatalf("seen marks read the view again %d times", got-n)
	}
	want("view after seen revalidated", get(view, etag4), http.StatusNotModified, "")
}

// TestTrackerAdapterRevalidates pins the page adapter's revalidation
// (dashboard-tracker.js, tkReadAll through tkReload): the first reload
// reads without a tag, the next sends each answer's ETag and, when every
// answer is a 304, keeps the cache and does not redraw; a new revision
// whose list is the same and whose view is a 304 does not redraw either
// but takes the list's revision; a changed view redraws; and a frame with
// nothing held revalidates a sibling frame's answer and takes it on a 304.
func TestTrackerAdapterRevalidates(t *testing.T) {
	block := dashboardJSBlock(t, "\n  function tkErrText(status, v) {", "\n  // tkRegistryWrite posts")
	runTrackerNode(t, "adapter revalidation", `
var tkApi = {}, tkOrder = [], tkHidden = [], tkInfo = {}, tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSupport = false, tkLastRev = null;
var tkBusy = false, tkQueued = false, tkNext = [], tkNudgedRev = null;
var sent = [], answers = {}, notes = 0, bad = [];
var tkSubs = [function () { notes++; }];
function wsT(k) { return k; }
function tkMount() {}
function tkUrl(id, op) { return "/tracker/stores/" + id + "/" + op; }
// Each path answers answers[path] ([status, body, etag]), or 304 to its etag.
function tkFetch(path, body, done, keepalive, signal, tag) {
  sent.push([path, tag || ""]);
  var a = answers[path];
  if (tag && tag === a[2]) { done(304, {}, a[2], ""); return; }
  done(a[0], a[1], a[2], JSON.stringify(a[1]));
}
`+block+`
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
// Pin legacy ETag/lending separately from summary cursor tests.
tkLegacy.s = true;
answers["/tracker/stores"] = [200, { rev: 1, stores: [{ id: "s" }] }, 'W/"l1"'];
answers["/tracker/stores/s/view"] = [200, { view: { v: 1 }, seen: { badge: ["b"], read: [] } }, 'W/"v1"'];
tkReload();
check("first reload sends no tag", sent, [["/tracker/stores", ""], ["/tracker/stores/s/view", ""]]);
check("first reload draws", [notes, tkViews.s, tkLastRev], [1, { v: 1 }, 1]);

sent = []; notes = 0; tkLastRev = null;
tkReload();
check("next reload revalidates", sent, [["/tracker/stores", 'W/"l1"'], ["/tracker/stores/s/view", 'W/"v1"']]);
check("every answer a 304", [notes, tkViews.s, tkLastRev, tkOrder], [0, { v: 1 }, 1, ["s"]]);

answers["/tracker/stores"] = [200, { rev: 2, stores: [{ id: "s" }] }, 'W/"l2"'];
notes = 0;
tkReload();
check("a new revision with nothing new", [notes, tkViews.s, tkLastRev], [0, { v: 1 }, 2]);

answers["/tracker/stores/s/view"] = [200, { view: { v: 2 }, seen: { badge: [], read: [] } }, 'W/"v2"'];
notes = 0;
tkReload();
check("a changed view redraws", [notes, tkViews.s, tkAnswers.s.tag], [1, { v: 2 }, 'W/"v2"']);

var window = { parent: { frames: [{ wsTrackerAnswer: function (id) {
  return id === "s" ? { tag: 'W/"v2"', text: JSON.stringify({ view: { v: 2 }, seen: { badge: ["x"], read: [] } }) } : null;
} }] } };
tkViews = {}; tkErrs = {}; tkSeen = {}; tkAnswers = {}; tkList = null; sent = []; notes = 0;
tkReload();
check("a new frame revalidates the sibling's answer", sent[1], ["/tracker/stores/s/view", 'W/"v2"']);
check("and takes it on a 304", [notes, tkViews.s, tkSeen.s.badge, tkAnswers.s.tag], [1, { v: 2 }, { x: true }, 'W/"v2"']);
process.stdout.write(bad.join("\n"));
`)
}
