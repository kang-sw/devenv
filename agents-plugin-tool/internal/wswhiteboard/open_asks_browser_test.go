package wswhiteboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// openAsksState is one topic whose history folds around an answered policy
// (1), an open check (2) and an open policy (3), each far enough from the
// others and from the latest line (which references (1)) that a jump to one
// leaves folds on both sides.
func openAsksState(t *testing.T) *State {
	t.Helper()
	st := &State{}
	seq := 0
	logs := func(n int) {
		for i := 0; i < n; i++ {
			seq++
			mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>log %d: a reasonably long line of text for the history window</wb:log></wb:topic>`, seq))
		}
	}
	logs(30)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>answered policy</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	logs(10)
	mustApply(t, st, `<wb:answer id="1" value="a">pick A</wb:answer>`)
	logs(100)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="check"><wb:title>open check</wb:title><wb:why>look at the draft</wb:why></wb:add></wb:topic>`)
	logs(100)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>open policy</wb:title><wb:option>X</wb:option><wb:option>Y</wb:option><wb:recommend>a</wb:recommend></wb:add></wb:topic>`)
	logs(150)
	mustApply(t, st, `<wb:topic name="t"><wb:log>the earlier pick {{1}} still holds</wb:log></wb:topic>`)
	return st
}

// Open asks in Chromium: the cards stack after the history in creation
// order with no check chip in the head; a card's jump and an item reference
// each materialize only the region around a folded target, ring it, and
// bring it into view. Needs WS_PLAYWRIGHT_MODULE. WS_OPEN_ASKS_PAGE=<path>
// keeps the rendered page; WS_OPEN_ASKS_SHOTS=<dir> saves dark captures at
// 806 and 420 px.
func TestOpenAsksStackAtTheCardBottomBrowser(t *testing.T) {
	view := BuildView(openAsksState(t), ViewInput{Layout: LayoutFull, ReplyID: "open-asks", Location: time.UTC})
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("WS_OPEN_ASKS_PAGE")
	if path == "" {
		path = filepath.Join(t.TempDir(), "open-asks.html")
	}
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/open-asks.browser.cjs", path, os.Getenv("WS_OPEN_ASKS_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("open asks: %v", err)
	}
}
