package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Run the real shell's workspace picker against stubbed host filesystems.
// No host directories, workspace mutations or shared daemon are involved.
func TestFilesWorkspacePickerBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for cached Chromium acceptance")
	}
	f := startPageFixture(t, withWorkspaces(t))
	shots := os.Getenv("WS_WORKSPACE_PICKER_SHOTS")
	if shots != "" {
		if !filepath.IsAbs(shots) {
			t.Fatal("WS_WORKSPACE_PICKER_SHOTS must be an absolute temporary directory")
		}
		if err := os.MkdirAll(shots, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/files-workspace-picker.browser.cjs", f.base, shots)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("workspace picker browser: %v", err)
	}
}
