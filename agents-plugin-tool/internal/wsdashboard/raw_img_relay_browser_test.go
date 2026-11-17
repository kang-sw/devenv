package wsdashboard

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestRawImgTagRelayBrowser: a raw <img src> with a file path in a topic
// body or a panel payload shows its image in the sandboxed frame, as
// ![](path) does: relayed through the host page, inside the root or out.
// WS_RAWIMG_SHOT, when set, is where the script saves a screenshot of the
// topic before it asserts.
func TestRawImgTagRelayBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})

	outside := filepath.Join(t.TempDir(), "outside.png")
	writeTestPNG(t, outside, color.RGBA{0xe0, 0x60, 0x20, 0xff})
	writeTestPNG(t, filepath.Join(f.project, "shots", "inside.png"), color.RGBA{0x20, 0x90, 0xe0, 0xff})
	body := "Raw tags:\n\n<img src=\"" + filepath.ToSlash(outside) + "\" alt=\"outside\" width=\"160\">\n\n<img src=\"shots/inside.png\" alt=\"inside\" width=\"160\">\n"
	panel := "<p>panel</p>\n<img src=\"" + filepath.ToSlash(outside) + "\" alt=\"outside\" width=\"160\">\n"
	st := &wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "Shots", Body: body, Changed: 1,
		Entries: []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntryPanel, PanelID: "p1", Recap: "Raw img panel", Text: panel}}}}}
	const reply = "livesession"
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: reply,
		FileText:     func(p string) string { return p },
		ImageSrc:     func(p string) string { return ImageSrc(f.project, reply, p) },
		DocImageSrc:  func(string) string { return "" },
		FileActions:  FileActions(f.project, reply),
		OutsideImage: OutsideImage(f.project, reply),
	})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	f.src.setPage(reply, PageContent{HasBoard: true, Seq: st.Seq, Board: regions})
	f.d.Index().Refresh()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/raw-img-relay.browser.cjs", f.base, os.Getenv("WS_RAWIMG_SHOT")).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium raw <img> relay acceptance: %v", err)
	}
}

func writeTestPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, c)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
