package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
)

func TestMailboxSweepThresholdTune(t *testing.T) {
	paths := wsmailbox.Paths{Config: wsconfig.Options{ConfigHome: t.TempDir()}, State: wsstate.Options{CacheHome: t.TempDir()}}
	reply, _ := paths.ReplyRegistry()
	sent := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	if err := wsmailbox.WithReplyLock(reply, func(s *wsmailbox.ReplyStore) error {
		s.Queues = wsmailbox.AppendQueue(s.Queues, "recipient", wsmailbox.Envelope{ReplyTo: "id:sender", Content: "hello", SentAt: sent.Format(time.RFC3339)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r := wsconfig.NewResolver(paths.Config, nil, nil, nil)
	tune := func(raw string) {
		t.Helper()
		if err := r.Set(mcp.DashboardMailboxUndeliveredKey, raw, wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
			t.Fatal(err)
		}
	}
	if got := mailboxUndeliveredThreshold(paths.Config); got != 3*time.Minute {
		t.Fatalf("default = %v", got)
	}
	if err := r.Set(mcp.DashboardMailboxUndeliveredKey, "1", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeProject}); err == nil {
		t.Fatal("project-scope tune accepted")
	}
	tune("10")
	if posted, err := paths.SweepUndelivered(sent.Add(4*time.Minute), mailboxUndeliveredThreshold(paths.Config)); err != nil || posted != 0 {
		t.Fatalf("threshold 10: %d, %v", posted, err)
	}
	tune("2")
	if posted, err := paths.SweepUndelivered(sent.Add(4*time.Minute), mailboxUndeliveredThreshold(paths.Config)); err != nil || posted != 1 {
		t.Fatalf("threshold 2: %d, %v", posted, err)
	}
	for _, raw := range []string{"0", "-1", "1.5", "invalid", "9223372036854775807"} {
		tune(raw) // raw store writes stand in for hand-edited invalid config.
		if got := mailboxUndeliveredThreshold(paths.Config); got != 3*time.Minute {
			t.Fatalf("invalid %q = %v", raw, got)
		}
	}
}

func TestMailboxSweepStartsWithoutDashboardSessionsAndWakes(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	paths := wsmailbox.Paths{}
	reply, _ := paths.ReplyRegistry()
	if err := wsmailbox.WithReplyLock(reply, func(s *wsmailbox.ReplyStore) error {
		s.Queues = wsmailbox.AppendQueue(s.Queues, "recipient", wsmailbox.Envelope{ReplyTo: "id:sender", Content: "hello", SentAt: time.Now().Add(-4 * time.Minute).UTC().Format(time.RFC3339)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wake := wswake.Mailbox.Subscribe()
	defer wake.Close()
	stop := startMailboxSweep(io.Discard)
	defer stop()
	select {
	case <-wake.Hard():
	case <-time.After(5 * time.Second):
		t.Fatal("sweep did not wake waits")
	}
	s, err := wsmailbox.LoadReplyStore(reply)
	if err != nil || len(s.Queues["sender"]) != 1 {
		t.Fatalf("system notice missing: %+v, %v", s, err)
	}
	// No listed dashboard registration was needed.
	cache, _ := wsstate.CacheRoot(wsstate.Options{})
	if _, err := os.Stat(filepath.Join(cache, "dashboard", "sessions")); !os.IsNotExist(err) {
		t.Fatalf("unexpected dashboard sessions: %v", err)
	}
}
