package wsdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestPanelRowsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for cached Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := panelRowsFixture(t)
	agenda, todo := "initial", "initial"
	panelRowsPublish(t, f, agenda, todo)
	var mu sync.Mutex
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Query().Get("op") {
		case "agenda":
			agenda = r.URL.Query().Get("value")
		case "todo":
			todo = r.URL.Query().Get("value")
		case "fallback":
			p := f.src.page("livesession")
			p.Board.Panels = "\n" + p.Board.Panels
			p.Seq++
			f.src.setPage("livesession", p)
			f.d.index.Nudge("livesession", f.project)
			f.d.index.Settle()
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.Error(w, "unknown fixture update", http.StatusBadRequest)
			return
		}
		panelRowsPublish(t, f, agenda, todo)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	functions := "var PANEL_SLOT = /<!--ws-panel-row-(\\d+)-->/g;\n" + dashboardFuncs(t, "keyedSame", "keyedInputPatch", "keyedInputHTML", "panelRowKey", "panelInputFull", "panelInputPatch", "panelInputHTML", "panelRowNode", "mergePanelRows", "panelReaderState", "restorePanelReader")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/panel-rows.browser.cjs")
	cmd.Env = append(os.Environ(), "PANEL_ROWS_BASE="+f.base, "PANEL_ROWS_UPDATES="+updates.URL, "PANEL_ROWS_FUNCTIONS="+functions)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("panel rows browser: %v", err)
	}
}
