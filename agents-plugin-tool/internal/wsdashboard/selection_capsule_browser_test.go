package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// A whiteboard selection sent from the board renders in the conversation as
// one capsule, in Chromium with the shipped CSS: each mention's topic tag
// flows inline with its quoted text (one paragraph per mention, no nested
// mention capsule), and a comment sits below a divider inside the same
// capsule. WS_SELECTION_SHOTS, when set to a directory, also receives dark
// and light screenshots at 1440 and 700 wide.
func TestSelectionCapsuleBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	d := assets.data
	fixtures := []wsinbox.Selection{
		{Mentions: []wsinbox.Mention{{Topic: "send-latency", Entry: "e3", Text: "세션마다 모델 응답 하나당 작은 파일이 하나씩 쌓이는 폴더가 있는데, 지우지 않는 설계"}}, Comment: "이게 지우는 설계가 아닌 이유가 따로 있을까요?"},
		{Mentions: []wsinbox.Mention{{Topic: "send-latency", Entry: "e4", Text: "the send path waits on the daemon's fsync before acknowledging"}}},
		{Mentions: []wsinbox.Mention{
			{Topic: "send-latency", ID: "3", Entry: "e5", Text: "first quoted excerpt from the board"},
			{Topic: "composer-ux", Entry: "e6", Text: "second quoted excerpt, from another topic"},
		}, Comment: "Do these two conflict?"},
		{Mentions: []wsinbox.Mention{{Topic: "whiteboard-center-reference-scroll-and-history-fold", Entry: "e7", Text: strings.Repeat("A long quoted passage that keeps going so the capsule has to wrap across several lines. ", 4)}}, Comment: "Long topic and long quote: the tag should wrap like text."},
	}
	ring := make([]wslive.RingRecord, 0, len(fixtures))
	for i, s := range fixtures {
		block, err := wsinbox.SelectionBlock(s)
		if err != nil {
			t.Fatal(err)
		}
		ring = append(ring, ringRecord(i+1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: block}))
	}
	body := presentationHTML(t, ring, PageContent{}, nil)
	doc := `<!doctype html><html data-theme="THEME"><head><meta charset="utf-8"><style>` + string(d.WhiteboardCSS) + string(d.CSS) + `</style></head><body class="dash live">` +
		string(d.WhiteboardSprite) + string(d.Sprite) + string(d.LucideSprite) +
		`<div class="conv">` + body + `</div></body></html>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/selection-capsule.browser.cjs", filepath.Join(dir, "page.html"), os.Getenv("WS_SELECTION_SHOTS")).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("selection capsule: %v", err)
	}
}
