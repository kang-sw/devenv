package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// cacheFiles is every regular file under dir by path, with its content's
// digest and its line count.
type cachedFile struct {
	sum   [sha256.Size]byte
	lines int
}

func cacheFiles(t *testing.T, dir string) map[string]cachedFile {
	t.Helper()
	out := map[string]cachedFile{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = cachedFile{sum: sha256.Sum256(data), lines: bytes.Count(data, []byte("\n"))}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// storeFiles is the legacy mailbox.json files under dir.
func storeFiles(t *testing.T, dir string) map[string]cachedFile {
	t.Helper()
	out := map[string]cachedFile{}
	for path, f := range cacheFiles(t, dir) {
		if filepath.Base(path) == "mailbox.json" {
			out[path] = f
		}
	}
	return out
}

// Mail to a daemon-owned child's address rides the memory-first mailbox
// store as any mail does: while the daemon runs, a send is one restore-ring
// line and touches no other file under the cache root, and no mailbox.json
// is written; the mail survives a daemon restart, and the claim reads it
// with its sender and relation, with recv and sent history and reply-id
// parity.
func TestDaemonOwnedMailRidesTheRestoreRing(t *testing.T) {
	setupMailboxTestEnv(t)
	if _, err := wsmailbox.OpenPersistent(wsmailbox.Paths{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wsmailbox.ClosePersistent() })
	cache, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	initGit(t, root)
	childKey, err := NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	childReply := wsmailbox.ReplyID(childKey)
	childAddr := wsmailbox.ReplyIDPrefix + childReply
	sender := NewServer(root, "test")
	senderKey := mailboxLogin(t, sender, 1, root)
	senderReply := wsmailbox.ReplyID(senderKey)
	senderAddr := wsmailbox.ReplyIDPrefix + senderReply

	// The first send registers the sender's reply-id; measure the next.
	mustSend(t, sender, 2, senderKey, childAddr, "warm-up")
	before := cacheFiles(t, cache)
	mustSend(t, sender, 3, senderKey, childAddr, "hello child")
	after := cacheFiles(t, cache)
	ringDir := filepath.Join(cache, wsmailbox.RingDirName) + string(filepath.Separator)
	var changed []string
	for path, now := range after {
		if was, ok := before[path]; !ok || was.sum != now.sum {
			changed = append(changed, path)
			if !strings.HasPrefix(path, ringDir) || now.lines != was.lines+1 {
				t.Errorf("the send wrote %s (%d lines, was %d): only one ring line may change", path, now.lines, was.lines)
			}
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("the send removed %s", path)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("the send changed %d files, want the ring segment alone: %v", len(changed), changed)
	}
	if got := storeFiles(t, cache); len(got) != 0 {
		t.Fatalf("a running daemon wrote mailbox.json: %v", got)
	}

	// A graceful daemon restart: the close exports the legacy files for a
	// downgrade, and the reopened daemon's traffic leaves them as they are.
	if err := wsmailbox.ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	if _, err := wsmailbox.OpenPersistent(wsmailbox.Paths{}); err != nil {
		t.Fatal(err)
	}
	exported := storeFiles(t, cache)

	mail := NewLeadMailbox(stateDir)
	claimed, err := mail.Drain(root, childKey, func(from string) string {
		if from == senderReply {
			return "parent"
		}
		return "peer"
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 || claimed[1].Sender != senderReply || !strings.HasSuffix(claimed[1].Text, "\nhello child") ||
		!strings.Contains(claimed[1].Text, "mail from "+senderAddr) || !strings.Contains(claimed[1].Text, "[relation: parent]") {
		t.Fatalf("the claimed mail after the restart = %+v", claimed)
	}

	recv, err := readMailboxHistory("", childKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 2 || recv[0].Direction != mailboxHistoryRecv || recv[0].Counterparty != senderAddr {
		t.Fatalf("the child's history = %+v", recv)
	}
	sent, err := readMailboxHistory("", senderKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0].Direction != mailboxHistorySent || sent[0].Counterparty != childAddr {
		t.Fatalf("the sender's history = %+v", sent)
	}

	// The child answers the reply-id the mail carried.
	mustSend(t, NewServer(root, "test"), 4, childKey, senderAddr, "reply from child")
	out := callToolWithKey(t, sender, 5, senderKey, "mailbox.recv", map[string]any{"format": "json"})
	var got []wsmailbox.Envelope
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("mailbox.recv = %s: %v", out, err)
	}
	if len(got) != 1 || got[0].Content != "reply from child" || (got[0].ReplyTo != childAddr && got[0].From != childAddr) {
		t.Fatalf("the reply the sender reads = %+v", got)
	}
	if now := storeFiles(t, cache); len(now) != len(exported) {
		t.Fatalf("mailbox.json files after the restart's traffic = %d, exported %d", len(now), len(exported))
	} else {
		for path, f := range now {
			if exported[path] != f {
				t.Errorf("the restarted daemon rewrote %s", path)
			}
		}
	}
}
