package wsdashboard

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestRailRowsDiff(t *testing.T) {
	row := func(id, text string) string {
		return `<div class="rs" data-rid="` + id + `"><b>` + text + `</b><template class="shp-source"><div>hover</div></template></div>`
	}
	raw := `<div id="rlist"><section>` + row("a", "A") + row("b", "B") + `</section></div>`
	first := splitRailRows(raw)
	if first == nil || !reflect.DeepEqual(first.order, []string{"a", "b"}) {
		t.Fatal("missing decomposition")
	}
	var rebuilt strings.Builder
	for i, id := range first.order {
		rebuilt.WriteString(first.shell[i])
		rebuilt.WriteString(first.rows[id].HTML)
	}
	rebuilt.WriteString(first.shell[len(first.order)])
	if rebuilt.String() != raw {
		t.Fatal("decomposition changed bytes")
	}
	state := keyedRowState{name: regionRail}
	r := newRegion(raw)
	if name, _ := state.event(r, first); name != "region" {
		t.Fatal("first event must be full")
	}
	state.previous = first
	next := splitRailRows(`<div id="rlist"><section>` + row("b", "B") + row("a", "new A") + `</section></div>`)
	name, data := state.event(newRegion("next"), next)
	patch, ok := data.(keyedRowsEvent)
	if name != "rail-rows" || !ok || !reflect.DeepEqual(patch.Order, []string{"b", "a"}) || len(patch.Rows) != 1 || patch.Rows[0].Key != "a" || patch.Rows[0].Hash == first.rows["a"].Hash {
		t.Fatalf("bad delta: %s %+v", name, data)
	}
	for _, raw := range []string{`<div>new shell</div>` + row("a", "A"), row("a", "A"), ""} {
		if name, _ := state.event(newRegion(raw), splitRailRows(raw)); name != "region" {
			t.Fatal("changed shell needs full fallback")
		}
	}
	if name, _ := state.event(r, nil); name != "region" {
		t.Fatal("uncertain decomposition needs full fallback")
	}
	for _, raw := range []string{row("a", "A") + row("a", "duplicate"), `<div class="rs" data-rid=""><b>x</b></div>`, `<div class="rs" data-rid="a">` + row("b", "nested") + `</div>`, `<div class="rs" data-rid="a">broken`} {
		if splitRailRows(raw) != nil {
			t.Fatalf("accepted uncertain rows: %s", raw)
		}
	}
}

// No session revision moves: only the injected clock crosses the adapter's
// stale boundary. The next rail wake must render and ship that row.
func TestRailRowsClockOnlyLiveness(t *testing.T) {
	f := startPageFixture(t)
	now := time.Now()
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}}})
	pages := f.d.pagesIn("en")
	pages.now = func() time.Time { return now }
	first, err := renderRailRegion(f.d.railView(pages), "en")
	if err != nil {
		t.Fatal(err)
	}
	pages.now = func() time.Time { return now.Add(91 * time.Second) }
	second, err := renderRailRegion(f.d.railView(pages), "en")
	if err != nil {
		t.Fatal(err)
	}
	state := keyedRowState{name: regionRail, previous: splitRailRows(string(first.HTML))}
	name, data := state.event(second, splitRailRows(string(second.HTML)))
	patch, ok := data.(keyedRowsEvent)
	if name != "rail-rows" || !ok {
		t.Fatalf("clock wake did not use row delta: %s", name)
	}
	found := false
	for _, r := range patch.Rows {
		if r.Key == "livesession" {
			found = true
			if strings.Contains(r.HTML, `class="rrun"`) {
				t.Fatal("clock-stale row still working")
			}
		}
	}
	if !found {
		t.Fatalf("clock-only change lost row: %+v", patch)
	}
}

// Phase 2 rendered every wake to avoid revision-only clock caching. The
// narrower trigger compares freshly read view inputs instead, never revisions.
func TestRailRowsInvisibleWakesReuseRender(t *testing.T) {
	f := startPageFixture(t)
	src := f.d.railPageSource("en", "")
	page := f.d.cache.acquire(src.key)
	defer f.d.cache.release(src.key, page)
	first, _ := f.d.cache.get(page, src, false)
	renders := f.d.stats.RailRenders.Load()
	for _, token := range []string{"unrelated-session", inboxDep("livesession"), gitDep(f.project), "livesession", ""} {
		f.d.broker.wake(token)
		second, _ := f.d.cache.get(page, src, false)
		if f.d.stats.RailRenders.Load() != renders || first.rail != second.rail || first.regions[regionRail].Hash != second.regions[regionRail].Hash {
			t.Fatalf("rail-invisible %q wake rendered unchanged inputs", token)
		}
	}
	// An indexed board-only change raises Rev but has no rail-visible data.
	f.change(t, "livesession", f.project, "board-only change", 8)
	f.d.cache.get(page, src, false)
	if f.d.stats.RailRenders.Load() != renders {
		t.Fatal("a revision-only change rendered the rail")
	}
}

