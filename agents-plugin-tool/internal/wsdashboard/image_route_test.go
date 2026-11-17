package wsdashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestImageRouteLocalPathsAndLANConfinement pins local traversal and
// symlink reads and unchanged LAN confinement. Both requested and resolved
// filenames must have image extensions; directories and missing files fail.
func TestImageRouteLocalPathsAndLANConfinement(t *testing.T) {
	f := startPageFixture(t, lanMode)
	_, lan := fsViewers(t, f)
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-INSIDE")
	writeFile(t, filepath.Join(f.project, "docs", "deep", "b.JPG"), "JPG-INSIDE")
	writeFile(t, filepath.Join(f.project, "notes.txt"), "TEXT-INSIDE")
	writeFile(t, filepath.Join(outside, "secret.png"), "PNG-OUTSIDE")
	if err := os.MkdirAll(filepath.Join(f.project, "dir.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinks := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(f.project, "escape.png")) == nil &&
		os.Symlink(filepath.Join(outside), filepath.Join(f.project, "outlink")) == nil &&
		os.Symlink(filepath.Join(f.project, "notes.txt"), filepath.Join(f.project, "renamed.png")) == nil &&
		os.Symlink(filepath.Join(f.project, "shot.png"), filepath.Join(f.project, "alias.png")) == nil &&
		os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(f.project, "image.txt")) == nil

	image := func(path string) (int, http.Header, string) {
		t.Helper()
		resp, err := testClient().Get(f.base + "/s/livesession/image?path=" + url.QueryEscape(path))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw := make([]byte, 64)
		n, _ := resp.Body.Read(raw)
		return resp.StatusCode, resp.Header, string(raw[:n])
	}

	served := map[string]string{
		"shot.png":         "PNG-INSIDE",
		"./shot.png":       "PNG-INSIDE",
		"docs/deep/b.JPG":  "JPG-INSIDE",
		"docs/../shot.png": "PNG-INSIDE",
		filepath.ToSlash(filepath.Join(f.project, "shot.png")): "PNG-INSIDE",
		filepath.Join(f.project, "docs", "deep", "b.JPG"):      "JPG-INSIDE",
	}
	if symlinks {
		served["alias.png"] = "PNG-INSIDE" // a symlink that stays inside
	}
	widened := map[string]string{filepath.Join(outside, "secret.png"): "PNG-OUTSIDE"}
	rel, err := filepath.Rel(f.project, filepath.Join(outside, "secret.png"))
	if err != nil {
		t.Fatal(err)
	}
	widened[rel] = "PNG-OUTSIDE"
	if symlinks {
		widened["escape.png"] = "PNG-OUTSIDE"
		widened["outlink/secret.png"] = "PNG-OUTSIDE"
	}
	for path, want := range widened {
		served[path] = want
		if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/image?path="+url.QueryEscape(path), ""); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), want) {
			t.Errorf("LAN image %q = %d %q, want 404", path, rec.Code, rec.Body.String())
		}
		if rec := imageAs(f.d, "127.0.0.1:50000", "127.0.0.1:"+strconv.Itoa(f.d.Endpoint().Port), "", "cross-site", path); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), want) {
			t.Errorf("cross-site local image %q = %d %q, want 404", path, rec.Code, rec.Body.String())
		}
	}
	for path, want := range served {
		code, h, body := image(path)
		if code != http.StatusOK || body != want {
			t.Errorf("image %q = %d %q, want 200 %q", path, code, body, want)
			continue
		}
		if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
			t.Errorf("image %q content type %q", path, ct)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("image %q headers %v", path, h)
		}
		assertNoCORS(t, h)
	}

	refused := []string{
		"",
		"notes.txt",
		"dir.png",
		"missing.png",
		".",
		"shot.png\x00.png",
	}
	if symlinks {
		refused = append(refused, "renamed.png", "image.txt")
	}
	for _, path := range refused {
		code, _, body := image(path)
		if code != http.StatusNotFound {
			t.Errorf("image %q = %d, want 404", path, code)
		}
		if strings.Contains(body, "OUTSIDE") || strings.Contains(body, "TEXT-INSIDE") {
			t.Errorf("image %q leaked %q", path, body)
		}
	}
	if code, _, _ := image("shot.png"); code != http.StatusOK {
		t.Fatal("control request failed")
	}
	// An unknown session has no root to serve from.
	if code, _ := get(t, f.base+"/s/nosuchsession/image?path=shot.png"); code != http.StatusNotFound {
		t.Errorf("image of an unknown session = %d, want 404", code)
	}
	// The route is read-only.
	if code, _, _ := postJSON(t, f.base+"/s/livesession/image?path=shot.png", f.base, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("POST image = %d, want 405", code)
	}
}

