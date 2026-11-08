package wsledger

import (
	"context"
	"testing"
)

func TestCreateJoinAppendSync(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	bo := w.join("bo", "c1")

	id := appendOne(t, ada, "item.create", map[string]string{"title": "x"})
	if got := ada.Pending().Uncommitted; got != 1 {
		t.Fatalf("pending uncommitted = %d, want 1", got)
	}
	wantVerdict(t, ada, id, Valid)
	if e := eventsByID(ada)[id]; !e.Pending {
		t.Fatalf("event should be pending before sync")
	}
	rep := syncOK(t, ada)
	if !rep.Closed {
		t.Fatalf("sync did not close the batch")
	}
	if e := eventsByID(ada)[id]; e.Pending {
		t.Fatalf("event still pending after sync")
	}

	fetchOK(t, bo)
	wantVerdict(t, bo, id, Valid)
	ident := bo.Identity()
	if len(ident.Members) != 2 || ident.Admins[0] != "ada" {
		t.Fatalf("identity = %+v", ident)
	}

	bid := appendOne(t, bo, "comment.add", map[string]string{"item": id.String()})
	syncOK(t, bo)
	fetchOK(t, ada)
	e := wantVerdict(t, ada, bid, Valid)
	if e.Seen["ada.c1"] != id.Seq {
		t.Fatalf("bo's seen = %v, want ada.c1 at %d", e.Seen, id.Seq)
	}
	if ada.Concurrent(id, bid) {
		t.Fatalf("bo saw ada's event; not concurrent")
	}
}

func TestRevokeConcurrentThenReissue(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	jon := w.join("jon", "c1")
	appendOne(t, jon, "note", map[string]int{"n": 2})
	appendOne(t, jon, "note", map[string]int{"n": 3})
	syncOK(t, jon)
	fetchOK(t, ada)

	oldKey := jon.pubKey
	e4 := appendOne(t, jon, "note", map[string]int{"n": 4}) // concurrent with the revocation
	rev := appendOne(t, ada, TypeRevokeKey, KeyPayload{Member: "jon", Key: oldKey})
	syncOK(t, ada)
	syncOK(t, jon) // closes e4 with its old seen
	fetchOK(t, ada)
	wantVerdict(t, ada, rev, Valid)
	e := wantVerdict(t, ada, e4, Ignored)
	t.Logf("ignored: %s", e.Reason)
	if !jon.Pending().KeyNotRegistered {
		t.Fatalf("jon's key should read as not registered")
	}

	// jon copies a new key in and an admin adds it.
	o := w.optsWithKey("jon", "c1", t.TempDir())
	jon2, err := Open(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	appendOne(t, ada, TypeAddKey, KeyPayload{Member: "jon", Key: jon2.pubKey})
	syncOK(t, ada)
	rep := syncOK(t, jon2)
	if rep.Reissued != 1 {
		t.Fatalf("reissued = %d, want 1", rep.Reissued)
	}
	fetchOK(t, ada)
	var found bool
	for _, ev := range eventsByID(ada) {
		if ev.ID.Member == "jon" && ev.ID.Seq == 5 {
			found = true
			if ev.Verdict != Valid || string(ev.Payload) != `{"n":4}` {
				t.Fatalf("reissued event = %+v", ev)
			}
		}
	}
	if !found {
		t.Fatalf("reissued event not found")
	}
	wantVerdict(t, ada, e4, Ignored)
}

// TestUnregisteredKeyKeepsWritesInJournal: a clone whose key the identity
// state does not hold commits nothing and re-issues nothing. An append and
// a sync leave its ref where it was and the event in the journal; once an
// admin adds the key, the next sync closes the batch, re-issues the event
// the revocation cut, and the commit verifies.
func TestUnregisteredKeyKeepsWritesInJournal(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	jon := w.join("jon", "c1")
	appendOne(t, jon, "note", map[string]int{"n": 2})
	syncOK(t, jon)
	fetchOK(t, ada)
	cut := appendOne(t, jon, "note", map[string]int{"n": 3}) // concurrent with the revocation
	appendOne(t, ada, TypeRevokeKey, KeyPayload{Member: "jon", Key: jon.pubKey})
	syncOK(t, ada)
	// jon's sync closes the cut event with its old seen; its key is now
	// revoked, so it re-issues nothing.
	if rep := syncOK(t, jon); rep.Reissued != 0 || rep.Pending.Uncommitted != 0 {
		t.Fatalf("sync under a revoked key = %+v, want nothing re-issued", rep)
	}
	wantVerdict(t, jon, cut, Ignored)

	// jon copies in a new key no admin has added yet.
	jon2, err := Open(context.Background(), w.optsWithKey("jon", "c1", t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !jon2.Pending().KeyNotRegistered {
		t.Fatal("the new key should read as not registered")
	}
	ref := memberRef("jon", "c1")
	tip := refTip(t, jon2, ref)
	id := appendOne(t, jon2, "note", map[string]int{"n": 4})
	rep := syncOK(t, jon2)
	if rep.Closed || rep.Reissued != 0 {
		t.Fatalf("sync under an unregistered key = %+v, want no batch closed and nothing re-issued", rep)
	}
	if got := refTip(t, jon2, ref); got != tip {
		t.Fatalf("a commit landed under an unregistered key: %s -> %s", tip, got)
	}
	if got := jon2.Pending().Uncommitted; got != 1 {
		t.Fatalf("uncommitted = %d, want the event kept in the journal", got)
	}
	if e := eventsByID(jon2)[id]; !e.Pending {
		t.Fatalf("event %s should still be pending: %+v", id, e)
	}
	fetchOK(t, ada)
	if _, ok := eventsByID(ada)[id]; ok {
		t.Fatalf("ada received %s, which was never committed", id)
	}

	appendOne(t, ada, TypeAddKey, KeyPayload{Member: "jon", Key: jon2.pubKey})
	syncOK(t, ada)
	if rep := syncOK(t, jon2); !rep.Closed || rep.Reissued != 1 {
		t.Fatalf("sync after the key was added = %+v, want the batch closed and the cut event re-issued", rep)
	}
	if got := refTip(t, jon2, ref); got == tip {
		t.Fatal("no commit landed after the key was added")
	}
	if got := jon2.Pending().Uncommitted; got != 0 {
		t.Fatalf("uncommitted = %d after the batch closed, want 0", got)
	}
	fetchOK(t, ada)
	if e := wantVerdict(t, ada, id, Valid); e.Pending {
		t.Fatalf("event %s still pending after the batch closed", id)
	}
}
