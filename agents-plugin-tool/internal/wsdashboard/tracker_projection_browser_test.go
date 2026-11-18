package wsdashboard

import (
	"os"
	"os/exec"
	"testing"
)

func TestTrackerProjectionFrontend(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "--test", "tracker_projection_adapter.test.js", "tracker_projection_render.test.js").CombinedOutput()
	if err != nil {
		t.Fatalf("projection frontend: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func TestTrackerProjectionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("WS_PLAYWRIGHT_MODULE not set")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "testdata/tracker-projection.browser.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("projection browser: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
