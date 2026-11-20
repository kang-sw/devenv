// Page route tests cover assets, framing, and access policy.

package wsdashboard

import (
	"html"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestPagesCarryTheTracker pins the tracker renderer on the pages that mount
// it, the framed session pages and the index (the shell around a session page
// leaves it to its frame): its stylesheet after the dashboard's, its script
// after the string table and before the page script that mounts it, and the
// header slot for the search palette's button (the modal itself mounts in the
// body).
func TestPagesCarryTheTracker(t *testing.T) {
	f := startPageFixture(t)
	for name, path := range map[string]string{"live": "/s/livesession?frame=1", "board-first": "/s/boardsession?frame=1", "index": "/?frame=1"} {
		code, page := get(t, f.base+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, code)
		}
		page = frameDependencies(t, page)
		css, trackerCSS := strings.Index(page, ".dwrap {"), strings.Index(page, ".tk.tk-drawer {")
		if css < 0 || trackerCSS < css {
			t.Errorf("%s page: the tracker stylesheet is missing or ahead of the dashboard's", name)
		}
		scriptAt, script := dashboardPageScript(t, f.base, page, name != "index")
		i18n, tracker := strings.Index(page, "window.wsT = function"), strings.Index(page, "window.WsTracker = { mount: mount }")
		if i18n < 0 || tracker < i18n || scriptAt < tracker {
			t.Errorf("%s page: the tracker script is missing or not between the string table and the page script", name)
		}
		// The whiteboard's own pages leave the tracker strings out; the
		// dashboard pages that mount the tracker must keep them.
		if !strings.Contains(page, `"js.tk.`) {
			t.Errorf("%s page: the string table lacks the tracker strings (js.tk.*)", name)
		}
		if strings.Count(page, `<span class="qsslot" id="qsslot"></span>`) != 1 {
			t.Errorf("%s page lacks the search-palette button slot", name)
		}
		if !strings.Contains(page, "class: 'tk tk-palette dr-search-palette'") || strings.Contains(page, `class="tk-qpop"`) {
			t.Errorf("%s page: centered search modal missing or old popover remains", name)
		}
		// The wiki in the Items panel: its stylesheet block, the renderer's
		// switch and card, the adapter's wiki methods, and the wiki strings.
		for _, hook := range []string{
			".tk .wk-sw {", ".tk .card > .cbody.wk-body {", ".tk a.wlink {", ".tk .wk-vs {",
			`attr('act', 'wk-mode')`, "function cardWiki(", "function wkRefused(", `'a[data-wiki][data-page]'`,
			`"js.tk.wk.tab"`,
		} {
			if !strings.Contains(page, hook) {
				t.Errorf("%s page lacks the wiki hook %q", name, hook)
			}
		}
		for _, hook := range []string{`tkUrl(id, "wiki")`, `"/save"), req,`} {
			if !strings.Contains(script, hook) {
				t.Errorf("%s page script lacks the wiki adapter hook %q", name, hook)
			}
		}
		// Settings is shell-owned; search is last in every framed header.
		tail := `id="qsslot"></span></div>`
		if i, j := strings.Index(page, `<div class="top">`), strings.Index(page, tail); i < 0 || j < i {
			t.Errorf("%s page: the search-palette button slot is not at the header's end", name)
		}
	}
}

func TestBoardFirstPageHasNoLayoutControl(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/s/boardsession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/boardsession = %d", code)
	}
	if !strings.Contains(body, "only the board") || !strings.Contains(body, `data-mode="board"`) {
		t.Errorf("board-first page lacks its board")
	}
	// The shell owns all preferences, including those for future live sessions.
	assertSettingsOwner(t, body, false)
	for _, absent := range []string{`<button data-layout=`, `<button data-input=`, `id="composer"`, `id="splitter"`, `id="convo"`, `data-region="entries"`, `data-region="convlive"`, `id="tabbar"`, `id="ibadge"`} {
		if strings.Contains(body, absent) {
			t.Errorf("board-first page carries %q", absent)
		}
	}
}

