package wsdashboard

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The index's main panel is the landing: both lockup variants inline (the
// light-ink one for dark, the dark-ink one for light) over the how-to, in
// the viewer's language; the stylesheet shows one variant per theme and the
// how-to only when the landing is the hash target (/#guide).
func TestIndexShowsTheLandingLogoAndGuide(t *testing.T) {
	f := startPageFixture(t)
	code, page := get(t, f.base+"/?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /?frame=1 = %d", code)
	}
	main := page[strings.Index(page, `<main id="grid"`):]
	main = main[:strings.Index(main, "</main>")]
	for _, want := range []string{
		`<div class="dlanding" id="guide">`, `<div class="dlogo" role="img" aria-label="Werkschaft">`,
		`<span class="dlogo-d"><svg`, `<span class="dlogo-l"><svg`,
		`<section class="dguide"`, wsi18n.T("en", "index.guide.title"), wsi18n.T("en", "index.guide.pick.sub"),
	} {
		if !strings.Contains(main, want) {
			t.Errorf("the index's main panel lacks %q", want)
		}
	}
	// The variants differ in the wordmark's ink: light on dark, dark on light.
	dark := main[strings.Index(main, `<span class="dlogo-d">`):strings.Index(main, `<span class="dlogo-l">`)]
	light := main[strings.Index(main, `<span class="dlogo-l">`):]
	if !strings.Contains(dark, `<g transform="translate(400 142)" fill="none" stroke="#F2EBDD"`) {
		t.Error("the dark-theme lockup's wordmark is not light ink")
	}
	if !strings.Contains(light, `<g transform="translate(400 142)" fill="none" stroke="#1E2235"`) {
		t.Error("the light-theme lockup's wordmark is not dark ink")
	}

	req, _ := http.NewRequest(http.MethodGet, f.base+"/?frame=1", nil)
	req.AddCookie(&http.Cookie{Name: wsi18n.CookieName, Value: "ko"})
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	ko := string(raw)
	for _, want := range []string{"시작하기", "새 세션 추가하기", "화이트보드 사용해주세요"} {
		if !strings.Contains(ko, want) {
			t.Errorf("the Korean landing lacks %q", want)
		}
	}

	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	css := string(a.data.CSS)
	for _, want := range []string{
		`.dlogo-d { display: none; }`,
		`:root:not([data-theme="light"]) .dlogo-d { display: block; }`,
		`:root[data-theme="dark"] .dlogo-d { display: block; }`,
		`:root[data-theme="dark"] .dlogo-l { display: none; }`,
		`.dlanding:target .dguide { display: block; }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet lacks the theme rule %q", want)
		}
	}
}

// Chromium acceptance over disposable fixture sessions: the landing's
// lockup is centred with the variant for each theme, and a second click
// (or Enter) on the open session's rail row closes its view, to /#guide and
// the how-to, while a click on another row opens it as before. With
// WS_LANDING_SHOTS set (an absolute directory) it also saves screenshots.
func TestLandingGuideBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	shots := os.Getenv("WS_LANDING_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_LANDING_SHOTS must be absolute")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/landing.browser.cjs", f.base, shots).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium landing acceptance: %v", err)
	}
}
