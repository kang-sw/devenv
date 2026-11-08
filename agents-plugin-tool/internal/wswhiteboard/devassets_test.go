package wswhiteboard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdevassets"
)

// devTree copies the embedded asset directories into a temp source root and
// turns dev assets on over it for the test. The returned function resolves a
// path under the whiteboard asset directory.
func devTree(t *testing.T) func(name string) string {
	t.Helper()
	root := t.TempDir()
	wb := filepath.Join(root, "internal", "wswhiteboard", "assets")
	for _, dir := range []string{filepath.Dir(wb), filepath.Join(root, "internal", "wsdashboard")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.CopyFS(wb, os.DirFS("assets")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "internal", "wsdashboard", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := wsdevassets.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	SetDevAssets(src)
	t.Cleanup(func() { SetDevAssets(nil) })
	return func(name string) string { return filepath.Join(wb, name) }
}

func editAsset(t *testing.T, path, old, repl string) {
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

func devView() View {
	return BuildView(sampleState(), ViewInput{Layout: LayoutFull})
}

func TestDevAssetsOffServesTheEmbeddedPage(t *testing.T) {
	embedded, err := RenderPage(devView())
	if err != nil {
		t.Fatal(err)
	}
	devTree(t)
	// An untouched copy renders byte for byte what the embedded assets do.
	dev, err := RenderPage(devView())
	if err != nil {
		t.Fatal(err)
	}
	if dev != embedded {
		t.Error("dev assets over an unchanged tree render differently from the embedded copy")
	}
	SetDevAssets(nil)
	if again, _ := RenderPage(devView()); again != embedded {
		t.Error("turning dev assets off does not restore the embedded page")
	}
}

func TestDevAssetsShowEditsOnTheNextRender(t *testing.T) {
	path := devTree(t)

	editAsset(t, path("page.css"), "{", "{/*devcss1*/")
	assets, err := PageAssets()
	if err != nil || !strings.Contains(string(assets.CSS), "devcss1") {
		t.Fatalf("a css edit is not served: %v", err)
	}
	if page, _ := RenderPage(devView()); !strings.Contains(page, "devcss1") {
		t.Error("a css edit does not reach the static page")
	}

	editAsset(t, path("page.tmpl"), `{{define "board"}}`, `{{define "board"}}<i>devboard</i>`)
	regions, err := RenderRegions(devView())
	if err != nil || !strings.Contains(string(regions.Board), "<i>devboard</i>") {
		t.Fatalf("a template edit is not served: %v", err)
	}
	if page, _ := RenderPage(devView()); !strings.Contains(page, "<i>devboard</i>") {
		t.Error("a template edit does not reach the static page")
	}
	for _, lang := range []string{"ko", "en"} {
		v := devView()
		v.Lang = lang
		if r, err := RenderRegions(v); err != nil || !strings.Contains(string(r.Board), "devboard") {
			t.Errorf("lang %q misses the template edit: %v", lang, err)
		}
	}
}

func TestDevAssetsReportABrokenTemplateAndRecover(t *testing.T) {
	path := devTree(t)
	good, err := os.ReadFile(path("page.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path("page.tmpl"), []byte(`{{define "page"}}{{if}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderPage(devView()); err == nil {
		t.Error("a broken template rendered")
	}
	if _, err := RenderRegions(devView()); err == nil {
		t.Error("a broken template rendered regions")
	}
	if err := os.WriteFile(path("page.tmpl"), good, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderPage(devView()); err != nil {
		t.Errorf("the fixed template still fails: %v", err)
	}
}

func TestDevAssetsRefuseASymlinkOutTheRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	path := devTree(t)
	outside := filepath.Join(t.TempDir(), "evil.css")
	if err := os.WriteFile(outside, []byte("evil{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path("page.css")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path("page.css")); err != nil {
		t.Fatal(err)
	}
	if assets, err := PageAssets(); err == nil {
		t.Errorf("a symlinked asset outside the root was served: %q", assets.CSS)
	}
}

func TestDevAssetsKeepMermaidEmbedded(t *testing.T) {
	name, data := MermaidAsset()
	devTree(t)
	if n, d := MermaidAsset(); n != name || len(d) != len(data) {
		t.Error("dev assets changed the mermaid asset")
	}
}
