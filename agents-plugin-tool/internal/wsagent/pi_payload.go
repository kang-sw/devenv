package wsagent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// Use the launcher's stdlib-only validator/materializer. It checks every byte
// before publishing a content-addressed closure; no npm or network
// is involved. Python is already a workflow package runtime prerequisite.
func piPreparedPayload(packageName string) (string, error) {
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		return "", err
	}
	plugin, err := stagePiPayloadInputs(stateDir, packageName)
	if err != nil {
		return "", fmt.Errorf("stage native Pi build inputs: %w", err)
	}
	return materializePiPayload(plugin)
}

// materializePiPayload is the on-disk Python boundary, kept separately testable
// from the build-owned package selection.
func materializePiPayload(plugin string) (string, error) {
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
	// Cold extraction of 9,680 files / ~70 MB measured ~28s on macOS.
	// Allow slower Windows disks/antivirus 300s; the helper reaps abandoned
	// stages after one hour, well beyond this budget.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	args := append(pre, "-I", filepath.Join(plugin, "bin", "ws-pi-payload.py"), "--plugin", plugin, "--materialize", filepath.Join(cache, "native-pi"))
	cmd := exec.CommandContext(ctx, python, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("native Pi payload preparation: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stderr.Len() > 0 {
		// Selection diagnostics belong in the daemon log, never in the path.
		fmt.Fprint(os.Stderr, stderr.String())
	}
	root := strings.TrimSpace(string(out))
	if !filepath.IsAbs(root) || !fileExists(filepath.Join(root, "package.json")) {
		return "", fmt.Errorf("native Pi payload preparation returned no package")
	}
	return root, nil
}
