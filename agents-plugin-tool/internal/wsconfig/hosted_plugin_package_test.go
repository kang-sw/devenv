package wsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostedPluginPackageGlobalOnly(t *testing.T) {
	if !GlobalOnly(HostedPluginPackageKey) || DefaultScope(HostedPluginPackageKey) != ScopeGlobal || RepoScoped(HostedPluginPackageKey, true) {
		t.Fatal("hosted package must resolve and write only at global scope")
	}
	sess := newFakeSessionStore()
	if err := sess.SetOverride("session", HostedPluginPackageKey, "ws"); err != nil {
		t.Fatal(err)
	}
	defaults := map[string]string{HostedPluginPackageKey: "wsflow"}
	_, opts := newTestResolver(t, defaults, sess)
	opts.RepoRoot = t.TempDir()
	repoDir := filepath.Join(opts.RepoRoot, ".ws-workflow")
	if err := os.MkdirAll(repoDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "config.json"), []byte(`{"overrides":{"hosted.plugin_package":"ws"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(opts, defaults, sess, sess)
	path, err := Path(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := setOverrideInFile(path, HostedPluginPackageKey, "ws"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get("session", HostedPluginPackageKey)
	if err != nil || got != (ResolvedValue{Value: "wsflow", Scope: ScopeBuiltin}) {
		t.Fatalf("non-global overlays must be ignored: %+v, %v", got, err)
	}
	for _, scope := range []Scope{ScopeSession, ScopeProject, ScopeRepo} {
		if err := r.Set(HostedPluginPackageKey, "ws", SetOptions{ExplicitScope: scope, SessionKey: "session"}); err == nil {
			t.Fatalf("accepted %s write", scope)
		}
		if err := r.Unset(HostedPluginPackageKey, SetOptions{ExplicitScope: scope, SessionKey: "session"}); err == nil {
			t.Fatalf("accepted %s reset", scope)
		}
	}
	if err := r.Set(HostedPluginPackageKey, "ws", SetOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err = r.Get("session", HostedPluginPackageKey)
	if err != nil || got != (ResolvedValue{Value: "ws", Scope: ScopeGlobal}) {
		t.Fatalf("default write scope: %+v, %v", got, err)
	}
}
