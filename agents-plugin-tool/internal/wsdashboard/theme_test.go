package wsdashboard

import (
	"bufio"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// getTheme fetches url as a browser with the given theme cookie value (empty
// sends none).
func getTheme(t *testing.T, url, cookie string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: themeCookie, Value: cookie})
	}
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// The cookie is the only input: light and dark reach the root element from
// the first byte; auto and anything else leave the root without the attribute.
func TestPagesRenderTheThemeCookieOnTheRoot(t *testing.T) {
	f := startPageFixture(t)
	writeFile(t, f.project+"/note.md", "# hello\n")
	writeFile(t, f.project+"/blob.bin", "\x00\x01\x02")
	for _, page := range []struct{ name, path string }{
		{"session list", "/"},
		{"board-first session", "/s/boardsession"},
		{"adapter session", "/s/livesession"},
		{"markdown file view", "/s/livesession/file?path=note.md"},
		{"not-viewable file view", "/s/livesession/file?path=blob.bin"},
	} {
		for _, c := range []struct{ cookie, want string }{
			{"light", `data-theme="light"`},
			{"dark", `data-theme="dark"`},
			{"", ""},
			{"auto", ""},
			{"Dark", ""},
			{`"><script>`, ""},
		} {
			code, body := getTheme(t, f.base+page.path, c.cookie)
			if code != http.StatusOK {
				t.Fatalf("%s: GET = %d", page.name, code)
			}
			root := body[strings.Index(body, "<html"):]
			root = root[:strings.Index(root, ">")]
			if c.want == "" {
				if strings.Contains(root, "data-theme") {
					t.Errorf("%s, cookie %q: root %q pins a theme", page.name, c.cookie, root)
				}
			} else if !strings.Contains(root, c.want) {
				t.Errorf("%s, cookie %q: root %q lacks %s", page.name, c.cookie, root, c.want)
			}
		}
	}
}

// frameRoots returns the root element (<html ...>) of every topic-body
// frame's srcdoc document in page, as a browser parses the attribute.
func frameRoots(t *testing.T, page string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`<iframe class="doc-frame"[^>]* srcdoc="([^"]*)"`).FindAllStringSubmatch(page, -1) {
		doc := html.UnescapeString(m[1])
		i := strings.Index(doc, "<html")
		if i < 0 {
			t.Fatalf("frame document without a root: %.200q", doc)
		}
		out = append(out, doc[i:i+strings.Index(doc[i:], ">")+1])
	}
	if len(out) == 0 {
		t.Fatalf("no topic-body frame in the page:\n%.400s", page)
	}
	return out
}

// A topic-body frame is its own document: it pins the page's theme from the
// first byte, on the page and on the session stream, or its body text would
// take the OS palette over the pinned card.
func TestTopicFramesCarryThePinnedTheme(t *testing.T) {
	f := startPageFixture(t)
	for _, c := range []struct{ cookie, want string }{
		{"light", `<html data-theme="light">`},
		{"dark", `<html data-theme="dark">`},
		{"", `<html>`},
		{"auto", `<html>`},
	} {
		for _, path := range []string{"/s/boardsession", "/s/livesession"} {
			_, body := getTheme(t, f.base+path, c.cookie)
			for _, root := range frameRoots(t, body) {
				if root != c.want {
					t.Errorf("%s, cookie %q: frame root %q, want %q", path, c.cookie, root, c.want)
				}
			}
		}

		req, _ := http.NewRequest(http.MethodGet, f.base+"/s/livesession/events", nil)
		if c.cookie != "" {
			req.AddCookie(&http.Cookie{Name: themeCookie, Value: c.cookie})
		}
		resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		events := make(chan sseEvent, 16)
		go readEvents(t, bufio.NewReader(resp.Body), events)
		board := ""
		for board == "" {
			ev, ok := nextRegion(t, events, 2*time.Second)
			if !ok {
				t.Fatalf("cookie %q: the stream sent no board region", c.cookie)
			}
			if ev.Name == "board" {
				board = ev.HTML
			}
		}
		resp.Body.Close()
		for _, root := range frameRoots(t, board) {
			if root != c.want {
				t.Errorf("stream, cookie %q: frame root %q, want %q", c.cookie, root, c.want)
			}
		}
	}
}

// The gear holds the theme section on every page that has it, the list and
// board-first pages included, in every language.
func TestGearHoldsTheThemeSection(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/", "/s/boardsession", "/s/livesession"} {
		for _, l := range wsi18n.Languages() {
			_, body := getLang(t, f.base+path, l.Code, "")
			for _, key := range []string{"gear.theme", "gear.theme.auto", "gear.theme.light", "gear.theme.dark"} {
				if !strings.Contains(body, wsi18n.T(l.Code, key)) {
					t.Errorf("%s (%s): no %s text", path, l.Code, key)
				}
			}
			for _, item := range []string{`data-theme-set=""`, `data-theme-set="light"`, `data-theme-set="dark"`} {
				if !strings.Contains(body, item) {
					t.Errorf("%s (%s): no %s item", path, l.Code, item)
				}
			}
		}
	}
}

// Every table carries the theme keys: a translated gear, not an English
// fallback, in each language the menu offers.
func TestEveryTableTranslatesTheThemeKeys(t *testing.T) {
	en := wsi18n.Table(wsi18n.Default)
	for _, key := range []string{"gear.theme", "gear.theme.auto", "gear.theme.auto.sub", "gear.theme.light", "gear.theme.dark", "gear.title", "gear.title.lang"} {
		if en[key] == "" {
			t.Fatalf("English table lacks %s", key)
		}
		for _, l := range wsi18n.Languages() {
			if l.Code != wsi18n.Default && wsi18n.Table(l.Code)[key] == "" {
				t.Errorf("%s: no %s", l.Code, key)
			}
		}
	}
}
