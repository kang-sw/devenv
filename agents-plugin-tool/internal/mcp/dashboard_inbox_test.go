package mcp

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func (f liveFixture) inboxDir(replyID string) string {
	return wsinbox.Dir(f.sessionDir(replyID))
}

func sendBoard(t *testing.T, dir, text string, chips ...wsinbox.Chip) {
	t.Helper()
	if _, _, err := wsinbox.Send(dir, wsinbox.Entry{Parts: []string{text}, Chips: chips}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// directDrain calls the handler without the stdio server, which would
// re-register an ended session and nudge on its own.
func directDrain(t *testing.T, s *Server, key string, args map[string]any) (text string, isError bool) {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["session_key"] = key
	resp := s.handleDashboardInboxDrain(json.RawMessage("1"), args)
	result := resp.Result.(map[string]any)
	content := result["content"].([]map[string]string)
	isError, _ = result["isError"].(bool)
	return content[0]["text"], isError
}

func TestDashboardInboxDrainDescriptionIsAdapterOnly(t *testing.T) {
	var desc string
	for _, tool := range tools() {
		if tool["name"] == dashboardInboxDrainTool {
			desc, _ = tool["description"].(string)
		}
	}
	if !strings.HasPrefix(desc, "Adapter-only:") {
		t.Fatalf("description = %q", desc)
	}
	for _, want := range []string{"an agent never calls it", "atomically claims", "oldest first", "can no longer be merged into or cancelled"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description lacks %q: %s", want, desc)
		}
	}
}

func TestDashboardInboxDrainTextAndJSONOutput(t *testing.T) {
	f := newLiveFixture(t, true)
	dir := f.inboxDir(f.replyA)

	if out := callToolWithKey(t, f.server, 3, f.keyA, dashboardInboxDrainTool, nil); out != "inbox empty\n" {
		t.Fatalf("empty inbox = %q", out)
	}
	if out := callToolWithKey(t, f.server, 4, f.keyA, dashboardInboxDrainTool, map[string]any{"format": "json"}); strings.TrimSpace(out) != `{"messages":[]}` {
		t.Fatalf("empty inbox json = %q", out)
	}

	sendBoard(t, dir, "first", wsinbox.Chip{ID: "3", Answer: "b"})
	sendBoard(t, dir, "second") // merges into the held entry as its own paragraph
	names, _ := wsinbox.Names(dir)
	if len(names) != 1 {
		t.Fatalf("held = %v", names)
	}
	rec, _ := wsinbox.Held(dir)
	id := rec[0].ID()
	sent := time.UnixMilli(rec[0].Entry.SentAt).UTC().Format(time.RFC3339)

	out := callToolWithKey(t, f.server, 5, f.keyA, dashboardInboxDrainTool, nil)
	want := "drained 1\nmessage " + id + " " + sent + "\nfirst\n\nsecond\n\n(3) b\n\n"
	if out != want {
		t.Fatalf("drain text = %q, want %q", out, want)
	}
	if out := callToolWithKey(t, f.server, 6, f.keyA, dashboardInboxDrainTool, nil); out != "inbox empty\n" {
		t.Fatalf("second drain = %q", out)
	}

	sendBoard(t, dir, "again", wsinbox.Chip{ID: "7", Answer: "ok", Note: "fine"})
	out = callToolWithKey(t, f.server, 7, f.keyA, dashboardInboxDrainTool, map[string]any{"format": "json"})
	var got struct {
		Messages []struct {
			ID     string   `json:"id"`
			SentAt int64    `json:"sent_at"`
			Text   string   `json:"text"`
			Parts  []string `json:"parts"`
			Chips  []struct {
				ID, Answer, Note string
			} `json:"chips"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages = %+v", got)
	}
	m := got.Messages[0]
	if m.ID == "" || m.SentAt == 0 || m.Text != "again\n\n(7) ok, note: fine" || len(m.Parts) != 1 || m.Parts[0] != "again" || len(m.Chips) != 1 || m.Chips[0].Note != "fine" {
		t.Fatalf("message = %+v", m)
	}

	// A chip-only message has empty, never null, parts.
	if _, _, err := wsinbox.Send(dir, wsinbox.Entry{Chips: []wsinbox.Chip{{ID: "1", Answer: "a"}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out = callToolWithKey(t, f.server, 8, f.keyA, dashboardInboxDrainTool, map[string]any{"format": "json"})
	if !strings.Contains(out, `"parts":[]`) || strings.Contains(out, "null") {
		t.Fatalf("chip-only json = %q", out)
	}
}

func TestDashboardInboxDrainIsPerSessionAndRefusesUnknown(t *testing.T) {
	f := newLiveFixture(t, true)
	sendBoard(t, f.inboxDir(f.replyB), "for B")

	if out := callToolWithKey(t, f.server, 3, f.keyA, dashboardInboxDrainTool, nil); out != "inbox empty\n" {
		t.Fatalf("A drained %q", out)
	}
	if names, _ := wsinbox.Names(f.inboxDir(f.replyB)); len(names) != 1 {
		t.Fatalf("A's drain touched B's inbox: %v", names)
	}
	if out := callToolWithKey(t, f.server, 4, "no-such-key-here", dashboardInboxDrainTool, nil); !strings.Contains(out, "unknown_session") {
		t.Fatalf("unknown key = %q", out)
	}
}

func TestDashboardInboxDrainWorksWithTheDashboardKeyOff(t *testing.T) {
	f := newLiveFixture(t, false)
	sendBoard(t, f.inboxDir(f.replyA), "queued")
	out := callToolWithKey(t, f.server, 3, f.keyA, dashboardInboxDrainTool, nil)
	if !strings.HasPrefix(out, "drained 1\n") || !strings.Contains(out, "queued") {
		t.Fatalf("drain with the key off = %q", out)
	}
}

// TestDashboardInboxDrainClaimsEachEntryOnce pins the atomic claim: two
// concurrent drains of one inbox get disjoint sets that together hold every
// entry, and a drained entry is never returned again.
func TestDashboardInboxDrainClaimsEachEntryOnce(t *testing.T) {
	f := newLiveFixture(t, true)
	dir := f.inboxDir(f.replyA)
	const entries = 12
	// Send merges into a held entry, so distinct held entries are written
	// through a scratch inbox beside the real one and renamed in.
	scratch := filepath.Join(f.sessionDir(f.replyA), "scratch-inbox")
	for i := 0; i < entries; i++ {
		name, _, err := wsinbox.Send(scratch, wsinbox.Entry{Parts: []string{"m"}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(scratch, name), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	const drainers = 6
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = map[string]int{}
	)
	for i := 0; i < drainers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, isErr := directDrain(t, f.server, f.keyA, map[string]any{"format": "json"})
			if isErr {
				t.Errorf("drain error: %s", text)
				return
			}
			var got struct {
				Messages []struct {
					ID string `json:"id"`
				} `json:"messages"`
			}
			if err := json.Unmarshal([]byte(text), &got); err != nil {
				t.Errorf("json: %v", err)
				return
			}
			mu.Lock()
			for _, m := range got.Messages {
				seen[m.ID]++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != entries {
		t.Fatalf("drained %d distinct entries, want %d", len(seen), entries)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("entry %s drained %d times", id, n)
		}
	}
	if out, _ := directDrain(t, f.server, f.keyA, nil); out != "inbox empty\n" {
		t.Fatalf("drain after the race = %q", out)
	}
}

// TestDashboardInboxDrainNudgesOnlyAfterClaiming pins the nudge: a drain
// that claimed an entry nudges the daemon with the session's reply-id so the
// held bubble clears; an empty drain sends none.
func TestDashboardInboxDrainNudgesOnlyAfterClaiming(t *testing.T) {
	f := newLiveFixture(t, true)
	fake := startFakeDaemon(t, f.stateDir, http.StatusNoContent, false)
	count := func() int {
		n := 0
		for _, body := range fake.received() {
			if body.ReplyID == f.replyA {
				n++
			}
		}
		return n
	}
	callToolWithKey(t, f.server, 3, f.keyA, "mailbox.lookup_peers", nil) // registers
	waitForNudge(t, fake, f.replyA)
	time.Sleep(100 * time.Millisecond)

	base := count()
	if out, _ := directDrain(t, f.server, f.keyA, nil); out != "inbox empty\n" {
		t.Fatalf("empty drain = %q", out)
	}
	time.Sleep(250 * time.Millisecond)
	if count() != base {
		t.Fatalf("an empty drain sent %d nudges", count()-base)
	}

	sendBoard(t, f.inboxDir(f.replyA), "hello")
	if out, _ := directDrain(t, f.server, f.keyA, nil); !strings.HasPrefix(out, "drained 1\n") {
		t.Fatalf("drain = %q", out)
	}
	waitFor(t, "the drain's nudge", func() bool { return count() > base })
}

// TestDashboardSessionFingerprintTracksTheInbox pins that a send and a
// drain each change the session's fingerprint, so the daemon's stream
// re-renders the held bubble and the sent mark.
func TestDashboardSessionFingerprintTracksTheInbox(t *testing.T) {
	f := newLiveFixture(t, true)
	callToolWithKey(t, f.server, 3, f.keyA, "mailbox.lookup_peers", nil) // registers
	reg, ok := readRegistration(t, f.stateDir, f.keyA)
	if !ok {
		t.Fatal("no registration")
	}
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return f.stateDir, nil }, now: time.Now}
	read := func() string {
		t.Helper()
		data, ok := src.ReadSession(reg)
		if !ok {
			t.Fatal("ReadSession found no session")
		}
		return data.Fingerprint
	}

	empty := read()
	dir := f.inboxDir(f.replyA)
	sendBoard(t, dir, "one")
	sent := read()
	if sent == empty {
		t.Fatal("a send kept the fingerprint")
	}
	sendBoard(t, dir, "two") // a merge replaces the held entry by a new name
	merged := read()
	if merged == sent {
		t.Fatal("a merge kept the fingerprint")
	}
	if _, isErr := directDrain(t, f.server, f.keyA, nil); isErr {
		t.Fatal("drain failed")
	}
	if drained := read(); drained == merged || drained == empty {
		t.Fatalf("a drain left the fingerprint at a previous value (%s)", drained)
	}
}
