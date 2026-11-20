package wsdashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	if !strings.Contains(shell, `<span class="gn">Saved in this browser.<span class="meta settings-version">v`+d.Endpoint().Version+`</span><a class="d-settings-github" href="https://github.com/`+releaseRepository+`" target="_blank" rel="noopener noreferrer" aria-label="GitHub repository" title="GitHub repository"><svg `) {
		t.Error("Settings footer lacks the version followed by the GitHub repository link at the right end of the saved notice row")
	}
	code, frame := get(t, base+"/?frame=1")
	if code != http.StatusOK || mainPanel(frame) == "" || strings.Contains(mainPanel(frame), `data-region="sessions"`) || strings.Contains(mainPanel(frame), `href="/s/`) || strings.Contains(mainPanel(frame), `data-rid="`) || strings.Contains(frame, `data-region="sessions"`) {
		t.Fatalf("empty root frame = %d, main carries session content or is missing", code)
	}
	for _, path := range []string{"/s/", "/s/INVALID", "/s/missing", "/s/missing?frame=1"} {
		if code, _ := get(t, base+path); code != http.StatusNotFound {
			t.Errorf("%s = %d, want independent missing/invalid-session handling", path, code)
		}
	}
}

func TestSettingsVersionFooterStyleKeepsNoticeAndVersionInline(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		`.gmenu .gn { display: flex;`,
		`white-space: nowrap;`,
		`.gmenu .gn .settings-version { margin-left: auto; flex: none; }`,
		`.gmenu .gn .d-settings-github { flex: none;`,
		`.gmenu .gn .d-settings-github:hover, .gmenu .gn .d-settings-github:focus-visible { color: var(--wb-fg); }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks Settings footer rule %q", want)
		}
	}
}

// The footer link, launcher download destinations, and plugin manifest links
// name one repository: the daemon restates runtime.json's release_repository
// (see releaseRepository).
func TestSettingsGitHubLinkMatchesReleaseDestinations(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	const expectedReleaseRepository = "kang-sw/werkschaft"
	if releaseRepository != expectedReleaseRepository {
		t.Fatalf("releaseRepository = %q, want %q", releaseRepository, expectedReleaseRepository)
	}
	expectedURL := "https://github.com/" + expectedReleaseRepository

	for _, path := range []string{
		"agents-plugin/runtime.json",
		"agents-plugin-wsflow/runtime.json",
		"agents-plugin-pi/runtime.json",
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var contract struct {
			ReleaseRepository string `json:"release_repository"`
		}
		if err := json.Unmarshal(raw, &contract); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if contract.ReleaseRepository != expectedReleaseRepository {
			t.Errorf("%s release_repository = %q, want %q", path, contract.ReleaseRepository, expectedReleaseRepository)
		}
	}

	for _, path := range []string{
		"agents-plugin/.codex-plugin/plugin.json",
		"agents-plugin/.claude-plugin/plugin.json",
		"agents-plugin-wsflow/.codex-plugin/plugin.json",
		"agents-plugin-wsflow/.claude-plugin/plugin.json",
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var manifest struct {
			Repository string `json:"repository"`
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if manifest.Repository != expectedURL {
			t.Errorf("%s repository = %q, want %q", path, manifest.Repository, expectedURL)
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
