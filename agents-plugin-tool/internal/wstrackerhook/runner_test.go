package wstrackerhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// fakeSource is a tracker of fake stores: each holds events added in order
// with a generation, read through a cursor the way wsledger's is (a
// cursor of another epoch reads everything).
type fakeSource struct {
	mu     sync.Mutex
	epoch  string
	gen    uint64
	stores []wstrackerhost.StoreEntry
	events map[string][]*fakeEvent
	views  map[string]*wstracker.View
	reads  int
}

type fakeEvent struct {
	id      wsledger.EventID
	ts      time.Time
	gen     uint64
	settled bool
	acts    []wstracker.Activity
}

func newFakeSource(stores ...string) *fakeSource {
	f := &fakeSource{epoch: "e1", events: map[string][]*fakeEvent{}, views: map[string]*wstracker.View{}}
	for _, s := range stores {
		f.stores = append(f.stores, wstrackerhost.StoreEntry{ID: s, Label: strings.ToUpper(s), Member: "ada", Nonce: "n-" + s})
		f.views[s] = &wstracker.View{
			Items: map[string]wstracker.ItemView{"maya.pc#1": {ID: "maya.pc#1", Alias: "a1", Title: "Index", Kind: "task"}},
			Members: map[string]wstracker.MemberView{
				"ada": {ID: "ada", Name: "Ada"}, "bo": {ID: "bo", Name: "Bo"}, "cy": {ID: "cy", Name: "Cy"}},
			ItemOrder:   []string{"maya.pc#1"},
			Assignments: map[string]wstracker.Assignment{},
		}
	}
	return f
}

// restart is a new Ledger instance: a new epoch, so every saved cursor
// reads everything again.
func (f *fakeSource) restart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.epoch += "'"
}

// add holds a settled event of store with activity of kind by actor,
// concerning concerns, at ts.
func (f *fakeSource) add(store string, seq int, ts time.Time, kind, actor string, concerns ...string) wsledger.EventID {
	id := wsledger.EventID{Member: actor, Clone: "pc", Seq: seq}
	f.addEvent(store, &fakeEvent{id: id, ts: ts, settled: true, acts: []wstracker.Activity{{
		Event: id, TS: ts, Kind: kind, Item: "maya.pc#1", Actor: actor, Concerns: concerns}}})
	return id
}

func (f *fakeSource) addEvent(store string, ev *fakeEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gen++
	ev.gen = f.gen
	for i, x := range f.events[store] {
		if x.id == ev.id {
			f.events[store][i] = ev
			return
		}
	}
	f.events[store] = append(f.events[store], ev)
}

func (f *fakeSource) Stores() ([]wstrackerhost.StoreEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wstrackerhost.StoreEntry(nil), f.stores...), nil
}

func (f *fakeSource) ReadActivity(_ context.Context, id string, after wsledger.Cursor, want func(wsledger.EventID, time.Time, bool) bool) (wstrackerhost.ActivityRead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	var entry wstrackerhost.StoreEntry
	for _, e := range f.stores {
		if e.ID == id {
			entry = e
		}
	}
	rd := wstrackerhost.ActivityRead{Entry: entry, Cursor: wsledger.Cursor{Gen: f.gen, Epoch: f.epoch}, CatchUp: after.Epoch != f.epoch, View: f.views[id]}
	for _, ev := range f.events[id] {
		if !rd.CatchUp && ev.gen <= after.Gen {
			continue
		}
		ae := wstrackerhost.ActivityEvent{ID: ev.id, TS: ev.ts}
		if want(ev.id, ev.ts, rd.CatchUp) {
			ae.Chosen, ae.Settled = true, ev.settled
			if ev.settled {
				ae.Activity = ev.acts
			}
		}
		rd.Events = append(rd.Events, ae)
	}
	return rd, nil
}

