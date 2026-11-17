package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPillLayout(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/pill-layout.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/pill-layout.test.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("pill layout: %v", err)
	}
}

func TestPillGrowBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium pill grow regression")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/pill-grow.browser.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("pill grow: %v", err)
	}
}
