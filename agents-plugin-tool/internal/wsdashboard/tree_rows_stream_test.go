package wsdashboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// No started-at timers: caption-only edits have no clock-dependent row inputs.
func treeRowsFixture(t *testing.T) (*pageFixture, wslive.Snapshot) {
	t.Helper()
	f := startPageFixture(t)
	now := time.Now()
	snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
		Agents: []wslive.Agent{
			{ID: "parent", Label: "parent", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
			{ID: "child", ParentID: "parent", Label: "child", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
			{ID: "other", Label: "other", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
		}}
	f.d.snapshots.SetLive(f.dir, "livesession", snap)
	return f, snap
}

func treeRowsRebuild(rows *treeRows) string {
	var fill func(string) string
	fill = func(raw string) string {
		for i, key := range rows.order {
			if strings.Contains(raw, treeRowSlot(i)) {
				raw = strings.ReplaceAll(raw, treeRowSlot(i), fill(rows.rows[key].HTML))
			}
		}
		return raw
	}
	return fill(rows.shell)
}

func treeStreamEvent(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	all, closed := collect(events)
	var found []sseEvent
	for _, e := range all {
		if e.name == "tree-rows" {
			found = append(found, e)
		} else if e.name == "region" {
			var r regionEvent
			if err := json.Unmarshal([]byte(e.data), &r); err != nil {
				t.Fatal(err)
			}
			if r.Name == regionTree {
				found = append(found, e)
			}
		}
	}
	if closed || len(found) != 1 {
		t.Fatalf("closed=%v tree events=%+v all=%+v", closed, found, all)
	}
	return found[0]
}

func TestTreeRowsRenderedSharedParity(t *testing.T) {
	f, _ := treeRowsFixture(t)
	f.d.snapshots.SetLive(f.dir, "boardsession", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
		Agents: []wslive.Agent{{ID: "peerchild", Label: "peer child", Status: wslive.StatusRunning}}})
	for _, lang := range []string{"en", "ko"} {
		src := f.d.sessionPageSource("livesession", lang, "")
		a, b := f.d.cache.acquire(src.key), f.d.cache.acquire(src.key)
		first, _ := f.d.cache.get(a, src, false)
		second, _ := f.d.cache.get(b, src, false)
		f.d.cache.release(src.key, a)
		f.d.cache.release(src.key, b)
		if first.tree == nil || len(first.tree.order) < 4 || first.tree != second.tree {
			t.Fatalf("%s: decomposition missing or not shared", lang)
		}
		r := first.regions[regionTree]
		if raw := treeRowsRebuild(first.tree); raw != string(r.HTML) || newRegion(raw).Hash != r.Hash || strings.Contains(raw, "<!--ws-tree-row-") {
			t.Fatalf("%s: full render bytes/hash changed", lang)
		}
		for _, row := range first.tree.rows {
			if newRegion(row.HTML).Hash != row.Hash {
				t.Fatalf("%s: partial row hash %s", lang, row.Key)
			}
		}
		if strings.Contains(first.tree.rows["agent-parent"].HTML, `data-key="agent-child"`) {
			t.Fatal("parent input contains descendant markup")
		}
		peer, ok := first.tree.rows["peer-boardsession"]
		if !ok || !strings.Contains(peer.HTML, "<!--ws-tree-row-") || strings.Contains(peer.HTML, "peer child") {
			t.Fatal("real expandable peer was not decomposed independently of its child")
		}
	}
}

func TestTreeRowsStreams(t *testing.T) {
	f, snap := treeRowsFixture(t)
	path := "/s/livesession/events?regions=tree&frame=1&treerows=1"
	sse := openSSE(t, f.base+path)
	ws := openWS(t, f.base, path, f.base, true).events
	legacy := openSSE(t, f.base+"/s/livesession/events?regions=tree&frame=1")
	first := treeStreamEvent(t, sse)
	if first.name != "region" || first != treeStreamEvent(t, ws) || first != treeStreamEvent(t, legacy) {
		t.Fatal("first full transport parity")
	}
	var full regionEvent
	if err := json.Unmarshal([]byte(first.data), &full); err != nil {
		t.Fatal(err)
	}
	baseline := splitTreeRows(full.HTML)
	reconnect := openSSE(t, f.base+path+"&h=tree."+full.Hash)
	reconnectWS := openWS(t, f.base, path+"&h=tree."+full.Hash, f.base, true).events
	if e := treeStreamEvent(t, reconnect); e != first || e != treeStreamEvent(t, reconnectWS) {
		t.Fatal("h=current reconnect did not send first full on both transports")
	}
	update := func() {
		f.d.snapshots.SetLive(f.dir, "livesession", snap)
		f.d.broker.wake(liveDep("livesession"))
	}
	checkDelta := func(keys []string) {
		t.Helper()
		delta := treeStreamEvent(t, sse)
		if delta.name != "tree-rows" || delta != treeStreamEvent(t, ws) || delta != treeStreamEvent(t, reconnect) || delta != treeStreamEvent(t, reconnectWS) {
			t.Fatalf("delta transport mismatch: %+v", delta)
		}
		var patch treeRowsEvent
		if err := json.Unmarshal([]byte(delta.data), &patch); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, row := range patch.Rows {
			got = append(got, row.Key)
			if row.Hash != newRegion(row.HTML).Hash {
				t.Fatal("delta hash not complete HTML")
			}
			baseline.rows[row.Key] = row
		}
		if !reflect.DeepEqual(got, keys) || !reflect.DeepEqual(patch.Order, baseline.order) {
			t.Fatalf("changed rows=%v want=%v order=%v", got, keys, patch.Order)
		}
		old := treeStreamEvent(t, legacy)
		var region regionEvent
		if old.name != "region" || json.Unmarshal([]byte(old.data), &region) != nil || region.HTML != treeRowsRebuild(baseline) || region.Hash != patch.Hash {
			t.Fatal("legacy full differs from reassembled delta")
		}
	}
	snap.Agents[1].Caption = "child-only edit"
	update()
	checkDelta([]string{"agent-child"})
	snap.Agents[0].Caption, snap.Agents[1].Caption = "parent edit", "second child edit"
	update()
	checkDelta([]string{"agent-parent", "agent-child"})
	// Topology changes require full HTML, even though stable keys still exist.
	snap.Agents[1].ParentID = ""
	update()
	for _, stream := range []<-chan sseEvent{sse, ws, legacy, reconnect, reconnectWS} {
		if treeStreamEvent(t, stream).name != "region" {
			t.Fatal("topology change assumed old slots")
		}
	}
	// A background-task chip changes the header shell without changing topology.
	background := 2
	snap.Usage = &wslive.Usage{Background: &background}
	update()
	for _, stream := range []<-chan sseEvent{sse, ws, legacy, reconnect, reconnectWS} {
		if treeStreamEvent(t, stream).name != "region" {
			t.Fatal("header shell change did not use full fallback")
		}
	}
}

func TestTreeRowsFailedSendAndUncertainBaseline(t *testing.T) {
	d := &Daemon{stats: &streamStats{}}
	r := newRegion(treeTestRow("a", "A", ""))
	rows := splitTreeRows(string(r.HTML))
	sink, state := &railRowsSink{fail: true}, &treeRowState{}
	held := map[string]string{regionTree: r.Hash}
	send := func(r renderedRegion, rows *treeRows) bool {
		_, ok := d.sendTreeRegions(sink, regionSet{regionTree: r}, held, nil, nil, nil, nil, state, rows)
		return ok
	}
	if send(r, rows) || state.initialized || state.previous != nil || len(sink.events) != 0 {
		t.Fatal("failed first write advanced baseline")
	}
	sink.fail = false
	if !send(r, rows) || !state.initialized || state.previous != rows || sink.events[0].name != "region" {
		t.Fatal("first successful write not full with h=current")
	}
	next := newRegion(treeTestRow("a", "changed", ""))
	sink.fail = true
	if send(next, splitTreeRows(string(next.HTML))) || held[regionTree] != r.Hash || state.previous != rows {
		t.Fatal("failed delta advanced held hash or row baseline")
	}
	sink.fail = false
	if !send(next, splitTreeRows(string(next.HTML))) || sink.events[1].name != "tree-rows" {
		t.Fatal("retry lost changed row")
	}
	uncertain := newRegion("unfamiliar tree")
	if !send(uncertain, nil) || state.previous != nil || sink.events[2].name != "region" {
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