// recorder is a fake Exec: it records each run as "<program> <kind> <id>"
// and decodes its event.
type recorder struct {
	mu     sync.Mutex
	runs   []string
	events []Event
	// fail and block, keyed by event id, make that run fail or wait.
	fail  map[string]error
	block map[string]chan struct{}
}

func (rc *recorder) exec(ctx context.Context, argv []string, dir string, stdin []byte) error {
	var ev Event
	if err := json.Unmarshal(stdin, &ev); err != nil {
		return err
	}
	rc.mu.Lock()
	rc.runs = append(rc.runs, argv[0]+" "+ev.Kind+" "+ev.ID)
	rc.events = append(rc.events, ev)
	ch, err := rc.block[ev.ID], rc.fail[ev.ID]
	rc.mu.Unlock()
	if ch != nil {
		<-ch
	}
	return err
}

func (rc *recorder) take() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := rc.runs
	rc.runs = nil
	return out
}

type world struct {
	t   *testing.T
	dir string
	src *fakeSource
	rec *recorder
	log *syncBuf
	now time.Time
	r   *Runner
}

// syncBuf is the log: the workers write to it concurrently.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var t0 = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

func newWorld(t *testing.T, stores ...string) *world {
	if len(stores) == 0 {
		stores = []string{"team"}
	}
	w := &world{t: t, dir: t.TempDir(), src: newFakeSource(stores...), rec: &recorder{}, log: &syncBuf{}, now: t0}
	t.Cleanup(func() {
		if w.r != nil {
			w.r.Close()
		}
	})
	return w
}

func (w *world) hooks(content string) {
	w.t.Helper()
	path := filepath.Join(w.dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		w.t.Fatal(err)
	}
	// a rewrite within the clock's resolution still reads as a change
	at := time.Now().Add(time.Duration(len(content)) * time.Second)
	if err := os.Chtimes(path, at, at); err != nil {
		w.t.Fatal(err)
	}
}

// start is a daemon start: a new runner over the same state directory,
// loading the hooks file and reading every store once.
func (w *world) start() {
	w.t.Helper()
	if w.r != nil {
		w.r.Close()
	}
	w.r = newRunner(Options{Dir: w.dir, Source: w.src, Log: w.log, Now: func() time.Time { return w.now },
		Location: time.UTC, Exec: w.rec.exec, BaseURL: func() string { return "http://127.0.0.1:7000" }})
	w.r.reload()
	w.scan()
}

// scan reads every store and waits for the runs it queued.
func (w *world) scan() {
	w.t.Helper()
	w.r.scanAll()
	w.drain()
}

func (w *world) drain() {
	w.t.Helper()
	done := make(chan struct{})
	go func() { w.r.pending.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		w.t.Fatal("the queued runs did not finish")
	}
}

// want checks the runs since the last check. The hooks run in parallel,
// each in its own order, so runs compare grouped by program, in order
// within each.
func (w *world) want(runs ...string) {
	w.t.Helper()
	got := byProgram(w.rec.take())
	if strings.Join(got, "\n") != strings.Join(byProgram(runs), "\n") {
		w.t.Fatalf("runs:\n got %q\nwant %q\nlog:\n%s", got, byProgram(runs), w.log)
	}
}

func byProgram(runs []string) []string {
	out := append([]string(nil), runs...)
	sort.SliceStable(out, func(i, j int) bool {
		pi, _, _ := strings.Cut(out[i], " ")
		pj, _, _ := strings.Cut(out[j], " ")
		return pi < pj
	})
	return out
}

const mineAndAll = `{"hooks": [{"command": ["mine"]}, {"command": ["all"], "scope": "all"}]}`

