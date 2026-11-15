package wswhiteboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A check-ask row's clock reads relative time ("방금" / "N분 전") like the lead
// row next to it, because every history clock shares the "clock" template.
// Needs WS_PLAYWRIGHT_MODULE; WS_CLOCK_SHOT=<png> also saves a screenshot.
func TestHistoryClockRelativeForEveryRowBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	env := testEnv()
	env.Now = time.Now().Add(-2 * time.Minute)
	st := &State{}
	for _, cmd := range []string{
		logXML("t", "lead note before the question"),
		addXML(ClassCheck, "t", "확인"),
		logXML("t", "lead note after the question"),
	} {
		if _, problems := Apply(st, cmd, env); len(problems) > 0 {
			t.Fatalf("Apply(%s): %v", cmd, problems)
		}
	}
	doc := &st.Topics[0]
	for i := range doc.Entries {
		if doc.Entries[i].ItemID == 1 {
			if doc.Entries[i].Seq == 0 {
				doc.Entries[i].Seq = 99
			}
			doc.Entries[i].Inputs = json.RawMessage(fmt.Sprintf(`[{"inbox_id":"x","sent_at":%d,"excerpt":"owner message","chip_ids":["1"]}]`, time.Now().Add(-3*time.Minute).UnixMilli()))
		}
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "clock", Location: time.Local})
	view.Lang = "ko"
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "clock.html")
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-clock.cjs", path, os.Getenv("WS_CLOCK_SHOT"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("history clocks: %v", err)
	}
}
