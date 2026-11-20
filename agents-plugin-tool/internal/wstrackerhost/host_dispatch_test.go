// Event dispatch and batch reference tests.
package wstrackerhost

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// DispatchWith hands build the id the batch's first event gets, so a batch
// can name an item an earlier event of it creates; an error from build
// writes nothing.
func TestDispatchWithNamesEarlierEventsOfTheBatch(t *testing.T) {
	w := newWorld(t)
	h := w.machine("ada")
	w.register(h, "team", "ada", &wstracker.Profile{Given: "Ada"})
	createItem(t, h, "team", "first")

	ctx := context.Background()
	var first wsledger.EventID
	ids, err := h.DispatchWith(ctx, "team", func(next wsledger.EventID) ([]wsledger.NewEvent, error) {
		first = next
		create, _ := json.Marshal(map[string]any{"kind": "task", "title": "parent"})
		child, _ := json.Marshal(map[string]any{"kind": "task", "title": "child", "parent": next.String()})
		return []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: create}, {Type: wstracker.TypeItemCreate, Payload: child}}, nil
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(ids) != 2 || ids[0] != first || ids[1].Seq != first.Seq+1 {
		t.Fatalf("ids = %v, want %v and the next seq", ids, first)
	}
	v := view(t, h, "team")
	if p := v.Items[ids[1].String()].Parent; p == nil || *p != ids[0].String() {
		t.Fatalf("child's parent = %v, want %s", p, ids[0])
	}

	refused := errors.New("refused by build")
	if _, err := h.DispatchWith(ctx, "team", func(wsledger.EventID) ([]wsledger.NewEvent, error) { return nil, refused }); !errors.Is(err, refused) {
		t.Fatalf("a build error must come back as is: %v", err)
	}
	if got := len(view(t, h, "team").Items); got != 3 {
		t.Fatalf("a refused build must write nothing: %d items", got)
	}
}

// Another process on this machine (here a second Host over the same root)
// cannot append between a batch's reload and its append, so the ids the
// batch was built against are the ids it gets.
func TestDispatchWithHoldsTheWriteLockAcrossProcesses(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	b, err := New(Options{Root: filepath.Join(w.dir, "ada"), Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })

	ctx := context.Background()
	type result struct {
		ids []wsledger.EventID
		err error
	}
	other := make(chan result, 1)
	ids, err := a.DispatchWith(ctx, "team", func(next wsledger.EventID) ([]wsledger.NewEvent, error) {
		go func() {
			payload, _ := json.Marshal(map[string]any{"kind": "task", "title": "other process"})
			ids, err := b.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}})
			other <- result{ids, err}
		}()
		select {
		case r := <-other:
			t.Errorf("another process wrote inside the batch's write lock: %v %v", r.ids, r.err)
			other <- r
		case <-time.After(300 * time.Millisecond):
		}
		create, _ := json.Marshal(map[string]any{"kind": "task", "title": "parent"})
		child, _ := json.Marshal(map[string]any{"kind": "task", "title": "child", "parent": next.String()})
		return []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: create}, {Type: wstracker.TypeItemCreate, Payload: child}}, nil
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	r := <-other
	if r.err != nil {
		t.Fatalf("the other process's write must go through after the lock: %v", r.err)
	}
	v := view(t, a, "team")
	if p := v.Items[ids[1].String()].Parent; p == nil || *p != ids[0].String() || v.Items[ids[0].String()].Title != "parent" {
		t.Fatalf("the child must name the batch's own parent: parent=%v ids=%v", p, ids)
	}
	if r.ids[0].Seq != ids[1].Seq+1 {
		t.Fatalf("the other write must follow the batch: %v after %v", r.ids, ids)
	}
}

// batchEvent builds one NewEvent from a payload map.
func batchEvent(t *testing.T, typ string, payload map[string]any) wsledger.NewEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return wsledger.NewEvent{Type: typ, Payload: raw}
}

// TestDispatchBatchResolvesReferences: a "$1" in a later event names the
// item the first event creates, and the stored comment carries its id.
func TestDispatchBatchResolvesReferences(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	createItem(t, a, "team", "earlier") // so the batch's ids do not start at 1

	ids, err := a.DispatchBatch(context.Background(), "team", []wsledger.NewEvent{
		batchEvent(t, wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "batched"}),
		batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$1", "body": "first"}),
		batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$1", "reply_to": "$2", "body": "second"}),
	})
	if err != nil || len(ids) != 3 {
		t.Fatalf("DispatchBatch = %v, %v", ids, err)
	}
	v := view(t, a, "team")
	item := ids[0].String()
	if !hasItem(v, item) {
		t.Fatalf("item %s missing from the view", item)
	}
	first, second := v.Comments[ids[1].String()], v.Comments[ids[2].String()]
	if first.Item != item || second.Item != item {
		t.Fatalf("comments on %q and %q, want %q", first.Item, second.Item, item)
	}
	if second.ReplyTo == nil || *second.ReplyTo != ids[1].String() {
		t.Fatalf("reply_to = %v, want %q", second.ReplyTo, ids[1].String())
	}
}

// TestDispatchBatchRejectsBadReferences: a reference that names no earlier
// event of the batch is an error and nothing is written.
func TestDispatchBatchRejectsBadReferences(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	item := createItem(t, a, "team", "base")
	before := len(view(t, a, "team").Items)
	count := func() int {
		n := 0
		err := a.with(context.Background(), "team", func(st *store) error {
			evs, _ := st.ledger.Events(wsledger.Cursor{})
			n = len(evs)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	events := count()

	create := batchEvent(t, wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "x"})
	for name, batch := range map[string][]wsledger.NewEvent{
		"$0":       {create, batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$0", "body": "b"})},
		"forward":  {batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$2", "body": "b"}), create},
		"self":     {batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$1", "body": "b"})},
		"$x":       {create, batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$x", "body": "b"})},
		"negative": {create, batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$-1", "body": "b"})},
		"padded":   {create, batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": "$01", "body": "b"})},
		"reply_to": {create, batchEvent(t, wstracker.TypeCommentAdd, map[string]any{"item": item, "reply_to": "$5", "body": "b"})},
	} {
		_, err := a.DispatchBatch(context.Background(), "team", batch)
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if !strings.Contains(err.Error(), "event ") || !strings.Contains(err.Error(), "field") {
			t.Fatalf("%s: error %q must name the event and field", name, err)
		}
		if got := count(); got != events {
			t.Fatalf("%s: store holds %d events, want %d (nothing written)", name, got, events)
		}
	}
	if got := len(view(t, a, "team").Items); got != before {
		t.Fatalf("items = %d, want %d", got, before)
	}
}

// TestDispatchBatchPassesOtherPayloadsThrough: a non-object payload and
// unrelated "$" strings are left alone by the resolver.
func TestDispatchBatchPassesOtherPayloadsThrough(t *testing.T) {
	next := wsledger.EventID{Member: "ada", Clone: "pc", Seq: 5}
	for _, raw := range []string{`"$9"`, `[1]`, `null`, `{"title":"$9","item":3,"body":"$x"}`} {
		got, err := resolveBatchRefs(json.RawMessage(raw), 0, next)
		if err != nil || string(got) != raw {
			t.Fatalf("resolveBatchRefs(%s) = %s, %v; want it unchanged", raw, got, err)
		}
	}
}
