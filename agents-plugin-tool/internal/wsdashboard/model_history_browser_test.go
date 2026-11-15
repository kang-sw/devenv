package wsdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Only disposable fixture snapshots and their normal region transport are used.
func TestModelHistoryEntriesBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	setModel := func(model string) {
		snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000}
		if model != "" {
			snap.Lead.Observed = &wslive.ObservedModel{
				Current: wslive.ModelObservation{ID: model, Model: model, Source: "pi.assistant", At: 1000},
				History: []wslive.ModelChange{{Model: model, Source: "pi.assistant", At: 1000}},
			}
		}
		f.d.snapshots.SetLive(f.dir, "livesession", snap)
		f.d.broker.wake(liveDep("livesession"))
	}
	setModel("")
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setModel(r.URL.Query().Get("model"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/model-history.browser.cjs", f.base, updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium model history entries: %v", err)
	}
}