func TestRailRowsSourceRechecksClockAndHover(t *testing.T) {
	f := startPageFixture(t)
	now := time.Now()
	pages := f.d.pagesIn("en")
	pages.now = func() time.Time { return now }
	src := f.d.railSource(pages, "en", "")
	page := f.d.cache.acquire(src.key)
	defer f.d.cache.release(src.key, page)
	first, _ := f.d.cache.get(page, src, false)
	renders := f.d.stats.RailRenders.Load()
	// Only the clock moves, not a session revision. A normally invisible wake
	// still rechecks adapter liveness before it can reuse an existing render.
	now = now.Add(91 * time.Second)
	f.d.broker.wake(inboxDep("livesession"))
	stale, _ := f.d.cache.get(page, src, false)
	if f.d.stats.RailRenders.Load() != renders+1 || stale.regions[regionRail].Hash == first.regions[regionRail].Hash {
		t.Fatal("clock-only liveness was cached by revision")
	}
	// Evidence changes only the inert hover source; it still invalidates rows.
	snap := f.d.snapshots.snapshotWithoutObserved(f.dir, "livesession")
	snap.UpdatedAt = now.UnixMilli()
	f.d.snapshots.SetLive(f.dir, "livesession", *snap)
	f.d.broker.wake(liveDep("livesession"))
	before, _ := f.d.cache.get(page, src, false)
	if err := wslive.AppendObserved(SessionDir(f.dir, "livesession"), []wslive.ObservedRecord{
		{ModelObservation: wslive.ModelObservation{ID: "a", Model: "first", Source: "pi.assistant", At: 1000}},
		{ModelObservation: wslive.ModelObservation{ID: "b", Model: "second", Source: "pi.assistant", At: 2000}},
	}); err != nil {
		t.Fatal(err)
	}
	// A production evidence change reaches the broker after its index lane
	// expires snapshot/observed validation. This test injects that edge directly.
	f.d.snapshots.Invalidate(f.dir, "livesession")
	f.d.broker.wake("livesession")
	after, _ := f.d.cache.get(page, src, false)
	if before.rail == after.rail || !strings.Contains(string(after.regions[regionRail].HTML), "first → second") {
		t.Fatal("inert model-change hover did not invalidate the rail")
	}
}

type railRowsSink struct {
	fail   bool
	events []sseEvent
}

func (s *railRowsSink) event(name string, data []byte) error {
	if s.fail {
		return errors.New("fixture write failure")
	}
	s.events = append(s.events, sseEvent{name: name, data: string(data)})
	return nil
}
func (*railRowsSink) flush() error          { return nil }
func (*railRowsSink) keepAlive() error      { return nil }
func (*railRowsSink) gone() <-chan struct{} { return nil }

func TestRailRowsFailedSendAndUncertainBaseline(t *testing.T) {
	d := &Daemon{stats: &streamStats{}}
	r := newRegion(`<div class="rs" data-rid="a">A</div>`)
	rows := splitRailRows(string(r.HTML))
	sink, state := &railRowsSink{fail: true}, &keyedRowState{name: regionRail}
	held := map[string]string{regionRail: r.Hash}
	if _, ok := d.sendRegions(sink, regionSet{regionRail: r}, held, nil, nil, []rowRegion{{state: state, next: rows}}); ok || state.initialized || state.previous != nil {
		t.Fatal("failed write advanced the baseline")
	}
	sink.fail = false
	if _, ok := d.sendRegions(sink, regionSet{regionRail: r}, held, nil, nil, []rowRegion{{state: state, next: rows}}); !ok || !state.initialized || state.previous != rows || sink.events[0].name != "region" {
		t.Fatal("first successful send was not full")
	}
	// A malformed/unfamiliar render clears the row baseline by a full send.
	r = newRegion("unfamiliar rail")
	d.sendRegions(sink, regionSet{regionRail: r}, held, nil, nil, []rowRegion{{state: state}})
	if state.previous != nil || sink.events[len(sink.events)-1].name != "region" {
		t.Fatal("uncertain render did not reset baseline")
	}
	count := len(sink.events)
	d.sendRegions(sink, regionSet{regionRail: r}, held, nil, nil, []rowRegion{{state: state}})
	if len(sink.events) != count {
		t.Fatal("unchanged uncertain full render resent")
	}
	r = newRegion(`<div class="rs" data-rid="a">back</div>`)
	d.sendRegions(sink, regionSet{regionRail: r}, held, nil, nil, []rowRegion{{state: state, next: splitRailRows(string(r.HTML))}})
	if sink.events[len(sink.events)-1].name != "region" {
		t.Fatal("recovered decomposition assumed an unproven baseline")
	}
}

