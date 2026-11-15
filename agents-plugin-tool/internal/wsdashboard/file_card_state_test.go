package wsdashboard

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCardThreeState(t *testing.T) {
	src, _ := json.Marshal(mustAsset(t, "preview.js"))
	fixture, err := os.ReadFile("testdata/file-card-state.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "three-state file cards", "var PREVIEW = "+string(src)+";\n"+outsideDOM+string(fixture))
}

// Only the test-owned HTTP fixture serves these assets; no installed daemon
// or plugin binary is started, rebuilt, or replaced.
func TestFileCardThreeStateBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f, root, _ := worktreePageFixture(t)
	files := map[string]string{
		"worktree_page.go":      "package wsdashboard\n\n// worktreePage renders the page\nfunc worktreePage() {\n\t// A disposable visual acceptance fixture.\n}\n",
		"worktree_page_test.go": "package wsdashboard\n\n// Preview cards never evict open cards.\nfunc TestWorktreePage() {\n\t// Double click promotes in place.\n}\n",
		"WORKFLOW.md":           "# Worktree page\n\nSingle click previews; double click opens.\n\n- One preview slot\n- Open cards stay\n- Pin uses the right column\n",
		"another.txt":           "Another preview, not another open card.\n",
	}
	for name, text := range files {
		for _, dir := range []string{root, f.project} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/file-card-state.browser.cjs", f.base, root)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("three-state Chromium acceptance: %v", err)
	}
}
