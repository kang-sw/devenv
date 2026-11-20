package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestComposerControlsAndMeterBrowser verifies the 2-control composer strip
// (Model + Effort only), context meter states, SSE popover survival, Codex 5h
// top-bar pace, and captures side-by-side screenshots against composer-meter-v2.html.
func TestComposerControlsAndMeterBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Playwright browser tests")
	}
	shots := os.Getenv("WS_COMPOSER_METER_SHOTS")
	if shots == "" {
		shots = t.TempDir()
	} else if !filepath.IsAbs(shots) {
		t.Fatal("WS_COMPOSER_METER_SHOTS must be absolute")
	}
	mockup, err := filepath.Abs(filepath.Join("..", "..", "..", "ai-docs", "assets", "mockups", "261120", "composer-footer-context-meter", "composer-meter-v2.html"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/composer-controls-and-meter.browser.cjs", mockup, shots).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("composer controls and meter browser test failed: %v", err)
	}
}