// TestImageSrcMapsBoardImagesToTheRoute pins the served page's image URL: a
// path inside the root, relative or absolute, maps to the route with its
// root-relative path; a path the route would refuse by its name maps to
// nothing, so the page shows it as text.
func TestImageSrcMapsBoardImagesToTheRoute(t *testing.T) {
	root := t.TempDir()
	for in, want := range map[string]string{
		"shots/a.png":   "/s/rid/image?path=shots%2Fa.png",
		"./a/../b.webp": "/s/rid/image?path=b.webp",
		filepath.ToSlash(filepath.Join(root, "docs", "c d.svg")): "/s/rid/image?path=docs%2Fc+d.svg",
		"../elsewhere/x.png": "",
		filepath.ToSlash(filepath.Join(filepath.Dir(root), "x.png")): "",
		"notes.md": "",
		"":         "",
	} {
		if got := ImageSrc(root, "rid", in); got != want {
			t.Errorf("ImageSrc(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ImageSrc(root, "Not/A-Reply-ID", "a.png"); got != "" {
		t.Errorf("ImageSrc with an invalid reply-id = %q", got)
	}
	if got := ImageSrc("", "rid", "a.png"); got != "" {
		t.Errorf("ImageSrc without a root = %q", got)
	}
}

// TestImageRouteAccessRule pins who the image route serves which image: a
// LAN viewer only an image inside the session root, never one outside it
// (by absolute or relative path) nor an attachment by its path; a loopback
// viewer an image inside or outside the root and a session's attachment
// image by its absolute path; and neither a credential location, under an
// image name too.
func TestImageRouteAccessRule(t *testing.T) {
	o := startOutsideFixture(t)
	writeFile(t, filepath.Join(o.project, "shots", "in.png"), "PNG-INSIDE")
	out := o.write(t, o.away, "out.png", "PNG-OUTSIDE")
	key := o.write(t, o.home, ".ssh/key.png", "PNG-CREDENTIAL")
	attach := filepath.Join(wsinbox.AttachmentsDir(sessionInbox(o.dir, "livesession")), "0123456789abcdef-1.png")
	writeFile(t, attach, "PNG-ATTACHMENT")
	rel, err := filepath.Rel(o.project, out)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path, want string
		v                viewer
	}{
		{"LAN in-root", "shots/in.png", "PNG-INSIDE", o.lan},
		{"LAN in-root, absolute", filepath.ToSlash(filepath.Join(o.project, "shots", "in.png")), "PNG-INSIDE", o.lan},
		{"LAN out-of-root", filepath.ToSlash(out), "", o.lan},
		{"LAN out-of-root, relative", filepath.ToSlash(rel), "", o.lan},
		{"LAN attachment", filepath.ToSlash(attach), "", o.lan},
		{"LAN credential", filepath.ToSlash(key), "", o.lan},
		{"LAN home path", "~/.ssh/key.png", "", o.lan},
		{"LAN without the token", "shots/in.png", "", o.bare},
		{"loopback in-root", "shots/in.png", "PNG-INSIDE", o.local},
		{"loopback out-of-root", filepath.ToSlash(out), "PNG-OUTSIDE", o.local},
		{"loopback out-of-root, relative", filepath.ToSlash(rel), "PNG-OUTSIDE", o.local},
		{"loopback attachment", filepath.ToSlash(attach), "PNG-ATTACHMENT", o.local},
		{"loopback attachment, native", attach, "PNG-ATTACHMENT", o.local},
		{"loopback credential", filepath.ToSlash(key), "", o.local},
		{"loopback credential, home path", "~/.ssh/key.png", "", o.local},
	} {
		rec := serve(o.d, c.v, http.MethodGet, "/s/livesession/image?path="+url.QueryEscape(c.path), "")
		if c.want != "" {
			if rec.Code != http.StatusOK || rec.Body.String() != c.want || rec.Header().Get("Content-Type") != "image/png" {
				t.Errorf("%s: %d %q %v, want 200 %q", c.name, rec.Code, rec.Body.String(), rec.Header(), c.want)
			}
			continue
		}
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "PNG-") {
			t.Errorf("%s: %d %q, want a refusal", c.name, rec.Code, rec.Body.String())
		}
	}
}

