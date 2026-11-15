package wsdashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Isolate every host-facing directory before starting the disposable daemon.
func panelRowsFixture(t *testing.T) *pageFixture {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"WS_CACHE_HOME", "WS_CONFIG_HOME", "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, filepath.Join(root, key))
	}
	return startPageFixture(t, func(o *DaemonOptions) { o.Home = filepath.Join(root, "HOME") })
}

func panelRowsPublish(t *testing.T, f *pageFixture, agenda, todo string) {
	t.Helper()
	var tasks []wswhiteboard.PanelRow
	for i := 0; i < 45; i++ {
		tasks = append(tasks, wswhiteboard.PanelRow{State: "pending", Text: fmt.Sprintf("%s task %02d", todo, i)})
	}
	var tree []wswhiteboard.TreeNode
	tree = append(tree, wswhiteboard.JSONTree("agenda/", "plan", []byte(fmt.Sprintf(`{"step":{"value":%q},"other":{"value":"retained"}}`, agenda))))
	for i := 0; i < 30; i++ {
		tree = append(tree, wswhiteboard.JSONTree("agenda/", fmt.Sprintf("extra%02d", i), []byte(`{"note":"stable reader content"}`)))
	}
	view := wswhiteboard.BuildView(&wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "Plan", Body: "unaffected board frame", Changed: 1}}}, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, Panels: []wswhiteboard.Panel{
		{ID: "todo", Title: "Tasks", Rows: tasks},
		{ID: "agenda", Title: "Agendas", Tree: tree},
		{ID: "tickets", Title: "Tickets", Lanes: []wswhiteboard.PanelLane{{Name: "ready", Rows: []wswhiteboard.PanelRow{{Text: "Stable ticket", Path: "ai-docs/tickets/ready/fixture.md"}}}}},
		{ID: "mailbox", Title: "Mailbox", Count: "1 unread", Rows: []wswhiteboard.PanelRow{{Label: "peer", Text: "Stable mail"}}},
	}})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	p := f.src.page("livesession")
	p.Board = regions
	p.Seq++
	f.src.setPage("livesession", p)
	if !f.d.index.Nudge("livesession", f.project) {
		t.Fatal("nudge refused")
	}
	f.d.index.Settle()
}

func panelStreamEvent(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	all, closed := collect(events)
	var found []sseEvent
	for _, e := range all {
		if e.name == "panel-rows" {
			found = append(found, e)
		} else if e.name == "region" {
			var r regionEvent
			if err := json.Unmarshal([]byte(e.data), &r); err != nil {
				t.Fatal(err)
			}
			if r.Name == regionPanels {
				found = append(found, e)
			}
		}
	}
	if closed || len(found) != 1 {
		t.Fatalf("closed=%v panels=%+v all=%+v", closed, found, all)
	}
	return found[0]
}

func TestPanelRowsRenderedSharedParity(t *testing.T) {
	f := panelRowsFixture(t)
	panelRowsPublish(t, f, "initial", "initial")
	for _, lang := range []string{"en", "ko"} {
		src := f.d.sessionPageSource("livesession", lang, "")
		a, b := f.d.cache.acquire(src.key), f.d.cache.acquire(src.key)
		first, _ := f.d.cache.get(a, src, false)
		second, _ := f.d.cache.get(b, src, false)
		if first.panels == nil || first.panels != second.panels || !reflect.DeepEqual(first.panels.order, []string{"p-todo", "p-agenda", "p-tickets", "p-mailbox"}) {
			t.Fatalf("%s: decomposition missing or not shared", lang)
		}
		r := first.regions[regionPanels]
		before, _ := json.Marshal([]any{first.panels.order, first.panels.shell, first.panels.rows})
		if panelRowsRebuild(first.panels) != string(r.HTML) || newRegion(panelRowsRebuild(first.panels)).Hash != r.Hash {
			t.Fatalf("%s: full bytes/hash changed", lang)
		}
		for _, row := range first.panels.rows {
			if row.Hash != newRegion(row.HTML).Hash {
				t.Fatalf("%s: incomplete row hash %s", lang, row.Key)
			}
		}
		panelRowsPublish(t, f, "changed-"+lang, "initial")
		next, _ := f.d.cache.get(a, src, false)
		after, _ := json.Marshal([]any{first.panels.order, first.panels.shell, first.panels.rows})
		if string(before) != string(after) || next.panels == first.panels || panelRowsRebuild(first.panels) != string(r.HTML) {
			t.Fatalf("%s: held decomposition mutated", lang)
		}
		if next.panels.rows["p-agenda"].Hash == first.panels.rows["p-agenda"].Hash || next.panels.rows["p-todo"] != first.panels.rows["p-todo"] {
			t.Fatal("card hash isolation lost")
		}
		f.d.cache.release(src.key, a)
		f.d.cache.release(src.key, b)
	}
}

