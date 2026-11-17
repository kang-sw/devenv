package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestComposerNoteChipsBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/composer-note-chips.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/composer-note-chips.test.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("composer note chips: %v", err)
	}
}

func TestBoardNoteTypingBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium note typing regression")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/note-typing.browser.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("board note typing: %v", err)
	}
}