func TestRootKeepsSessionsInTheRailOnly(t *testing.T) {
	f := startStoredRailFixture(t)
	code, body := get(t, f.base+"/?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /?frame=1 = %d", code)
	}
	for _, absent := range []string{`data-region="sessions"`, `href="/s/livesession"`, `id="finding"`, `class="lanbox"`} {
		if strings.Contains(body, absent) {
			t.Errorf("empty root frame carries %q", absent)
		}
	}
	// Landing content may live in main; session content may not.
	main := mainPanel(body)
	if main == "" {
		t.Fatal("root frame has no main panel")
	}
	for _, absent := range []string{`href="/s/`, `data-region="sessions"`, `data-rid="`, `id="composer"`, `id="convo"`} {
		if strings.Contains(main, absent) {
			t.Errorf("root main panel carries session content %q", absent)
		}
	}
	_, shell := get(t, f.base+"/")
	for _, want := range []string{`data-rid=""`, `href="/s/livesession"`, `href="/s/boardsession"`, filepath.Base(f.project), `id="gear"`, `class="meta settings-version"`} {
		if !strings.Contains(shell, want) {
			t.Errorf("root shell lacks %q", want)
		}
	}
}

// TestTrackerPagesPolicyLeavesWikiHTMLFramesFree pins the policy of the
// pages that host the tracker (the index and the session page as the iframe
// shell frames them, and the shell documents at / and /s/<id> that frame
// them): a wiki HTML page's srcdoc
// frame inherits it, so a script-src, connect-src, style-src, or
// default-src here would silently break author pages that load a library
// from another origin (wswhiteboard.SandboxedPage). The framed pages admit
// only the shell's own origin (frame-ancestors 'self'); see
// TestPagesRefuseFraming.
func TestTrackerPagesPolicyLeavesWikiHTMLFramesFree(t *testing.T) {
	f := startPageFixture(t)
	for path, want := range map[string]string{
		"/": "frame-src 'self'; frame-ancestors 'none'", "/s/livesession": "frame-src 'self'; frame-ancestors 'none'",
		"/?frame=1": "frame-ancestors 'self'", "/s/livesession?frame=1": "frame-src 'self'; frame-ancestors 'self'",
	} {
		resp, err := testClient().Get(f.base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if csp := resp.Header.Get("Content-Security-Policy"); resp.StatusCode != http.StatusOK || csp != want {
			t.Errorf("GET %s = %d, policy %q, want %q", path, resp.StatusCode, csp, want)
		}
	}
}

func TestUnknownPagesAreNotFound(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/s/nosuchsession", "/s/NOT-VALID", "/s/livesession/more", "/elsewhere", "/s/nosuchsession/events"} {
		if code, _ := get(t, f.base+path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
	// A registered session whose page cannot be read is not served.
	f.src.mu.Lock()
	delete(f.src.pages, "boardsession")
	f.src.mu.Unlock()
	if code, _ := get(t, f.base+"/s/boardsession"); code != http.StatusNotFound {
		t.Errorf("unreadable page = %d, want 404", code)
	}
	// The page has one URL, since it loads its assets by relative path.
	noFollow := &http.Client{Timeout: 500 * time.Millisecond, Transport: localhostTestHTTPTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if resp, err := noFollow.Get(f.base + "/s/livesession/"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/s/livesession" {
			t.Errorf("trailing slash = %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// A stream answers GET only.
	for _, path := range []string{"/events", "/s/livesession/events"} {
		req, _ := http.NewRequest(http.MethodHead, f.base+path, nil)
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("HEAD %s = %d", path, resp.StatusCode)
		}
	}
	resp, err := testClient().Post(f.base+"/s/livesession", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST page = %d", resp.StatusCode)
	}
}

func TestMermaidAssetIsServedBesideThePages(t *testing.T) {
	f := startPageFixture(t)
	name, data := wswhiteboard.MermaidAsset()
	resp, err := testClient().Get(f.base + "/s/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(body) != len(data) || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("mermaid asset = %d, %d bytes, %q", resp.StatusCode, len(body), resp.Header.Get("Cache-Control"))
	}
	// The page names it by relative path.
	_, page := get(t, f.base+"/s/livesession?frame=1")
	if !strings.Contains(frameDependencies(t, page), `window.wbMermaidFile = "`+name+`"`) {
		t.Errorf("page does not name the mermaid file %s relatively", name)
	}
}

// TestFrameAssetsAreServedBesideThePages pins the shared topic-body frame
// assets (wswhiteboard.FrameAsset): a frame's document links its stylesheet
// and script by relative, content-hashed names instead of inlining them,
// with the copy/relative-age subset in its own hashed script ahead of the
// frame script; the daemon serves all three as immutable, and in LAN mode to a
// viewer without the token, since a frame's opaque origin sends no cookie.
func TestFrameAssetsAreServedBesideThePages(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession?frame=1")
	m := regexp.MustCompile(`<div class="doc-frame"[^>]* data-srcdoc="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no topic-body frame in the page")
	}
	doc := html.UnescapeString(m[1])
	css := regexp.MustCompile(`<link rel="stylesheet" href="(wb-frame-[0-9a-f]{12}\.css)">`).FindStringSubmatch(doc)
	scripts := regexp.MustCompile(`<script src="(wb-frame-[0-9a-f]{12}\.js)"></script>`).FindAllStringSubmatch(doc, -1)
	if css == nil || len(scripts) != 2 {
		t.Fatalf("frame document does not link the shared assets by hashed name:\n%.600s", doc)
	}
	subset, js := scripts[0], scripts[1]
	if strings.Contains(doc, "<style>") || strings.Contains(doc, "wbDocMermaid = function") {
		t.Errorf("frame document still inlines the stylesheet or the frame script")
	}
	if strings.Contains(doc, "window.wsI18n") || strings.Index(doc, subset[1]) >= strings.Index(doc, js[1]) {
		t.Errorf("frame document must reference its subset ahead of the frame script")
	}
	data, _, ok := wswhiteboard.FrameAsset(subset[1])
	if !ok || string(data) != string(wsi18n.SubsetScript("en", "js.wb.")) {
		t.Fatal("frame subset changed its translation bytes")
	}
	for _, c := range []struct{ name, ctype, holds string }{
		{css[1], "text/css", "background: transparent"},
		{subset[1], "text/javascript", "window.wsI18n"},
		{js[1], "text/javascript", "wbDocMermaid"},
	} {
		data, ctype, ok := wswhiteboard.FrameAsset(c.name)
		if !ok || !strings.HasPrefix(ctype, c.ctype) {
			t.Fatalf("FrameAsset(%s) = %v, %q", c.name, ok, ctype)
		}
		resp, err := testClient().Get(f.base + "/s/" + c.name)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != string(data) || !strings.Contains(string(body), c.holds) {
			t.Fatalf("%s = %d, %d bytes", c.name, resp.StatusCode, len(body))
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "public") {
			t.Errorf("%s Cache-Control = %q", c.name, cc)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, c.ctype) {
			t.Errorf("%s Content-Type = %q", c.name, got)
		}
	}
	if _, _, ok := wswhiteboard.FrameAsset("wb-frame-000000000000.css"); ok {
		t.Errorf("FrameAsset answers a name no asset carries")
	}

	lan := startPageFixture(t, lanMode)
	stranger := viewer{name: "foreign peer, no token", peer: "192.168.7.30:50000", host: "192.168.7.20:" + strconv.Itoa(lan.d.Endpoint().Port)}
	for _, name := range []string{css[1], subset[1], js[1]} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			if rec := serve(lan.d, stranger, method, "/s/"+name, ""); rec.Code != http.StatusOK || len(rec.Result().Cookies()) != 0 {
				t.Errorf("LAN %s %s without the token = %d", method, name, rec.Code)
			}
		}
		for _, target := range []struct{ method, path string }{
			{http.MethodPost, "/s/" + name},
			{http.MethodGet, "/s/livesession/" + name},
			{http.MethodGet, "/s/" + strings.Replace(name, "wb-frame-", "wb-frame-0", 1)},
		} {
			if rec := serve(lan.d, stranger, target.method, target.path, ""); !refusedByGuard(rec) {
				t.Errorf("LAN %s %s without the token = %d, want the guard's 401", target.method, target.path, rec.Code)
			}
		}
	}
}

func TestPagesRefuseANonLoopbackHost(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/", "/events", "/s/livesession", "/s/livesession/events"} {
		req, _ := http.NewRequest(http.MethodGet, f.base+path, nil)
		req.Host = "evil.example"
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s with a foreign Host = %d", path, resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("GET %s sent a CORS header", path)
		}
	}
}
