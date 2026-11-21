package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestLocalBrowseAnchorCannotBeGenerated(t *testing.T) {
	if !ValidReplyID(LocalBrowseAnchor) || !strings.ContainsAny(LocalBrowseAnchor, "0123456789") {
		t.Fatalf("anchor %q must be a valid registry name with a digit", LocalBrowseAnchor)
	}
	shape := regexp.MustCompile(`^[a-z]{12,36}$`)
	if shape.MatchString(LocalBrowseAnchor) {
		t.Fatal("anchor matches the reply-id generator's shape")
	}
	letters := regexp.MustCompile(`^[a-z]+$`)
	for _, word := range wskey.Words() {
		if !letters.MatchString(word) {
			t.Fatalf("generator word %q is not lowercase letters only", word)
		}
	}
	for _, seed := range []string{"", "test", "host0", "another-session"} {
		if id := wsmailbox.ReplyID(seed); !shape.MatchString(id) || id == LocalBrowseAnchor {
			t.Fatalf("ReplyID(%q) = %q", seed, id)
		}
	}
}

func TestLocalBrowseAnchorDispatchRefusesNonSessionRoutes(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	paths := []string{"", "/", "/?frame=1", "/files", "/fs", "/terminals-other", "/mailbox", "/web", "/unknown"}
	for _, route := range guardedRoutes() {
		if sub, ok := strings.CutPrefix(route.path, "/s/livesession"); ok && !strings.HasPrefix(sub, "/fs/") && !strings.HasPrefix(sub, "/terminals") {
			paths = append(paths, sub)
		}
	}
	for _, sub := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
			rec := serve(f.d, local, method, "/s/"+LocalBrowseAnchor+sub, `{}`)
			if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" || rec.Header().Get("Location") != "" {
				t.Errorf("%s anchor%s = %d %q, location %q; want bare 404", method, sub, rec.Code, rec.Body.String(), rec.Header().Get("Location"))
			}
		}
	}
}

