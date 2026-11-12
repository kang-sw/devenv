package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// ReadOutsideLinks lists the outside files a test linked on id's board.
func (p *pageFake) ReadOutsideLinks(reg Registration) ([]string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.outside[reg.ReplyID], true
}

// link puts paths on id's board as outside links.
func (p *pageFake) link(id string, paths ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.outside == nil {
		p.outside = map[string][]string{}
	}
	p.outside[id] = append(p.outside[id], paths...)
}

// outsideFixture is a page fixture with a home of its own and a tree
// outside the session root; both paths are resolved, as a grant's are.
type outsideFixture struct {
	*pageFixture
	home, away string
	local, lan viewer
	bare       viewer
}

func startOutsideFixture(t *testing.T) *outsideFixture {
	t.Helper()
	resolved := func(dir string) string {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		return real
	}
	o := &outsideFixture{home: filepath.Join(resolved(t.TempDir()), "home"), away: filepath.Join(resolved(t.TempDir()), "away", "site")}
	for _, dir := range []string{o.home, o.away} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", o.home)
	t.Setenv("USERPROFILE", o.home)
	o.pageFixture = startPageFixture(t, lanMode)
	o.local, o.lan = fsViewers(t, o.pageFixture)
	o.bare = viewer{name: "LAN peer, no token", peer: "192.168.7.31:50000", host: "192.168.7.20:" + strconv.Itoa(o.d.Endpoint().Port)}
	return o
}

// write writes a file under the home or the outside tree and returns its
// absolute path.
func (o *outsideFixture) write(t *testing.T, base, name, body string) string {
	t.Helper()
	path := filepath.Join(base, filepath.FromSlash(name))
	writeFile(t, path, body)
	return path
}

// open posts the mint for path as v and returns the status and the answer.
func (o *outsideFixture) open(t *testing.T, v viewer, path string) (int, fsOutsideAnswer) {
	t.Helper()
	body, _ := json.Marshal(fsOutsideRequest{Path: path})
	rec := serve(o.d, v, http.MethodPost, "/s/livesession/fs/outside", string(body))
	var a fsOutsideAnswer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("mint %s: %v in %s", path, err, rec.Body.String())
		}
	}
	return rec.Code, a
}

// linked links path on the board and opens it as the local viewer, which
// must succeed.
func (o *outsideFixture) linked(t *testing.T, path string) fsOutsideAnswer {
	t.Helper()
	o.src.link("livesession", path)
	code, a := o.open(t, o.local, path)
	if code != http.StatusOK {
		t.Fatalf("mint %s = %d, want 200", path, code)
	}
	return a
}

// refused links path on the board and expects the mint to refuse it.
func (o *outsideFixture) refused(t *testing.T, why, path string) {
	t.Helper()
	o.src.link("livesession", path)
	if code, a := o.open(t, o.local, path); code != http.StatusNotFound {
		t.Errorf("%s: mint %s = %d %+v, want 404", why, path, code, a)
	}
}

// read gets a capability URL as the local viewer.
func (o *outsideFixture) read(target string) *httptest.ResponseRecorder {
	return serve(o.d, o.local, http.MethodGet, target, "")
}

// under is the URL of name under the capability prefix of a document URL.
func under(doc, name string) string { return doc[:strings.LastIndex(doc, "/")+1] + name }