func TestMineScopeTakesOnlyWhatConcernsTheMember(t *testing.T) {
	w := newWorld(t)
	w.hooks(mineAndAll)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")     // to ada: both
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "cy")      // unrelated: all only
	w.src.add("team", 3, t0, wstracker.ActComment, "ada", "ada")     // ada's own: all only
	w.src.add("team", 4, t0, wstracker.ActStatus, "cy", "ada", "cy") // on ada's item: both
	w.scan()
	w.want(
		"mine assigned bo.pc#1", "mine status cy.pc#4",
		"all assigned bo.pc#1", "all assigned bo.pc#2", "all comment ada.pc#3", "all status cy.pc#4",
	)
}

func TestEventsArriveInOrderAndAFailureDoesNotStopTheQueue(t *testing.T) {
	w := newWorld(t)
	w.hooks(`{"hooks": [{"command": ["toast"]}]}`)
	w.start()
	w.rec.fail = map[string]error{"bo.pc#2": errors.New("exit status 3: boom")}
	release := make(chan struct{})
	w.rec.block = map[string]chan struct{}{"bo.pc#1": release}
	for i := 1; i <= 5; i++ {
		w.src.add("team", i, t0, wstracker.ActAssigned, "bo", "ada")
	}
	go func() { time.Sleep(50 * time.Millisecond); close(release) }()
	w.scan()
	w.want("toast assigned bo.pc#1", "toast assigned bo.pc#2", "toast assigned bo.pc#3", "toast assigned bo.pc#4", "toast assigned bo.pc#5")
	if !strings.Contains(w.log.String(), `hook "toast" failed: exit status 3: boom`) {
		t.Fatalf("the failure is not logged:\n%s", w.log)
	}
}

func TestFirstStartDeliversNothingFromBeforeIt(t *testing.T) {
	w := newWorld(t)
	w.src.add("team", 1, t0.Add(-time.Hour), wstracker.ActAssigned, "bo", "ada")
	w.hooks(mineAndAll)
	w.start()
	w.want()
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#2", "all assigned bo.pc#2")
	// and a restart does not reach back to it either
	w.src.restart()
	w.start()
	w.want()
}

func TestReloadAndRescanDeliverNoDuplicate(t *testing.T) {
	w := newWorld(t)
	w.hooks(mineAndAll)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#1", "all assigned bo.pc#1")
	w.scan()
	w.want()
	// the file changes: the kept hook keeps its place, the new one starts
	// from the current cursor with no backfill
	w.hooks(`{"hooks": [{"command": ["mine"]}, {"command": ["mail"], "scope": "all"}]}`)
	if !w.r.reload() {
		t.Fatal("the changed file is not picked up")
	}
	w.scan()
	w.want()
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#2", "mail assigned bo.pc#2")
}

func TestRestartCatchesUpWithinTheWindow(t *testing.T) {
	w := newWorld(t)
	w.hooks(mineAndAll)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#1", "all assigned bo.pc#1")
	// the daemon is down for 30 hours; events arrive meanwhile
	w.r.Close()
	w.src.add("team", 2, t0.Add(time.Hour), wstracker.ActAssigned, "bo", "ada")   // older than 24 h at restart
	w.src.add("team", 3, t0.Add(20*time.Hour), wstracker.ActComment, "cy", "ada") // within
	w.src.add("team", 4, t0.Add(25*time.Hour), wstracker.ActComment, "cy", "cy")  // within, unrelated
	w.now = t0.Add(30 * time.Hour)
	w.src.restart()
	w.start()
	w.want("mine comment cy.pc#3", "all comment cy.pc#3", "all comment cy.pc#4")
	// an epoch change while running redelivers nothing
	w.src.restart()
	w.scan()
	w.want()
}

func TestRestartRedeliversNothingFilteredOrBackfills(t *testing.T) {
	w := newWorld(t)
	w.hooks(`{"hooks": [{"command": ["mine"]}]}`)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "cy") // dropped by the mine filter
	w.scan()
	w.want()
	// an all hook is added while running: it starts at the current cursor
	w.hooks(mineAndAll)
	w.r.reload()
	w.scan()
	w.want()
	w.now = t0.Add(time.Hour)
	w.src.restart()
	w.start()
	w.want()
}

