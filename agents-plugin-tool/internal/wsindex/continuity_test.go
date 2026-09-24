package wsindex

import (
	"strings"
	"testing"
)

// Scenario IDs in test names (TestI18I22..., TestI21..., and so on) come from
// ticket 260924-feat-origin-ticket-ownership-index, whose Results map each ID
// to its test. The IDs are the traceability key; keep them.

// I21: a remote tip older than the shared cache (a sibling's push landed in
// between) is a stale read: nothing is discarded and the cache stays.
func TestI21StaleReadKeepsCacheAndPending(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	h.initIndex(x)
	cl := x.client()
	h.submit(cl, "260924-feat-t5")
	t5 := h.remoteTip()
	h.submit(cl, "260924-feat-t6")
	t6 := h.remoteTip()
	if _, err := cl.RecordPending(bg, registerEntry("260924-feat-pending")); err != nil {
		t.Fatal(err)
	}
	tip, reports, err := cl.reconcile(bg, t5)
	if err != nil || tip != t6 || len(reports) != 0 {
		t.Fatalf("reconcile(T5) = %s, %v, %v; want the T6 cache kept with no report", tip, reports, err)
	}
	if pending, _ := cl.Pending(bg); len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
}

// I18 and I22: a deleted remote ref discards the pending log (one report,
// only when entries existed) and the cache; the clone is index-absent after.
func TestI18I22RemoteDeletionDiscards(t *testing.T) {
	for _, withPending := range []bool{true, false} {
		h := newHarness(t)
		x := h.clone("x", "x@example.com")
		h.initIndex(x)
		cl := x.client()
		if withPending {
			for _, s := range []string{"260924-feat-p1", "260924-feat-p2"} {
				if _, err := cl.RecordPending(bg, registerEntry(s)); err != nil {
					t.Fatal(err)
				}
			}
		}
		gitT(t, h.origin, "update-ref", "-d", RemoteRef)
		res := h.submit(cl, "260924-feat-after")
		if res.Status != WriteAbsent {
			t.Fatalf("status = %s, want absent", res.Status)
		}
		switch {
		case withPending && (len(res.Reports) != 1 || !strings.HasPrefix(res.Reports[0], "2 offline entries were discarded")):
			t.Fatalf("reports = %v, want one discard report for 2 entries", res.Reports)
		case !withPending && len(res.Reports) != 0:
			t.Fatalf("a cache-only discard must be silent, got %v", res.Reports)
		}
		if seen, _ := cl.Seen(bg); seen {
			t.Fatal("cache ref survived the remote deletion")
		}
		if pending, _ := cl.Pending(bg); len(pending) != 0 {
			t.Fatal("pending log survived the remote deletion")
		}
		if h.remoteTip() != "" {
			t.Fatal("an index-absent write recreated the ref")
		}
	}
}

// I19: a re-initialized remote (unrelated history) discards the pending log
// with one report and adopts the new index; the discarded entries never reach
// it, while the reconnecting write itself lands.
func TestI19ReinitDiscardsAndAdopts(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	y := h.clone("y", "y@example.com")
	h.initIndex(x)
	cx := x.client()
	if _, err := cx.RecordPending(bg, registerEntry("260924-feat-discarded")); err != nil {
		t.Fatal(err)
	}
	gitT(t, h.origin, "update-ref", "-d", RemoteRef)
	h.clock.Advance(DefaultAbsenceTTL * 2)
	h.initIndex(y)
	res := h.submit(cx, "260924-feat-fresh")
	if res.Status != WriteWritten || len(res.Reports) != 1 || !strings.Contains(res.Reports[0], "discarded") {
		t.Fatalf("result = %+v", res)
	}
	idx := h.remoteIndex()
	if _, ok := idx.Registrations["260924-feat-discarded"]; ok {
		t.Fatal("a discarded pending entry reached the new index")
	}
	if _, ok := idx.Registrations["260924-feat-fresh"]; !ok {
		t.Fatal("the reconnecting write did not land")
	}
}

// I20: an unreachable remote never discards; pending entries survive
// repeated offline calls.
func TestI20OfflineNeverDiscards(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	h.initIndex(x)
	x.setOriginURL(unreachableURL)
	cl := x.client()
	for i, s := range []string{"260924-feat-o1", "260924-feat-o2", "260924-feat-o3"} {
		res := h.submit(cl, s)
		if res.Status != WritePending || len(res.Reports) != 0 {
			t.Fatalf("offline submit = %+v", res)
		}
		if pending, _ := cl.Pending(bg); len(pending) != i+1 {
			t.Fatalf("pending = %d, want %d", len(pending), i+1)
		}
		if view, _ := cl.Read(bg); view.State != ViewStale && view.State != ViewFresh {
			t.Fatalf("offline read state = %s", view.State)
		}
	}
	if seen, _ := cl.Seen(bg); !seen {
		t.Fatal("offline calls dropped the cache ref")
	}
}
