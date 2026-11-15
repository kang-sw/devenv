package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Real page, history and stream handlers on disposable loopback fixtures only.
func TestEntryRowsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	ringDir := wslive.RingDir(SessionDir(f.dir, "livesession"))
	var records []wslive.RingEntry
	for i := 0; i < 58; i++ {
		records = append(records, wslive.RingEntry{Kind: wslive.RingReply, Text: strings.Repeat(fmt.Sprintf("Reader anchor %02d. ", i), 24)})
	}
	effect, ok := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{{Kind: "log", Topic: "Plan", EntryID: "e1", Field: "text", After: "entry-row-effect-marker"}})
	if !ok {
		t.Fatal("empty effects fixture")
	}
	records = append(records, effect,
		wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:boardsession", Title: "entry-row-mail-a", HistoryKey: "row-a"},
		wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailSent, Peer: "gone@machine", Title: "entry-row-mail-b", HistoryKey: "row-b"})
	if err := wslive.AppendRing(ringDir, records); err != nil {
		t.Fatal(err)
	}
	availableA, availableB := true, true
	publish := func() {
		bodies := map[string]string{"hist-1.json": "mail first line\nmail second line"}
		if availableA {
			bodies["row-a"] = "entry-row-mail-a\nbody A"
		}
		if availableB {
			bodies["row-b"] = "entry-row-mail-b\nbody B"
		}
		entryRowsSetBodies(t, f, bodies)
	}
	publish()
	var mu sync.Mutex
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		switch q.Get("op") {
		case "a":
			availableA = q.Get("value") == "on"
			publish()
		case "b":
			availableB = q.Get("value") == "on"
			publish()
		case "peer":
			if err := writeSessionLabel(SessionDir(f.dir, "boardsession"), q.Get("value")); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			f.d.index.Reread("boardsession")
			f.d.index.Settle()
		case "model":
			model := q.Get("value")
			f.d.snapshots.SetLive(f.dir, "livesession", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000,
				Lead: wslive.Lead{Observed: &wslive.ObservedModel{Current: wslive.ModelObservation{ID: model, Model: model, Source: "pi.assistant", At: 1000}, History: []wslive.ModelChange{{Model: model, Source: "pi.assistant", At: 1000}}}}})
			f.d.broker.wake(liveDep("livesession"))
		case "gap":
			appendUsers(t, f, "livesession", 1000, 55)
			f.d.broker.wake(ringDep("livesession"))
		default:
			http.Error(w, "unknown fixture update", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	functions := "var ENTRY_SLOT = /<!--ws-entry-row-(\\d+)-->/g; var ENTRY_CAP = 150;\n" + dashboardFuncs(t,
		"frag", "entriesBox", "entryKeys", "resetEntries", "mergeEntries", "gapped", "evictEntries", "entrySame", "entryInputFull", "entryRowNode", "entryInputPatch", "entryInputHTML", "mergeEntriesRegion")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/entry-rows.browser.cjs")
	cmd.Env = append(os.Environ(), "ENTRY_ROWS_BASE="+f.base, "ENTRY_ROWS_UPDATES="+updates.URL, "ENTRY_ROWS_FUNCTIONS="+functions)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("entry rows browser: %v", err)
	}
}
