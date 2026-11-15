package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Opt in with cached, explicit paths: this test never installs a browser or
// contacts a server. Geometry and observer delivery come from real Chromium.
func TestViewedPerformanceBrowser(t *testing.T) {
	playwright, chromium := os.Getenv("WS_VIEWED_PERF_PLAYWRIGHT"), os.Getenv("WS_VIEWED_PERF_CHROMIUM")
	if playwright == "" || chromium == "" {
		t.Skip("set WS_VIEWED_PERF_PLAYWRIGHT and WS_VIEWED_PERF_CHROMIUM to cached local paths")
	}
	for _, path := range []string{playwright, chromium} {
		if !filepath.IsAbs(path) {
			t.Fatalf("browser dependency path must be absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/viewed-performance.cjs")
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("viewed performance browser: %v", err)
	}
}