// TestFSOutsideMintsOnlyForALinkTheSessionShows pins the mint route's
// gate: a loopback, same-origin POST naming a path the session's board, a
// lead reply in its live ring, or its streaming reply links outside the
// root gets the card's answer; a path nothing links, a path a reply only
// carries inside a code span (no link on the page), a path the root
// holds, a path another session links, a GET, a LAN viewer with the token,
// and another origin's POST get none.
func TestFSOutsideMintsOnlyForALinkTheSessionShows(t *testing.T) {
	o := startOutsideFixture(t)
	board := o.write(t, o.away, "board.txt", "TEXT-BOARD")
	reply := o.write(t, o.away, "reply.txt", "TEXT-REPLY")
	streamed := o.write(t, o.away, "streamed.txt", "TEXT-STREAMED")
	unlinked := o.write(t, o.away, "unlinked.txt", "TEXT-UNLINKED")
	mentioned := o.write(t, o.away, "mentioned.txt", "TEXT-MENTIONED")
	other := o.write(t, o.away, "other.txt", "TEXT-OTHER")
	inside := filepath.Join(o.project, "notes.txt")
	writeFile(t, inside, "TEXT-INSIDE")

	o.src.link("livesession", board)
	o.src.link("boardsession", other)
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(o.dir, "livesession")), []wslive.RingEntry{
		{Kind: wslive.RingReply, Text: "see [the reply file](" + filepath.ToSlash(reply) + "), [inside](notes.txt), and the command `cat " + filepath.ToSlash(mentioned) + "`"},
		{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "a user line is no lead reply: [x](" + filepath.ToSlash(unlinked) + ")"},
	}); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, o.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "still writing [this](" + filepath.ToSlash(streamed) + ")"}}})

	for path, want := range map[string]string{board: "TEXT-BOARD", reply: "TEXT-REPLY", streamed: "TEXT-STREAMED"} {
		code, a := o.open(t, o.local, path)
		if code != http.StatusOK || a.Kind != "text" || a.Text != want || a.Abs != path || a.Frame != "" || a.Src != "" || a.View != "" {
			t.Errorf("mint %s = %d %+v, want its text", path, code, a)
		}
	}
	for why, path := range map[string]string{
		"a path nothing links":             unlinked,
		"a path a reply only mentions":     mentioned,
		"a path another session links":     other,
		"a path inside the root":           inside,
		"a relative path":                  "board.txt",
		"an empty path":                    "",
		"a path that does not exist":       filepath.Join(o.away, "missing.txt"),
		"the linked file's folder":         o.away,
		"a path past the linked one":       board + string(filepath.Separator) + "x",
		"a network path":                   "//server/share/board.txt",
		"the linked path under a dot step": filepath.Join(o.away, "..", "site", "unlinked.txt"),
	} {
		if code, a := o.open(t, o.local, path); code != http.StatusNotFound {
			t.Errorf("%s: mint %q = %d %+v, want 404", why, path, code, a)
		}
	}
	// The path matches as the renderer resolved it, in either slash
	// direction and uncleaned.
	if code, _ := o.open(t, o.local, filepath.Join(o.away, "..", "site", "board.txt")); code != http.StatusOK {
		t.Errorf("mint of the linked path spelled through a dot step = %d, want 200", code)
	}

	// A LAN viewer holding the token is refused as if the route did not
	// exist; a peer without it meets the guard.
	if code, a := o.open(t, o.lan, board); code != http.StatusNotFound {
		t.Errorf("mint as a LAN token holder = %d %+v, want 404", code, a)
	}
	body, _ := json.Marshal(fsOutsideRequest{Path: board})
	if rec := serve(o.d, o.bare, http.MethodPost, "/s/livesession/fs/outside", string(body)); !refusedByGuard(rec) {
		t.Errorf("mint as a LAN peer without the token = %d, want the guard's 401", rec.Code)
	}
	// Another origin's POST, an opaque origin's, and one with no Origin.
	for _, origin := range []string{"http://evil.test", "http://127.0.0.1:1", "null", ""} {
		req := httptest.NewRequest(http.MethodPost, "/s/livesession/fs/outside", strings.NewReader(string(body)))
		req.RemoteAddr, req.Host = o.local.peer, o.local.host
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		o.d.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "TEXT-BOARD") {
			t.Errorf("mint with Origin %q = %d, want 403", origin, rec.Code)
		}
	}
	// A GET never mints.
	if rec := o.read("/s/livesession/fs/outside?path=" + board); rec.Code != http.StatusMethodNotAllowed || strings.Contains(rec.Body.String(), "TEXT-BOARD") {
		t.Errorf("GET of the mint route = %d, want 405", rec.Code)
	}
	if rec := serve(o.d, o.local, http.MethodPost, "/s/nosuchsession/fs/outside", string(body)); rec.Code != http.StatusNotFound {
		t.Errorf("mint on an unknown session = %d, want 404", rec.Code)
	}
}

