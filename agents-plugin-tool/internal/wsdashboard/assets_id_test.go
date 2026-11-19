package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dataAssets is the page root's data-assets value; empty when it has none.
func dataAssets(t *testing.T, page string) string {
	t.Helper()
	m := regexp.MustCompile(`<html [^>]*data-assets="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	return m[1]
}

// firstAssets reads a stream's first event, which must be "assets", and
// returns its id.
func firstAssets(t *testing.T, url string) string {
	t.Helper()
	ev, ok := <-openSSE(t, url)
	if !ok {
		t.Fatalf("stream %s closed before its first event", url)
	}
	if ev.name != "assets" {
		t.Fatalf("stream %s sent %q first, want assets", url, ev.name)
	}
	var got assetsEvent
	if err := json.Unmarshal([]byte(ev.data), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID
}

// TestAssetIDFollowsAssetContent pins the identity's input: the same assets
// give the same identity on every build, and a changed script, stylesheet,
// or template gives another.
func TestAssetIDFollowsAssetContent(t *testing.T) {
	build := func(edit string) string {
		t.Helper()
		a, err := buildDashboardAssets(func(name string) ([]byte, error) {
			raw, err := embeddedDashboardAsset(name)
			if name == edit {
				raw = append(append([]byte{}, raw...), "\n/*edit*/"...)
			}
			return raw, err
		})
		if err != nil {
			t.Fatal(err)
		}
		return a.data.ID
	}
	base := build("")
	if base == "" || build("") != base {
		t.Fatalf("the same assets gave identities %q and %q", base, build(""))
	}
	if a, _ := loadDashboardAssets(); a.data.ID != base {
		t.Errorf("the embedded assets' identity %q differs from a rebuild's %q", a.data.ID, base)
	}
	names := append([]string{}, dashboardScriptFiles...)
	names = append(names, dashboardStyleFiles...)
	names = append(names, "dashboard.tmpl", "rail.js", "shell.js", "tracker.css", "memo.js", "memo.css")
	for _, name := range names {
		if build(name) == base {
			t.Errorf("an edit to %s keeps the identity", name)
		}
	}
}

func TestDevAssetsIDFollowsEverySourceFragment(t *testing.T) {
	dash, wb := devSource(t)
	base, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, files := range [][]string{dashboardScriptFiles, dashboardStyleFiles} {
		for _, name := range files {
			paths = append(paths, dash(name))
		}
	}
	whiteboard, err := filepath.Glob(wb("page*.css"))
	if err != nil || len(whiteboard) == 0 {
		t.Fatalf("whiteboard stylesheet sources: %v, %v", whiteboard, err)
	}
	paths = append(paths, whiteboard...)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatal(err)
				}
			}()
			if err := os.WriteFile(path, append(append([]byte{}, raw...), "\n/*fragment edit*/"...), 0o644); err != nil {
				t.Fatal(err)
			}
			a, err := loadDashboardAssets()
			if err != nil {
				t.Fatal(err)
			}
			if a.data.ID == base.data.ID {
				t.Errorf("an edit to %s keeps the identity", path)
			}
		})
	}
}

// A whiteboard fragment is shared by the inline page and the external frame
// stylesheet. Its content must reach both the reload identity and the URL.
func TestWhiteboardFragmentChangesFrameAsset(t *testing.T) {
	_, wb := devSource(t)
	f := startPageFixture(t)
	base, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(wb("page*.css"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("whiteboard stylesheet sources: %v, %v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Error(err)
				}
			})
			const marker = "\n/* whiteboard cache identity regression */\n"
			if err := os.WriteFile(path, append(append([]byte(nil), raw...), marker...), 0o644); err != nil {
				t.Fatal(err)
			}
			changed, err := loadDashboardAssets()
			if err != nil {
				t.Fatal(err)
			}
			name := changed.data.FrameWhiteboardFile
			if changed.data.ID == base.data.ID || name == base.data.FrameWhiteboardFile {
				t.Fatalf("whiteboard edit did not change asset ID and frame URL: %q, %q", changed.data.ID, name)
			}
			css := string(changed.data.WhiteboardCSS)
			sum := sha256.Sum256([]byte(css))
			if want := "dashboard-" + hex.EncodeToString(sum[:12]) + ".css"; name != want || !strings.Contains(css, marker) {
				t.Fatalf("frame stylesheet name/content = %q; want %q and marker", name, want)
			}
			request := func(target string) *http.Request {
				r := authRequest(http.MethodGet, target, "")
				r.Host = fmt.Sprintf("127.0.0.1:%d", f.d.Endpoint().Port)
				r.AddCookie(authCookie(t, f.d))
				return r
			}
			page := authServe(f.d, request("/s/livesession?frame=1"))
			if page.Code != http.StatusOK || dataAssets(t, page.Body.String()) != changed.data.ID || !strings.Contains(page.Body.String(), `<link rel="stylesheet" href="/s/`+name+`">`) {
				t.Fatalf("frame did not reference updated identity and stylesheet: status %d", page.Code)
			}
			asset := authServe(f.d, request("/s/"+name))
			if asset.Code != http.StatusOK || asset.Body.String() != css || asset.Header().Get("Content-Type") != "text/css; charset=utf-8" || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || asset.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("frame stylesheet response: status %d, headers %v", asset.Code, asset.Header())
			}
		})
	}
}

// TestPagesAndStreamsCarryTheAssetID pins the delivery: every page kind that
// inlines dashboard.js and holds a stream (the iframe shell, the session
// page it frames, the index) carries the identity on its root, and each
// stream's first event names the same one, on every connect.
func TestPagesAndStreamsCarryTheAssetID(t *testing.T) {
	f := startPageFixture(t)
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ page, stream string }{{"/s/livesession", railEventsRoute}, {"/s/livesession?frame=1", "/s/livesession/events?frame=1"}, {"/", "/events"}} {
		_, body := get(t, f.base+c.page)
		if got := dataAssets(t, body); got != a.data.ID {
			t.Errorf("page %s carries data-assets %q, want %q", c.page, got, a.data.ID)
		}
		for range 2 {
			if got := firstAssets(t, f.base+c.stream); got != a.data.ID {
				t.Errorf("stream %s names assets %q first, want %q", c.stream, got, a.data.ID)
			}
		}
	}
	// A stream that names its regions (the support chat) still names the
	// assets first; its page ignores the event.
	if got := firstAssets(t, f.base+"/s/livesession/events?regions=entries"); got != a.data.ID {
		t.Errorf("a region-filtered stream names assets %q first, want %q", got, a.data.ID)
	}
}

// TestDevAssetsIDFollowsTheServedFiles pins dev assets: the identity follows
// the files on disk, so a page and the stream it opens agree while nothing
// changed (no reload) and disagree once a file did (one reload).
func TestDevAssetsIDFollowsTheServedFiles(t *testing.T) {
	dash, _ := devSource(t)
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession?frame=1")
	before := dataAssets(t, page)
	if before == "" {
		t.Fatal("the dev page carries no data-assets")
	}
	if got := firstAssets(t, f.base+"/s/livesession/events"); got != before {
		t.Fatalf("unchanged dev files: the stream names %q, the page %q", got, before)
	}
	editFile(t, dash("dashboard.js"), "(function", "/*devid*/(function")
	stream := firstAssets(t, f.base+"/s/livesession/events")
	if stream == before {
		t.Fatal("a script edit keeps the dev identity")
	}
	_, page = get(t, f.base+"/s/livesession?frame=1")
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	_, script := get(t, f.base+"/s/"+a.data.ScriptFile)
	if !strings.Contains(page, a.data.ScriptFile) || !strings.Contains(script, "/*devid*/") || dataAssets(t, page) != stream {
		t.Errorf("the reloaded dev page carries %q, the stream %q", dataAssets(t, page), stream)
	}
}
