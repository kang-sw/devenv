package wsdashboard

import (
	"encoding/json"
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
	for _, name := range []string{"dashboard.js", "dashboard.css", "dashboard.tmpl", "rail.js", "tracker.css", "memo.js", "memo.css"} {
		if build(name) == base {
			t.Errorf("an edit to %s keeps the identity", name)
		}
	}
}

// TestPagesAndStreamsCarryTheAssetID pins the delivery: both page kinds that
// inline dashboard.js and hold a stream carry the identity on their root,
// and each stream's first event names the same one, on every connect.
func TestPagesAndStreamsCarryTheAssetID(t *testing.T) {
	f := startPageFixture(t)
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ page, stream string }{{"/s/livesession", "/s/livesession/events"}, {"/", "/events"}} {
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
	_, page := get(t, f.base+"/s/livesession")
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
	_, page = get(t, f.base+"/s/livesession")
	if !strings.Contains(page, "/*devid*/") || dataAssets(t, page) != stream {
		t.Errorf("the reloaded dev page carries %q, the stream %q", dataAssets(t, page), stream)
	}
}