// TestFSOutsideGrantsTheLinkedFilesFolder pins what a link opens: an HTML
// file is a frame on the ocap route whose relative files load under the
// same capability, with the capability route's policy, and its oview URL
// serves it as a page; an image and a video load from the ocap route;
// markdown renders with its relative links and images left as path text; a
// patch is a diff. Nothing outside the granted folder is read, through a
// ".." step or a symlink.
func TestFSOutsideGrantsTheLinkedFilesFolder(t *testing.T) {
	o := startOutsideFixture(t)
	page := o.write(t, o.away, "page.html", `<link rel="stylesheet" href="style.css"><img src="img/a.png"><h1>HTML-AWAY</h1>`)
	o.write(t, o.away, "style.css", "body{color:red}/*CSS-AWAY*/")
	o.write(t, o.away, "img/a.png", "PNG-AWAY")
	o.write(t, o.away, "other.html", "<h1>OTHER-AWAY</h1>")
	secret := o.write(t, filepath.Dir(o.away), "secret.txt", "SECRET-PARENT")
	escape := os.Symlink(secret, filepath.Join(o.away, "escape.txt")) == nil &&
		os.Symlink(filepath.Dir(o.away), filepath.Join(o.away, "uplink")) == nil

	a := o.linked(t, page)
	if a.Kind != "frame" || !strings.HasPrefix(a.Frame, "/s/livesession/fs/ocap/") || !strings.HasSuffix(a.Frame, "/page.html") ||
		!strings.HasPrefix(a.View, "/s/livesession/fs/oview/") || !strings.HasSuffix(a.View, "/page.html") || a.TTL != int(fsCapabilityTTL/time.Second) || a.Text != "" {
		t.Fatalf("mint of an HTML file = %+v", a)
	}
	for _, doc := range []string{a.Frame, a.View} {
		rec := o.read(doc)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HTML-AWAY") || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("GET %s = %d %q", doc, rec.Code, rec.Body.String())
		}
		csp, prefix := rec.Header().Get("Content-Security-Policy"), "http://"+o.local.host+under(doc, "")
		for _, want := range []string{"sandbox allow-scripts;", "default-src " + prefix + ";", "connect-src 'none';"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: policy lacks %q: %s", doc, want, csp)
			}
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: headers = %v", doc, rec.Header())
		}
		for name, want := range map[string]string{"style.css": "CSS-AWAY", "img/a.png": "PNG-AWAY", "other.html": "OTHER-AWAY"} {
			if rec := o.read(under(doc, name)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
				t.Errorf("GET %s = %d %q, want %s", under(doc, name), rec.Code, rec.Body.String(), want)
			}
		}
		refused := []string{"../secret.txt", "img/../../secret.txt", "missing.css", "img", "", strings.TrimPrefix(filepath.ToSlash(secret), "/"), "/" + filepath.ToSlash(secret)}
		if escape {
			refused = append(refused, "escape.txt", "uplink/secret.txt")
		}
		for _, name := range refused {
			if rec := o.read(under(doc, name)); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "SECRET") {
				t.Errorf("GET %s = %d %q, want a refusal", under(doc, name), rec.Code, rec.Body.String())
			}
		}
		// A fetch reads nothing, as on the in-root routes.
		if rec := capFetch(o.pageFixture, o.local, under(doc, "style.css"), capShape{":daemon", "same-origin", "cors", "empty"}); rec.Code != http.StatusNotFound {
			t.Errorf("a fetch of %s = %d, want 404", under(doc, "style.css"), rec.Code)
		}
	}

	shot := o.linked(t, o.write(t, o.away, "img/shot.png", "PNG-SHOT"))
	if shot.Kind != "image" || !strings.HasPrefix(shot.Src, "/s/livesession/fs/ocap/") || shot.View != "" {
		t.Fatalf("mint of an image = %+v", shot)
	}
	if rec := o.read(shot.Src); rec.Code != http.StatusOK || rec.Body.String() != "PNG-SHOT" || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("GET %s = %d %q", shot.Src, rec.Code, rec.Body.String())
	}
	clip := o.linked(t, o.write(t, o.away, "clip.mp4", "MP4-CLIP"))
	if rec := o.read(clip.Src); clip.Kind != "video" || rec.Code != http.StatusOK || rec.Body.String() != "MP4-CLIP" || rec.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("mint of a video = %+v, GET = %d", clip, rec.Code)
	}
	art := o.linked(t, o.write(t, o.away, "art.svg", "<svg><script>1</script></svg>"))
	if art.Kind != "frame" || art.Frame == "" || art.View != "" {
		t.Errorf("mint of an SVG = %+v, want a frame and no page", art)
	}

	md := o.linked(t, o.write(t, o.away, "notes.md", "# Title\n\n[next](other.md) ![pic](img/a.png) [site](https://example.com/)\n"))
	if md.Kind != "markdown" || !strings.Contains(md.HTML, "<h1>Title</h1>") || strings.Contains(md.HTML, "<img") ||
		strings.Contains(md.HTML, `href="other.md"`) || strings.Contains(md.HTML, "/fs/") || strings.Contains(md.HTML, "/file?") {
		t.Errorf("mint of markdown = %+v, want it rendered with relative targets as text", md)
	}
	diff := o.linked(t, o.write(t, o.away, "change.patch", "--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-a\n+b\n"))
	if diff.Kind != "diff" || len(diff.Diff) != 1 || diff.Diff[0].Path != "x.txt" {
		t.Errorf("mint of a patch = %+v", diff)
	}
	if bin := o.linked(t, o.write(t, o.away, "blob.dat", "a\x00b")); bin.Kind != "none" || bin.Reason == "" || bin.Text != "" {
		t.Errorf("mint of a binary file = %+v, want the not-viewable note", bin)
	}

	// A link that is itself a symlink opens the file it resolves to, in
	// that file's own folder.
	if escape {
		alias := filepath.Join(filepath.Dir(o.away), "alias.html")
		if err := os.Symlink(page, alias); err != nil {
			t.Fatal(err)
		}
		if a := o.linked(t, alias); a.Kind != "frame" || a.Abs != page || !strings.HasSuffix(a.Frame, "/page.html") {
			t.Errorf("mint of a symlinked link = %+v, want the resolved file", a)
		}
		// The path a link resolves to opens too: it names the same file,
		// and it is the one the re-mint page posts.
		target := o.write(t, o.away, "sub/target.txt", "TEXT-TARGET")
		written := filepath.Join(filepath.Dir(o.away), "written.txt")
		if err := os.Symlink(target, written); err != nil {
			t.Fatal(err)
		}
		o.src.link("livesession", written)
		if code, a := o.open(t, o.local, target); code != http.StatusOK || a.Text != "TEXT-TARGET" {
			t.Errorf("mint of the path a link resolves to = %d %+v, want its text", code, a)
		}
	}
}

