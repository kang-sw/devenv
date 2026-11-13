package wsmailbox

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func sweepTestPaths(t *testing.T) Paths {
	t.Helper()
	return Paths{Config: wsconfig.Options{ConfigHome: filepath.Join(t.TempDir(), "config")}, State: wsstate.Options{CacheHome: filepath.Join(t.TempDir(), "cache")}}
}

func TestSweepUndelivered(t *testing.T) {
	for _, scope := range []string{"reply", "machine", "worktree", "clone"} {
		for _, senderKind := range []string{"reply", "named", "machine"} {
			t.Run(scope+"/"+senderKind, func(t *testing.T) {
				p := sweepTestPaths(t)
				reply, _ := p.ReplyRegistry()
				machine, _ := p.Machine()
				source, recipient := reply, "id:recipient"
				switch scope {
				case "machine":
					source, recipient = machine, "recipient@machine"
				case "worktree":
					source, recipient = filepath.Join(p.State.CacheHome, "proj", "deadbeef", "mailbox.json"), "recipient@worktree"
				case "clone":
					source, recipient = filepath.Join(p.State.CacheHome, "proj", "deadbeef", "shared", "mailbox.json"), "recipient@clone"
				}
				sender, destination, destName := "id:sender", reply, "sender"
				if senderKind == "machine" || (senderKind == "named" && scope == "reply") {
					sender, destination = "sender@machine", machine
				} else if senderKind == "named" {
					sender, destination = "sender@"+scope, source
				}
				sent := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
				msg := Envelope{Content: "first line\nsecond line", SentAt: sent.Format(time.RFC3339)}
				if senderKind == "reply" {
					msg.ReplyTo = sender
				} else {
					msg.From = sender
				}
				writeSweepQueue(t, source, source == reply, "recipient", []Envelope{msg, {Content: "system", SentAt: msg.SentAt}})
				for _, age := range []time.Duration{2 * time.Minute, 3 * time.Minute, 20 * time.Minute} {
					posted, err := p.SweepUndelivered(sent.Add(age), 3*time.Minute)
					want := 0
					if age == 3*time.Minute {
						want = 1
					}
					if err != nil || posted != want {
						t.Fatalf("age %v: posted %d, err %v", age, posted, err)
					}
				}
				queue := readSweepQueue(t, source, source == reply, "recipient")
				if len(queue) != 2 || !queue[0].UndeliveredNotified || queue[1].UndeliveredNotified {
					t.Fatalf("original mail changed or system mail marked: %+v", queue)
				}
				notices := readSweepQueue(t, destination, destination == reply, destName)
				if len(notices) != 1 || notices[0].From != "" || notices[0].ReplyTo != "" {
					t.Fatalf("notices: %+v", notices)
				}
				text := notices[0].Content
				for _, part := range []string{recipient, msg.SentAt, "first line", "may still be read", "may have exited"} {
					if !strings.Contains(text, part) {
						t.Fatalf("notice missing %q: %s", part, text)
					}
				}
				if strings.Contains(text, "second line") {
					t.Fatalf("notice contains more than first line: %s", text)
				}
				// A late claim removes the original without a follow-up notice.
				writeSweepQueue(t, source, source == reply, "recipient", nil)
				if posted, err := p.SweepUndelivered(sent.Add(time.Hour), 3*time.Minute); err != nil || posted != 0 {
					t.Fatalf("late read: %d, %v", posted, err)
				}
			})
		}
	}
}

func TestSweepClaimBeforeThreshold(t *testing.T) {
	p := sweepTestPaths(t)
	reply, _ := p.ReplyRegistry()
	sent := time.Now().UTC()
	writeSweepQueue(t, reply, true, "recipient", []Envelope{{ReplyTo: "id:sender", Content: "read me", SentAt: sent.Format(time.RFC3339)}})
	if posted, err := p.SweepUndelivered(sent.Add(time.Minute), 3*time.Minute); err != nil || posted != 0 {
		t.Fatalf("early sweep: %d, %v", posted, err)
	}
	// A claim's durable unread signal is deletion from the locked queue.
	if err := WithReplyLock(reply, func(s *ReplyStore) error { delete(s.Queues, "recipient"); return nil }); err != nil {
		t.Fatal(err)
	}
	if posted, err := p.SweepUndelivered(sent.Add(5*time.Minute), 3*time.Minute); err != nil || posted != 0 {
		t.Fatalf("claimed mail: %d, %v", posted, err)
	}
	if queue := readSweepQueue(t, reply, true, "sender"); len(queue) != 0 {
		t.Fatalf("unexpected notice: %+v", queue)
	}
}

func TestSweepSelfMailAtQueueLimit(t *testing.T) {
	p := sweepTestPaths(t)
	reply, _ := p.ReplyRegistry()
	sent := time.Now().UTC().Add(-time.Hour)
	queue := make([]Envelope, MaxQueueLen)
	for i := range queue {
		queue[i] = Envelope{Content: "system", SentAt: sent.Format(time.RFC3339)}
	}
	queue[0].From = "id:self"
	writeSweepQueue(t, reply, true, "self", queue)
	if posted, err := p.SweepUndelivered(time.Now(), 3*time.Minute); err != nil || posted != 1 {
		t.Fatalf("self mail: %d, %v", posted, err)
	}
	if posted, err := p.SweepUndelivered(time.Now(), 3*time.Minute); err != nil || posted != 0 {
		t.Fatalf("self mail again: %d, %v", posted, err)
	}
}

func writeSweepQueue(t *testing.T, path string, reply bool, name string, queue []Envelope) {
	t.Helper()
	var err error
	if reply {
		err = WithReplyLock(path, func(s *ReplyStore) error { s.Queues[name] = queue; return nil })
	} else {
		err = WithLock(path, func(s *StoreFile) error { s.Queues[name] = queue; return nil })
	}
	if err != nil {
		t.Fatal(err)
	}
}

func readSweepQueue(t *testing.T, path string, reply bool, name string) []Envelope {
	t.Helper()
	if reply {
		s, err := LoadReplyStore(path)
		if err != nil {
			t.Fatal(err)
		}
		return s.Queues[name]
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s.Queues[name]
}
