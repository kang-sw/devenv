package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestMailboxRecvConcurrentCallsNeverDoubleDeliver pins the atomicity of
// mailbox.recv's drain: two recv calls racing on one session key, against
// both the session's own reply-id queue and its owned named inbox, must
// together return every seeded envelope exactly once. The drain is a
// load-modify-save under the store's flock, so a duplicate or a lost
// envelope here means that read-modify-write lost its atomicity.
func TestMailboxRecvConcurrentCallsNeverDoubleDeliver(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	receiver := NewServer(root, "test")
	receiverKey := mailboxLogin(t, receiver, 1, root)

	t.Setenv(envMailbox, "")
	sender := NewServer(root, "test")
	senderKey := mailboxLogin(t, sender, 1, root)

	// The receiver publishes its reply-id by sending once; that message is
	// not part of the seeded set.
	callToolWithKey(t, receiver, 2, receiverKey, "mailbox.send", map[string]any{
		"to": "bob@worktree", "content": "prime",
	})
	replyAddr := "id:" + wsmailbox.ReplyID(receiverKey)

	type result struct {
		contents []string
		err      error
	}
	// A receiver stops only on an empty recv issued after the sender is
	// done, so the two receivers race each other and the sender throughout
	// and every seeded envelope is eventually reachable.
	sendDone := make(chan struct{})
	drain := func(base int) result {
		var res result
		for call := 0; ; call++ {
			finished := false
			select {
			case <-sendDone:
				finished = true
			default:
			}
			raw, err := recvJSONNoFatal(receiver, base+call, receiverKey)
			if err != nil {
				res.err = err
				return res
			}
			var envs []wsmailbox.Envelope
			if err := json.Unmarshal([]byte(raw), &envs); err != nil {
				res.err = fmt.Errorf("recv JSON: %w: %s", err, raw)
				return res
			}
			for _, e := range envs {
				res.contents = append(res.contents, e.Content)
			}
			if finished && len(envs) == 0 {
				return res
			}
		}
	}

	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = drain(100000 * (i + 1))
		}(i)
	}

	const perQueue = 50
	want := map[string]bool{}
	id := 10
	for i := 0; i < perQueue; i++ {
		for _, q := range []struct{ to, prefix string }{
			{replyAddr, "reply"},
			{"alice@worktree", "named"},
		} {
			content := fmt.Sprintf("%s-%03d", q.prefix, i)
			want[content] = true
			callToolWithKey(t, sender, id, senderKey, "mailbox.send", map[string]any{
				"to": q.to, "content": content,
			})
			id++
		}
	}
	close(sendDone)
	wg.Wait()

	// A receiver that failed stopped draining, so its share of the seeded
	// envelopes would read as lost: report the failure alone, once.
	got := map[string]int{}
	for _, res := range results {
		if res.err != nil {
			t.Fatal(res.err)
		}
	}
	for _, res := range results {
		for _, c := range res.contents {
			got[c]++
		}
	}
	for c, n := range got {
		if n != 1 {
			t.Errorf("envelope %q delivered %d times, want exactly once", c, n)
		}
		if !want[c] {
			t.Errorf("unexpected envelope %q", c)
		}
	}
	for c := range want {
		if got[c] == 0 {
			t.Errorf("envelope %q was lost", c)
		}
	}
}

// recvJSONNoFatal is one mailbox.recv call (format json) that reports failure
// as an error instead of failing the test, so receiver goroutines can run it:
// t.Fatal is only valid on the test goroutine.
func recvJSONNoFatal(server *Server, id int, key string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "mailbox.recv",
			"arguments": map[string]any{"session_key": key, "format": "json"},
		},
	})
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(string(payload)+"\n"), &out); err != nil {
		return "", fmt.Errorf("recv %d: ServeStdio: %w", id, err)
	}
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var line string
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var probe struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(l), &probe) == nil && string(probe.ID) == fmt.Sprint(id) {
			line = l
		}
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return "", fmt.Errorf("recv %d: response: %w: %q", id, err, out.String())
	}
	if resp.Error != nil {
		return "", fmt.Errorf("recv %d: %s", id, resp.Error.Message)
	}
	if resp.Result.IsError || len(resp.Result.Content) != 1 {
		return "", fmt.Errorf("recv %d: unexpected result: %s", id, line)
	}
	return resp.Result.Content[0].Text, nil
}