// TestFSOutsideShrinksAWideFolderToTheFile pins the wide-folder rule: a
// link directly in a volume root's child, in the home, or in a folder
// directly under the home opens the linked file alone, so its siblings
// answer 404 through its capability; one level further down the folder is
// granted. A link inside a dot folder opens either way.
func TestFSOutsideShrinksAWideFolderToTheFile(t *testing.T) {
	o := startOutsideFixture(t)
	for _, c := range []struct{ name, folder string }{
		{"the home", o.home},
		{"a folder directly under the home", filepath.Join(o.home, "Documents")},
		{"a dot folder directly under the home", filepath.Join(o.home, ".cache")},
	} {
		page := o.write(t, c.folder, "x.html", "<h1>HTML-WIDE</h1>")
		o.write(t, c.folder, "sibling.css", "CSS-SIBLING")
		o.write(t, c.folder, "sub/deep.css", "CSS-SIBLING")
		a := o.linked(t, page)
		if a.Kind != "frame" || a.View == "" {
			t.Fatalf("%s: mint = %+v", c.name, a)
		}
		for _, doc := range []string{a.Frame, a.View} {
			if rec := o.read(doc); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HTML-WIDE") {
				t.Errorf("%s: GET %s = %d, want the linked file", c.name, doc, rec.Code)
			}
			for _, name := range []string{"sibling.css", "sub/deep.css"} {
				if rec := o.read(under(doc, name)); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SIBLING") {
					t.Errorf("%s: GET %s = %d, want 404: the file alone is granted", c.name, under(doc, name), rec.Code)
				}
			}
		}
		text := o.linked(t, o.write(t, c.folder, "x.txt", "TEXT-WIDE"))
		if text.Kind != "text" || text.Text != "TEXT-WIDE" {
			t.Errorf("%s: mint of a text file = %+v", c.name, text)
		}
	}
	// One level further down, a dot folder included, the folder is granted.
	for _, folder := range []string{filepath.Join(o.home, "work", "proj"), filepath.Join(o.home, ".cache", "proj")} {
		page := o.write(t, folder, "x.html", "<h1>HTML-DEEP</h1>")
		o.write(t, folder, "sibling.css", "CSS-SIBLING")
		o.write(t, folder, ".hidden/key.css", "SECRET-HIDDEN")
		o.write(t, folder, ".env", "SECRET-ENV")
		a := o.linked(t, page)
		if rec := o.read(under(a.Frame, "sibling.css")); rec.Code != http.StatusOK || rec.Body.String() != "CSS-SIBLING" {
			t.Errorf("%s: GET of a sibling = %d %q, want it served", folder, rec.Code, rec.Body.String())
		}
		// A hidden segment below the granted folder stays refused.
		for _, name := range []string{".hidden/key.css", ".env"} {
			for _, doc := range []string{a.Frame, a.View} {
				if rec := o.read(under(doc, name)); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SECRET") {
					t.Errorf("%s: GET %s = %d, want 404", folder, under(doc, name), rec.Code)
				}
			}
		}
	}

	homes := []string{o.home}
	for dir, want := range map[string]bool{
		filepath.VolumeName(o.home) + string(filepath.Separator):                              true,
		filepath.Join(filepath.VolumeName(o.home)+string(filepath.Separator), "tmp"):          true,
		filepath.Join(filepath.VolumeName(o.home)+string(filepath.Separator), "tmp", "build"): false,
		o.home:                                true,
		filepath.Join(o.home, "Documents"):    true,
		filepath.Join(o.home, "work", "proj"): false,
	} {
		if got := wideFolder(dir, homes); got != want {
			t.Errorf("wideFolder(%s) = %v, want %v", dir, got, want)
		}
	}
	// Without a home nothing can be placed, so every folder is too wide.
	if !wideFolder(filepath.Join(o.home, "work", "proj"), nil) {
		t.Error("wideFolder without a home granted a folder")
	}
}

