package wsagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// Legacy installed-root evidence used by isolation/rejoin tests. Hosted launch
// selection must not depend on it, even after the test removes the manifest.
func leadBundledFixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	writeLeadPluginFixture(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"`+name+`","version":"`+hostedPluginTestVersion+`"}`)
	if err := os.MkdirAll(filepath.Join(root, "rsrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(root, "rsrc"))
	return root
}

func writeLeadPluginFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