func TestLateSyncedEventIsDeliveredWhileRunning(t *testing.T) {
	w := newWorld(t)
	w.hooks(mineAndAll)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActComment, "ada", "ada")
	w.scan()
	w.want("all comment ada.pc#1")
	// a teammate's event written two days ago, synced only now after later
	// local events
	w.src.add("team", 2, t0.Add(time.Minute), wstracker.ActComment, "ada", "ada")
	w.src.add("team", 7, t0.Add(-48*time.Hour), wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#7", "all comment ada.pc#2", "all assigned bo.pc#7")
}

func TestWaitingEventIsDeliveredOnceSettled(t *testing.T) {
	w := newWorld(t)
	w.hooks(mineAndAll)
	w.start()
	id := wsledger.EventID{Member: "bo", Clone: "pc", Seq: 1}
	w.src.addEvent("team", &fakeEvent{id: id, ts: t0})
	w.scan()
	w.want()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#1", "all assigned bo.pc#1")
}

func TestHookAppliesToEveryStore(t *testing.T) {
	w := newWorld(t, "team", "home")
	w.hooks(`{"hooks": [{"command": ["mine"]}]}`)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.src.add("home", 1, t0, wstracker.ActAssigned, "cy", "ada")
	w.scan()
	w.want("mine assigned bo.pc#1", "mine assigned cy.pc#1")
	if w.rec.events[0].Store != (StoreRef{ID: "team", Name: "TEAM"}) || w.rec.events[1].Store != (StoreRef{ID: "home", Name: "HOME"}) {
		t.Fatalf("stores: %+v %+v", w.rec.events[0].Store, w.rec.events[1].Store)
	}
}

func TestEventJSONCarriesTheVersionedFields(t *testing.T) {
	w := newWorld(t)
	w.hooks(`{"hooks": [{"command": ["mine"]}]}`)
	w.start()
	id := wsledger.EventID{Member: "bo", Clone: "pc", Seq: 1}
	long := strings.Repeat("가", commentLimit+20)
	w.src.views["team"].Items["maya.pc#1"] = wstracker.ItemView{ID: "maya.pc#1", Alias: "a1", Title: "Index", Kind: "task", Due: ptr("2026-11-20")}
	w.src.addEvent("team", &fakeEvent{id: id, ts: t0, settled: true, acts: []wstracker.Activity{{
		Event: id, TS: t0, Kind: wstracker.ActComment, Item: "maya.pc#1", Actor: "bo", Target: "ada", Comment: long, Concerns: []string{"ada"}}}})
	w.scan()
	w.want("mine comment bo.pc#1")
	got := w.rec.events[0]
	want := Event{V: 1, ID: "bo.pc#1", Kind: "comment", TS: "2026-11-11T12:00:00Z",
		Store: StoreRef{ID: "team", Name: "TEAM"}, Item: ItemRef{ID: "maya.pc#1", Alias: "a1", Title: "Index", Kind: "task"},
		Actor: &Member{ID: "bo", Name: "Bo"}, Target: &Member{ID: "ada", Name: "Ada"},
		Detail: Detail{Comment: strings.Repeat("가", commentLimit), Due: "2026-11-20T00:00:00Z"},
		URL:    "http://127.0.0.1:7000/#/team/maya.pc%231"}
	gj, _ := json.Marshal(got)
	wj, _ := json.Marshal(want)
	if string(gj) != string(wj) {
		t.Fatalf("event:\n got %s\nwant %s", gj, wj)
	}
}

func ptr(s string) *string { return &s }