// TestFSOutsideNeverOpensAnAlwaysDeniedPath pins the always-deny list: a
// link at or under ~/.ssh, ~/.gnupg, ~/.aws, ~/.config/gh, ~/.netrc, or
// ~/.docker opens nothing, whatever its type (text, markdown, an image, an
// HTML file), by the path as written even when it resolves elsewhere, and
// by the path it resolves to even when it is written elsewhere; a path that
// only resembles one opens.
func TestFSOutsideNeverOpensAnAlwaysDeniedPath(t *testing.T) {
	o := startOutsideFixture(t)
	for _, name := range []string{
		".ssh/config", ".ssh/id.txt", ".ssh/notes.md", ".ssh/key.png", ".ssh/page.html", ".ssh/sub/deep.txt",
		".gnupg/pubring.txt", ".aws/credentials", ".config/gh/hosts.yml", ".netrc", ".docker/config.json",
	} {
		o.refused(t, "an always-denied path", o.write(t, o.home, name, "SECRET-DENIED"))
	}
	o.refused(t, "an always-denied path in another case", filepath.Join(o.home, ".SSH", "config"))
	// Near misses open: the list is those folders, not their names.
	for _, name := range []string{".sshx/notes.txt", ".config/other/notes.txt", "work/.ssh-notes/notes.txt", ".netrc.d/notes.txt"} {
		if a := o.linked(t, o.write(t, o.home, name, "TEXT-NEAR")); a.Kind != "text" || a.Text != "TEXT-NEAR" {
			t.Errorf("mint of %s = %+v, want its text", name, a)
		}
	}

	// A link written elsewhere that resolves into a denied folder.
	into := filepath.Join(o.away, "into.txt")
	folder := filepath.Join(o.away, "keys")
	if os.Symlink(filepath.Join(o.home, ".ssh", "id.txt"), into) == nil && os.Symlink(filepath.Join(o.home, ".ssh"), folder) == nil {
		o.refused(t, "a symlink into a denied folder", into)
		// A denied path as written stays closed under the name it resolves
		// to, which nothing else links.
		plain := o.write(t, o.away, "plain.txt", "SECRET-PLAIN")
		if err := os.Symlink(plain, filepath.Join(o.home, ".ssh", "plain.txt")); err != nil {
			t.Fatal(err)
		}
		o.src.link("livesession", filepath.Join(o.home, ".ssh", "plain.txt"))
		if code, a := o.open(t, o.local, plain); code != http.StatusNotFound {
			t.Errorf("mint of the path a denied link resolves to = %d %+v, want 404", code, a)
		}
		o.refused(t, "a path through a symlinked denied folder", filepath.Join(folder, "id.txt"))
	}
	// A denied folder that is itself a symlink: the path as written under
	// it is refused though it resolves elsewhere, and so is the folder it
	// resolves to.
	vault := filepath.Join(o.away, "vault")
	o.write(t, vault, "token.txt", "SECRET-VAULT")
	if os.Symlink(vault, filepath.Join(o.home, ".aws-link")) == nil && os.RemoveAll(filepath.Join(o.home, ".gnupg")) == nil && os.Symlink(vault, filepath.Join(o.home, ".gnupg")) == nil {
		o.refused(t, "a denied path as written that resolves elsewhere", filepath.Join(o.home, ".gnupg", "token.txt"))
		o.refused(t, "the folder a denied folder resolves to", filepath.Join(vault, "token.txt"))
	}

	// A granted folder never serves a denied path either: the check runs
	// on every read, not only on the link.
	homes := []string{o.home}
	for path, want := range map[string]bool{
		filepath.Join(o.home, ".ssh"):                   true,
		filepath.Join(o.home, ".ssh", "id_rsa"):         true,
		filepath.Join(o.home, ".config", "gh", "x"):     true,
		filepath.Join(o.home, ".netrc"):                 true,
		filepath.Join(o.home, ".config", "git", "x"):    false,
		filepath.Join(o.home, ".cache", "x"):            false,
		filepath.Join(o.away, ".ssh", "id_rsa"):         false,
		filepath.Join(o.home, "work", ".ssh", "id.txt"): false,
	} {
		if got := alwaysDenied(path, homes); got != want {
			t.Errorf("alwaysDenied(%s) = %v, want %v", path, got, want)
		}
	}
	// Without a home the names are refused wherever they stand.
	if !alwaysDenied(filepath.Join(o.away, ".ssh", "id_rsa"), nil) || !alwaysDenied(filepath.Join(o.away, ".config", "gh", "x"), nil) || alwaysDenied(filepath.Join(o.away, "notes.txt"), nil) {
		t.Error("alwaysDenied without a home does not refuse by name alone")
	}
}