// TestImageRouteAttachmentCarveOutKeepsTheStateDirDenied pins the attachment
// exception against the real deny set, with the daemon's state directory at
// its default place under the home, which outsideDenied refuses whole: an
// attachment there opens for a loopback viewer, while another file of the
// state directory does not, whatever image name or path spelling reaches it,
// and a symlink with an attachment's name is refused.
func TestImageRouteAttachmentCarveOutKeepsTheStateDirDenied(t *testing.T) {
	o := startOutsideFixture(t)
	d := &Daemon{dir: filepath.Join(o.home, ".cache", wsstate.DefaultCacheDirName, "dashboard")}
	attachDir := wsinbox.AttachmentsDir(sessionInbox(d.dir, "livesession"))
	attach := o.write(t, attachDir, "0123456789abcdef-1.png", "PNG-ATTACHMENT")
	o.write(t, d.dir, "secret.png", "PNG-STATE")
	o.write(t, attachDir, "secret.png", "PNG-STATE")
	f, ctype, _, ok := d.openImage(o.project, filepath.ToSlash(attach), true)
	if !ok || ctype != "image/png" {
		t.Fatalf("attachment = %v %q", ok, ctype)
	}
	f.Close()
	if f, _, _, ok := d.openImage(o.project, filepath.ToSlash(attach), false); ok {
		f.Close()
		t.Error("an attachment opened for a LAN viewer")
	}
	refused := []string{
		filepath.Join(d.dir, "secret.png"),
		filepath.Join(attachDir, "secret.png"),
	}
	// Unjoined, so the route gets the dot-dot steps uncleaned.
	paths := []string{filepath.ToSlash(attach) + "/../../../../../secret.png", filepath.ToSlash(attachDir) + "/../attachments/secret.png"}
	if os.Symlink(filepath.Join(d.dir, "secret.png"), filepath.Join(attachDir, "fedcba9876543210-1.png")) == nil {
		refused = append(refused, filepath.Join(attachDir, "fedcba9876543210-1.png"))
	}
	for _, path := range refused {
		paths = append(paths, filepath.ToSlash(path))
	}
	for _, path := range paths {
		if f, _, _, ok := d.openImage(o.project, path, true); ok {
			f.Close()
			t.Errorf("state-directory file %q opened", path)
		}
	}
}

// TestImageRouteCachesByVersion pins the route's cache headers: the ETag is
// the file's ImageVersion; a URL whose &v= is that version is cached as
// immutable, any other is revalidated; a matching If-None-Match answers 304;
// an edit changes the version.
func TestImageRouteCachesByVersion(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	file := filepath.Join(f.project, "a.png")
	writeFile(t, file, "PNG-A")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	version := ImageVersion(info)
	for _, v := range []viewer{local, lan} {
		target := "/s/livesession/image?path=a.png"
		rec := serve(f.d, v, http.MethodGet, target, "")
		if rec.Code != http.StatusOK || rec.Header().Get("ETag") != `"`+version+`"` || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s unversioned: %d %v", v.name, rec.Code, rec.Header())
		}
		rec = serve(f.d, v, http.MethodGet, target+"&v="+version, "")
		if rec.Code != http.StatusOK || rec.Body.String() != "PNG-A" || rec.Header().Get("Cache-Control") != imageCacheImmutable {
			t.Errorf("%s current version: %d %v", v.name, rec.Code, rec.Header())
		}
		rec = serve(f.d, v, http.MethodGet, target+"&v=stale.1", "")
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s stale version: %d %v", v.name, rec.Code, rec.Header())
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr, req.Host = v.peer, v.host
		req.Header.Set("If-None-Match", `"`+version+`"`)
		if v.cookie != "" {
			req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
		}
		if v.localCookie {
			addLocalhostTestCookie(t, f.d, req)
		}
		rec = httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
			t.Errorf("%s If-None-Match: %d %q", v.name, rec.Code, rec.Body.String())
		}
	}
	writeFile(t, file, "PNG-AB")
	if info, err := os.Stat(file); err != nil || ImageVersion(info) == version {
		t.Fatalf("an edit kept the version %q (%v)", version, err)
	}
}