func TestFSLocalBrowseAnchorGuardsAndReads(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)
	writeFile(t, filepath.Join(f.project, "art.svg"), "<svg>SOURCE-SVG</svg>")
	root := "root=" + url.QueryEscape(f.project)
	base := "/s/" + LocalBrowseAnchor + "/fs/"
	for _, route := range []string{"list", "preview?path=notes.txt", "raw?path=shot.png", "find?q=notes", "search?q=TEXT"} {
		sep := "?"
		if strings.Contains(route, "?") {
			sep = "&"
		}
		target := base + route + sep + root
		if rec := serve(f.d, local, http.MethodGet, target, ""); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "OUTSIDE") {
			t.Errorf("local %s = %d %s", route, rec.Code, rec.Body.String())
		}
		for _, v := range []viewer{lan, {name: "LAN Host", peer: local.peer, host: lan.host, cookie: lan.cookie}, {name: "remote peer, loopback Host", peer: lan.peer, host: local.host, cookie: lan.cookie}} {
			if rec := serve(f.d, v, http.MethodGet, target, ""); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", v.name, route, rec.Code)
			}
		}
		for _, headers := range []map[string]string{{"Sec-Fetch-Site": "cross-site"}, {"Sec-Fetch-Site": "same-site"}, {"Origin": "http://evil.example"}, {"Origin": "null"}} {
			if rec := termRequest(f.d, local.peer, local.host, http.MethodGet, target, "", headers); rec.Code != http.StatusNotFound {
				t.Errorf("%s with %v = %d, want 404", route, headers, rec.Code)
			}
		}
		for _, bad := range []string{"", "root=", "root=relative", "root=" + url.QueryEscape(filepath.Join(f.project, "missing")), "root=" + url.QueryEscape(filepath.Join(f.project, "notes.txt")), "root=" + url.QueryEscape(`\\server\share`)} {
			if rec := serve(f.d, local, http.MethodGet, base+route+sep+bad, ""); rec.Code != http.StatusNotFound {
				t.Errorf("%s with %q = %d, want 404", route, bad, rec.Code)
			}
		}
		if rec := serve(f.d, local, http.MethodPost, target, `{}`); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want read-only 405", route, rec.Code)
		}
	}
	var listing fsListAnswer
	rec := serve(f.d, local, http.MethodGet, base+"list?"+root, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil || listing.Session || listing.Root != filepath.ToSlash(f.project) {
		t.Fatalf("anchor listing = %+v (%v), want non-session root", listing, err)
	}
	for _, file := range []string{"docs/page.html", "art.svg"} {
		var preview fsPreviewAnswer
		rec := serve(f.d, local, http.MethodGet, base+"preview?"+root+"&path="+url.QueryEscape(file), "")
		if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil || rec.Code != http.StatusOK || preview.Kind != "text" || !preview.Source || preview.Frame != "" || preview.TTL != 0 {
			t.Errorf("anchor preview %s = %d %+v (%v), want source without capability", file, rec.Code, preview, err)
		}
	}
	for _, dir := range []string{"..", outside, "outlink", "outlink/sub"} {
		if !symlinks && strings.HasPrefix(dir, "outlink") {
			continue
		}
		if rec := serve(f.d, local, http.MethodGet, base+"list?"+root+"&dir="+url.QueryEscape(dir), ""); rec.Code != http.StatusOK {
			t.Errorf("anchor list %q = %d, want 200", dir, rec.Code)
		}
	}
	// The explicit root is a navigation base, not a local read boundary.
	paths := map[string]string{
		filepath.Join(outside, "secret.txt"): "TEXT-OUTSIDE",
		filepath.Join(outside, "secret.png"): "PNG-OUTSIDE",
	}
	for _, name := range []string{"secret.txt", "secret.png"} {
		rel, err := filepath.Rel(f.project, filepath.Join(outside, name))
		if err != nil {
			t.Fatal(err)
		}
		paths[rel] = paths[filepath.Join(outside, name)]
	}
	if symlinks {
		paths["alias.txt"] = "TEXT-INSIDE"
		paths["escape.txt"] = "TEXT-OUTSIDE"
		paths["outlink/secret.txt"] = "TEXT-OUTSIDE"
		paths["escape.png"] = "PNG-OUTSIDE"
		paths["outlink/secret.png"] = "PNG-OUTSIDE"
	}
	for path, body := range paths {
		var preview fsPreviewAnswer
		rec := serve(f.d, local, http.MethodGet, base+"preview?"+root+"&path="+url.QueryEscape(path), "")
		image := strings.HasSuffix(path, ".png")
		if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil || rec.Code != http.StatusOK ||
			(image && preview.Kind != "image") || (!image && (preview.Kind != "text" || preview.Text != body)) {
			t.Errorf("anchor preview %q = %d %+v (%v)", path, rec.Code, preview, err)
		}
		// Widened reads do not widen the raw route's media extensions.
		rec = serve(f.d, local, http.MethodGet, base+"raw?"+root+"&path="+url.QueryEscape(path), "")
		if image {
			if rec.Code != http.StatusOK || rec.Body.String() != body {
				t.Errorf("anchor raw %q = %d %q, want image bytes", path, rec.Code, rec.Body.String())
			}
		} else if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), body) {
			t.Errorf("anchor raw %q = %d %q, want 404", path, rec.Code, rec.Body.String())
		}
	}
	// No index is needed; the anchor cannot become session-capable even
	// when its explicit root is also a real session's project.
	r := httptest.NewRequest(http.MethodGet, "/?"+root, nil)
	r.RemoteAddr, r.Host = local.peer, local.host
	if got, session, ok := (&Daemon{}).browseRoot(r, LocalBrowseAnchor); !ok || session || got != f.project {
		t.Errorf("index-less anchor browseRoot = %q %v %v", got, session, ok)
	}
	if data, err := os.ReadFile(filepath.Join(f.project, "notes.txt")); err != nil || string(data) != "TEXT-INSIDE" {
		t.Errorf("read-only requests changed the fixture: %q (%v)", data, err)
	}
}

func TestFSActLocalBrowseAnchorRequiresRootAndTrust(t *testing.T) {
	host := &hostRecorder{}
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.hostFiles = host.host("linux", false) })
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)
	target := "/s/" + LocalBrowseAnchor + "/fs/act"
	for _, action := range []string{"open", "reveal"} {
		if rec := serve(f.d, local, http.MethodPost, target, actBody("notes.txt", action, f.project)); rec.Code != http.StatusNoContent || len(host.take()) != 1 {
			t.Errorf("anchor %s = %d, want one host command", action, rec.Code)
		}
	}
	bodies := []string{actBody("notes.txt", "open", ""), actBody("notes.txt", "open", "relative"), actBody("notes.txt", "delete", f.project), actBody(filepath.Join(outside, "secret.txt"), "open", f.project)}
	if symlinks {
		bodies = append(bodies, actBody("outlink/secret.txt", "open", f.project))
	}
	for _, body := range bodies {
		if rec := serve(f.d, local, http.MethodPost, target, body); rec.Code != http.StatusBadRequest || len(host.take()) != 0 {
			t.Errorf("anchor act %s = %d; want 400, no command", body, rec.Code)
		}
	}
	body := actBody("notes.txt", "open", f.project)
	if rec := serve(f.d, lan, http.MethodPost, target, body); rec.Code != http.StatusNotFound || len(host.take()) != 0 {
		t.Errorf("LAN anchor act = %d, want 404, no command", rec.Code)
	}
	for _, headers := range []map[string]string{nil, {"Origin": "http://evil.example"}, {"Origin": "null"}, {"Origin": "http://" + local.host, "Sec-Fetch-Site": "cross-site"}} {
		if rec := termRequest(f.d, local.peer, local.host, http.MethodPost, target, body, headers); rec.Code != http.StatusForbidden || len(host.take()) != 0 {
			t.Errorf("anchor act with %v = %d, want 403, no command", headers, rec.Code)
		}
	}
}