func TestRailRowsRenderedParity(t *testing.T) {
	f := startPageFixture(t)
	view := f.d.railView(f.d.pagesIn("en"))
	for _, lang := range []string{"en", "ko"} {
		region, err := renderRailRegion(view, lang)
		if err != nil {
			t.Fatal(err)
		}
		rows := splitRailRows(string(region.HTML))
		if rows == nil || len(rows.order) != 2 {
			t.Fatalf("missing real rows: %+v", rows)
		}
		var rebuilt strings.Builder
		for i, id := range rows.order {
			rebuilt.WriteString(rows.shell[i])
			rebuilt.WriteString(rows.rows[id].HTML)
		}
		rebuilt.WriteString(rows.shell[len(rows.order)])
		if newRegion(rebuilt.String()).Hash != region.Hash || rebuilt.String() != string(region.HTML) {
			t.Fatal("rail HTML/hash changed")
		}
		// Every hash covers the entire rendered row, including inert hover evidence.
		for _, r := range rows.rows {
			if newRegion(r.HTML).Hash != r.Hash {
				t.Fatal("partial row hash")
			}
		}
	}
}

func TestRailRowsStreams(t *testing.T) {
	f := startPageFixture(t)
	path := "/rail/events?railrows=1"
	sse := openSSE(t, f.base+path)
	ws := openWS(t, f.base, path, f.base, true).events
	old := openSSE(t, f.base+"/rail/events")
	railEvent := func(events <-chan sseEvent) sseEvent {
		t.Helper()
		all, closed := collect(events)
		var rails []sseEvent
		for _, e := range all {
			if e.name == "rail-rows" {
				rails = append(rails, e)
			}
			if e.name == "region" {
				var r regionEvent
				json.Unmarshal([]byte(e.data), &r)
				if r.Name == regionRail {
					rails = append(rails, e)
				}
			}
		}
		if closed || len(rails) != 1 {
			t.Fatalf("closed=%v rail events=%+v all=%+v", closed, rails, all)
		}
		return rails[0]
	}
	first := railEvent(sse)
	if first != railEvent(ws) || first.name != "region" {
		t.Fatal("first-full transport mismatch")
	}
	railEvent(old)
	var initial regionEvent
	json.Unmarshal([]byte(first.data), &initial)
	recon := openSSE(t, f.base+path+"&h=rail."+initial.Hash)
	if e := railEvent(recon); e.name != "region" {
		t.Fatal("reconnect trusting held region hash")
	}
	// Also opt in on the side rail of an unframed session stream.
	side := openSSE(t, f.base+"/s/livesession/events?regions=rail&railrows=1")
	if e := railEvent(side); e.name != "region" {
		t.Fatal("side rail not first-full")
	}
	update := func(label string) {
		t.Helper()
		if err := writeSessionLabel(SessionDir(f.dir, "livesession"), label); err != nil {
			t.Fatal(err)
		}
		f.src.set("livesession", SessionData{HasBoard: true, Fingerprint: label})
		f.d.Index().Refresh()
	}
	update("changed row")
	delta := railEvent(sse)
	if delta != railEvent(ws) || delta.name != "rail-rows" {
		t.Fatalf("delta transports: %+v", delta)
	}
	if e := railEvent(side); e != delta {
		t.Fatal("side rail delta mismatch")
	}
	if e := railEvent(old); e.name != "region" {
		t.Fatal("legacy connection lost full region")
	}
	var patch keyedRowsEvent
	json.Unmarshal([]byte(delta.data), &patch)
	if len(patch.Rows) != 1 || patch.Rows[0].Key != "livesession" || len(patch.Order) != 2 || strings.Contains(patch.Rows[0].HTML, `data-rid="boardsession"`) {
		t.Fatalf("not changed rows plus order: %+v", patch)
	}
	// A layout generation/shell change has no trustworthy row-slot baseline.
	if code, _, raw := postJSON(t, f.base+railLayoutRoute, f.base, `{"op":"star","rid":"livesession"}`); code != 200 {
		t.Fatalf("layout %d %s", code, raw)
	}
	if e := railEvent(sse); e.name != "region" {
		t.Fatal("shell change did not fall back")
	}
}
