package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Uses only a disposable fixture daemon and cached headless Chromium. The
// real page, modal code, CSS and streamed rail update run end to end; only
// the missing-Git response is faked. Optional shots stay outside the repo.
func TestGitInitBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for cached Chromium acceptance")
	}
	f, light := gitInitFixture(t)
	dark := mkDir(t, filepath.Join(filepath.Dir(light), "dark", "notes"))
	if _, err := f.d.workspaces.add(dark, false); err != nil {
		t.Fatal(err)
	}
	shots := os.Getenv("WS_GIT_INIT_SHOTS")
	if shots != "" {
		if !filepath.IsAbs(shots) {
			t.Fatal("WS_GIT_INIT_SHOTS must be an absolute temporary directory")
		}
		if err := os.MkdirAll(shots, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-git-init.browser.cjs", f.base, light, dark, shots)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("git-init browser: %v", err)
	}
}
