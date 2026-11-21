package wstracker

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

func TestIncrementalCheckpointSamplingIsBoundedAndTailDense(t *testing.T) {
	for _, total := range []int{0, 1500, 10_000, 1_000_000} {
		count, last := 0, 0
		for pos := 1; pos <= total; pos++ {
			if checkpointAt(pos, total) {
				count++
				last = pos
			}
		}
		if total > 0 && last != total {
			count++ // the terminal save
		}
		if count > foldCheckpointLimit+2 {
			t.Fatalf("cold fold creates %d deep copies for %d events", count, total)
		}
		if total > foldCheckpointStride && total-last > foldCheckpointStride {
			t.Fatalf("tail is not densely checkpointed: total=%d last=%d", total, last)
		}
	}
}

func incrementalEvent(member string, seq int, typ, payload string, second int64, seen map[string]int) wsledger.Event {
	return wsledger.Event{ID: wsledger.EventID{Member: member, Clone: "pc", Seq: seq}, TS: time.Unix(second, 0), Type: typ,
		Payload: json.RawMessage(payload), Seen: seen, Verdict: wsledger.Valid}
}

// The final fold can reject a concurrent claim that passed its causal check.
// A later descendant of the losing claim sees that claim, but not its winner:
// resuming at a terminal checkpoint must judge it exactly as a zero replay.
func TestIncrementalFoldCausalVerdictSurvivesFinalFlag(t *testing.T) {
	g := json.RawMessage(`{"admins":["a"],"members":[{"id":"a","keys":["a"]},{"id":"b","keys":["b"]},{"id":"c","keys":["c"]}]}`)
	create := incrementalEvent("a", 1, TypeItemCreate, `{"kind":"task","title":"open work"}`, 1, nil)
	open := incrementalEvent("a", 2, TypeItemOpen, `{"item":"a.pc#1","scope":"store"}`, 2, nil)
	winner := incrementalEvent("b", 1, TypeItemClaim, `{"item":"a.pc#1"}`, 3, map[string]int{"a.pc": 2})
	loser := incrementalEvent("c", 1, TypeItemClaim, `{"item":"a.pc#1"}`, 4, map[string]int{"a.pc": 2})
	src := newMemSource(g, "a.pc", []wsledger.Event{create, open, winner, loser})
	tr := New(src, src.stream)
	claim := tr.r.byID[loser.ID.String()]
	if claim.status != "flagged" || claim.causalStatus != "applied" || !strings.Contains(claim.reason, concurrently) {
		t.Fatalf("fixture needs a final-fold-only flag: %+v", claim)
	}
	status := incrementalEvent("c", 2, TypeItemStatus, `{"item":"a.pc#1","status":"doing"}`, 5, map[string]int{"a.pc": 2})
	src.add(status)
	assertIncrementalFold(t, tr, src)
	if tr.r.memo.replayedFrom != 4 {
		t.Fatalf("did not exercise terminal-checkpoint resume: %d", tr.r.memo.replayedFrom)
	}
	if n := tr.r.byID[status.ID.String()]; n.status != "applied" {
		t.Fatalf("causal descendant lost its claim: %s %s", n.status, n.reason)
	}
	if claim.status != "flagged" {
		t.Fatal("causal checks erased the prefix's final-fold flag")
	}
}

// wsledger.closeBatch can update an existing pending event's Seen once a
// remote event enables its signing key. Timestamp priority must not let that
// old event run before its NEW dependency; the old reverse edge must also be
// removed if an existing source event is replaced again without that edge.
func TestIncrementalFoldReindexesReplacementSeen(t *testing.T) {
	g := json.RawMessage(`{"admins":["a","b"],"members":[{"id":"a","keys":["old-key"]},{"id":"b","keys":["b-key"]}]}`)
	pending := incrementalEvent("a", 1, TypeItemCreate, `{"kind":"task","title":"pending key"}`, 1, nil)
	pending.Verdict, pending.Reason = wsledger.Flagged, "key not live in causal past"
	child := incrementalEvent("a", 2, TypeCommentAdd, `{"item":"a.pc#1","body":"dependent"}`, 2, nil)
	key := incrementalEvent("b", 1, wsledger.TypeAddKey, `{"member":"a","key":"new-key"}`, 10, nil)
	src := newMemSource(g, "a.pc", []wsledger.Event{pending, child, key})
	tr := New(src, src.stream)
	pending.Verdict, pending.Reason, pending.Seen = wsledger.Valid, "", map[string]int{"b.pc": 1}
	src.add(pending)
	assertIncrementalFold(t, tr, src)
	ids := func() []string {
		out := []string{}
		for _, n := range tr.r.order {
			out = append(out, n.id)
		}
		return out
	}
	if !slices.Equal(ids(), []string{"b.pc#1", "a.pc#1", "a.pc#2"}) {
		t.Fatalf("new Seen dependency ignored: %v", ids())
	}
	if tr.r.byID[child.ID.String()].status != "applied" {
		t.Fatal("replacement failed to rejudge existing descendants")
	}
	pending.Seen = nil
	src.add(pending)
	assertIncrementalFold(t, tr, src)
	if !slices.Equal(ids(), []string{"a.pc#1", "a.pc#2", "b.pc#1"}) || tr.r.memo.succ[key.ID.String()][pending.ID.String()] {
		t.Fatalf("removed Seen dependency retained: order=%v successors=%v", ids(), tr.r.memo.succ[key.ID.String()])
	}
}

