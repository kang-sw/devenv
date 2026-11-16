package wsdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Disposable loopback fixture only: no shared daemon, provider or harness.
func TestTreeRowsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	f, snap := treeRowsFixture(t)
	for i := 0; i < 7; i++ {
		snap.Agents[0].Batches = append(snap.Agents[0].Batches, wslive.BatchView{Seq: int64(i + 1), ToolNames: []string{"Read"}, CallCount: 1, StartedAt: 1000, EndedAt: 2000})
	}
	// Enough real rows to exercise the popover's own scroll pane.
	for i := 0; i < 18; i++ {
		snap.Agents = append(snap.Agents, wslive.Agent{ID: "extra" + strconv.Itoa(i), Label: "extra " + strconv.Itoa(i), Status: wslive.StatusRunning})
	}
	f.d.snapshots.SetLive(f.dir, "livesession", snap)
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("op") {
		case "parent":
			snap.Agents[0].Caption = q.Get("text")
		case "child":
			snap.Agents[1].Caption = q.Get("text")
		case "both":
			snap.Agents[0].Caption, snap.Agents[1].Caption = q.Get("text"), q.Get("text")
		case "permission":
			snap.Agents[1].Phase = wslive.PhasePermission
			snap.Agents[1].Permission = &wslive.Permission{ToolName: "Bash", ArgsPreview: "tree fixture approval"}
		case "clear":
			snap.Agents[1].Phase, snap.Agents[1].Permission = wslive.PhaseThinking, nil
		case "shell":
			background := 2
			snap.Usage = &wslive.Usage{Background: &background}
			snap.Agents[0].Caption = q.Get("text")
		default:
			http.Error(w, "unknown fixture update", http.StatusBadRequest)
			return
		}
		f.d.snapshots.SetLive(f.dir, "livesession", snap)
		f.d.broker.wake(liveDep("livesession"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	functions := "var TREE_SLOT = /<!--ws-tree-row-(\\d+)-->/g;\n" + dashboardFuncs(t,
		"treeRowKey", "treeSlot", "treeSlots", "treeInputFull", "treeRowNode", "keyedSame", "keyedInputPatch", "keyedInputHTML", "treeInputPatch", "treeInputHTML", "mergeTreeRows")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/tree-rows.browser.cjs")
	cmd.Env = append(os.Environ(), "TREE_ROWS_BASE="+f.base, "TREE_ROWS_UPDATES="+updates.URL, "TREE_ROWS_FUNCTIONS="+functions)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("tree rows browser: %v", err)
	}
}
