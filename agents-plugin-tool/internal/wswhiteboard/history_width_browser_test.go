package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Uses cached browser dependencies only; no daemon, server or download.
func TestHistoryItemTextWidthBrowser(t *testing.T) {
	playwright, chromium := os.Getenv("WS_VIEWED_PERF_PLAYWRIGHT"), os.Getenv("WS_VIEWED_PERF_CHROMIUM")
	if playwright == "" || chromium == "" {
		t.Skip("set WS_VIEWED_PERF_PLAYWRIGHT and WS_VIEWED_PERF_CHROMIUM to cached local paths")
	}
	for _, path := range []string{playwright, chromium} {
		if !filepath.IsAbs(path) {
			t.Fatalf("browser dependency path must be absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Skipf("browser dependency unavailable: %v", err)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	st := &State{}
	mustApply(t, st, `<wb:topic name="width"><wb:add class="policy"><wb:title>Session-free folder picker and terminal connection policy</wb:title><wb:option>Use this computer</wb:option><wb:option>Use a remote host</wb:option></wb:add><wb:add class="check"><wb:title>파일 탐색기 카드 1단계: 파일 탭이나 툴바 파일창에서 카드로 열기를 눌러 확인 / Verify the file explorer card</wb:title></wb:add></wb:topic>`)
	mustApply(t, st, `<wb:answer id="1" value="a">Use this computer</wb:answer><wb:answer id="2" value="looks right">Confirmed the file explorer card</wb:answer>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "widthfixture", Location: time.UTC})
	view.Lang = "ko"
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "history.html")
	if err := os.WriteFile(fixture, []byte(page), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-width.cjs", fixture)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("history item text width browser: %v", err)
	}
}
