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
