package wsdashboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func entryRowsRebuild(rows *keyedRows) string {
	var out strings.Builder
	for i, key := range rows.order {
		out.WriteString(rows.shell[i])
		out.WriteString(rows.rows[key].HTML)
	}
	out.WriteString(rows.shell[len(rows.order)])
	return out.String()
}

// Replace maps rather than mutating a render input another subscriber holds.
func entryRowsSetBodies(t *testing.T, f *pageFixture, bodies map[string]string) {
	t.Helper()
	p := f.src.page("livesession")
	p.MailBodies = bodies
	p.Seq++
	f.src.setPage("livesession", p)
	if !f.d.index.Nudge("livesession", f.project) {
		t.Fatal("nudge refused")
	}
	f.d.index.Settle()
}

func entryStreamEvent(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	return keyedStreamEvent(t, events, regionEntries, "entry-rows")
}

func TestEntryRowsRenderedSharedParity(t *testing.T) {
	f := startPageFixture(t)
	for _, lang := range []string{"en", "ko"} {
		src := f.d.sessionPageSource("livesession", lang, "")
		a, b := f.d.cache.acquire(src.key), f.d.cache.acquire(src.key)
		first, firstAt := f.d.cache.get(a, src, false)
		second, secondAt := f.d.cache.get(b, src, false)
		if first.entries == nil {
			t.Fatalf("%s: decomposition missing", lang)
		}
		if len(first.entries.order) != 4 {
			t.Fatalf("%s: decomposition has %d rows, want 4", lang, len(first.entries.order))
		}
		if first.entries != second.entries {
			t.Fatalf("%s: decomposition not shared (broker sequence %d -> %d)", lang, firstAt, secondAt)
		}
		r := first.regions[regionEntries]
		before, _ := json.Marshal(first.entries.rows)
		if raw := entryRowsRebuild(first.entries); raw != string(r.HTML) || newRegion(raw).Hash != r.Hash {
			t.Fatalf("%s: full render bytes/hash changed", lang)
		}
		for _, row := range first.entries.rows {
			if newRegion(row.HTML).Hash != row.Hash {
				t.Fatalf("%s: incomplete row hash %s", lang, row.Key)
			}
		}
		bodies := map[string]string{}
		if lang == "ko" {
			bodies["hist-1.json"] = "available again"
		}
		entryRowsSetBodies(t, f, bodies)
		next, _ := f.d.cache.get(a, src, false)
		after, _ := json.Marshal(first.entries.rows)
		if string(before) != string(after) || next.entries == first.entries || entryRowsRebuild(first.entries) != string(r.HTML) {
			t.Fatalf("%s: later render mutated held decomposition", lang)
		}
		f.d.cache.release(src.key, a)
		f.d.cache.release(src.key, b)
	}
}

func TestEntryRowsStreams(t *testing.T) {
	f := startPageFixture(t)
	path := "/s/livesession/events?regions=entries&frame=1&entryrows=1"
	sse := openSSE(t, f.base+path)
	ws := openWS(t, f.base, path, f.base, true).events
	legacy := openSSE(t, f.base+"/s/livesession/events?regions=entries&frame=1")
	first := entryStreamEvent(t, sse)
	if first.name != "region" || first != entryStreamEvent(t, ws) || first != entryStreamEvent(t, legacy) {
		t.Fatal("first full transport parity")
	}
	var full regionEvent
	if err := json.Unmarshal([]byte(first.data), &full); err != nil {
		t.Fatal(err)
	}
	baseline := splitEntryRows(full.HTML)
	initialHTML := entryRowsRebuild(baseline)
	reconnect := openSSE(t, f.base+path+"&h=entries."+full.Hash)
	reconnectWS := openWS(t, f.base, path+"&h=entries."+full.Hash, f.base, true).events
	if e := entryStreamEvent(t, reconnect); e != first || e != entryStreamEvent(t, reconnectWS) {
		t.Fatal("h=current reconnect did not send first full on both transports")
	}
	streams := []<-chan sseEvent{sse, ws, reconnect, reconnectWS}
	checkDelta := func(indices []int) {
		t.Helper()
		delta := entryStreamEvent(t, sse)
		if delta.name != "entry-rows" {
			t.Fatalf("wanted delta: %+v", delta)
		}
		for _, stream := range streams[1:] {
			if delta != entryStreamEvent(t, stream) {
				t.Fatal("delta transport parity")
			}
		}
		var patch keyedRowsEvent
		if err := json.Unmarshal([]byte(delta.data), &patch); err != nil {
			t.Fatal(err)
		}
		var got, want []string
		for _, i := range indices {
			want = append(want, baseline.order[i])
		}
		for _, row := range patch.Rows {
			got = append(got, row.Key)
			if row.Hash != newRegion(row.HTML).Hash {
				t.Fatal("delta hash not complete HTML")
			}
			baseline.rows[row.Key] = row
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(patch.Order, baseline.order) {
			t.Fatalf("changed rows=%v want=%v order=%v", got, want, patch.Order)
		}
		old := entryStreamEvent(t, legacy)
		var region regionEvent
		if old.name != "region" || json.Unmarshal([]byte(old.data), &region) != nil || region.HTML != entryRowsRebuild(baseline) || region.Hash != patch.Hash || newRegion(region.HTML).Hash != patch.Hash || region.Hash == full.Hash {
			t.Fatal("legacy full differs from reconstructed delta/hash")
		}
		if len(delta.data) >= len(old.data) {
			t.Fatal("delta did not omit unchanged rows and shell")
		}
	}
	entryRowsSetBodies(t, f, map[string]string{})
	checkDelta([]int{2})
	entryRowsSetBodies(t, f, map[string]string{"hist-1.json": "mail first line\nmail second line", "hist-gone.json": "now available"})
	checkDelta([]int{2, 3})
	if initialHTML == entryRowsRebuild(baseline) {
		t.Fatal("fixture did not change entries")
	}
	checkFull := func(label string) {
		t.Helper()
		e := entryStreamEvent(t, sse)
		if e.name != "region" {
			t.Fatalf("%s did not fall back: %+v", label, e)
		}
		for _, stream := range append(streams[1:], legacy) {
			if entryStreamEvent(t, stream) != e {
				t.Fatalf("%s full transport parity", label)
			}
		}
	}
	// Model history is shell even when every keyed row is unchanged.
	f.d.snapshots.SetLive(f.dir, "livesession", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000,
		Lead: wslive.Lead{Observed: &wslive.ObservedModel{Current: wslive.ModelObservation{ID: "fixture-model", Model: "fixture-model", Source: "pi.assistant", At: 1000}, History: []wslive.ModelChange{{Model: "fixture-model", Source: "pi.assistant", At: 1000}}}}})
	f.d.broker.wake(liveDep("livesession"))
	checkFull("model history")
	appendUsers(t, f, "livesession", 0, 1)
	f.d.broker.wake(ringDep("livesession"))
	checkFull("title and window")
}

