package wswake

import "testing"

func pending(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func TestHubDeliversEachKindToEverySubscriber(t *testing.T) {
	h := &Hub{}
	h.Wake() // no subscriber: nothing to deliver, nothing blocks
	a, b := h.Subscribe(), h.Subscribe()
	defer a.Close()

	h.Wake()
	h.Wake() // folds into the pending one
	if !pending(a.Hard()) || !pending(b.Hard()) {
		t.Fatal("a hard wake did not reach every subscriber")
	}
	if pending(a.Hard()) || pending(a.Nudge()) {
		t.Fatal("folded wakes left a second signal, or a hard wake reached the nudge channel")
	}

	b.Close()
	b.Close()
	h.Nudge()
	if !pending(a.Nudge()) || pending(a.Hard()) {
		t.Fatal("a nudge did not reach the nudge channel alone")
	}
	if pending(b.Nudge()) {
		t.Fatal("a closed subscriber still received a nudge")
	}
}
