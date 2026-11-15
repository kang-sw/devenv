package wsagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// Use the launcher's stdlib-only validator/materializer. It checks every byte
// before publishing an immutable content-addressed closure; no npm or network
// is involved. Python is already a workflow package runtime prerequisite.
func piPreparedPayload(plugin string) (string, error) {
	cache, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	python := os.Getenv("WS_MCP_HOLDER_PYTHON")
	var pre []string
	if python == "" {
		python, err = exec.LookPath("python3")
		if err != nil && runtime.GOOS == "windows" {
			python, err = exec.LookPath("py")
			pre = []string{"-3"}
		}
		if err != nil {
			return "", fmt.Errorf("native Pi payload needs Python 3: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append(pre, "-I", filepath.Join(plugin, "bin", "ws-pi-payload.py"), "--plugin", plugin, "--materialize", filepath.Join(cache, "native-pi"))
	out, err := exec.CommandContext(ctx, python, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("native Pi payload preparation: %w: %s", err, strings.TrimSpace(string(out)))
	}
	root := strings.TrimSpace(string(out))
	if !filepath.IsAbs(root) || !fileExists(filepath.Join(root, "package.json")) {
		return "", fmt.Errorf("native Pi payload preparation returned no package")
	}
	return root, nil
}