// TestImageRouteMissBuildsNoDenySet pins a miss's cost: a loopback viewer's
// missing image, in the root or outside it, and an attachment, found or
// missing, build no deny set; an outside image that exists still does.
func TestImageRouteMissBuildsNoDenySet(t *testing.T) {
	o := startOutsideFixture(t)
	out := o.write(t, o.away, "out.png", "PNG-OUTSIDE")
	attachDir := wsinbox.AttachmentsDir(sessionInbox(o.dir, "livesession"))
	attach := o.write(t, attachDir, "0123456789abcdef-1.png", "PNG-ATTACHMENT")
	builds := func(path string, want int) int64 {
		t.Helper()
		before := deniedBasePathBuilds.Load()
		rec := serve(o.d, o.local, http.MethodGet, "/s/livesession/image?path="+url.QueryEscape(path), "")
		if rec.Code != want {
			t.Errorf("image %q = %d, want %d", path, rec.Code, want)
		}
		return deniedBasePathBuilds.Load() - before
	}
	for _, c := range []struct {
		path string
		want int
	}{
		{"missing.png", http.StatusNotFound},
		{filepath.ToSlash(filepath.Join(o.away, "missing.png")), http.StatusNotFound},
		{"~/missing.png", http.StatusNotFound},
		{filepath.ToSlash(attach), http.StatusOK},
		{filepath.ToSlash(filepath.Join(attachDir, "fedcba9876543210-1.png")), http.StatusNotFound},
	} {
		if got := builds(c.path, c.want); got != 0 {
			t.Errorf("image %q built %d deny sets, want 0", c.path, got)
		}
	}
	if got := builds(filepath.ToSlash(out), http.StatusOK); got != 1 {
		t.Errorf("an existing outside image built %d deny sets, want 1", got)
	}
}

// imageAs fetches the image route for path as a request from peer to host,
// with the LAN token cookie (when set) and Sec-Fetch-Site (when set).
func imageAs(d *Daemon, peer, host, cookie, fetchSite, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/s/livesession/image?path="+url.QueryEscape(path), nil)
	req.RemoteAddr, req.Host = peer, host
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: cookie})
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	rec := httptest.NewRecorder()
	localhostTestHandler(nil, d).ServeHTTP(rec, req)
	return rec
}

