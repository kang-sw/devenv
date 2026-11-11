package mcp

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestFormatMailBatchMatchesTheClaudeAdapterPrompt pins the text a hosted
// session reads for its mail to the prompt the Claude adapter's mailbox wait
// submits. The one-message literal is the same one the adapter's own suite
// pins for its formatBatch (watcher.test.ts, "the batch prompt, as the
// daemon reproduces it for a session it hosts"); change the two together.
func TestFormatMailBatchMatchesTheClaudeAdapterPrompt(t *testing.T) {
	one := wsmailbox.Envelope{
		ReplyTo: "id:abc123",
		Content: "[ws note: from a peer lead session (claude, not a worker), with the same authority as yours]\nplease review the branch",
		SentAt:  "2026-01-02T03:04:05Z",
	}
	want := "[ws mailbox] 1 message(s) arrived for session_key lead-key. They were already received through mailbox.recv; do not call it again for these.\n\n" +
		"mail from id:abc123 (2026-01-02T03:04:05Z):\n" +
		"[ws note: from a peer lead session (claude, not a worker), with the same authority as yours]\nplease review the branch"
	if got := strings.Join(formatMailBatch("ws mailbox", "mailbox.recv", "lead-key", []wsmailbox.Envelope{one}), "\n\n"); got != want {
		t.Fatalf("one-message batch:\n got %q\nwant %q", got, want)
	}

	// One text per message, and joined they read as one batch under one
	// head: a named sender wins over the reply handle, and a message with
	// neither a sender nor a stamp still reads.
	two := []wsmailbox.Envelope{
		{From: " alice@machine ", ReplyTo: "id:ignored", Content: "first", SentAt: "2026-01-02T03:04:05Z"},
		{Content: "second"},
	}
	texts := formatMailBatch("ws mailbox", "mailbox.recv", "lead-key", two)
	if len(texts) != 2 {
		t.Fatalf("one text per message, got %d: %q", len(texts), texts)
	}
	want = "[ws mailbox] 2 message(s) arrived for session_key lead-key. They were already received through mailbox.recv; do not call it again for these.\n\n" +
		"mail from alice@machine (2026-01-02T03:04:05Z):\nfirst\n\n" +
		"mail from unknown:\nsecond"
	if got := strings.Join(texts, "\n\n"); got != want {
		t.Fatalf("two-message batch:\n got %q\nwant %q", got, want)
	}
	if got := formatMailBatch("ws mailbox", "mailbox.recv", "lead-key", nil); len(got) != 0 {
		t.Fatalf("an empty batch has no text, got %q", got)
	}
}

// TestLeadMailboxDeliversClaimedMailOnce: mail sent to a hosted lead's key
// is signalled to its watch, claimed once with the text the session reads,
// and gone for every later reader; the claim shows in the session's mail
// history and conversation ring; the watch marks the session as listening
// and ends at stop.
func TestLeadMailboxDeliversClaimedMailOnce(t *testing.T) {
	setupMailboxTestEnv(t)
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	initGit(t, root)
	leadKey, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	leadReply := wsmailbox.ReplyID(leadKey)
	sender := NewServer(root, "test")
	senderKey := mailboxLogin(t, sender, 1, root)
	senderHandle := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(senderKey)

	mail := NewLeadMailbox(stateDir)
	signals := make(chan struct{}, 16)
	stop := mail.Watch(root, leadKey, func() {
		select {
		case signals <- struct{}{}:
		default:
		}
	})
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()
	signalled := func(within time.Duration) bool {
		select {
		case <-signals:
			return true
		case <-time.After(within):
			return false
		}
	}
	if signalled(300 * time.Millisecond) {
		t.Fatal("an empty mailbox was signalled")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		marker, ok, _ := wsmailbox.ReadListeningMarker(leadKey)
		if ok && marker.PID == os.Getpid() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a watched session reads as listening through this process's marker, got %+v (present %v)", marker, ok)
		}
		time.Sleep(10 * time.Millisecond)
	}

	mustSend(t, sender, 2, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "hello lead")
	if !signalled(5 * time.Second) {
		t.Fatal("mail in the lead's mailbox was not signalled")
	}
	texts, err := mail.Drain(root, leadKey)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("one message, one text: %q", texts)
	}
	head := "[ws mailbox] 1 message(s) arrived for session_key " + leadKey + ". They were already received through mailbox.recv; do not call it again for these.\n\nmail from " + senderHandle + " ("
	if !strings.HasPrefix(texts[0], head) || !strings.HasSuffix(texts[0], "\nhello lead") || !strings.Contains(texts[0], peerLeadNoteBody) {
		t.Fatalf("the text the session reads:\n%s", texts[0])
	}

	// Claimed: gone for a second drain and for the session's own recv.
	if again, err := mail.Drain(root, leadKey); err != nil || len(again) != 0 {
		t.Fatalf("a second drain returns nothing, got %q, %v", again, err)
	}
	if out := strings.TrimSpace(callToolWithKey(t, NewServer(root, "test"), 3, leadKey, "mailbox.recv", map[string]any{"format": "json"})); out != "null" && out != "[]" {
		t.Fatalf("the session's own recv finds nothing after the claim, got %s", out)
	}

	records, err := readMailboxHistory("", leadKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Direction != mailboxHistoryRecv || records[0].Counterparty != senderHandle || !strings.HasSuffix(records[0].Body, "hello lead") {
		t.Fatalf("the claim is one received history entry, got %+v", records)
	}
	ring, err := wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(stateDir, leadReply)))
	if err != nil {
		t.Fatal(err)
	}
	if len(ring) != 1 || ring[0].Kind != wslive.RingMail || ring[0].Direction != mailboxHistoryRecv || ring[0].Peer != senderHandle || ring[0].HistoryKey != records[0].Name {
		t.Fatalf("the claim is one mail line of the session's ring, got %+v", ring)
	}

	// The watch looks again after the claim: a second message is signalled
	// and delivered alone.
	mustSend(t, sender, 4, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "second")
	if !signalled(5 * time.Second) {
		t.Fatal("mail after a claim was not signalled")
	}
	if texts, err := mail.Drain(root, leadKey); err != nil || len(texts) != 1 || !strings.HasSuffix(texts[0], "\nsecond") {
		t.Fatalf("the second message alone, got %q, %v", texts, err)
	}

	stop()
	stop()
	stopped = true
	for len(signals) > 0 {
		<-signals
	}
	if _, ok, _ := wsmailbox.ReadListeningMarker(leadKey); ok {
		t.Fatal("stop clears the listening marker")
	}
	mustSend(t, sender, 5, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "after stop")
	if signalled(300 * time.Millisecond) {
		t.Fatal("a stopped watch signalled")
	}

	if _, err := mail.Drain(root, "no-such-key"); err == nil {
		t.Fatal("an unknown key is an error")
	}
}
