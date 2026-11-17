package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestFSLocalOutsidePathsAndSymlinks(t *testing.T) {
	o := startOutsideFixture(t)
	text := o.write(t, o.away, "note.txt", "LOCAL-OUTSIDE")
	image := o.write(t, o.away, "pic.png", "PNG-OUTSIDE")
	page := o.write(t, o.away, "page.html", "<h1>OUTSIDE-HTML</h1>")
	home := o.write(t, o.home, "docs/home.txt", "HOME-TEXT")
	rel, err := filepath.Rel(o.project, text)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{text, rel}
	images := []string{image}
	pages := []string{page}
	dirs := []string{o.away}
	if os.Symlink(o.away, filepath.Join(o.project, "away")) == nil {
		paths = append(paths, "away/note.txt")
		images = append(images, "away/pic.png")
		pages = append(pages, "away/page.html")
		dirs = append(dirs, "away")
	}
	if os.Symlink(text, filepath.Join(o.project, "alias.txt")) == nil {
		paths = append(paths, "alias.txt")
	}
	for _, path := range paths {
		for _, route := range []string{"file", "fs/preview"} {
			target := "/s/livesession/" + route + "?path=" + url.QueryEscape(path)
			for _, v := range []viewer{o.local, o.lan} {
				r := serve(o.d, v, http.MethodGet, target, "")
				want := http.StatusNotFound
				if v.name == o.local.name {
					want = http.StatusOK
				}
				if r.Code != want || (want == http.StatusOK && !strings.Contains(r.Body.String(), "LOCAL-OUTSIDE")) {
					t.Errorf("%s %s = %d %s, want %d", v.name, target, r.Code, r.Body.String(), want)
				}
			}
		}
	}
	for _, path := range images {
		for _, route := range []string{"image", "fs/raw"} {
			target := "/s/livesession/" + route + "?path=" + url.QueryEscape(path)
			if r := o.read(target); r.Code != 200 || r.Body.String() != "PNG-OUTSIDE" {
				t.Errorf("local %s = %d %s", target, r.Code, r.Body.String())
			}
			if r := serve(o.d, o.lan, http.MethodGet, target, ""); r.Code != 404 {
				t.Errorf("LAN %s = %d", target, r.Code)
			}
		}
	}
	for _, path := range pages {
		r := o.read("/s/livesession/fs/preview?path=" + url.QueryEscape(path))
		var a fsPreviewAnswer
		if err := json.Unmarshal(r.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		if a.Kind != "text" || !a.Source || a.Frame != "" {
			t.Errorf("outside HTML gained own-root frame: %+v", a)
		}
		r = o.read("/s/livesession/file?path=" + url.QueryEscape(path))
		if r.Code != 200 || r.Header().Get("Location") != "" || !strings.HasPrefix(r.Header().Get("Content-Type"), "text/plain") {
			t.Errorf("outside file HTML = %d %v", r.Code, r.Header())
		}
	}
	for _, dir := range dirs {
		target := "/s/livesession/fs/list?dir=" + url.QueryEscape(dir)
		if r := o.read(target); r.Code != 200 || !strings.Contains(r.Body.String(), "note.txt") {
			t.Errorf("local list %s = %d %s", dir, r.Code, r.Body.String())
		}
		if r := serve(o.d, o.lan, http.MethodGet, target, ""); r.Code != 404 {
			t.Errorf("LAN list %s = %d", dir, r.Code)
		}
	}
	for _, target := range []string{
		"/s/livesession/file?path=~%2Fdocs%2Fhome.txt",
		"/s/livesession/fs/preview?path=~%2Fdocs%2Fhome.txt",
		"/s/livesession/fs/preview?root=~%2Fdocs&path=home.txt",
	} {
		if r := o.read(target); r.Code != 200 || !strings.Contains(r.Body.String(), "HOME-TEXT") {
			t.Errorf("home %s = %d %s", target, r.Code, r.Body.String())
		}
		if r := serve(o.d, o.lan, http.MethodGet, target, ""); r.Code != 404 {
			t.Errorf("LAN home %s = %d", target, r.Code)
		}
	}
	if code, a := o.open(t, o.local, "~/docs/home.txt"); code != 200 || a.Abs != home || a.Text != "HOME-TEXT" {
		t.Errorf("home mint = %d %+v", code, a)
	}
	if code, _ := o.open(t, o.lan, "~/docs/home.txt"); code != 404 {
		t.Errorf("LAN home mint = %d", code)
	}

	// Rendering never expands the home or gives LAN clients an own-root href.
	out := string(wswhiteboard.RenderText("~/docs/home.txt `~/docs/home.txt` [home](~/docs/home.txt)", servedTargets(o.project, "livesession")))
	if strings.Count(out, `data-abs="~/docs/home.txt"`) != 3 || strings.Contains(out, o.home) || strings.Contains(out, "data-rel=") {
		t.Errorf("home markup = %s", out)
	}
}

func TestImageLocalInRootRequiresLoginAndOwnOrigin(t *testing.T) {
	o := startOutsideFixture(t)
	const secret = "LARGE-LOCAL-SECRET"
	data := secret + strings.Repeat("x", 1<<20)
	o.write(t, o.project, "large.png", data)
	for _, c := range []struct {
		site  string
		login bool
		want  int
	}{
		{"same-origin", false, http.StatusUnauthorized},
		{"cross-site", false, http.StatusUnauthorized},
		{"cross-site", true, http.StatusNotFound},
		{"same-site", true, http.StatusNotFound},
		{"same-origin", true, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/s/livesession/image?path=large.png", nil)
		req.RemoteAddr, req.Host = o.local.peer, o.local.host
		req.Header.Set("Sec-Fetch-Site", c.site)
		if c.site == "cross-site" {
			req.Header.Set("Origin", "null")
		}
		if c.login {
			addLocalhostTestCookie(t, o.d, req)
		}
		rec := httptest.NewRecorder()
		o.d.Handler().ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("site=%s login=%v: status=%d, want %d", c.site, c.login, rec.Code, c.want)
		}
		if c.want == http.StatusOK {
			if rec.Body.String() != data || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
				t.Errorf("authenticated same-origin image: size=%d, headers=%v", rec.Body.Len(), rec.Header())
			}
		} else if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("site=%s login=%v leaked image bytes", c.site, c.login)
		}
	}
}

