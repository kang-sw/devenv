package wsdashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// trackerMockupI18nRegen is the environment switch that rewrites the mockup's
// lookup file instead of comparing it.
const trackerMockupI18nRegen = "WS_REGEN_TRACKER_MOCKUP_I18N"

// trackerMockupI18n is the content of the tracker mockup's i18n-en.js: the
// English tracker strings (js.tk.*) with the same wsT and wsTn the dashboard
// page inlines, so the mockup, which loads from file:// and fetches nothing,
// renders tracker.js exactly as the product does in English.
func trackerMockupI18n() string {
	return "// Generated from the English client table (js.tk.* keys). Regenerate from the Go module root: " + trackerMockupI18nRegen +
		"=1 go test ./internal/wsdashboard/ -run TestTrackerMockupI18nIsCurrent\n" + string(wsi18n.SubsetScript(wsi18n.Default, "js.tk."))
}

// The checked-in mockup lookup file matches the tables. The mockup lives
// outside this module, so the test skips where it is absent.
func TestTrackerMockupI18nIsCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "ai-docs", "ref", "tracker-mockup")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no tracker mockup at %s: %v", dir, err)
	}
	path, want := filepath.Join(dir, "i18n-en.js"), trackerMockupI18n()
	if os.Getenv(trackerMockupI18nRegen) == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", path)
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (set %s=1 to write it)", err, trackerMockupI18nRegen)
	}
	if string(got) != want {
		t.Fatalf("%s is stale against the English table; set %s=1 and rerun this test to rewrite it", path, trackerMockupI18nRegen)
	}
}
