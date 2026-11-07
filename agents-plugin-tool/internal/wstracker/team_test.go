package wstracker

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// team is an in-memory world of clones over the translated team-alpha
// fixture: a shared pool of pushed events and one memSource per clone. An
// online clone fetches the pool before it acts or reads and pushes its new
// events after it acts; an offline clone does neither until it comes back.
type team struct {
	t       *testing.T
	genesis json.RawMessage
	pool    []wsledger.Event
	inPool  map[wsledger.EventID]bool
	src     map[string]*memSource
	trk     map[string]*Tracker
	offline map[string]bool
	now     time.Time
}

// newTeam seeds the world with everything the team-alpha store holds (the
// omar.pc golden: his clone holds every pushed commit) at its clock,
// 2026-11-06 09:00.
func newTeam(t *testing.T) *team {
	t.Helper()
	doc, src := loadGolden(t, "testdata/golden/team-alpha__omar.pc.json")
	now, err := time.Parse(time.RFC3339, doc.Now)
	if err != nil {
		t.Fatal(err)
	}
	tm := &team{t: t, genesis: doc.Genesis, inPool: map[wsledger.EventID]bool{}, src: map[string]*memSource{},
		trk: map[string]*Tracker{}, offline: map[string]bool{}, now: now}
	for _, e := range src.events {
		tm.pool = append(tm.pool, e)
		tm.inPool[e.ID] = true
	}
	return tm
}

func (tm *team) tick() time.Time {
	tm.now = tm.now.Add(time.Minute)
	return tm.now
}

func (tm *team) clone(s string) *Tracker {
	if tm.trk[s] == nil {
		src := newMemSource(tm.genesis, s, tm.pool)
		src.clock = tm.tick
		tm.src[s], tm.trk[s] = src, New(src, s)
	}
	return tm.trk[s]
}

// fetch adds the pool events clone s lacks; push adds s's events to the pool.
func (tm *team) fetch(s string) {
	src := tm.src[s]
	var add []wsledger.Event
	for _, e := range tm.pool {
		if _, ok := src.byID[e.ID]; !ok {
			add = append(add, e)
		}
	}
	if len(add) > 0 {
		src.add(add...)
	}
}

func (tm *team) push(s string) {
	for _, e := range tm.src[s].events {
		if !tm.inPool[e.ID] {
			tm.pool = append(tm.pool, e)
			tm.inPool[e.ID] = true
		}
	}
}

func (tm *team) goOffline(s string) {
	tm.clone(s)
	tm.fetch(s)
	tm.offline[s] = true
}

func (tm *team) comeOnline(s string) {
	delete(tm.offline, s)
	tm.push(s)
	tm.fetch(s)
}

func (tm *team) dispatch(s, typ string, payload any, origin string) ([]wsledger.EventID, error) {
	trk := tm.clone(s)
	if !tm.offline[s] {
		tm.fetch(s)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		tm.t.Fatal(err)
	}
	ids, err := trk.Dispatch(context.Background(), []wsledger.NewEvent{{Type: typ, Origin: origin, Payload: raw}})
	if err == nil && !tm.offline[s] {
		tm.push(s)
	}
	return ids, err
}

// do dispatches one event from clone s and fails the test on a refusal.
func (tm *team) do(s, typ string, payload any) string { return tm.doAs(s, typ, payload, "") }

func (tm *team) doAs(s, typ string, payload any, origin string) string {
	tm.t.Helper()
	ids, err := tm.dispatch(s, typ, payload, origin)
	if err != nil {
		tm.t.Fatalf("%s %s: %v", s, typ, err)
	}
	return ids[0].String()
}

// refused dispatches one event from clone s, expects a refusal, and
// returns its reason.
func (tm *team) refused(s, typ string, payload any) string {
	tm.t.Helper()
	_, err := tm.dispatch(s, typ, payload, "")
	var ref *Refusal
	if !errors.As(err, &ref) {
		tm.t.Fatalf("%s %s: want a refusal, got %v", s, typ, err)
	}
	return ref.Reason
}

func (tm *team) view(s string) *View {
	trk := tm.clone(s)
	if !tm.offline[s] {
		tm.fetch(s)
	}
	return trk.View(tm.now, 0)
}

// ---------------------------------------------------------------- lookups

func itemID(t *testing.T, v *View, title string) string {
	t.Helper()
	for id, it := range v.Items {
		if it.Title == title {
			return id
		}
	}
	t.Fatalf("no item titled %q", title)
	return ""
}

func asg(t *testing.T, v *View, id string) Assignment {
	t.Helper()
	a, ok := v.Assignments[id]
	if !ok {
		t.Fatalf("no assignment %s", id)
	}
	return a
}

func event(t *testing.T, v *View, id string) EventView {
	t.Helper()
	for _, e := range v.Events {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no event %s", id)
	return EventView{}
}

func wantEq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func wantIn(t *testing.T, what string, list []string, x string) {
	t.Helper()
	if !slices.Contains(list, x) {
		t.Fatalf("%s = %v, want it to hold %s", what, list, x)
	}
}

func wantNotIn(t *testing.T, what string, list []string, x string) {
	t.Helper()
	if slices.Contains(list, x) {
		t.Fatalf("%s = %v, want it without %s", what, list, x)
	}
}

func teamRow(t *testing.T, v *View, member string) Team {
	t.Helper()
	for _, r := range v.My.Team {
		if r.ID == member {
			return r
		}
	}
	t.Fatalf("%s: no My team row for %s", v.Viewer.Stream, member)
	return Team{}
}

func markerKinds(e EventView) []string {
	var out []string
	for _, m := range e.Markers {
		out = append(out, m.Kind+":"+m.Other)
	}
	return out
}