// TestFSOutsideExemptsOnlyTheLinkedFile pins the dot and gitignore rules
// below a granted folder: the linked file itself opens when it is
// dot-named or ignored, since the lead named it, while its dot-named and
// ignored siblings answer 404 on both routes, as the in-root capability
// route refuses them.
func TestFSOutsideExemptsOnlyTheLinkedFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	o := startOutsideFixture(t)
	proj := filepath.Join(o.away, "proj")
	for name, body := range map[string]string{
		"page.html":        "<h1>HTML-PROJ</h1>",
		"style.css":        "CSS-PROJ",
		".report.html":     "<h1>HTML-DOT</h1>",
		".env":             "SECRET-ENV",
		".secrets/key.css": "SECRET-KEY",
		"debug.log":        "SECRET-LOG",
		"build/page.html":  "<h1>HTML-BUILD</h1>",
		"build/out.css":    "SECRET-BUILT",
		"build/notes.txt":  "TEXT-BUILD",
	} {
		o.write(t, proj, name, body)
	}
	gitRun(t, proj, "init", "-q")
	o.write(t, proj, ".gitignore", "build/\n*.log\n")

	siblings := func(why string, a fsOutsideAnswer, served, refused []string) {
		t.Helper()
		for _, doc := range []string{a.Frame, a.View} {
			for _, name := range served {
				if rec := o.read(under(doc, name)); rec.Code != http.StatusOK {
					t.Errorf("%s: GET %s = %d, want it served", why, under(doc, name), rec.Code)
				}
			}
			for _, name := range refused {
				if rec := o.read(under(doc, name)); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SECRET") {
					t.Errorf("%s: GET %s = %d, want 404", why, under(doc, name), rec.Code)
				}
			}
		}
	}
	hidden := []string{".env", ".secrets/key.css", ".gitignore", "debug.log", "build/out.css", "build/page.html"}

	plain := o.linked(t, filepath.Join(proj, "page.html"))
	siblings("a plain linked file", plain, []string{"page.html", "style.css"}, append([]string{".report.html"}, hidden...))

	// A dot-named linked file opens; the other dot names stay refused.
	dot := o.linked(t, filepath.Join(proj, ".report.html"))
	if rec := o.read(dot.Frame); dot.Kind != "frame" || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HTML-DOT") {
		t.Fatalf("mint of a dot-named linked file = %+v, GET = %d", dot, rec.Code)
	}
	siblings("a dot-named linked file", dot, []string{".report.html", "style.css"}, hidden)
	if env := o.linked(t, filepath.Join(proj, ".env")); env.Kind != "text" || env.Text != "SECRET-ENV" {
		t.Errorf("mint of a dot-named text file the lead linked = %+v, want its text", env)
	}

	// An ignored linked file opens; its ignored siblings stay refused, at
	// the accepted cost of its own assets.
	built := o.linked(t, filepath.Join(proj, "build", "page.html"))
	if rec := o.read(built.Frame); built.Kind != "frame" || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HTML-BUILD") {
		t.Fatalf("mint of an ignored linked file = %+v, GET = %d", built, rec.Code)
	}
	siblings("an ignored linked file", built, []string{"page.html"}, []string{"out.css", "notes.txt"})
	if text := o.linked(t, filepath.Join(proj, "build", "notes.txt")); text.Kind != "text" || text.Text != "TEXT-BUILD" {
		t.Errorf("mint of an ignored text file the lead linked = %+v, want its text", text)
	}
	if log := o.linked(t, filepath.Join(proj, "debug.log")); log.Kind != "text" || log.Text != "SECRET-LOG" {
		t.Errorf("mint of an ignored log the lead linked = %+v, want its text", log)
	}
}

// TestFSOutsideIsLoopbackOnly pins that a capability that left this
// machine reads nothing: a LAN viewer holding the token and a valid
// outside capability gets 404 from both serving routes, a peer without the
// token meets the guard, and a loopback peer that addressed a foreign Host
// gets 404, for the document, a relative file, and an image alike.
func TestFSOutsideIsLoopbackOnly(t *testing.T) {
	o := startOutsideFixture(t)
	a := o.linked(t, o.write(t, o.away, "page.html", "<h1>HTML-AWAY</h1>"))
	o.write(t, o.away, "style.css", "CSS-AWAY")
	shot := o.linked(t, o.write(t, o.away, "shot.png", "PNG-AWAY"))
	rebound := viewer{name: "loopback peer, foreign Host, token", peer: "127.0.0.1:50000", host: o.lan.host, cookie: o.lan.cookie}
	for _, target := range []string{a.Frame, a.View, under(a.Frame, "style.css"), under(a.View, "style.css"), shot.Src} {
		if rec := o.read(target); rec.Code != http.StatusOK {
			t.Fatalf("GET %s as the local viewer = %d, want 200", target, rec.Code)
		}
		for _, v := range []viewer{o.lan, rebound} {
			if rec := serve(o.d, v, http.MethodGet, target, ""); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "AWAY") {
				t.Errorf("GET %s as %s = %d, want 404", target, v.name, rec.Code)
			}
		}
		if rec := serve(o.d, o.bare, http.MethodGet, target, ""); !refusedByGuard(rec) {
			t.Errorf("GET %s as a LAN peer without the token = %d, want the guard's 401", target, rec.Code)
		}
	}
	// An expired page's navigation gets a LAN viewer no re-mint page.
	req := httptest.NewRequest(http.MethodGet, strings.Replace(a.View, "/oview/", "/oview/0", 1), nil)
	req.RemoteAddr, req.Host = o.lan.peer, o.lan.host
	req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: o.lan.cookie})
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	o.d.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "fs/outside") {
		t.Errorf("an expired page's navigation as a LAN viewer = %d, want 404", rec.Code)
	}
}

