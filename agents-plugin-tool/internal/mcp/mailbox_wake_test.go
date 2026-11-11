package mcp

import (
	"net/http"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

func requireHardWake(t *testing.T, sub *wswake.Sub) {
	t.Helper()
	select {
	case <-sub.Hard():
	default:
		t.Fatal("the send did not wake this process's mailbox waits")
	}
}

// TestDirectMailboxSendNudgesFromAnUnregisteredSender pins the send-side
// nudge on the direct path: a sender whose dashboard is off (so never
// registered) still nudges the daemon, naming its own reply-id, so a
// mailbox wait the daemon hosts rechecks; the daemon's 404 for the unknown
// session changes nothing for the send.
func TestDirectMailboxSendNudgesFromAnUnregisteredSender(t *testing.T) {
	f := newLiveFixture(t, false)
	fake := startFakeDaemon(t, f.stateDir, http.StatusNotFound, false)
	sub := wswake.Mailbox.Subscribe()
	defer sub.Close()

	mustSend(t, f.server, 3, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello")
	requireHardWake(t, sub)
	waitForNudge(t, fake, f.replyA)
	for _, n := range fake.received() {
		if n.ReplyID == f.replyA && n.Project == "" {
			t.Fatalf("nudge without a project: %+v", n)
		}
	}
	if _, ok := readRegistration(t, f.stateDir, f.keyA); ok {
		t.Fatal("the dashboard-off sender was registered")
	}
}

// TestHostedMailboxSendPostsNoNudge pins the hosted path: the write is in
// this process, so the in-process wake covers its waits and no nudge goes
// out.
func TestHostedMailboxSendPostsNoNudge(t *testing.T) {
	f := newLiveFixture(t, false)
	f.server.live.host = &fakeLiveHost{serve: true}
	fake := startFakeDaemon(t, f.stateDir, http.StatusNoContent, false)
	sub := wswake.Mailbox.Subscribe()
	defer sub.Close()

	mustSend(t, f.server, 3, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello")
	requireHardWake(t, sub)
	time.Sleep(150 * time.Millisecond)
	if got := fake.received(); len(got) != 0 {
		t.Fatalf("a hosted send posted nudges: %+v", got)
	}
}
