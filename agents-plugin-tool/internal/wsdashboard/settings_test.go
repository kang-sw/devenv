package wsdashboard

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRootWithoutSessionsKeepsEmptySelectionAndSettings(t *testing.T) {
	d := startIndexedDaemon(t, t.TempDir(), newPageFake(), time.Hour)
	base := endpointURL(d.Endpoint(), "")
	code, shell := get(t, base+"/")
	if code != http.StatusOK || !strings.Contains(shell, `data-rid=""`) {
		t.Fatalf("empty root = %d, not an empty selection", code)
	}
	assertSettingsOwner(t, shell, true)
	if !strings.Contains(shell, `class="meta settings-version">v`+d.Endpoint().Version+`</span>`) {
		t.Error("Settings lost the endpoint version")
	}
	code, frame := get(t, base+"/?frame=1")
	if code != http.StatusOK || !strings.Contains(frame, `<main id="grid" class="dgrid"></main>`) || strings.Contains(frame, `data-region="sessions"`) {
		t.Fatalf("empty root frame = %d, main is not empty", code)
	}
	for _, path := range []string{"/s/", "/s/INVALID", "/s/missing", "/s/missing?frame=1"} {
		if code, _ := get(t, base+path); code != http.StatusNotFound {
			t.Errorf("%s = %d, want independent missing/invalid-session handling", path, code)
		}
	}
}

// Opt in with an already cached Playwright module and browser. Both daemons
// own temporary stores/listeners; usage is fulfilled by the browser fixture.
func TestSettingsAndEmptyRootBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	f := startPageFixture(t)
	empty := startIndexedDaemon(t, t.TempDir(), newPageFake(), time.Hour)
	cmd := exec.Command("node", "testdata/settings-root.browser.cjs", f.base, endpointURL(empty.Endpoint(), ""))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Settings/root browser: %v", err)
	}
}
