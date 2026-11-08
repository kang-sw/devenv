package wsdashboard

import (
	"io"
	"net/http"
	"strings"
	"testing"

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
