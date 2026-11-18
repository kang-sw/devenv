package mcp

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// useResourceFixture gives synthetic authoring trees an explicit daemon-owned
// checkout. Keep both packages available for namespace-specific render tests.
func useResourceFixture(t *testing.T, root string) {
	t.Helper()
	checkout := t.TempDir()
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		if err := os.CopyFS(filepath.Join(checkout, pkg, "rsrc"), os.DirFS(root)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WS_RSRC_DEV_CHECKOUT", checkout)
}

func TestServerBuildOwnedPackageSelectionIgnoresConnectionRoots(t *testing.T) {
	checkout := t.TempDir()
	for pkg, identity := range map[string]string{"agents-plugin": "ws", "agents-plugin-wsflow": "wsflow"} {
		for _, tree := range []string{"rsrc", "skills"} {
			mustWrite(t, checkout, filepath.Join(pkg, tree, "identity.txt"), identity+"/"+tree)
		}
	}
	t.Setenv("WS_RSRC_DEV_CHECKOUT", checkout)
	stale := t.TempDir()
	mustWrite(t, stale, "identity.txt", "stale installed bytes")
	var servers []*Server
	for _, namespace := range []string{"wsflow", "", "custom"} {
		s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Version: "test", Diag: io.Discard, Env: []string{
			"WS_MCP_NAMESPACE=" + namespace,
			"WS_RSRC_ROOT=" + stale,
			"WS_SKILLS_ROOT=" + stale,
			"WS_RSRC_DEV_CHECKOUT=" + t.TempDir(),
		}})
		t.Cleanup(s.Close)
		servers = append(servers, s)
	}
	// Alternate callers twice: a wsflow hello or a malicious hello dev
	// override cannot redirect either the default package or another caller.
	for range 2 {
		for i, s := range servers {
			want := "ws"
			if i == 0 {
				want = "wsflow"
			}
			for _, tree := range []string{"rsrc", "skills"} {
				root, err := s.rsrcRoot()
				if tree == "skills" {
					root, err = s.skillsRoot()
				}
				if err != nil {
					t.Fatal(err)
				}
				got, err := fs.ReadFile(root, "identity.txt")
				if err != nil || string(got) != want+"/"+tree {
					t.Fatalf("caller %d %s = %q, %v; want %s", i, tree, got, err, want)
				}
			}
		}
	}
}
