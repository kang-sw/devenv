package wsdashboard

import (
	"reflect"
	"testing"
)

func TestSplitBoardTopics(t *testing.T) {
	a := `<section class="topic" data-topic="a&amp;b" data-th="h1"><template><section>nested</section></template><input><div data-srcdoc="&lt;section&gt;"></div></section>`
	b := `<section class="topic closed" data-topic="b" data-th="h2"></section>`
	tail := `<template data-history-topic="older"><section class="topic" data-topic="older" data-th="h3"></section></template><div class="reply"><textarea></textarea></div>`
	got := splitBoardTopics("\n" + a + b + tail)
	if got == nil || !reflect.DeepEqual(got.order, []string{"a&b", "b"}) || got.topics["a&b"].HTML != a || got.topics["b"].Hash != "h2" {
		t.Fatalf("split = %+v", got)
	}
	// The shell excludes only direct topic sections, never deferred topics.
	if got.shell != "\n"+tail {
		t.Fatalf("shell = %q", got.shell)
	}
	for _, raw := range []string{
		`<section class="topic" data-topic="a"></section>`,
		a + a,
		a + `<div>interleaved</div>` + b,
		`<section class="topic" data-topic="a" data-th="h">`,
	} {
		if splitBoardTopics(raw) != nil {
			t.Errorf("accepted unsafe board %q", raw)
		}
	}
	if empty := splitBoardTopics(`<div class="reply"></div>`); empty == nil || len(empty.order) != 0 {
		t.Fatalf("empty board = %+v", empty)
	}
}