// TestImageRouteServesOutsideImagesToLoopbackViewersOnly pins the route's
// local widening: a browser on this machine is served an image outside the
// root by absolute or relative path, symlinks followed to a file that is an image
// too, under the image CSP with Cross-Origin-Resource-Policy: same-origin.
// A request the user started (Sec-Fetch-Site: none) is served too. A LAN
// viewer with the token, a request another site started (Sec-Fetch-Site
// same-site or cross-site), a non-image, a
// directory, a missing file, a symlink to a non-image,
// and a UNC or device path (refused on its spelling, even where it names a
// real file) all get the uniform 404. In-root images share the fetch-site
// check and CORP header.
func TestImageRouteServesOutsideImagesToLoopbackViewersOnly(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local, lanHost := "127.0.0.1:"+port, "192.168.7.20:"+port
	const peer, lanPeer = "127.0.0.1:50000", "192.168.7.30:50000"
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.png")
	writeFile(t, secret, "PNG-OUTSIDE")
	writeFile(t, filepath.Join(outside, "notes.txt"), "TEXT-OUTSIDE")
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-INSIDE")
	if err := os.MkdirAll(filepath.Join(outside, "dir.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinks := os.Symlink(secret, filepath.Join(outside, "alias.png")) == nil &&
		os.Symlink(filepath.Join(outside, "notes.txt"), filepath.Join(outside, "renamed.png")) == nil

	rel, err := filepath.Rel(f.project, secret)
	if err != nil {
		t.Fatal(err)
	}
	served := []string{secret, filepath.ToSlash(secret), rel, filepath.ToSlash(rel)}
	if symlinks {
		served = append(served, filepath.Join(outside, "alias.png"))
	}
	for _, path := range served {
		for _, site := range []string{"", "same-origin", "none"} {
			r := imageAs(f.d, peer, local, "", site, path)
			h := r.Header()
			if r.Code != http.StatusOK || r.Body.String() != "PNG-OUTSIDE" {
				t.Errorf("outside image %q (Sec-Fetch-Site %q) = %d %q, want 200", path, site, r.Code, r.Body.String())
				continue
			}
			if h.Get("Content-Type") != "image/png" || h.Get("Content-Security-Policy") != imageContentSecurityPolicy ||
				h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cross-Origin-Resource-Policy") != "same-origin" {
				t.Errorf("outside image %q headers %v", path, h)
			}
			assertNoCORS(t, h)
		}
	}

	refused := map[string]*httptest.ResponseRecorder{
		"LAN peer with the token":           imageAs(f.d, lanPeer, lanHost, token, "", secret),
		"loopback peer, LAN Host, token":    imageAs(f.d, peer, lanHost, token, "", secret),
		"cross-site fetch":                  imageAs(f.d, peer, local, "", "cross-site", secret),
		"same-site fetch":                   imageAs(f.d, peer, local, "", "same-site", secret),
		"non-image":                         imageAs(f.d, peer, local, "", "", filepath.Join(outside, "notes.txt")),
		"directory":                         imageAs(f.d, peer, local, "", "", filepath.Join(outside, "dir.png")),
		"missing":                           imageAs(f.d, peer, local, "", "", filepath.Join(outside, "missing.png")),
		"LAN relative escape":               imageAs(f.d, lanPeer, lanHost, token, "", rel),
		"cross-site relative escape":        imageAs(f.d, peer, local, "", "cross-site", rel),
		"double-slash spelling of the file": imageAs(f.d, peer, local, "", "", "/"+filepath.ToSlash(secret)),
		"UNC":                               imageAs(f.d, peer, local, "", "", `\\server\share\x.png`),
		"UNC, forward slashes":              imageAs(f.d, peer, local, "", "", "//server/share/x.png"),
		"device path":                       imageAs(f.d, peer, local, "", "", `\\?\C:\x.png`),
		"device path, forward slashes":      imageAs(f.d, peer, local, "", "", "//?/C:/x.png"),
		"dot device path":                   imageAs(f.d, peer, local, "", "", `\\.\x.png`),
		"NT namespace path":                 imageAs(f.d, peer, local, "", "", `\??\UNC\s\sh\x.png`),
		"NT namespace, forward slashes":     imageAs(f.d, peer, local, "", "", "/??/UNC/s/sh/x.png"),
	}
	if symlinks {
		refused["symlink to a non-image"] = imageAs(f.d, peer, local, "", "", filepath.Join(outside, "renamed.png"))
	}
	for name, r := range refused {
		if r.Code != http.StatusNotFound || strings.Contains(r.Body.String(), "OUTSIDE") {
			t.Errorf("%s = %d %q, want 404", name, r.Code, r.Body.String())
		}
		if r.Header().Get("Cross-Origin-Resource-Policy") != "" {
			t.Errorf("%s carries CORP", name)
		}
	}

	in := imageAs(f.d, lanPeer, lanHost, token, "cross-site", "shot.png")
	if in.Code != http.StatusNotFound || strings.Contains(in.Body.String(), "PNG-INSIDE") {
		t.Errorf("in-root image to a LAN viewer, cross-site = %d %q %v", in.Code, in.Body.String(), in.Header())
	}
}

// TestNetworkPathRefusesUNCAndDevicePaths pins the pure string check that
// refuses a Windows UNC or device path in either slash direction before the
// filesystem is touched.
func TestNetworkPathRefusesUNCAndDevicePaths(t *testing.T) {
	for path, want := range map[string]bool{
		`\\server\share\x.png`: true, "//server/share/x.png": true, `\\?\C:\x.png`: true, "//?/C:/x.png": true,
		`\\.\PhysicalDrive0`: true, "//./x.png": true, `/\server\x.png`: true, `\/server/x.png`: true,
		`\??\C:\x.png`: true, `\??\UNC\s\sh\x.png`: true, "/??/UNC/s/sh/x.png": true, "/??x.png": false,
		"/tmp/x.png": false, `C:\x.png`: false, "C:/x.png": false, `\x.png`: false, "x.png": false, "": false, "/": false,
	} {
		if got := networkPath(path); got != want {
			t.Errorf("networkPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestOutsideImageMapsAbsoluteImagesOutsideTheRoot pins the served page's
// rule for an image outside the root: a path written absolute with an image
// extension, outside the root, gets the image route URL of its absolute path
// with the image and outside marks; a relative path that leaves the root, a
// path inside the root, a non-image, a UNC path, and an invalid reply-id get
// nothing.
func TestOutsideImageMapsAbsoluteImagesOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(filepath.Dir(root), "elsewhere", "a b.png")
	rule := OutsideImage(root, "rid")
	want := wswhiteboard.FileAction{Href: "/s/rid/image?path=" + url.QueryEscape(filepath.ToSlash(out)), Abs: out, Image: true, Outside: true}
	for in, w := range map[string]wswhiteboard.FileAction{
		out:                           want,
		filepath.ToSlash(out):         want,
		"../elsewhere/a b.png":        {},
		filepath.Join(root, "in.png"): {},
		filepath.Join(filepath.Dir(root), "elsewhere", "x.md"): {},
		"//server/share/x.png":                                 {},
		"":                                                     {},
	} {
		if got := rule(in); got != w {
			t.Errorf("OutsideImage(%q) = %+v, want %+v", in, got, w)
		}
	}
	if got := OutsideImage(root, "Not/A-Reply-ID")(out); got != (wswhiteboard.FileAction{}) {
		t.Errorf("OutsideImage with an invalid reply-id = %+v", got)
	}
}

// TestServedTargetsOutsideImages pins a lead reply's rule on the served
// page: an absolute image path outside the root renders inline on the image
// route and links through it, both marked data-out; a relative path that
// leaves the root and a non-image outside it stay copy-only; an in-root
// image link is marked data-img.
func TestServedTargetsOutsideImages(t *testing.T) {
	root := t.TempDir()
	out := filepath.ToSlash(filepath.Join(filepath.Dir(root), "elsewhere", "shot.png"))
	note := filepath.ToSlash(filepath.Join(filepath.Dir(root), "elsewhere", "n.md"))
	src := "/s/rid/image?path=" + url.QueryEscape(out)
	html := string(wswhiteboard.RenderText("![pic]("+out+") [lnk]("+out+") ![rel](../elsewhere/shot.png) [note]("+note+") [in](docs/a.png)", servedTargets(root, "rid")))
	for _, want := range []string{
		`<img src="` + src + `" data-out="" data-abs="` + filepath.FromSlash(out) + `" alt="pic">`,
		`data-wl="file" data-abs="` + filepath.FromSlash(out) + `" data-img="" data-out="" title="` + out + `">lnk</a>`,
		`rel <a tabindex="0" data-wl="file" data-abs=`,
		`<a tabindex="0" data-wl="file" data-abs="` + filepath.FromSlash(note) + `" title="` + note + `">note</a>`,
		`<a href="/s/rid/file?path=docs%2Fa.png" data-wl="file" data-rel="docs/a.png" data-abs="` + filepath.Join(root, "docs", "a.png") + `" data-img="" title="docs/a.png">in</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("reply lacks %q:\n%s", want, html)
		}
	}
	if strings.Count(html, "data-out") != 2 {
		t.Errorf("reply marks %d targets data-out, want 2:\n%s", strings.Count(html, "data-out"), html)
	}
}

// TestDriveOrRootPathRefusesNamespaceVolumes pins the volume check on
// Windows: only a drive-letter volume passes, so no spelling of a UNC,
// device, or NT-namespace path reaches the filesystem; on Unix every
// absolute path passes, having no volume.
func TestDriveOrRootPathRefusesNamespaceVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		if !driveOrRootPath("/tmp/x.png") {
			t.Error("driveOrRootPath refused a Unix absolute path")
		}
		return
	}
	for path, want := range map[string]bool{
		`C:\x.png`: true, `c:\a\b.png`: true,
		`\\?\C:\x.png`: false, `\\.\C:\x.png`: false, `\??\C:\x.png`: false, `\??\UNC\s\sh\x.png`: false, `\\s\sh\x.png`: false,
	} {
		if got := driveOrRootPath(path); got != want {
			t.Errorf("driveOrRootPath(%q) = %v, want %v", path, got, want)
		}
	}
}
