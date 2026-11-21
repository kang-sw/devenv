package mcp

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardRegistryIdleReopen(t *testing.T) {
	f := newWhiteboardFixture(t)
	stores := f.s.sessions.boards
	stores.idle = 25 * time.Millisecond
	f.update(t, `<wb:topic name="recovered"><wb:log>persisted through idle</wb:log></wb:topic>`)
	var original *wswhiteboard.BoardDB
	if err := stores.use(f.root, f.key, nil, true, func(s *whiteboardSlot) error {
		original = s.board
		return nil
	}); err != nil || original == nil {
		t.Fatalf("first board: %p %v", original, err)
	}

	waitFor(t, "the held board to close", func() bool {
		stores.mu.Lock()
		defer stores.mu.Unlock()
		return len(stores.slots) == 0
	})
	if err := stores.use(f.root, f.key, nil, true, func(s *whiteboardSlot) error {
		if s.board == original || s.board == nil {
			t.Fatalf("idle board did not reopen: %p, original %p", s.board, original)
		}
		state := s.board.State()
		if len(state.Topics) != 1 || state.Topics[0].Name != "recovered" {
			t.Fatalf("reopened state = %+v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardRegistryIndexReadDoesNotRenewIdleBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="indexed"><wb:log>retained</wb:log></wb:topic>`)
	stores := f.s.sessions.boards
	dir := whiteboardBoardDir(f.root, f.key)
	deadline := func() time.Time {
		stores.mu.Lock()
		defer stores.mu.Unlock()
		return stores.slots[dir].due
	}
	before := deadline()
	src := NewDashboardSessionSource(stores)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	if data, ok, err := src.ReadSession(reg); err != nil || !ok || !data.HasBoard {
		t.Fatalf("index read lost held board: found=%v, %v", ok, err)
	}
	if after := deadline(); !after.Equal(before) {
		t.Fatalf("index read moved idle deadline from %v to %v", before, after)
	}
	if page, ok, err := src.ReadPage(reg); err != nil || !ok || !page.HasBoard {
		t.Fatalf("page read lost held board: found=%v, %v", ok, err)
	}
	if after := deadline(); !after.After(before) {
		t.Fatalf("page read did not renew idle deadline: %v", after)
	}
}

func TestWhiteboardRegistryDrainHandsOffHeldBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="handoff"><wb:log>previous owner</wb:log></wb:topic>`)
	prior := f.s.sessions.boards
	next := NewWhiteboardStores(nil)
	defer next.Close()

	open := func(stores *WhiteboardStores) error {
		return stores.use(f.root, f.key, nil, true, func(s *whiteboardSlot) error {
			if s.board == nil || s.board.State().Topics[0].Name != "handoff" {
				t.Error("successor received the wrong board")
			}
			return nil
		})
	}
	if err := open(next); err == nil || !strings.Contains(err.Error(), "busy after "+wswhiteboard.BoardDBOpenRetryBound.String()) {
		t.Fatalf("unreleased owner: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- open(next) }()
	select {
	case err := <-done:
		t.Fatalf("opened before the previous owner drained: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	if err := prior.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(wswhiteboard.BoardDBOpenRetryBound):
		t.Fatal("successor timed out after the owner closed its board")
	}
	if err := open(prior); !errors.Is(err, errWhiteboardDraining) {
		t.Fatalf("drained owner reopened board: %v", err)
	}
}

func TestWhiteboardSummaryRequiresOwningDaemon(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="recovery"><wb:log>Preserve this topic summary</wb:log></wb:topic>`)
	f.call(t, "todo.add", map[string]any{"key": "todo", "title": "Keep session todos"})
	raw := NewServer(f.root, "raw")
	defer raw.Close()

	for _, tool := range []string{"workflow_manual", "workflow_state"} {
		args := map[string]any{"session_key": f.key}
		response := callToolOnce(t, raw, 1, tool, args)
		text := toolText(t, response)
		if isToolError(response) || !strings.Contains(text, "Keep session todos") || strings.Count(text, "Open-board summaries require the ws dashboard daemon, which owns the board.") != 1 || strings.Contains(text, "Preserve this topic summary") {
			t.Fatalf("raw %s gave unexpected session state: %s", tool, text)
		}
		response = f.call(t, tool, args)
		text = toolText(t, response)
		if isToolError(response) || !strings.Contains(text, "Keep session todos") || !strings.Contains(text, "Preserve this topic summary") || strings.Contains(text, "Open-board summaries require") {
			t.Fatalf("daemon %s gave unexpected session state: %s", tool, text)
		}
	}
	render := HostedRender{Harness: "claude", Boards: f.s.sessions.boards}
	bundle, err := RenderHostedRestoreBundle(os.Environ(), f.root, render, f.key, "restart", nil)
	if err != nil || !strings.Contains(bundle, "Preserve this topic summary") || strings.Contains(bundle, "Open-board summaries require") {
		t.Fatalf("hosted restart bundle lost board summaries: %v\n%s", err, bundle)
	}
}
