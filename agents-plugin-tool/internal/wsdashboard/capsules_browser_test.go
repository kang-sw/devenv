package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// This layout regression uses an installed Chromium, without a server or daemon.
func TestInlineCapsulesBrowserWrapping(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium layout regression")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/capsules.browser.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("inline capsule wrapping: %v", err)
	}
}
