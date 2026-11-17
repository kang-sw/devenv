package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findAs runs a name search under root as the local viewer.
func findAs(t *testing.T, o *outsideFixture, root, q string) []fileItem {
	t.Helper()
	rec := o.read("/s/livesession/fs/find?root=" + url.QueryEscape(root) + "&q=" + url.QueryEscape(q))
	var a fsFindAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("find %q under %s = %d %s", q, root, rec.Code, rec.Body.String())
	}
	return a.Items
}

// A find checks its candidates by string under one deny set per request:
// before, every candidate recomputed the set (and statted every base) twice.
func TestFSFindComputesDenySetOncePerRequest(t *testing.T) {
	o := startOutsideFixture(t)
	const n = 200
	for i := 0; i < n; i++ {
		o.write(t, o.away, fmt.Sprintf("dir%d/file%03d.txt", i%10, i), "x")
	}
	before := deniedBasePathBuilds.Load()
	if items := findAs(t, o, o.away, "file"); len(items) == 0 {
		t.Fatal("find answered no candidates")
	}
	if got := deniedBasePathBuilds.Load() - before; got != 1 {
		t.Errorf("find over %d candidates computed the deny set %d times, want 1", n, got)
	}
}

// Name-only checks still hide a denied location by name, under the root as
// written and as resolved.
func TestFSFindHidesDeniedNamesByString(t *testing.T) {
	o := startOutsideFixture(t)
	o.write(t, o.home, "Library/Keychains/login.png", "DENIED")
	o.write(t, o.home, "docs/notes.txt", "ORDINARY")
	roots := []string{o.home}
	link := filepath.Join(o.away, "home-link")
	if err := os.Symlink(o.home, link); err == nil {
		roots = append(roots, link)
	}
	for _, root := range roots {
		items := findAs(t, o, root, "")
		var notes bool
		for _, it := range items {
			if strings.HasPrefix(it.Path, "Library/Keychains") {
				t.Errorf("find under %s listed denied %q", root, it.Path)
			}
			notes = notes || it.Path == "docs/notes.txt"
		}
		if !notes {
			t.Errorf("find under %s lost an ordinary file: %+v", root, items)
		}
	}
	// The prefix check ends at a separator, as filepath.Rel's did.
	homes := []string{o.home}
	for path, want := range map[string]bool{
		filepath.Join(o.home, ".ssh"):           true,
		filepath.Join(o.home, ".SSH", "id_rsa"): true,
		filepath.Join(o.home, ".sshx"):          false,
		filepath.Join(o.home, ".netrc.txt"):     false,
	} {
		if got := alwaysDenied(path, homes); got != want {
			t.Errorf("alwaysDenied(%s) = %v, want %v", path, got, want)
		}
	}
}

// A linked credential shows in a find by its own name, but every content
// route still refuses it by identity, and a content search, which opens
// many files, still computes the deny set once.
func TestFSLinkedCredentialRefusedAtReadTime(t *testing.T) {
	o := startOutsideFixture(t)
	secret := o.write(t, o.home, ".codex/auth.json", "needle-secret")
	o.write(t, o.away, "plain.txt", "needle-plain")
	var aliases []string
	if err := os.Link(secret, filepath.Join(o.away, "hard.txt")); err == nil {
		aliases = append(aliases, "hard.txt")
	}
	if err := os.Symlink(secret, filepath.Join(o.away, "sym.txt")); err == nil {
		aliases = append(aliases, "sym.txt")
	}
	if len(aliases) == 0 {
		t.Skip("neither hardlinks nor symlinks are available")
	}
	root := "root=" + url.QueryEscape(o.away)
	for _, name := range aliases {
		for _, route := range []string{"fs/preview", "fs/raw"} {
			target := "/s/livesession/" + route + "?" + root + "&path=" + name
			if r := o.read(target); r.Code != http.StatusNotFound || strings.Contains(r.Body.String(), "needle-secret") {
				t.Errorf("linked credential %s = %d %s", target, r.Code, r.Body.String())
			}
		}
		assertSensitiveReadsRefused(t, o, filepath.Join(o.away, name))
	}
	before := deniedBasePathBuilds.Load()
	code, a := fsSearch(t, o.pageFixture, root+"&q=needle")
	if code != http.StatusOK || len(a.Files) != 1 || a.Files[0].Path != "plain.txt" {
		t.Errorf("search past linked credentials = %d %+v", code, a)
	}
	if got := deniedBasePathBuilds.Load() - before; got != 1 {
		t.Errorf("search computed the deny set %d times, want 1", got)
	}
}

// A content search whose request is already cancelled stops before it
// opens any file.
func TestFSSearchCancelledRequestStopsEarly(t *testing.T) {
	o := startOutsideFixture(t)
	for i := 0; i < 20; i++ {
		o.write(t, o.away, fmt.Sprintf("f%02d.txt", i), "needle")
	}
	// serve hands the handler an already cancelled request context.
	rec := o.read("/s/livesession/fs/search?root=" + url.QueryEscape(o.away) + "&q=needle")
	var a fsSearchAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("cancelled search = %d %s", rec.Code, rec.Body.String())
	}
	if len(a.Files) != 0 || !a.More {
		t.Errorf("cancelled search read files: %+v", a)
	}
}

// A root that aliases an ancestor of a denied base by identity, not by a
// spelling EvalSymlinks or case folding repairs (a subst drive, a firmlink),
// still hides the base's names from string checks. A symlink stands in for
// such an alias here: aliasRoots matches by identity only.
func TestFSDenySetAliasRootsSpellBasesUnderTheRoot(t *testing.T) {
	o := startOutsideFixture(t)
	o.write(t, o.home, "Library/Keychains/login.png", "DENIED")
	link := filepath.Join(o.away, "home-alias")
	if err := os.Symlink(o.home, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	under := filepath.Join(link, "Library", "Keychains", "login.png")
	deny := newDenySet()
	if deny.lexical(under) {
		t.Fatal("fixture: the alias spelling is already denied by string")
	}
	deny.aliasRoots([]string{link})
	if !deny.lexical(under) || !deny.lexical(filepath.Join(link, "library", "keychains")) {
		t.Error("aliasRoots did not spell the denied base under the aliased root")
	}
	if deny.lexical(filepath.Join(link, "Library", "Other")) {
		t.Error("aliasRoots denied an ordinary sibling")
	}
}

// The macOS Data-volume firmlink of the home is such an alias in practice.
func TestFSFindHidesDeniedNamesUnderFirmlinkRoot(t *testing.T) {
	o := startOutsideFixture(t)
	o.write(t, o.home, "Library/Keychains/login.png", "DENIED")
	o.write(t, o.home, "docs/notes.txt", "ORDINARY")
	alias := "/System/Volumes/Data" + o.home
	actual, err := os.Stat(o.home)
	if err != nil {
		t.Fatal(err)
	}
	if aliased, err := os.Stat(alias); err != nil || !os.SameFile(actual, aliased) {
		t.Skipf("filesystem does not expose the Data-volume alias: %v", err)
	}
	var notes bool
	for _, it := range findAs(t, o, alias, "") {
		if strings.HasPrefix(it.Path, "Library/Keychains") {
			t.Errorf("find under the firmlink root listed denied %q", it.Path)
		}
		notes = notes || it.Path == "docs/notes.txt"
	}
	if !notes {
		t.Error("find under the firmlink root lost an ordinary file")
	}
}
