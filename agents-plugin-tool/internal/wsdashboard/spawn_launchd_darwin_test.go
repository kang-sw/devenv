package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchdPlistEscapesAndWraps(t *testing.T) {
	p := string(launchdPlist("l", "gui/1/l", "/x/a&b", []string{"--c=<d>"}, "/w", []string{"K=v&1", "bad"}))
	for _, want := range []string{"a&amp;b", "--c=&lt;d&gt;", "<key>K</key><string>v&amp;1</string>", "<string>Interactive</string>", "bootout"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "<key>bad</key>") {
		t.Error("entry without = leaked into env")
	}
}

// TestLaunchdSpawnRunsAndCleansUp: the job runs with the given env and cwd.
// Skips when the gui domain is unavailable (the production path falls back).
func TestLaunchdSpawnRunsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	t.Setenv("WS_LAUNCHD_PROBE", "yes")
	if err := launchdSpawn("/bin/sh", []string{"-c", `echo "$WS_LAUNCHD_PROBE $PWD pri=$(ps -o pri= -p $$)" > ` + out}, dir); err != nil {
		t.Skipf("launchd unavailable: %v", err)
	}
	var got []byte
	for i := 0; i < 50 && len(got) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		got, _ = os.ReadFile(out)
	}
	t.Logf("job output: %s", got)
	if !strings.HasPrefix(string(got), "yes ") {
		t.Fatalf("job output = %q", got)
	}
}
