package wstracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

func publishedStatusFixture(t *testing.T) (*Tracker, *memSource, wsledger.Event, *View) {
	t.Helper()
	g := json.RawMessage(`{"admins":["a"],"members":[{"id":"a","keys":["key"]}]}`)
	parent := incrementalEvent("a", 1, TypeItemCreate, `{"kind":"task","title":"parent"}`, 1, nil)
	child := incrementalEvent("a", 2, TypeItemCreate, `{"kind":"task","title":"child","parent":"a.pc#1","due":"2026-12-01","start":"2026-11-01"}`, 2, nil)
	status := incrementalEvent("a", 3, TypeItemStatus, `{"item":"a.pc#2","status":"doing"}`, 3, nil)
	status.Origin = OriginHuman
	src := newMemSource(g, "a.pc", []wsledger.Event{parent, child, status})
	tr := New(src, src.stream)
	v := tr.View(time.Unix(10, 0), 0)
	if n := tr.r.byID[status.ID.String()]; n.status != "applied" {
		t.Fatalf("status fixture not applied: %s", n.reason)
	}
	var found bool
	for _, a := range v.Assignments {
		if a.Item == child.ID.String() && a.StatusOrigin != nil && *a.StatusOrigin == OriginHuman {
			found = true
		}
	}
	if !found {
		t.Fatal("fixture has no status origin")
	}
	return tr, src, status, v
}

func TestIncrementalPublishedViewOwnsFoldData(t *testing.T) {
	tr, src, status, published := publishedStatusFixture(t)
	before, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	// Pending-batch close replaces the existing status node, including origin.
	status.Origin = OriginAgent
	src.add(status)
	tr.Refresh()
	if got, err := json.Marshal(published); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("Refresh changed a published snapshot: %v\n%s\nwant %s", err, got, before)
	}
	// All borrowed pointer/slice fields must be detached at publication, not
	// merely the origin that currently gets rewritten by incremental refresh.
	it := tr.r.S.items["a.pc#2"]
	if it.due == nil || it.start == nil || it.parent == "" {
		t.Fatal("fixture needs dates and parent")
	}
	*it.due, *it.start, it.parent = "2099-12-01", "2099-11-01", "changed"
	tr.r.byID[status.ID.String()].ev.Payload[0] = '['
	if got, err := json.Marshal(published); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("published view aliases mutable fold data: %v\n%s\nwant %s", err, got, before)
	}
}

func TestIncrementalPublishedViewEncodeDuringStatusReplacement(t *testing.T) {
	tr, src, status, published := publishedStatusFixture(t)
	before, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 1000 {
			got, err := json.Marshal(published)
			if err != nil || !bytes.Equal(got, before) {
				errs <- fmt.Errorf("old view changed while Refresh ran: %v", err)
				return
			}
		}
	}()
	close(start)
	for i := range 1000 {
		if i%2 == 0 {
			status.Origin = OriginAgent
		} else {
			status.Origin = OriginHuman
		}
		src.add(status)
		tr.Refresh()
	}
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
}