func TestIncrementalFoldOrgStateMatchesFullReplay(t *testing.T) {
	g := json.RawMessage(`{"admins":["a"],"members":[{"id":"a","keys":["a"]},{"id":"b","keys":["b"],"leads":["a"]},{"id":"c","keys":["c"],"leads":["b"]}]}`)
	item := incrementalEvent("a", 1, TypeItemCreate, `{"kind":"task","title":"org fold"}`, 1, nil)
	set := incrementalEvent("a", 2, TypeOrgSet, `{"org":"o-team","name":"Team","head":"b"}`, 2, nil)
	cc := incrementalEvent("a", 3, TypeItemCCOrg, `{"item":"a.pc#1","org":"o-team"}`, 3, nil)
	assign := incrementalEvent("a", 4, TypeItemAssign, `{"item":"a.pc#1","completion":"assigner","targets":[{"root":"b","org":"o-team","include_root":true}]}`, 4, nil)
	src := newMemSource(g, "a.pc", []wsledger.Event{item, set, cc, assign})
	tr := New(src, src.stream)

	moveHead := incrementalEvent("a", 5, TypeOrgSet, `{"org":"o-team","name":"Team","head":"c"}`, 5, nil)
	src.add(moveHead)
	assertIncrementalFold(t, tr, src)
	if tr.r.memo.replayedFrom != 4 {
		t.Fatalf("org append did not resume from terminal checkpoint: %d", tr.r.memo.replayedFrom)
	}
	uncc := incrementalEvent("a", 6, TypeItemUnccOrg, `{"item":"a.pc#1","org":"o-team"}`, 6, nil)
	src.add(uncc)
	assertIncrementalFold(t, tr, src)
	if len(tr.r.S.items["a.pc#1"].cc) != 0 || tr.r.S.ruleByID["a.pc#4~b"].root != "c" {
		t.Fatalf("org checkpoint state diverged: cc=%v rule=%+v", tr.r.S.items["a.pc#1"].cc, tr.r.S.ruleByID["a.pc#4~b"])
	}
}

func TestIncrementalFoldOrgSetCanNameJoinedHead(t *testing.T) {
	g := json.RawMessage(`{"admins":["a"],"members":[{"id":"a","keys":["a"]}]}`)
	join := incrementalEvent("b", 1, wsledger.TypeMemberJoin, `{"member":"b","key":"b"}`, 1, nil)
	src := newMemSource(g, "a.pc", []wsledger.Event{join})
	tr := New(src, src.stream)

	set := incrementalEvent("a", 1, TypeOrgSet, `{"org":"o-team","name":"Team","head":"b"}`, 2, map[string]int{"b.pc": 1})
	src.add(set)
	assertIncrementalFold(t, tr, src)
	n := tr.r.byID[set.ID.String()]
	if n.status != "applied" || tr.r.S.orgs["o-team"] == nil || tr.r.S.orgs["o-team"].head != "b" {
		t.Fatalf("joined member could not head org: status=%s reason=%s org=%+v", n.status, n.reason, tr.r.S.orgs["o-team"])
	}
}

func TestIncrementalFoldOrgConflictMatchesFullReplay(t *testing.T) {
	g := json.RawMessage(`{"admins":["a","b"],"members":[{"id":"a","keys":["a"]},{"id":"b","keys":["b"]},{"id":"h","keys":["h"]}]}`)
	first := incrementalEvent("a", 1, TypeOrgSet, `{"org":"o-a","name":"A","head":"h"}`, 1, nil)
	src := newMemSource(g, "a.pc", []wsledger.Event{first})
	tr := New(src, src.stream)

	conflict := incrementalEvent("b", 1, TypeOrgSet, `{"org":"o-b","name":"B","head":"h"}`, 2, nil)
	src.add(conflict)
	assertIncrementalFold(t, tr, src)
	if n := tr.r.byID[conflict.ID.String()]; n.status != "flagged" || !strings.Contains(n.reason, "already heads") {
		t.Fatalf("incremental org conflict = %s %q", n.status, n.reason)
	}
}
