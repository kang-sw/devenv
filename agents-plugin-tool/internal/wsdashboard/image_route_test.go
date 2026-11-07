package wsdashboard

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestImageRouteServesOnlyInsideTheProjectRoot pins the image route's
// containment: a file under the session's project root is served, by
// relative or absolute path; an absolute path outside it, a ".." escape, a
// symlink pointing outside, a non-image file or a symlink to one, a
// directory, and a missing file all answer the same 404 without the file's
// bytes.
func TestImageRouteServesOnlyInsideTheProjectRoot(t *testing.T) {
	f := startPageFixture(t)
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
		os.Symlink(filepath.Join(f.project, "shot.png"), filepath.Join(f.project, "alias.png")) == nil

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
		filepath.Join(outside, "secret.png"),
		filepath.ToSlash(filepath.Join(outside, "secret.png")),
		"../" + filepath.Base(outside) + "/secret.png",
		"docs/../../outside/secret.png",
		"notes.txt",
		"dir.png",
		"missing.png",
		".",
		"shot.png\x00.png",
	}
	if rel, err := filepath.Rel(f.project, filepath.Join(outside, "secret.png")); err == nil {
		refused = append(refused, rel, filepath.ToSlash(rel))
	}
	if symlinks {
		refused = append(refused, "escape.png", "outlink/secret.png", "renamed.png")
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

// TestImageDataURLInlinesContainedImages pins the Now frame's image source:
// an image the route would serve comes back as a data: URL of its bytes with
// the route's content type; one the route refuses (outside the root, not an
// image, a symlink escaping the root) or one above imageDataMax comes back
// empty, so the page falls back to the route's URL.
func TestImageDataURLInlinesContainedImages(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(root, "shots", "a.png"), "ABC")
	writeFile(t, filepath.Join(root, "d.svg"), "<svg/>")
	writeFile(t, filepath.Join(root, "notes.txt"), "TEXT")
	writeFile(t, filepath.Join(outside, "secret.png"), "SECRET")
	writeFile(t, filepath.Join(root, "big.png"), strings.Repeat("x", imageDataMax+1))
	escape := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(root, "escape.png")) == nil
	for in, want := range map[string]string{
		"shots/a.png": "data:image/png;base64,QUJD",
		filepath.ToSlash(filepath.Join(root, "d.svg")): "data:image/svg+xml;base64,PHN2Zy8+",
		"notes.txt":   "",
		"big.png":     "",
		"missing.png": "",
		filepath.ToSlash(filepath.Join(outside, "secret.png")): "",
		"../" + filepath.Base(outside) + "/secret.png":         "",
	} {
		if got := ImageDataURL(root, in); got != want {
			t.Errorf("ImageDataURL(%q) = %.60q, want %q", in, got, want)
		}
	}
	if escape {
		if got := ImageDataURL(root, "escape.png"); got != "" {
			t.Errorf("ImageDataURL through a symlink leaving the root = %.60q", got)
		}
	}
}
