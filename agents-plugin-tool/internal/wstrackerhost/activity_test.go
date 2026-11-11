package wstrackerhost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// TestReadActivityReadsSinceTheCursor: a read from the zero cursor chooses
// through want and derives what each chosen event did; a read from the
// cursor it returned holds only what arrived since, here bo's comment
// synced from the other clone.
func TestReadActivityReadsSinceTheCursor(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	syncStore(t, b, "team")
	syncStore(t, a, "team")
	item := createItem(t, a, "team", "ship the caller")
	assign, _ := json.Marshal(map[string]any{"item": item, "targets": []any{"bo"}})
	if _, err := a.Dispatch(context.Background(), "team", []wsledger.NewEvent{{Type: wstracker.TypeItemAssign, Payload: assign}}); err != nil {
		t.Fatal(err)
	}
	// pushed first, so the second read sees no pending state change of these
	syncStore(t, a, "team")

	ctx := context.Background()
	var asked int
	rd, err := a.ReadActivity(ctx, "team", wsledger.Cursor{}, func(id wsledger.EventID, ts time.Time, catchUp bool) bool {
		if !catchUp {
			t.Error("a read from the zero cursor is a catch-up")
		}
		asked++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if rd.Entry.Member != "ada" || rd.View == nil || rd.View.Items[item].Title != "ship the caller" {
		t.Fatalf("read entry %+v, view item %+v", rd.Entry, rd.View.Items[item])
	}
	var pending []wstracker.Activity
	for _, e := range rd.Events {
		if !e.Chosen || !e.Settled {
			t.Fatalf("%s not settled", e.ID)
		}
		pending = append(pending, e.Activity...)
	}
	if asked != len(rd.Events) || len(pending) != 1 || pending[0].Kind != wstracker.ActAssignPending || pending[0].Target != "bo" {
		t.Fatalf("asked %d of %d events; activity %+v", asked, len(rd.Events), pending)
	}

	syncStore(t, b, "team")
	body, _ := json.Marshal(map[string]any{"item": item, "body": "on it"})
	if _, err := b.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeCommentAdd, Payload: body}}); err != nil {
		t.Fatal(err)
	}
	syncStore(t, b, "team")
	syncStore(t, a, "team")
	next, err := a.ReadActivity(ctx, "team", rd.Cursor, func(_ wsledger.EventID, _ time.Time, catchUp bool) bool { return !catchUp })
	if err != nil {
		t.Fatal(err)
	}
	if next.CatchUp {
		t.Fatal("a read from the cursor of the same ledger is no catch-up")
	}
	var kinds []string
	for _, e := range next.Events {
		for _, act := range e.Activity {
			kinds = append(kinds, act.Kind+" by "+act.Actor)
		}
	}
	if len(kinds) != 1 || kinds[0] != "comment by bo" {
		t.Fatalf("since the cursor: %v (%d events)", kinds, len(next.Events))
	}
}
