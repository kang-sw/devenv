package wswhiteboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// History an owner loaded above a fold folds back into the gap once the whole
// topic card has left the viewport, while the visible line below stays put;
// the fold row loads again afterwards. Also pins the load control as a filled,
// outline-free capsule in both themes. Needs WS_PLAYWRIGHT_MODULE.
func TestHistoryAutoFoldBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	st := &State{}
	for _, topic := range []string{"a", "b"} {
		for i := 1; i <= 150; i++ {
			mustApply(t, st, fmt.Sprintf(`<wb:topic name="%s"><wb:log>%s log %d: a reasonably long line of text for the history window</wb:log></wb:topic>`, topic, topic, i))
		}
	}
	page, err := RenderPage(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "fold", Location: time.UTC}))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "board.html")
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	shots := os.Getenv("WS_AUTOFOLD_SHOTS")
	if shots == "" {
		shots = dir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-autofold.cjs", path, shots)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("auto-fold: %v", err)
	}
}
