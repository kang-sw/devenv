package mcp

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// TestDashboardReadTopicHistory pins the served board's lazy history: the
// board leaves an earlier panel version's body, the inert gap rows and the
// capped closed topics out, and ReadTopicHistory renders them in full for
// the topics named, or for the one topic holding an entry, panel, or item
// target. No fragment carries the session key.
func TestDashboardReadTopicHistory(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	if _, ok := src.ReadTopicHistory(reg, "", []string{"x"}, ""); ok {
		t.Fatal("ReadTopicHistory read a session without a board")
	}
	f.update(t, `<wb:topic name="hist"><wb:panel recap="plan">first-version-marker</wb:panel><wb:log>older-log-marker</wb:log></wb:topic>`)
	f.update(t, `<wb:topic name="hist"><wb:panel id="p1" recap="plan">second-version-marker</wb:panel></wb:topic>`)
	f.update(t, `<wb:topic name="hist"><wb:log>latest-log-marker</wb:log></wb:topic>`)
	var cmds strings.Builder
	for i := 1; i <= 31; i++ {
		fmt.Fprintf(&cmds, `<wb:topic name="c%d"><wb:log>closed-marker-%d</wb:log><wb:close result="Ending %d."/></wb:topic>`, i, i, i)
	}
	f.update(t, cmds.String())

	board := string(readPageOK(t, src, reg).Board.Board)
	for _, gone := range []string{"first-version-marker", "older-log-marker", "closed-marker-1<"} {
		if strings.Contains(board, gone) {
			t.Errorf("the served board carries lazy history %q", gone)
		}
	}
	if !strings.Contains(board, "latest-log-marker") || !strings.Contains(board, `<template data-history-topic="c1" data-lazy></template>`) || !strings.Contains(board, `<div class="history-gap" data-lazy`) {
		t.Errorf("the served board lacks its live rows or lazy markers:\n%.1200s", board)
	}

	frags, ok := src.ReadTopicHistory(reg, "", []string{"hist", "c1"}, "")
	if !ok || len(frags) != 2 || frags[0].Name != "hist" || frags[1].Name != "c1" {
		t.Fatalf("ReadTopicHistory(names) = %+v, %t", frags, ok)
	}
	hist := string(frags[0].HTML)
	for _, want := range []string{"first-version-marker", "second-version-marker", "older-log-marker", "latest-log-marker"} {
		if !strings.Contains(hist, want) {
			t.Errorf("the complete topic lacks %q", want)
		}
	}
	if strings.Contains(hist, "data-lazy") || !strings.Contains(string(frags[1].HTML), "closed-marker-1<") {
		t.Error("a fragment must carry its complete history")
	}

	entry := regexp.MustCompile(`data-entry="(e[0-9]+)"[^>]*data-kind="agent"`).FindStringSubmatch(string(frags[1].HTML))
	if entry == nil {
		t.Fatal("the closed topic has no agent entry to target")
	}
	for _, target := range []string{entry[1], "p1"} {
		got, ok := src.ReadTopicHistory(reg, "", nil, target)
		want := map[string]string{entry[1]: "c1", "p1": "hist"}[target]
		if !ok || len(got) != 1 || got[0].Name != want {
			t.Errorf("ReadTopicHistory(target %s) = %+v, %t; want topic %s", target, got, ok, want)
		}
	}
	if got, ok := src.ReadTopicHistory(reg, "", nil, "e99999"); !ok || len(got) != 0 {
		t.Errorf("an unknown target = %+v, %t; want no fragment", got, ok)
	}
	for _, fr := range frags {
		if strings.Contains(string(fr.HTML), f.key) {
			t.Error("a topic fragment carries the session key")
		}
	}
}
