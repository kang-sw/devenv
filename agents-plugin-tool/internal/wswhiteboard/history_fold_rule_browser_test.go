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

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// One history with closed questions, their answers, an older owner input and
// one still-open question, each far above the bottom 1000px. In Chromium:
// the initial view is the bottom-up cut (whole rows to >= 1000px, crossing
// row included, no more); closed Q/A, the old input and the open question's
// ask line all stay folded in one top fold (the open question's card stacks
// after the history); every expand adds the same cut while the row under the
// fold keeps its viewport position.
// WS_FOLD_SHOTS=<dir> also saves dark/light screenshots of the initial and an
// expanded view. Needs WS_PLAYWRIGHT_MODULE. WS_FOLD_PAGE=<dir> keeps the
// rendered fold.html and labels.json there.
func TestHistoryFoldRuleBrowser(t *testing.T) {
	st := &State{}
	seq := 0
	logs := func(n int) {
		for i := 0; i < n; i++ {
			seq++
			mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>log %d: a reasonably long line of text for the history window</wb:log></wb:topic>`, seq))
		}
	}
	logs(30)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>closed policy question</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	logs(20)
	mustApply(t, st, `<wb:answer id="1" value="a">first answer</wb:answer>`)
	logs(20)
	oldInput := len(st.Topics[0].Entries)
	logs(1)
	logs(20)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>still open question</wb:title></wb:add></wb:topic>`)
	logs(70)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>closed technical question</wb:title></wb:add></wb:topic>`)
	logs(10)
	mustApply(t, st, `<wb:answer id="3" value="ok">second answer</wb:answer>`)
	logs(80)
	// Owner inputs attach to a recorded update batch.
	last := len(st.Topics[0].Entries) - 1
	st.Topics[0].Entries[oldInput].Seq, st.Topics[0].Entries[last].Seq = 9001, 9002
	sent := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
	st.Topics[0].Entries[oldInput].Inputs = json.RawMessage(fmt.Sprintf(`[{"inbox_id":"x","sent_at":%d,"excerpt":"older owner request","chip_ids":[]}]`, sent))
	st.Topics[0].Entries[last].Inputs = json.RawMessage(fmt.Sprintf(`[{"inbox_id":"y","sent_at":%d,"excerpt":"latest owner request","chip_ids":[]}]`, sent+60000))

	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "fold", Location: time.UTC})
	// The server's own row classification, keyed by data-row.
	labels := map[string]string{}
	for _, p := range view.Topics[0].Timeline {
		for _, r := range p.Rows {
			label := "log"
			switch {
			case len(r.Inputs) > 0:
				label = "input"
				if r.Key == st.Topics[0].Entries[last].ID {
					label = "latest-input"
				}
			case r.Answer:
				label = "answer"
			case r.Ask && r.Item != nil && r.Item.Status == StatusOpen:
				label = "open-ask"
			case r.Ask && r.Item != nil:
				label = "closed-ask"
			}
			labels[r.Key] = label
		}
	}
	want := map[string]int{}
	for _, l := range labels {
		want[l]++
	}
	for _, l := range []string{"closed-ask", "answer", "input", "open-ask", "latest-input"} {
		if want[l] == 0 {
			t.Fatalf("fixture lacks a %s row: %v", l, want)
		}
	}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	page := `<!DOCTYPE html><html><head><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><aside>` + string(regions.Panels) + `</aside><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
	dir := os.Getenv("WS_FOLD_PAGE")
	if dir == "" {
		dir = t.TempDir()
	}
	html, meta := filepath.Join(dir, "fold.html"), filepath.Join(dir, "labels.json")
	if err := os.WriteFile(html, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(labels)
	if err := os.WriteFile(meta, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-fold-rule.cjs", html, meta, os.Getenv("WS_FOLD_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("fold rule: %v", err)
	}
}