// TestFSOutsideCapabilityVerifiesOnItsOwnRouteOnly pins the capability's
// scope: an outside capability does not verify on the in-root routes nor
// an in-root one on the outside routes, the frame's does not verify on the
// page route nor the reverse, and one with its grant, its mode, its
// session, or its MAC altered, or past its expiry, answers 404.
func TestFSOutsideCapabilityVerifiesOnItsOwnRouteOnly(t *testing.T) {
	clock := &capClock{now: time.Unix(1_800_000_000, 0)}
	o := startOutsideFixture(t)
	o.d.fsNow = clock.read
	writeFile(t, filepath.Join(o.project, "page.html"), "<h1>HTML-INSIDE</h1>")
	page := o.write(t, filepath.Join(o.home, "Documents"), "page.html", "<h1>HTML-AWAY</h1>")
	o.write(t, filepath.Join(o.home, "Documents"), "style.css", "CSS-AWAY")
	a := o.linked(t, page)
	token := func(doc string) string {
		rest := doc[strings.Index(doc, "/fs/")+len("/fs/"):]
		rest = rest[strings.Index(rest, "/")+1:]
		return rest[:strings.Index(rest, "/")]
	}
	frame, view := token(a.Frame), token(a.View)
	in, inView := o.d.mintFSCapability("livesession", o.project), o.d.mintCapability(fsViewRoutePart, "livesession", o.project)
	get := func(route, capability, name string) int {
		return o.read("/s/livesession/fs/" + route + "/" + capability + "/" + name).Code
	}
	if get("ocap", frame, "page.html") != http.StatusOK || get("oview", view, "page.html") != http.StatusOK ||
		get("cap", in, "page.html") != http.StatusOK || get("view", inView, "page.html") != http.StatusOK {
		t.Fatal("a capability does not verify on its own route")
	}
	for why, c := range map[string][2]string{
		"the frame's capability on the page route":      {"oview", frame},
		"the page's capability on the frame route":      {"ocap", view},
		"an outside capability on the in-root route":    {"cap", frame},
		"an outside capability on the in-root page":     {"view", view},
		"an in-root capability on the outside route":    {"ocap", in},
		"an in-root page capability on the outside one": {"oview", inView},
	} {
		if code := get(c[0], c[1], "page.html"); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", why, code)
		}
	}

	// The grant is the MAC's: no field of it can be rewritten. The folder
	// was too wide, so the capability holds the file alone.
	f := strings.Split(frame, ".")
	if len(f) != 5 || f[2] != "f" {
		t.Fatalf("capability = %q, want five fields and a file-only grant", frame)
	}
	if get("ocap", frame, "style.css") != http.StatusNotFound {
		t.Error("a file-only capability served a sibling")
	}
	enc := func(s string) string { return outsideEncoding.EncodeToString([]byte(s)) }
	altered := map[string]string{
		"widened to the folder":   strings.Join([]string{f[0], f[1], "d", f[3], f[4]}, "."),
		"moved to another folder": strings.Join([]string{f[0], f[1], f[2], enc(o.project), f[4]}, "."),
		"renamed":                 strings.Join([]string{f[0], f[1], f[2], f[3], enc("style.css")}, "."),
		"a later expiry":          strings.Join([]string{strconv.FormatInt(clock.read().Add(time.Hour).Unix(), 36), f[1], f[2], f[3], f[4]}, "."),
		"another MAC":             strings.Join([]string{f[0], strings.Repeat("0", 32), f[2], f[3], f[4]}, "."),
		"an in-root MAC":          strings.Join([]string{f[0], strings.SplitN(in, "-", 2)[1], f[2], enc(o.project), f[4]}, "."),
		"padded":                  frame + "=",
		"a field short":           strings.Join(f[:4], "."),
	}
	for why, capability := range altered {
		for _, name := range []string{"page.html", "style.css"} {
			if code := get("ocap", capability, name); code != http.StatusNotFound {
				t.Errorf("a capability %s read %s = %d, want 404", why, name, code)
			}
		}
	}
	if rec := o.read(strings.Replace(a.Frame, "/s/livesession/", "/s/boardsession/", 1)); rec.Code != http.StatusNotFound {
		t.Errorf("the capability under another session = %d, want 404", rec.Code)
	}

	clock.advance(fsCapabilityTTL - time.Second)
	if get("ocap", frame, "page.html") != http.StatusOK {
		t.Error("the capability stopped before its expiry")
	}
	clock.advance(2 * time.Second)
	if get("ocap", frame, "page.html") != http.StatusNotFound || get("oview", view, "page.html") != http.StatusNotFound {
		t.Error("an expired capability still reads")
	}
}

