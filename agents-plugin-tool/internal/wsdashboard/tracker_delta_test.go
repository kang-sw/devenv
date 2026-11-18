package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type projectionTracker struct {
	*countingTracker
	projectionMu                 sync.Mutex
	snapshot                     TrackerProjection
	projectionReads, detailReads atomic.Int64
	started, release             chan struct{}
}

func (f *projectionTracker) Projection(ctx context.Context, id string) (TrackerProjection, error) {
	f.projectionReads.Add(1)
	f.projectionMu.Lock()
	snapshot, started, release := f.snapshot, f.started, f.release
	f.projectionMu.Unlock()
	if started != nil {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return TrackerProjection{}, ctx.Err()
		}
	}
	return snapshot, nil
}

func newProjectionTracker() *projectionTracker {
	f := &projectionTracker{countingTracker: &countingTracker{fakeTracker: newFakeTracker(), raw: `{"items":1}`}}
	f.setProjection("v1", `{"title":"one"}`, true)
	return f
}

func (f *projectionTracker) setProjection(version, data string, other bool) {
	snapshot := TrackerProjection{
		Items:   map[string]TrackerSummaryRow{"one": {Data: json.RawMessage(data), DetailVersion: version}},
		Globals: map[string]json.RawMessage{"itemOrder": json.RawMessage(`["one","two"]`), "stats": json.RawMessage(`{"events":1}`)},
		Feed:    json.RawMessage(`[{"key":"r:one","item":"one"}]`),
		Keys:    map[string]TrackerSeen{"one": {Badge: []string{"a:one"}, Read: []string{"r:one"}}},
	}
	if other {
		snapshot.Items["two"] = TrackerSummaryRow{Data: json.RawMessage(`{"title":"two"}`), DetailVersion: "other"}
	}
	snapshot.Detail = func(_ context.Context, ids []string) (map[string]json.RawMessage, error) {
		f.detailReads.Add(1)
		out := map[string]json.RawMessage{}
		for _, id := range ids {
			out[id] = json.RawMessage(fmt.Sprintf(`{"body":%q}`, version+":"+id))
		}
		return out, nil
	}
	f.projectionMu.Lock()
	f.snapshot = snapshot
	f.projectionMu.Unlock()
}

func summaryGet(t *testing.T, d *Daemon, path string) (trackerSummaryAnswer, *httptest.ResponseRecorder) {
	t.Helper()
	r := callRaw(d, http.MethodGet, "/tracker/stores/team/"+path, nil, nil)
	var answer trackerSummaryAnswer
	if r.Code == http.StatusOK {
		if err := json.Unmarshal(r.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
	}
	return answer, r
}

// Block at a deterministic socket boundary without delaying host work.
type trackerBlockedWriter struct {
	*httptest.ResponseRecorder
	started, release chan struct{}
	headersOnly      bool
}

func (w *trackerBlockedWriter) WriteHeader(status int) {
	if w.headersOnly {
		close(w.started)
		<-w.release
	}
	w.ResponseRecorder.WriteHeader(status)
}

func (w *trackerBlockedWriter) Write(body []byte) (int, error) {
	close(w.started)
	<-w.release
	return w.ResponseRecorder.Write(body)
}

func TestTrackerProjectionSlowSocketDoesNotBlockSameStore(t *testing.T) {
	for _, op := range []string{"summary", "delta?since=foreign.1", "detail?id=one", "delta?since=current"} {
		t.Run(op, func(t *testing.T) {
			f := newProjectionTracker()
			d := startTrackerDaemon(t, f)
			first, _ := summaryGet(t, d, "summary")
			headersOnly := strings.HasSuffix(op, "current")
			path := strings.ReplaceAll(op, "current", first.Cursor)
			w := &trackerBlockedWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{}), headersOnly}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(w.release) }) }
			defer release()
			finished := make(chan struct{})
			go func() {
				d.trackerProjectionRead(w, httptest.NewRequest(http.MethodGet, "/"+path, nil), "team", strings.SplitN(op, "?", 2)[0], 1, time.Now())
				close(finished)
			}()
			select {
			case <-w.started:
			case <-time.After(5 * time.Second):
				t.Fatal("first response did not reach socket write")
			}
			// Publish a different generation while the first writer is blocked.
			// This also proves its serialized body is detached from mutable maps.
			f.setProjection("v2", `{"title":"next"}`, false)
			next := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				rec := httptest.NewRecorder()
				d.trackerProjectionRead(rec, httptest.NewRequest(http.MethodGet, "/", nil), "team", "summary", 2, time.Now())
				next <- rec
			}()
			select {
			case rec := <-next:
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "next") {
					t.Fatalf("concurrent projection: %d %s", rec.Code, rec.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("same-store projection waited for slow socket")
			}
			select {
			case <-finished:
				t.Fatal("slow writer unexpectedly finished before release")
			default:
			}
			release()
			<-finished
			wantStatus := http.StatusOK
			if headersOnly {
				wantStatus = http.StatusNotModified
			}
			if w.Code != wantStatus || w.Header().Get("ETag") != `W/"`+first.Cursor+`"` {
				t.Fatalf("blocked response status/headers changed: %d %v", w.Code, w.Header())
			}
			if strings.Contains(w.Body.String(), "next") || strings.Contains(w.Body.String(), "v2") {
				t.Fatalf("blocked response changed after publication: %s", w.Body.String())
			}
		})
	}
}