func TestEntryRowsFailedSendAndUncertainBaseline(t *testing.T) {
	d := &Daemon{stats: &streamStats{}}
	raw := `<div class="thead">title<span class="sum"><button class="modelhistory-trigger"></button><template class="modelhistory-source">model</template></span></div><div class="centries"><div data-entry="a">A</div><div data-entry="b">B</div></div>`
	r := newRegion(raw)
	rows := splitEntryRows(raw)
	sink, state := &railRowsSink{fail: true}, &keyedRowState{name: regionEntries}
	held := map[string]string{regionEntries: r.Hash}
	send := func(r renderedRegion, rows *keyedRows) bool {
		_, ok := d.sendRegions(sink, regionSet{regionEntries: r}, held, nil, nil, []rowRegion{{state: state, next: rows}})
		return ok
	}
	if send(r, rows) || state.initialized || state.previous != nil || len(sink.events) != 0 {
		t.Fatal("failed first write advanced baseline")
	}
	sink.fail = false
	if !send(r, rows) || !state.initialized || state.previous != rows || sink.events[0].name != "region" {
		t.Fatal("first successful write not full with h=current")
	}
	next := newRegion(strings.Replace(raw, ">A<", ">changed<", 1))
	sink.fail = true
	if send(next, splitEntryRows(string(next.HTML))) || held[regionEntries] != r.Hash || state.previous != rows {
		t.Fatal("failed delta advanced held hash or row baseline")
	}
	sink.fail = false
	if !send(next, splitEntryRows(string(next.HTML))) || sink.events[1].name != "entry-rows" {
		t.Fatal("retry lost changed row")
	}
	for label, changed := range map[string]string{
		"title":         strings.Replace(raw, "title", "new title", 1),
		"model history": strings.Replace(raw, ">model<", ">new model<", 1),
		"order":         strings.Replace(strings.Replace(raw, `data-entry="a"`, `data-entry="b"`, 1), `data-entry="b">B`, `data-entry="a">B`, 1),
		"window":        strings.Replace(raw, `data-entry="b"`, `data-entry="c"`, 1),
	} {
		state.previous = rows
		n := len(sink.events)
		if !send(newRegion(changed), splitEntryRows(changed)) || sink.events[n].name != "region" {
			t.Fatalf("%s assumed old shell/window", label)
		}
	}
	uncertain := newRegion("unfamiliar entries")
	if !send(uncertain, nil) || state.previous != nil || sink.events[len(sink.events)-1].name != "region" {
		t.Fatal("uncertain full failed to reset baseline")
	}
	n := len(sink.events)
	if !send(uncertain, nil) || len(sink.events) != n {
		t.Fatal("unchanged uncertain region resent")
	}
	if !send(r, rows) || sink.events[n].name != "region" {
		t.Fatal("recovered decomposition assumed unproven baseline")
	}
}