func TestPanelRowsStreams(t *testing.T) {
	f := panelRowsFixture(t)
	panelRowsPublish(t, f, "initial", "initial")
	path := "/s/livesession/events?regions=panels&frame=1&panelrows=1"
	sse := openSSE(t, f.base+path)
	ws := openWS(t, f.base, path, f.base, true).events
	legacy := openSSE(t, f.base+"/s/livesession/events?regions=panels&frame=1")
	first := panelStreamEvent(t, sse)
	if first.name != "region" || first != panelStreamEvent(t, ws) || first != panelStreamEvent(t, legacy) {
		t.Fatal("first full transport parity")
	}
	var full regionEvent
	if err := json.Unmarshal([]byte(first.data), &full); err != nil {
		t.Fatal(err)
	}
	baseline := splitPanelRows(full.HTML)
	if baseline == nil {
		t.Fatal("missing native cards")
	}
	reconnect := openSSE(t, f.base+path+"&h=panels."+full.Hash)
	reconnectWS := openWS(t, f.base, path+"&h=panels."+full.Hash, f.base, true).events
	if e := panelStreamEvent(t, reconnect); e != first || e != panelStreamEvent(t, reconnectWS) {
		t.Fatal("h=current must send first full on both transports")
	}
	streams := []<-chan sseEvent{sse, ws, reconnect, reconnectWS}
	check := func(want []string) {
		t.Helper()
		delta := panelStreamEvent(t, sse)
		if delta.name != "panel-rows" {
			t.Fatalf("wanted delta: %+v", delta)
		}
		for _, stream := range streams[1:] {
			if delta != panelStreamEvent(t, stream) {
				t.Fatal("delta transport parity")
			}
		}
		var patch panelRowsEvent
		if err := json.Unmarshal([]byte(delta.data), &patch); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, row := range patch.Rows {
			got = append(got, row.Key)
			if row.Hash != newRegion(row.HTML).Hash {
				t.Fatal("incomplete row hash")
			}
			baseline.rows[row.Key] = row
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(patch.Order, baseline.order) {
			t.Fatalf("rows=%v want=%v order=%v", got, want, patch.Order)
		}
		old := panelStreamEvent(t, legacy)
		var region regionEvent
		if old.name != "region" || json.Unmarshal([]byte(old.data), &region) != nil || region.HTML != panelRowsRebuild(baseline) || region.Hash != patch.Hash || newRegion(region.HTML).Hash != patch.Hash || patch.Hash == full.Hash {
			t.Fatal("legacy full differs from reconstructed delta/hash")
		}
		if len(delta.data) >= len(old.data) {
			t.Fatal("delta did not omit unchanged cards and shell")
		}
	}
	panelRowsPublish(t, f, "changed", "initial")
	check([]string{"p-agenda"})
	panelRowsPublish(t, f, "changed", "changed")
	check([]string{"p-todo"})
	panelRowsPublish(t, f, "both", "both")
	check([]string{"p-todo", "p-agenda"})
	for _, change := range []struct{ before, after, key string }{
		{"Stable ticket", "Updated ticket", "p-tickets"},
		{"Stable mail", "Updated mail", "p-mailbox"},
	} {
		p := f.src.page("livesession")
		p.Board.Panels = template.HTML(strings.Replace(string(p.Board.Panels), change.before, change.after, 1))
		p.Seq++
		f.src.setPage("livesession", p)
		f.d.index.Nudge("livesession", f.project)
		f.d.index.Settle()
		check([]string{change.key})
	}
}

func TestPanelRowsFailedSendAndUncertainBaseline(t *testing.T) {
	d := &Daemon{stats: &streamStats{}}
	raw := `<details class="panel" id="p-agenda"><summary>Agenda</summary><details data-key="agenda/plan"><summary>nested</summary>A</details></details> <details class="panel" id="p-todo"><summary>Tasks</summary>B</details>`
	r, rows := newRegion(raw), splitPanelRows(raw)
	sink, state := &railRowsSink{fail: true}, &panelRowState{}
	held := map[string]string{regionPanels: r.Hash}
	send := func(r renderedRegion, rows *panelRows) bool {
		_, ok := d.sendPanelRegions(sink, regionSet{regionPanels: r}, held, nil, nil, nil, nil, nil, nil, nil, nil, state, rows)
		return ok
	}
	if send(r, rows) || state.initialized || state.previous != nil || len(sink.events) != 0 || held[regionPanels] != r.Hash {
		t.Fatal("failed first write advanced baseline")
	}
	sink.fail = false
	if !send(r, rows) || !state.initialized || state.previous != rows || sink.events[0].name != "region" {
		t.Fatal("first success must be full with h=current")
	}
	next := newRegion(strings.Replace(raw, ">A<", ">changed<", 1))
	sink.fail = true
	if send(next, splitPanelRows(string(next.HTML))) || held[regionPanels] != r.Hash || state.previous != rows {
		t.Fatal("failed delta advanced held hash/baseline")
	}
	sink.fail = false
	if !send(next, splitPanelRows(string(next.HTML))) || sink.events[1].name != "panel-rows" {
		t.Fatal("retry lost changed card")
	}
	for label, changed := range map[string]string{
		"shell":      "\n" + raw,
		"order":      strings.Replace(strings.Replace(raw, `id="p-agenda"`, `id="p-todo"`, 1), `id="p-todo"><summary>Tasks`, `id="p-agenda"><summary>Tasks`, 1),
		"membership": strings.Replace(raw, `id="p-todo"`, `id="p-mailbox"`, 1),
	} {
		state.previous = rows
		n := len(sink.events)
		sink.fail = true
		oldHash := held[regionPanels]
		if send(newRegion(changed), splitPanelRows(changed)) || held[regionPanels] != oldHash || state.previous != rows {
			t.Fatalf("%s failed full advanced baseline", label)
		}
		sink.fail = false
		if !send(newRegion(changed), splitPanelRows(changed)) || sink.events[n].name != "region" {
			t.Fatalf("%s assumed old shell/order", label)
		}
	}
	uncertain := newRegion("unfamiliar panels")
	if !send(uncertain, nil) || state.previous != nil || sink.events[len(sink.events)-1].name != "region" {
		t.Fatal("uncertain full did not reset baseline")
	}
	n := len(sink.events)
	if !send(uncertain, nil) || len(sink.events) != n {
		t.Fatal("unchanged uncertain region resent")
	}
	if !send(r, rows) || sink.events[n].name != "region" {
		t.Fatal("recovered decomposition assumed unproven baseline")
	}
}