func TestFSLocalOutsideAndDeniedFilesRequireLogin(t *testing.T) {
	o := startOutsideFixture(t)
	const content = "LOCAL-FILE-CONTENT-MUST-NOT-LEAK"
	for _, c := range []struct {
		base    string
		allowed bool
	}{{o.away, true}, {filepath.Join(o.home, ".ssh"), false}} {
		path := o.write(t, c.base, "secret.png", content)
		body, err := json.Marshal(fsOutsideRequest{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		routes := []struct{ method, target, body string }{
			{http.MethodGet, "/s/livesession/file?path=" + url.QueryEscape(path), ""},
			{http.MethodGet, "/s/livesession/image?path=" + url.QueryEscape(path), ""},
			{http.MethodGet, "/s/livesession/fs/preview?path=" + url.QueryEscape(path), ""},
			{http.MethodGet, "/s/livesession/fs/raw?path=" + url.QueryEscape(path), ""},
			{http.MethodGet, "/s/livesession/fs/list?root=" + url.QueryEscape(filepath.Dir(path)), ""},
			{http.MethodPost, "/s/livesession/fs/outside", string(body)},
		}
		for _, route := range routes {
			for _, login := range []bool{false, true} {
				v := o.local
				v.localCookie = login
				r := serve(o.d, v, route.method, route.target, route.body)
				want := http.StatusUnauthorized
				if login {
					want = http.StatusNotFound
					if c.allowed {
						want = http.StatusOK
					}
				}
				if r.Code != want || (want != http.StatusOK && strings.Contains(r.Body.String(), content)) {
					t.Errorf("%s login=%v: %d, want %d", route.target, login, r.Code, want)
				}
			}
		}
	}
}

func TestFSLocalRechecksResolvedParentBeforeReading(t *testing.T) {
	o := startOutsideFixture(t)
	path := o.write(t, o.away, "safe/secret.txt", "SAFE")
	secret := o.write(t, o.home, ".ssh/secret.txt", "DENIED")
	_, real, ok := localPath(o.project, path)
	if !ok {
		t.Fatal("safe path refused")
	}
	parent := filepath.Dir(real)
	if err := os.Rename(parent, parent+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(secret), parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if f, _, _, ok := openLocalContained(parent, filepath.Base(real)); ok {
		f.Close()
		t.Error("read followed a parent swapped after localPath into a sensitive directory")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr, req.Host = o.local.peer, o.local.host
	if _, err := readViewerDir(req, parent); err == nil {
		t.Error("listing followed a swapped sensitive directory")
	}
}

func TestFSLocalDenylistOnOwnRootCapabilities(t *testing.T) {
	o := startOutsideFixture(t)
	secret := o.write(t, o.project, "keys/secret.png", "DENIED-CAPABILITY")
	if err := os.Symlink(filepath.Dir(secret), filepath.Join(o.home, ".ssh")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, target := range []string{
		o.d.fsCapabilityURL("livesession", o.project, "keys/secret.png"),
		o.d.fsViewURL("livesession", o.project, "keys/secret.png"),
	} {
		if target == "" {
			t.Fatal("missing test capability")
		}
		if r := o.read(target); r.Code != 404 {
			t.Errorf("local sensitive capability = %d %s", r.Code, r.Body.String())
		}
		// The same ordinary, in-root path keeps the previous LAN policy.
		if r := serve(o.d, o.lan, http.MethodGet, target, ""); r.Code != 200 {
			t.Errorf("LAN in-root capability = %d %s", r.Code, r.Body.String())
		}
	}
}

func TestFSLocalDenylistAcrossReadRoutes(t *testing.T) {
	o := startOutsideFixture(t)
	o.write(t, o.home, "safe.txt", "SEARCH-SAFE")
	o.write(t, o.home, ".ssh/secret.txt", "SEARCH-DENIED")
	for _, route := range []string{"find?q=secret", "search?q=SEARCH"} {
		r := o.read("/s/livesession/fs/" + route + "&root=" + url.QueryEscape(o.home))
		if r.Code != 200 || strings.Contains(r.Body.String(), "secret.txt") || strings.Contains(r.Body.String(), "SEARCH-DENIED") {
			t.Errorf("sensitive search %s = %d %s", route, r.Code, r.Body.String())
		}
	}
	if code, a := fsSearch(t, o.pageFixture, "q=SEARCH&root="+url.QueryEscape(o.home)); code != 200 || len(a.Files) != 1 || a.Files[0].Path != "safe.txt" {
		t.Errorf("sensitive content search = %d %+v", code, a)
	}
	for _, name := range outsideDenied {
		dir := filepath.Join(o.home, filepath.FromSlash(name))
		secret := o.write(t, dir, "secret.png", "DENIED-SECRET")
		rel, err := filepath.Rel(o.project, secret)
		if err != nil {
			t.Fatal(err)
		}
		paths := []string{secret, rel, "~/" + name + "/secret.png"}
		dirs := []string{dir}
		alias := filepath.Join(o.project, "alias-"+strings.ReplaceAll(name, "/", "-"))
		if os.Symlink(dir, alias) == nil {
			paths = append(paths, filepath.Join(alias, "secret.png"))
			dirs = append(dirs, alias)
		}
		for _, path := range paths {
			if code, _ := o.open(t, o.local, path); code != 404 {
				t.Errorf("deny mint %s = %d", path, code)
			}
			for _, route := range []string{"file", "image", "fs/preview", "fs/raw"} {
				target := "/s/livesession/" + route + "?path=" + url.QueryEscape(path)
				if r := o.read(target); r.Code != 404 || strings.Contains(r.Body.String(), "DENIED-SECRET") {
					t.Errorf("deny %s = %d %s", target, r.Code, r.Body.String())
				}
			}
		}
		for _, dir := range dirs {
			for _, query := range []string{"dir=" + url.QueryEscape(dir), "root=" + url.QueryEscape(dir)} {
				if r := o.read("/s/livesession/fs/list?" + query); r.Code != 404 {
					t.Errorf("deny list %s = %d", query, r.Code)
				}
			}
		}
	}
}
