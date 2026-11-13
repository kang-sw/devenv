package wsagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func leadBundledFixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	writeLeadPluginFixture(t, filepath.Join(root, ".claude-plugin", "plugin.json"), `{"name":"`+name+`"}`)
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

func TestBundledPluginRootUsesDaemonPackage(t *testing.T) {
	for _, name := range []string{"ws", "wsflow"} {
		t.Run(name, func(t *testing.T) {
			want := leadBundledFixture(t, name)
			got, err := bundledPluginRoot()
			if err != nil || got != want {
				t.Fatalf("bundledPluginRoot = %q, %v; want %q", got, err, want)
			}
			// Relative launcher roots are made absolute before being handed
			// to a lead running in a different project directory.
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(cwd, filepath.Join(want, "rsrc"))
			if err != nil {
				// Windows test temp and source directories can be on
				// different drives, which have no relative path.
				t.Logf("relative-path case unavailable: %v", err)
				return
			}
			t.Setenv(wsrsrc.EnvRsrcRoot, rel)
			got, err = bundledPluginRoot()
			if err != nil || got != want {
				t.Fatalf("relative bundledPluginRoot = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestBundledPluginRootInvalidExplicitRootDoesNotFallBack(t *testing.T) {
	for _, manifest := range []string{"", `{"name":"other"}`, `{`} {
		t.Run(manifest, func(t *testing.T) {
			root := t.TempDir()
			if manifest != "" {
				writeLeadPluginFixture(t, filepath.Join(root, ".claude-plugin", "plugin.json"), manifest)
			}
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(root, "rsrc"))
			if got, err := bundledPluginRoot(); err == nil || got != "" {
				t.Fatalf("bad explicit root must refuse, not use checkout: %q, %v", got, err)
			}
		})
	}
}

func TestBundledPluginRootSourceBuild(t *testing.T) {
	t.Setenv(wsrsrc.EnvRsrcRoot, "")
	root, err := bundledPluginRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("source-build root is not absolute: %s", root)
	}
	if err := validateBundledPlugin(root); err != nil {
		t.Fatal(err)
	}
}
