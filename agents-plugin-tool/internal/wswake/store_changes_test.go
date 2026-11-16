package wswake

import (
	"reflect"
	"testing"
)

func TestStoreChangesAreOptInCoalescedAndLevelTriggered(t *testing.T) {
	h := &Hub{}
	ordinary := h.Subscribe()
	defer ordinary.Close()
	sub := h.SubscribeStores()
	defer sub.Close()
	for range 20 {
		h.StoreChanged("b")
		h.StoreChanged("a")
	}
	if ordinary.paths != nil || ordinary.Stores() != nil {
		t.Fatal("ordinary wait retained store history")
	}
	select {
	case <-ordinary.Hard():
		t.Fatal("store maintenance publication changed ordinary hard-wake policy")
	default:
	}
	select {
	case <-sub.Stores():
	default:
		t.Fatal("missing store wake")
	}
	if got := sub.TakeStores(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("coalesced stores = %v", got)
	}
	if len(sub.TakeStores()) != 0 {
		t.Fatal("store set was not drained")
	}
	h.StoreChanged("c")
	if got := sub.TakeStores(); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("subsequent publication lost: %v", got)
	}
	sub.Close()
	h.StoreChanged("d")
	if len(sub.TakeStores()) != 0 {
		t.Fatal("closed subscription received a store change")
	}
}