func TestLocalBrowseRootsRoute(t *testing.T) {
	f := startPageFixture(t, lanMode, withBrowseRootsFixture(t))
	local, lan := fsViewers(t, f)
	target := "/s/" + LocalBrowseAnchor + "/fs/roots"
	rec := serve(f.d, local, http.MethodGet, target, "")
	var roots browseRoots
	want := browseRoots{Host: "fixture-host", Home: "/fixture-home", Volumes: []string{"/", "/Volumes/mac"}}
	if err := json.Unmarshal(rec.Body.Bytes(), &roots); err != nil || rec.Code != http.StatusOK || !reflect.DeepEqual(roots, want) {
		t.Fatalf("discovery = %d %+v (%v), want %+v", rec.Code, roots, err, want)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("discovery must not be cached")
	}
	for _, v := range []viewer{lan, {peer: local.peer, host: lan.host, cookie: lan.cookie}, {peer: lan.peer, host: local.host, cookie: lan.cookie}} {
		if rec := serve(f.d, v, http.MethodGet, target, ""); rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
			t.Errorf("non-local discovery = %d %q, want bare 404", rec.Code, rec.Body.String())
		}
	}
	for _, headers := range []map[string]string{{"Sec-Fetch-Site": "cross-site"}, {"Sec-Fetch-Site": "same-site"}, {"Origin": "null"}, {"Origin": "http://evil.example"}} {
		if rec := termRequest(f.d, local.peer, local.host, http.MethodGet, target, "", headers); rec.Code != http.StatusNotFound {
			t.Errorf("discovery with %v = %d, want 404", headers, rec.Code)
		}
	}
	if rec := serve(f.d, local, http.MethodPost, target, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST discovery = %d, want 405", rec.Code)
	}
	if rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/roots", ""); rec.Code != http.StatusNotFound {
		t.Errorf("session discovery = %d, want 404", rec.Code)
	}
}

func withBrowseRootsFixture(t *testing.T) func(*DaemonOptions) {
	t.Helper()
	readDir := browseRootsFixture(t)
	return func(o *DaemonOptions) {
		o.browseRoots = func() browseRoots {
			return discoverBrowseRoots("darwin", "fixture-host", "/fixture-home", readDir)
		}
	}
}

// browseRootsFixture maps virtual OS paths to scratch directories and
// supplies deterministic permission errors.
func browseRootsFixture(t *testing.T) func(string) ([]os.DirEntry, error) {
	t.Helper()
	base := t.TempDir()
	for _, dir := range []string{"Volumes/mac", "Volumes/locked", "mnt/c", "mnt/linux", "mnt/locked", "media/user/usb", "run/media/user/disk", "driveC", "driveD", "driveZ"} {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(base, "Volumes", "not-a-volume.txt"), "file")
	return func(dir string) ([]os.DirEntry, error) {
		if strings.HasSuffix(dir, "/locked") || dir == `Z:\` {
			return nil, os.ErrPermission
		}
		if len(dir) == 3 && dir[1:] == `:\` {
			return os.ReadDir(filepath.Join(base, "drive"+dir[:1]))
		}
		return os.ReadDir(filepath.Join(base, filepath.FromSlash(strings.TrimPrefix(dir, "/"))))
	}
}

func TestLocalBrowseRootsEveryOSAndUnreadableVolumes(t *testing.T) {
	readDir := browseRootsFixture(t)
	for _, c := range []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"/", "/Volumes/mac"}},
		{"linux", []string{"/", "/media/user/usb", "/mnt/c", "/mnt/linux", "/run/media/user/disk"}},
		{"windows", []string{`C:\`, `D:\`}},
		{"freebsd", []string{"/"}},
	} {
		t.Run(c.goos, func(t *testing.T) {
			got := discoverBrowseRoots(c.goos, "fixture-host", "/fixture-home", readDir)
			if got.Host != "fixture-host" || got.Home != "/fixture-home" || !reflect.DeepEqual(got.Volumes, c.want) {
				t.Errorf("discovery = %+v, want %v", got, c.want)
			}
		})
	}
	t.Run("unreadable-root", func(t *testing.T) {
		got := discoverBrowseRoots("freebsd", "fixture-host", "/fixture-home", func(string) ([]os.DirEntry, error) {
			return nil, os.ErrPermission
		})
		if len(got.Volumes) != 0 {
			t.Errorf("discovery offers an unreadable root: %v", got.Volumes)
		}
	})
}
