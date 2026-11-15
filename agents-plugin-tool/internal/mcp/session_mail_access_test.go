package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestSessionAccessMailKeepsOneClaimOwnerAcrossEntryModes(t *testing.T) {
	for _, hostedFirst := range []bool{false, true} {
		name := "cli-first"
		if hostedFirst {
			name = "hosted-first"
		}
		t.Run(name, func(t *testing.T) {
			setupMailboxTestEnv(t)
			root := t.TempDir()
			initGit(t, root)
			key, err := NewAgentKeyStore().MintLead(root)
			if err != nil {
				t.Fatal(err)
			}
			dir, err := wsdashboard.Dir("")
			if err != nil {
				t.Fatal(err)
			}
			mail := NewLeadMailbox(dir)
			defer mail.server.Close()
			cli := NewServer(t.TempDir(), "test") // caller cwd is not queue authority
			defer cli.Close()
			path, err := cli.mailboxPaths().ReplyRegistry()
			if err != nil {
				t.Fatal(err)
			}
			if err := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
				for _, body := range []string{"first", "second"} {
					store.Queues = wsmailbox.AppendQueue(store.Queues, wsmailbox.ReplyID(key), wsmailbox.Envelope{Content: body, SentAt: "2026-01-02T03:04:05Z"})
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			cliRecv := func() string {
				return callToolWithKey(t, cli, 1, key, "mailbox.recv", map[string]any{"format": "json"})
			}
			if hostedFirst {
				texts, err := mail.Drain(root, key)
				if err != nil || len(texts) != 2 || !strings.HasSuffix(texts[0], "\nfirst") || !strings.HasSuffix(texts[1], "\nsecond") {
					t.Fatalf("hosted claim = %q, %v", texts, err)
				}
				if got := strings.TrimSpace(cliRecv()); got != "null" && got != "[]" {
					t.Fatalf("CLI duplicated hosted claim: %s", got)
				}
			} else {
				got := cliRecv()
				var envelopes []wsmailbox.Envelope
				if err := json.Unmarshal([]byte(got), &envelopes); err != nil || len(envelopes) != 2 || envelopes[0].Content != "first" || envelopes[1].Content != "second" {
					t.Fatalf("CLI claim = %s, %v", got, err)
				}
				if texts, err := mail.Drain(root, key); err != nil || len(texts) != 0 {
					t.Fatalf("hosted delivery duplicated CLI claim: %q, %v", texts, err)
				}
			}
			// Reopening access does not reopen mail or fork the history store.
			reopened := NewServer(t.TempDir(), "test")
			defer reopened.Close()
			history := callToolWithKey(t, reopened, 2, key, "mailbox.history", map[string]any{"format": "json", "body": true})
			var records []mailboxHistoryOut
			if err := json.Unmarshal([]byte(history), &records); err != nil || len(records) != 2 {
				t.Fatalf("reopened history = %s, %v", history, err)
			}
			bodies := map[string]bool{}
			for _, record := range records {
				if record.Direction != mailboxHistoryRecv || record.Body == nil {
					t.Fatalf("reopened history entry = %+v", record)
				}
				bodies[*record.Body] = true
			}
			if !bodies["first"] || !bodies["second"] {
				t.Fatalf("reopened history lost a message: %s", history)
			}
			if history := callToolWithKey(t, reopened, 3, "unknown-session", "mailbox.history", nil); !strings.Contains(history, "unknown_session") {
				t.Fatalf("unknown history key = %s", history)
			}
		})
	}
}
