package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestDashboardHydratedHistoryRetainsImageInvalidation(t *testing.T) {
	for _, route := range []string{"topic", "closed"} {
		t.Run(route, func(t *testing.T) {
			t.Setenv("WS_MCP_NO_DASHBOARD", "1")
			f := newWhiteboardFixture(t)
			src := dashboardPageSource(t, nil, f.s.sessions.boards)
			reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
			path := filepath.Join(f.root, "shots", "closed.png")
			mustWrite(t, f.root, "shots/closed.png", "first-image")
			f.update(t, `<wb:topic name="old"><wb:body>![shot](shots/closed.png) <img src="shots/missing.png"></wb:body><wb:close result="done"/></wb:topic>`)
			var commands strings.Builder
			for i := 0; i < wswhiteboard.ClosedCap; i++ {
				fmt.Fprintf(&commands, `<wb:topic name="new-%d"><wb:log>line</wb:log><wb:close result="done"/></wb:topic>`, i)
			}
			f.update(t, commands.String())
			initial := readPageOK(t, src, reg)
			if !slices.Contains(initial.Watch, path) {
				t.Error("lazy image must be watched before any history request")
			}
			if !strings.Contains(string(initial.Board.Board), `<template data-history-topic="old" data-lazy></template>`) {
				t.Fatal("fixture must put the image topic beyond the closed cap")
			}
			fetch := func() string {
				t.Helper()
				if route == "closed" {
					html, ok := src.ReadClosedTopics(reg, "", wswhiteboard.ClosedCap, 1)
					if !ok {
						t.Fatal("closed history not found")
					}
					return string(html)
				}
				fragments, ok := src.ReadTopicHistory(reg, "", []string{"old"}, "")
				if !ok || len(fragments) != 1 {
					t.Fatalf("history = %v, %t", fragments, ok)
				}
				return string(fragments[0].HTML)
			}
			old := fetch()
			page := readPageOK(t, src, reg)
			for _, want := range []string{path, filepath.Join(f.root, "shots", "missing.png")} {
				if !slices.Contains(page.Watch, want) {
					t.Errorf("hydrated image missing from page watch: %s", filepath.Base(want))
				}
			}
			if page.Board != initial.Board {
				t.Error("enrolling history watches must not resend an unchanged board")
			}
			// Memo hits and unrelated topic writes must not drop hydrated watches.
			readPageOK(t, src, reg)
			f.update(t, `<wb:topic name="other"><wb:body>unrelated</wb:body></wb:topic>`)
			for i := 0; i < 3; i++ {
				page = readPageOK(t, src, reg)
				if !slices.Contains(page.Watch, path) {
					t.Error("a later render dropped the hydrated image watch")
				}
			}
			before := page
			mustWrite(t, f.root, "shots/closed.png", "replacement-image-two")
			at := time.Now().Add(time.Second)
			if err := os.Chtimes(path, at, at); err != nil {
				t.Fatal(err)
			}
			waitForDashboardValidation(t)
			page = readPageOK(t, src, reg)
			if page.Board.Board == before.Board.Board {
				t.Error("an image edit must invalidate the lazy board, not just its memo")
			}
			if page.BoardUnits == nil || page.BoardUnits == before.BoardUnits {
				t.Error("image edit retained the old transport units")
			}
			if next := fetch(); next == old || !strings.Contains(next, "&amp;v=") {
				t.Error("restored history did not get a new image relay version")
			}
			stable := readPageOK(t, src, reg)
			if stable.Board != page.Board {
				t.Error("unchanged history image stamp must settle")
			}
			mustWrite(t, f.root, "shots/missing.png", "newly-created-image")
			waitForDashboardValidation(t)
			if next := readPageOK(t, src, reg); next.Board.Board == stable.Board.Board {
				t.Error("creating a missing hydrated image must invalidate the lazy board")
			}
			// Dependencies and the marker must be reproducible after daemon
			// replacement, even if the client reconnects without another fetch.
			current := readPageOK(t, src, reg)
			fresh := readPageOK(t, dashboardPageSource(t, nil, f.s.sessions.boards), reg)
			if !slices.Contains(fresh.Watch, path) || fresh.Board != current.Board {
				t.Error("replacement source lost image dependencies or changed the deterministic stamp")
			}
		})
	}
}
