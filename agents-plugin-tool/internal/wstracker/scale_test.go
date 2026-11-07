package wstracker

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// scaleStore generates a store of n events: 31 members in a three-level
// tree (root and lead0 admins) writing from their own clones, each
// refreshing what it has seen now and then (so writes are often
// concurrent), with creates, self-assignments, "everyone within N" rules,
// status writes, closes, edits, comments, and lead changes. Deterministic.
func scaleStore(n int) (json.RawMessage, []wsledger.Event) {
	rng := rand.New(rand.NewSource(1))
	type mem struct {
		id, lead string
	}
	var members []mem
	members = append(members, mem{"root", ""})
	for i := 0; i < 5; i++ {
		lead := fmt.Sprintf("lead%d", i)
		members = append(members, mem{lead, "root"})
		for j := 0; j < 5; j++ {
			members = append(members, mem{fmt.Sprintf("m%d-%d", i, j), lead})
		}
	}
	var gm []map[string]any
	for _, m := range members {
		leads := []string{}
		if m.lead != "" {
			leads = append(leads, m.lead)
		}
		gm = append(gm, map[string]any{"id": m.id, "name": m.id, "keys": []string{m.id + "-k1"}, "leads": leads})
	}
	genesis, _ := json.Marshal(map[string]any{"admins": []string{"root", "lead0"}, "members": gm})

	heads := map[string]int{}
	known := map[string]map[string]int{}
	own := map[string][]string{} // member -> items they created and self-assigned
	var items []string
	t := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var out []wsledger.Event
	emit := func(m mem, typ string, payload any) string {
		stream := m.id + ".pc"
		if known[stream] == nil || rng.Intn(4) == 0 {
			known[stream] = map[string]int{}
			for s, q := range heads {
				known[stream][s] = q
			}
		}
		seen := map[string]int{}
		for s, q := range known[stream] {
			if s != stream {
				seen[s] = q
			}
		}
		heads[stream]++
		raw, _ := json.Marshal(payload)
		t = t.Add(time.Duration(30+rng.Intn(60)) * time.Second)
		id := wsledger.EventID{Member: m.id, Clone: "pc", Seq: heads[stream]}
		out = append(out, wsledger.Event{ID: id, TS: t, Type: typ, Payload: raw, Seen: seen, Verdict: wsledger.Valid})
		return id.String()
	}
	statuses := []string{"todo", "doing", "blocked"}
	for len(out) < n {
		m := members[rng.Intn(len(members))]
		switch r := rng.Intn(100); {
		case r < 12 || len(own[m.id]) == 0:
			id := emit(m, TypeItemCreate, map[string]any{"kind": "task", "title": "task " + m.id, "priority": rng.Intn(3)})
			emit(m, TypeItemAssign, map[string]any{"item": id, "mode": "chosen", "people": []string{m.id}})
			own[m.id] = append(own[m.id], id)
			items = append(items, id)
		case r < 15 && m.lead == "root":
			id := emit(m, TypeItemCreate, map[string]any{"kind": "task", "title": "team task"})
			emit(m, TypeItemAssign, map[string]any{"item": id, "mode": "all", "levels": 1})
			items = append(items, id)
		case r < 55:
			it := own[m.id][rng.Intn(len(own[m.id]))]
			emit(m, TypeItemStatus, map[string]any{"item": it, "status": statuses[rng.Intn(3)], "note": "progress"})
		case r < 62:
			emit(m, TypeItemClose, map[string]any{"item": own[m.id][rng.Intn(len(own[m.id]))]})
		case r < 67:
			emit(m, TypeItemReopen, map[string]any{"item": own[m.id][rng.Intn(len(own[m.id]))]})
		case r < 75:
			emit(m, TypeItemEdit, map[string]any{"item": own[m.id][rng.Intn(len(own[m.id]))], "body": fmt.Sprintf("body %d", rng.Int())})
		case r < 98:
			emit(m, TypeCommentAdd, map[string]any{"item": items[rng.Intn(len(items))], "body": "note"})
		default: // two admins moving members concurrently: concurrent-change markers
			x := members[6+rng.Intn(len(members)-6)]
			emit(members[rng.Intn(2)], TypeSetLead, map[string]any{"member": x.id, "leads": []string{fmt.Sprintf("lead%d", rng.Intn(5))}})
		}
	}
	return genesis, out
}

// TestScale is the (298) gate: a generated 5,000-event store derives in
// under 5 s. The bound is generous on purpose; this is not a benchmark.
func TestScale(t *testing.T) {
	genesis, evs := scaleStore(5000)
	src := newMemSource(genesis, "root.pc", evs)
	start := time.Now()
	v := Derive(src, "root.pc", time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), 0)
	took := time.Since(start)
	t.Logf("derived %d events (%d flagged, %d concurrent, %d items, %d assignments) in %v",
		v.Stats.Events, v.Stats.Flagged, v.Stats.Concurrent, len(v.Items), len(v.Assignments), took)
	if v.Stats.Events != len(evs) {
		t.Fatalf("derived %d events, want %d", v.Stats.Events, len(evs))
	}
	if v.Stats.Flagged*2 > v.Stats.Events {
		t.Fatalf("the generated store is mostly flagged (%d of %d); it no longer exercises the fold", v.Stats.Flagged, v.Stats.Events)
	}
	if took > 5*time.Second {
		t.Fatalf("deriving 5,000 events took %v, over the 5 s bound", took)
	}
}

// TestVerdictsEqualFullReplay pins the verdict shortcut (stateBefore folds
// only what the event's rules read) to the reference definition: fold the
// event's whole causal past, then judge.
func TestVerdictsEqualFullReplay(t *testing.T) {
	genesis, evs := scaleStore(1500)
	src := newMemSource(genesis, "root.pc", evs)
	r := replay(evs, genesis, src.Concurrent)
	for _, n := range r.order {
		if n.ev.Verdict != wsledger.Valid || isIdentity(n.typ) {
			continue
		}
		S := newState(r.g, r.hb, false)
		for _, x := range r.order[:n.pos] {
			if x.status == "applied" && r.hb(x, n) {
				S.apply(x)
			}
		}
		want := "applied"
		if rej := S.check(n); rej != "" {
			want = "flagged: " + string(rej)
		}
		got := n.status
		if got == "flagged" {
			got += ": " + n.reason
		}
		if got != want {
			t.Fatalf("%s %s: shortcut %q, full replay %q", n.id, n.typ, got, want)
		}
	}
}