// TestFSOutsideExpiredPageRemintsByPostOnly pins what a tab on an outside
// HTML file gets once its capability expired: its navigation is answered
// with the re-mint page, which carries the linked path and the mint route
// for its script to post and no file content, and neither that GET nor any
// other mints a capability or redirects; the page's post then yields a
// fresh URL that serves the file. A request that is no document navigation
// answers 404.
func TestFSOutsideExpiredPageRemintsByPostOnly(t *testing.T) {
	clock := &capClock{now: time.Unix(1_800_000_000, 0)}
	o := startOutsideFixture(t)
	o.d.fsNow = clock.read
	page := o.write(t, o.away, "page.html", "<h1>HTML-AWAY</h1>")
	o.write(t, o.away, "sub/next page.html", "<h1>NEXT-AWAY</h1>")
	a := o.linked(t, page)
	clock.advance(fsCapabilityTTL + time.Second)

	navigate := func(target string) *httptest.ResponseRecorder {
		return capFetch(o.pageFixture, o.local, target, capShape{"", "none", "navigate", "document"})
	}
	next := under(a.View, "sub/next%20page.html")
	for _, target := range []string{a.View, next} {
		rec := navigate(target)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" || strings.Contains(body, "AWAY") {
			t.Fatalf("an expired page's navigation to %s = %d %q, want the re-mint page", target, rec.Code, rec.Header().Get("Location"))
		}
		for _, want := range []string{`data-mint="/s/livesession/fs/outside"`, `data-base="/s/livesession/fs/oview/"`, `data-abs="` + page + `"`, `method: "POST"`, "location.replace("} {
			if !strings.Contains(body, want) {
				t.Errorf("the re-mint page for %s lacks %q:\n%s", target, want, body)
			}
		}
		// The GET minted nothing: the page names no capability URL.
		if strings.Contains(body, "/fs/oview/"+strings.Split(a.View, "/")[5][:1]) || strings.Count(body, "/fs/oview/") != 1 {
			t.Errorf("the re-mint page for %s carries a capability URL:\n%s", target, body)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'none'", "script-src 'nonce-", "connect-src 'self'", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("the re-mint page's policy lacks %q: %s", want, csp)
			}
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("the re-mint page's headers = %v", rec.Header())
		}
	}
	if body := navigate(next).Body.String(); !strings.Contains(body, `data-rel="sub/next%20page.html"`) {
		t.Errorf("the re-mint page does not keep the path the tab showed:\n%s", body)
	}
	// No document navigation, no page: a frame, an asset, a bare client.
	for _, shape := range []capShape{{"", "same-origin", "navigate", "iframe"}, {"", "cross-site", "no-cors", "style"}, {}} {
		if rec := capFetch(o.pageFixture, o.local, a.View, shape); rec.Code != http.StatusNotFound {
			t.Errorf("an expired page asked as %+v = %d, want 404", shape, rec.Code)
		}
	}
	// The frame route never re-mints, and a URL that is no capability's
	// shape names nothing to re-mint.
	if rec := navigate(a.Frame); rec.Code != http.StatusNotFound {
		t.Errorf("an expired frame's navigation = %d, want 404", rec.Code)
	}
	if rec := navigate("/s/livesession/fs/oview/nonsense/page.html"); rec.Code != http.StatusNotFound {
		t.Errorf("a navigation with no capability's shape = %d, want 404", rec.Code)
	}

	// The page's POST is the mint: its answer's URL serves the file again.
	code, fresh := o.open(t, o.local, page)
	if code != http.StatusOK || fresh.View == a.View {
		t.Fatalf("the re-mint = %d %+v, want a fresh URL", code, fresh)
	}
	if rec := navigate(under(fresh.View, "sub/next%20page.html")); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "NEXT-AWAY") {
		t.Errorf("the fresh URL = %d, want the file", rec.Code)
	}
	// Once the board no longer links the file, the re-mint is refused.
	o.src.mu.Lock()
	o.src.outside = nil
	o.src.mu.Unlock()
	if code, _ := o.open(t, o.local, page); code != http.StatusNotFound {
		t.Errorf("the re-mint of a link the board dropped = %d, want 404", code)
	}
}
