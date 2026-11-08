package wsdashboard

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdevassets"
)

// devSource copies both embedded asset directories into a temp source root
// and turns dev assets on over it for the test. The returned function
// resolves a dashboard asset path. Dev assets are process-level: these tests
// never run in parallel.
func devSource(t *testing.T) (dashboard func(name string) string, whiteboard func(name string) string) {
	t.Helper()
	root := t.TempDir()
	dash := filepath.Join(root, "internal", "wsdashboard", "assets")
	wb := filepath.Join(root, "internal", "wswhiteboard", "assets")
	for _, c := range []struct{ dst, src string }{{dash, "assets"}, {wb, filepath.Join("..", "wswhiteboard", "assets")}} {
		if err := os.MkdirAll(filepath.Dir(c.dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.CopyFS(c.dst, os.DirFS(c.src)); err != nil {
			t.Fatal(err)
		}
	}
	src, err := wsdevassets.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	SetDevAssets(src)
	t.Cleanup(func() { SetDevAssets(nil) })
	return func(name string) string { return filepath.Join(dash, name) }, func(name string) string { return filepath.Join(wb, name) }
}

func editFile(t *testing.T, path, old, repl string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatalf("%s lacks %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, repl, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDevAssetsOffServesTheEmbeddedPages(t *testing.T) {
	f := startPageFixture(t)
	_, session := get(t, f.base+"/s/livesession")
	if strings.Contains(session, "devmark") {
		t.Fatal("embedded page carries the dev marker")
	}
	dash, _ := devSource(t)
	editFile(t, dash("dashboard.css"), "{", "{/*devmark*/")
	if _, body := get(t, f.base+"/s/livesession"); !strings.Contains(body, "devmark") {
		t.Fatal("dev assets on did not serve the edit")
	}
	SetDevAssets(nil)
	if _, body := get(t, f.base+"/s/livesession"); strings.Contains(body, "devmark") {
		t.Error("turning dev assets off still serves the working tree")
	}
}

func TestDevAssetsShowEditsWithoutARestart(t *testing.T) {
	dash, wb := devSource(t)
	f := startPageFixture(t)

	// The dashboard's own stylesheet, script, and template.
	editFile(t, dash("dashboard.css"), "{", "{/*devdashcss*/")
	editFile(t, dash("dashboard.js"), "(function", "/*devdashjs*/(function")
	editFile(t, dash("dashboard.tmpl"), `{{define "r-head"}}`, `{{define "r-head"}}<i>devdashtmpl</i>`)
	_, session := get(t, f.base+"/s/livesession")
	for _, want := range []string{"devdashcss", "devdashjs", "<i>devdashtmpl</i>"} {
		if !strings.Contains(session, want) {
			t.Errorf("an edit marked %q does not show on the session page", want)
		}
	}
	if _, index := get(t, f.base+"/"); !strings.Contains(index, "devdashcss") {
		t.Error("a css edit does not show on the index page")
	}

	// The whiteboard stylesheet the session page inlines through PageAssets.
	editFile(t, wb("page.css"), "{", "{/*devwbcss*/")
	if _, body := get(t, f.base+"/s/livesession"); !strings.Contains(body, "devwbcss") {
		t.Error("a whiteboard css edit does not show on the session page")
	}

	// A further edit shows on the next request too.
	editFile(t, dash("dashboard.css"), "devdashcss", "devdashcss2")
	if _, body := get(t, f.base+"/s/livesession"); !strings.Contains(body, "devdashcss2") {
		t.Error("a second edit does not show")
	}
}

func TestDevAssetsReportABrokenTemplateAndRecover(t *testing.T) {
	dash, _ := devSource(t)
	f := startPageFixture(t)
	path := dash("dashboard.tmpl")
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{{define "r-head"}}{{if}}{{end}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, f.base+"/s/livesession"); code != http.StatusInternalServerError {
		t.Errorf("a broken template served status %d, want 500", code)
	}
	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, f.base+"/s/livesession"); code != http.StatusOK || !strings.Contains(body, `data-region="board"`) {
		t.Errorf("the fixed template did not recover: %d", code)
	}
}

func TestDevAssetsRefuseASymlinkOutTheRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	dash, _ := devSource(t)
	f := startPageFixture(t)
	outside := filepath.Join(t.TempDir(), "evil.css")
	if err := os.WriteFile(outside, []byte("evil{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dash("dashboard.css")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dash("dashboard.css")); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, f.base+"/s/livesession")
	if code == http.StatusOK || strings.Contains(body, "evil{}") {
		t.Errorf("a symlinked asset outside the root was served: %d", code)
	}
}

func TestEnableDevAssets(t *testing.T) {
	t.Cleanup(func() { SetDevAssets(nil) })
	if root, err := EnableDevAssets(""); root != nil || err != nil {
		t.Errorf("empty source = %v, %v; want off", root, err)
	}
	if devAssets.Load() != nil {
		t.Error("an empty source turned dev assets on")
	}
	if _, err := EnableDevAssets(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), DevSourceEnv) {
		t.Errorf("a missing source root = %v, want an error naming %s", err, DevSourceEnv)
	}
	if devAssets.Load() != nil {
		t.Error("a failed enable left dev assets on")
	}
	root, err := EnableDevAssets("../..")
	if err != nil || root == nil || devAssets.Load() == nil {
		t.Fatalf("the real source root = %v, %v", root, err)
	}
	var log strings.Builder
	LogDevAssets(&log, root)
	if !strings.Contains(log.String(), root.Dir()) {
		t.Errorf("log line lacks the root: %q", log.String())
	}
}