func TestTrackerSummaryDeltaDetailAndLegacyView(t *testing.T) {
	f := newProjectionTracker()
	d := startTrackerDaemon(t, f)
	initial, rec := summaryGet(t, d, "summary")
	if rec.Code != http.StatusOK || !initial.Reset || len(initial.Items) != 2 || initial.Cursor == "" {
		t.Fatalf("initial summary: %d %s", rec.Code, rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("missing weak ETag: %s", etag)
	}
	_, rec = summaryGet(t, d, "delta?since="+url.QueryEscape(initial.Cursor))
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("equal cursor: %d %s", rec.Code, rec.Body.String())
	}
	rec = callRaw(d, http.MethodGet, "/tracker/stores/team/summary", nil, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified || f.projectionReads.Load() != 1 || f.storeReads.Load() != 1 || f.detailReads.Load() != 0 {
		t.Fatalf("unchanged reads should be host/detail-free: %d projections=%d stores=%d details=%d", rec.Code, f.projectionReads.Load(), f.storeReads.Load(), f.detailReads.Load())
	}
	// Detail-only verdict reason change leaves row Data alone, changes only
	// DetailVersion, and must still appear as a changed item in the delta.
	f.setProjection("reason2", `{"title":"one"}`, true)
	d.TrackerChanged()
	changed, rec := summaryGet(t, d, "delta?since="+url.QueryEscape(initial.Cursor))
	if rec.Code != http.StatusOK || changed.Reset || len(changed.Items) != 1 || changed.Items["one"].DetailVersion != "reason2" || len(changed.Globals) != 0 || len(changed.Feed) != 0 {
		t.Fatalf("detail-only delta: %d %s", rec.Code, rec.Body.String())
	}
	rec = callRaw(d, http.MethodGet, "/tracker/stores/team/detail?id=one&id=one&id=missing", nil, nil)
	var detail struct {
		Cursor  string                         `json:"cursor"`
		Items   map[string]TrackerDetailBundle `json:"items"`
		Missing []string                       `json:"missing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || detail.Cursor != changed.Cursor || len(detail.Items) != 1 || detail.Items["one"].DetailVersion != "reason2" || !slices.Equal(detail.Missing, []string{"missing"}) || !strings.Contains(string(detail.Items["one"].Data), "reason2:one") {
		t.Fatalf("coherent deduplicated detail batch: %d %s", rec.Code, rec.Body.String())
	}
	if f.projectionReads.Load() != 2 || f.detailReads.Load() != 1 {
		t.Fatal("detail reread the host or rendered an unrequested item")
	}
	// New paths must not alter old full-view bytes or conditional behavior.
	legacy := callRaw(d, http.MethodGet, "/tracker/stores/team/view", nil, nil)
	if legacy.Body.String() != "{\"view\":{\"items\":1},\"seen\":{\"badge\":[],\"read\":[]}}\n" {
		t.Fatalf("legacy shape changed: %s", legacy.Body.String())
	}
	legacy = callRaw(d, http.MethodGet, "/tracker/stores/team/view", nil, map[string]string{"If-None-Match": legacy.Header().Get("ETag")})
	if legacy.Code != http.StatusNotModified || legacy.Body.Len() != 0 {
		t.Fatalf("legacy 304 changed: %d %s", legacy.Code, legacy.Body.String())
	}
}

func TestTrackerDeltaSeenUsesItemDiffWithoutHostRead(t *testing.T) {
	f := newProjectionTracker()
	d := startTrackerDaemon(t, f)
	first, _ := summaryGet(t, d, "summary")
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/seen", `{"badge":["a:one","irrelevant"],"read":["r:one",""]}`); code != http.StatusNoContent {
		t.Fatalf("seen: %d %s", code, body)
	}
	answer, rec := summaryGet(t, d, "delta?since="+first.Cursor)
	if rec.Code != http.StatusOK || answer.Reset || len(answer.Items) != 1 || !slices.Equal(answer.Items["one"].Seen.Badge, []string{"a:one"}) || !slices.Equal(answer.Items["one"].Seen.Read, []string{"r:one"}) {
		t.Fatalf("seen item delta: %d %s", rec.Code, rec.Body.String())
	}
	if f.projectionReads.Load() != 1 || f.detailReads.Load() != 0 {
		t.Fatal("seen-only delta reread host or detail")
	}
	if code, _ := call(d, http.MethodPost, "/tracker/stores/team/seen", `{"badge":["a:one"]}`); code != http.StatusNoContent {
		t.Fatal(code)
	}
	_, rec = summaryGet(t, d, "delta?since="+answer.Cursor)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("duplicate seen minted a generation: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTrackerDeltaResetRingAndRemovals(t *testing.T) {
	f := newProjectionTracker()
	d := startTrackerDaemon(t, f)
	first, _ := summaryGet(t, d, "summary")
	for _, foreign := range []string{"different-epoch.1", "", "not-a-cursor", first.Cursor + "0", strings.Split(first.Cursor, ".")[0] + ".0"} {
		answer, rec := summaryGet(t, d, "delta?since="+url.QueryEscape(foreign))
		if rec.Code != http.StatusOK || !answer.Reset || len(answer.Items) != 2 {
			t.Fatalf("foreign/malformed reset %q: %d %s", foreign, rec.Code, rec.Body.String())
		}
	}
	var boundary string
	for i := 0; i < trackerChangeRingSize+1; i++ {
		f.setProjection(fmt.Sprint(i), `{"title":"one"}`, i%2 == 0)
		d.TrackerChanged()
		answer, rec := summaryGet(t, d, "summary")
		if rec.Code != http.StatusOK {
			t.Fatalf("generation %d: %d", i, rec.Code)
		}
		if i == 0 {
			boundary = answer.Cursor
		}
	}
	old, rec := summaryGet(t, d, "delta?since="+first.Cursor)
	if rec.Code != http.StatusOK || !old.Reset {
		t.Fatalf("expired ring: %d %s", rec.Code, rec.Body.String())
	}
	retained, rec := summaryGet(t, d, "delta?since="+boundary)
	if rec.Code != http.StatusOK || retained.Reset || len(retained.Removed) != 0 || len(retained.Items) != 2 {
		t.Fatalf("oldest retained cursor/re-added item: %d %s", rec.Code, rec.Body.String())
	}
	f.setProjection("removed", `{"title":"one"}`, false)
	d.TrackerChanged()
	removed, rec := summaryGet(t, d, "delta?since="+retained.Cursor)
	if rec.Code != http.StatusOK || removed.Reset || !slices.Equal(removed.Removed, []string{"two"}) {
		t.Fatalf("removed item: %d %s", rec.Code, rec.Body.String())
	}
	// A fresh daemon, or another store in the same daemon, cannot accept the
	// old cursor even when its numerical generation happens to be equal.
	other := startTrackerDaemon(t, newProjectionTracker())
	answer, rec := summaryGet(t, other, "delta?since="+removed.Cursor)
	if rec.Code != http.StatusOK || !answer.Reset {
		t.Fatalf("foreign daemon: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTrackerProjectionTTLGlobalsAndDetailValidation(t *testing.T) {
	f := newProjectionTracker()
	d := startTrackerDaemon(t, f)
	first, _ := summaryGet(t, d, "summary")
	p := d.trackerAnswers.projection("team")
	p.mu.Lock()
	p.stamp.at = time.Now().Add(-trackerAnswerTTL - time.Second)
	p.mu.Unlock()
	f.projectionMu.Lock()
	f.snapshot.Globals = map[string]json.RawMessage{"itemOrder": json.RawMessage(`["one","two"]`), "stats": json.RawMessage(`{"events":2}`)}
	f.projectionMu.Unlock()
	changed, rec := summaryGet(t, d, "delta?since="+first.Cursor)
	if rec.Code != http.StatusOK || changed.Reset || len(changed.Items) != 0 || len(changed.Globals) != 1 || string(changed.Globals["stats"]) != `{"events":2}` || f.projectionReads.Load() != 2 {
		t.Fatalf("TTL/global-only reconcile: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"detail", "detail?id=", "detail?" + strings.Repeat("id=x&", trackerDetailBatchLimit+1)} {
		_, rec := summaryGet(t, d, path)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad batch %s: %d", path, rec.Code)
		}
	}
}

func TestTrackerProjectionSeenAcrossBuildAndConcurrentPublication(t *testing.T) {
	f := newProjectionTracker()
	f.started, f.release = make(chan struct{}), make(chan struct{})
	d := startTrackerDaemon(t, f)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- callRaw(d, http.MethodGet, "/tracker/stores/team/summary", nil, nil) }()
	select {
	case <-f.started:
	case <-time.After(5 * time.Second):
		t.Fatal("projection build did not start")
	}
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/seen", `{"read":["r:one"]}`); code != http.StatusNoContent {
		t.Fatalf("seen while building: %d %s", code, body)
	}
	close(f.release)
	var first trackerSummaryAnswer
	select {
	case rec := <-done:
		if rec.Code != http.StatusOK {
			t.Fatalf("build: %d %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projection build deadlocked")
	}
	if !slices.Equal(first.Items["one"].Seen.Read, []string{"r:one"}) {
		t.Fatal("in-flight build lost a seen write")
	}
	f.projectionMu.Lock()
	f.started, f.release = nil, nil
	f.projectionMu.Unlock()
	f.setProjection("next", `{"title":"next"}`, true)
	d.TrackerChanged()
	var wg sync.WaitGroup
	cursors := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := callRaw(d, http.MethodGet, "/tracker/stores/team/summary", nil, nil)
			var a trackerSummaryAnswer
			if r.Code == http.StatusOK && json.Unmarshal(r.Body.Bytes(), &a) == nil {
				cursors <- a.Cursor
			}
		}()
	}
	wg.Wait()
	close(cursors)
	cursor, count := "", 0
	for c := range cursors {
		if cursor != "" && cursor != c {
			t.Fatal("concurrent reads published different generations")
		}
		cursor = c
		count++
	}
	if count != 12 || f.projectionReads.Load() != 2 {
		t.Fatalf("concurrent publication: responses=%d host reads=%d", count, f.projectionReads.Load())
	}
}

func TestTrackerProjectionOptionalHostAndStoreErrors(t *testing.T) {
	d := startTrackerDaemon(t, newFakeTracker())
	for _, op := range []string{"summary", "delta?since=old.1", "detail?id=one"} {
		_, rec := summaryGet(t, d, op)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("old host %s: %d %s", op, rec.Code, rec.Body.String())
		}
	}
	f := newProjectionTracker()
	d = startTrackerDaemon(t, f)
	r := callRaw(d, http.MethodGet, "/tracker/stores/missing/summary", nil, nil)
	if r.Code != http.StatusNotFound {
		t.Fatal("unknown store accepted")
	}
	r = callRaw(d, http.MethodPost, "/tracker/stores/team/summary", nil, nil)
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatal("summary write accepted")
	}
}
