package wsdashboard

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// guideSteps are the landing guide's steps, in order: each names its
// screenshots (assets/guide/<lang>-<theme>-<step>.webp).
var guideSteps = []string{"open", "workspace", "new", "ask", "board", "answer", "files", "settings"}

var guideImg = regexp.MustCompile(`<img class="dshot-([dl])" src="(/s/guide-([a-z]+)-(dark|light)-([a-z]+)-[0-9a-f]{12}\.webp)" width="([0-9]+)" height="([0-9]+)" alt="([^"]+)" loading="lazy" decoding="async">`)

// The index's main panel is the landing: both lockup variants inline (the
// light-ink one for dark, the dark-ink one for light) over the illustrated
// how-to, in the viewer's language. Each step has a title, a line, and its
// screenshot in both themes, served as a separate image (never inlined),
// Korean captures for Korean and English ones for every other language;
// the stylesheet shows one lockup and one screenshot per theme.
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
		`<article class="dguide" aria-labelledby="dguide-h">`, wsi18n.T("en", "index.guide.title"), wsi18n.T("en", "index.guide.lead"),
		wsi18n.T("en", "index.guide.workspace"), wsi18n.T("en", "index.guide.settings.sub"), wsi18n.T("en", "index.guide.settings"),
	} {
		if !strings.Contains(main, want) {
			t.Errorf("the index's main panel lacks %q", want)
		}
	}
	if strings.Contains(page, "data:image/") {
		t.Error("the landing inlines an image; the guide's screenshots are separate assets")
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

	// shots checks a page's steps: in order, each with its dark and light
	// screenshot in want's captures, and every screenshot served.
	served := map[string]bool{}
	shots := func(name, page, want string) {
		t.Helper()
		steps := regexp.MustCompile(`<li class="dstep" data-step="([a-z]+)">`).FindAllStringSubmatch(page, -1)
		var got []string
		for _, m := range steps {
			got = append(got, m[1])
		}
		if strings.Join(got, ",") != strings.Join(guideSteps, ",") {
			t.Errorf("%s: steps = %v, want %v", name, got, guideSteps)
		}
		imgs := guideImg.FindAllStringSubmatch(page, -1)
		if len(imgs) != 2*len(guideSteps) {
			t.Fatalf("%s: %d guide screenshots, want %d (dark and light per step)", name, len(imgs), 2*len(guideSteps))
		}
		for i, m := range imgs {
			class, src, lang, theme, step := m[1], m[2], m[3], m[4], m[5]
			if lang != want || step != guideSteps[i/2] || theme != []string{"dark", "light"}[i%2] || class != theme[:1] {
				t.Errorf("%s: screenshot %d = %s (class dshot-%s), want %s %s %s", name, i, src, class, want, guideSteps[i/2], []string{"dark", "light"}[i%2])
			}
			if w, _ := strconv.Atoi(m[6]); w < 200 || w > 800 {
				t.Errorf("%s: %s is %d CSS px wide", name, src, w)
			}
			if served[src] {
				continue
			}
			served[src] = true
			resp, err := testClient().Get(f.base + src)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/webp" || !bytes.HasPrefix(body, []byte("RIFF")) {
				t.Errorf("GET %s = %d %q (%d bytes)", src, resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
			}
			if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
				t.Errorf("GET %s: Cache-Control %q", src, resp.Header.Get("Cache-Control"))
			}
			if len(body) > 150<<10 {
				t.Errorf("%s is %d bytes, over 150 KiB", src, len(body))
			}
		}
	}
	shots("en", main, "en")

	_, ko := getLang(t, f.base+"/?frame=1", "ko", "")
	for _, want := range []string{"시작하기", "새 세션 추가하기", "화이트보드 사용해주세요", "설정 바꾸기"} {
		if !strings.Contains(ko, want) {
			t.Errorf("the Korean landing lacks %q", want)
		}
	}
	shots("ko", ko, "ko")
	_, de := getLang(t, f.base+"/?frame=1", "de", "")
	if !strings.Contains(de, wsi18n.T("de", "index.guide.view")) {
		t.Error("the German landing is not in German")
	}
	shots("de", de, "en")

	// A guide name that is not one is no asset.
	if code, _ := get(t, f.base+"/s/guide-en-dark-open-000000000000.webp"); code == http.StatusOK {
		t.Error("an unknown guide screenshot name is served")
	}

	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	css := string(a.data.CSS)
	for _, want := range []string{
		`.dlogo-d, .dshot-d { display: none; }`,
		`:root:not([data-theme="light"]) .dlogo-d, :root:not([data-theme="light"]) .dshot-d { display: block; }`,
		`:root[data-theme="dark"] .dlogo-d, :root[data-theme="dark"] .dshot-d { display: block; }`,
		`:root[data-theme="dark"] .dlogo-l, :root[data-theme="dark"] .dshot-l { display: none; }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet lacks the theme rule %q", want)
		}
	}
}

// Every screenshot the capture writes has its CSS size in guide.json, and
// each step has all four (two languages, two themes).
func TestLandingGuideAssetsAreComplete(t *testing.T) {
	set := guideAssets()
	for _, lang := range []string{"ko", "en"} {
		for _, theme := range []string{"dark", "light"} {
			for _, step := range guideSteps {
				shot, ok := set.shots[lang+"-"+theme+"-"+step]
				if !ok || shot.W == 0 || shot.H == 0 {
					t.Errorf("guide screenshot %s-%s-%s = %+v, %v", lang, theme, step, shot, ok)
				}
			}
		}
	}
	if n := len(set.files); n != 2*2*len(guideSteps) {
		t.Errorf("%d guide screenshots embedded, want %d", n, 2*2*len(guideSteps))
	}
}

// Chromium acceptance over disposable fixture sessions: the landing's
// lockup over the illustrated how-to is one page that scrolls in the main
// panel and fits at 1440 and 700px, showing the lockup and screenshots for
// each theme (and fetching only those), and a second click (or Enter) on
// the open session's rail row keeps it open. Home returns to / and the
// landing, while another row opens as before. With WS_LANDING_SHOTS
// set (an absolute directory) it also saves full-page screenshots and the
// add-workspace step close up.
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