func TestDueSoonFiresOnceAtTheThreshold(t *testing.T) {
	w := newWorld(t)
	w.hooks(`{"hooks": [{"command": ["mine"]}, {"command": ["all"], "scope": "all"}], "due_soon_hours": 6}`)
	v := w.src.views["team"]
	v.Items["maya.pc#1"] = wstracker.ItemView{ID: "maya.pc#1", Alias: "a1", Title: "Index", Kind: "task", Due: ptr("2026-11-12"),
		Assignments: []string{"x>ada", "x>cy"}}
	v.Items["maya.pc#2"] = wstracker.ItemView{ID: "maya.pc#2", Title: "Other", Kind: "task", Due: ptr("2026-11-12"), Assignments: []string{"y>cy"}}
	v.Items["maya.pc#3"] = wstracker.ItemView{ID: "maya.pc#3", Title: "Done", Kind: "task", Due: ptr("2026-11-12"), Assignments: []string{"z>ada"}}
	v.ItemOrder = []string{"maya.pc#1", "maya.pc#2", "maya.pc#3"}
	v.Assignments = map[string]wstracker.Assignment{
		"x>ada": {ID: "x>ada", To: "ada", State: "live", Status: "doing"},
		"x>cy":  {ID: "x>cy", To: "cy", State: "live", Status: "done"},
		"y>cy":  {ID: "y>cy", To: "cy", State: "live", Status: "todo"},
		"z>ada": {ID: "z>ada", To: "ada", State: "live", Status: "done"},
	}
	w.now = time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC) // 7 h before
	w.start()
	w.want()
	w.now = time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC) // 6 h before
	w.scan()
	w.want("mine due_soon due:maya.pc#1:2026-11-12", "all due_soon due:maya.pc#1:2026-11-12", "all due_soon due:maya.pc#2:2026-11-12")
	if ev := w.rec.events[0]; ev.Actor != nil || ev.TS != "2026-11-11T18:00:00Z" || ev.Detail.Due != "2026-11-12T00:00:00Z" {
		t.Fatalf("due event %+v", ev)
	}
	w.now = w.now.Add(time.Hour)
	w.scan()
	w.want()
	w.src.restart()
	w.start()
	w.want()
}

func TestInvalidFileRunsNothingAndAChangeIsPickedUp(t *testing.T) {
	w := newWorld(t)
	w.hooks(`{"hooks": [{"command": ["mine"], "scop": "all"}]}`)
	w.start()
	if !strings.Contains(w.log.String(), "is invalid, so no hook runs") {
		t.Fatalf("the invalid file is not logged:\n%s", w.log)
	}
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want()
	w.hooks(`{"hooks": [{"command": ["mine"]}]}`)
	w.r.reload()
	w.scan()
	w.want() // the fix starts no backfill
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#2")
}

func TestNoFileWritesNothingAndItsAppearanceStartsAtTheHead(t *testing.T) {
	w := newWorld(t)
	w.start()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	if _, err := os.Stat(filepath.Join(w.dir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("an idle runner wrote state: %v", err)
	}
	w.hooks(`{"hooks": [{"command": ["mine"]}]}`)
	w.r.reload()
	w.scan()
	w.want()
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	w.want("mine assigned bo.pc#2")
}

func TestParseConfig(t *testing.T) {
	for _, bad := range []string{
		`{"hooks": [{"command": []}]}`,
		`{"hooks": [{"command": [""]}]}`,
		`{"hooks": [{"command": ["x"], "scope": "team"}]}`,
		`{"hooks": [{"command": ["x"]}, {"command": ["x"], "scope": "mine"}]}`,
		`{"hooks": [], "due_soon_hours": -1}`,
		`{"hooks": []} {}`,
		`{"hooks": "x"}`,
	} {
		if _, err := parseConfig([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	c, err := parseConfig([]byte(`{"hooks": [{"command": ["x", "a b"]}, {"command": ["x"], "scope": "all"}], "due_soon_hours": 1.5}`))
	if err != nil || c.Hooks[0].Scope != ScopeMine || c.dueSoon() != 90*time.Minute {
		t.Fatalf("parse: %+v, %v", c, err)
	}
}
