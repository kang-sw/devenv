package wspackage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDevCheckout(t *testing.T) {
	checkout := t.TempDir()
	t.Setenv(EnvDevCheckout, checkout)
	t.Setenv("WS_RSRC_ROOT", t.TempDir()) // Never a fallback or selector.
	for name, dir := range map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"} {
		root := filepath.Join(checkout, dir)
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "runtime.json"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := Open(name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := fs.ReadFile(got, "runtime.json")
		if err != nil || string(data) != name {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	for _, name := range []string{"", "pi", "../ws", "WS"} {
		if _, err := Open(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
		if _, err := Embedded(name); err == nil {
			t.Fatalf("embedded accepted %q", name)
		}
	}
	t.Setenv(EnvDevCheckout, filepath.Join(checkout, "missing"))
	if _, err := Open("ws"); err == nil || !strings.Contains(err.Error(), EnvDevCheckout) {
		t.Fatalf("%v", err)
	}
	// Embedded must ignore even a broken dev override. With or without generated
	// assets, its result must be identical to reading the build-owned FS directly.
	_, got := Embedded("ws")
	_, want := embedded(assets, "ws")
	if (got == nil) != (want == nil) {
		t.Fatalf("override affected Embedded: %v / %v", got, want)
	}
}

func TestEmbeddedSelectionAndPlaceholder(t *testing.T) {
	fixture := fstest.MapFS{
		"generated/PLACEHOLDER":         {Data: []byte(`{"ws":"flagship-dir","wsflow":"conservative-dir"}`)},
		"generated/inventory.json":      {Data: []byte("inventory")},
		"generated/ws/runtime.json":     {Data: []byte("ws")},
		"generated/wsflow/runtime.json": {Data: []byte("wsflow")},
	}
	for _, name := range []string{"ws", "wsflow"} {
		root, err := embedded(fixture, name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := fs.ReadFile(root, "runtime.json")
		if err != nil || string(data) != name {
			t.Fatalf("%q %v", data, err)
		}
	}
	placeholder := fstest.MapFS{"generated/PLACEHOLDER": fixture["generated/PLACEHOLDER"]}
	if _, err := embedded(placeholder, "ws"); err == nil || !strings.Contains(err.Error(), EnvDevCheckout) {
		t.Fatalf("placeholder must fail actionably: %v", err)
	}
	delete(fixture, "generated/ws/runtime.json")
	if _, err := embedded(fixture, "ws"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("%v", err)
	}
}

func TestPackageDirectoryIndex(t *testing.T) {
	fixture := fstest.MapFS{
		"generated/PLACEHOLDER": {Data: []byte(`{"fixture":"workflow-dir"}`)},
	}
	if dir, err := packageDirIn(fixture, "fixture"); err != nil || dir != "workflow-dir" {
		t.Fatalf("generated mapping = %q, %v", dir, err)
	}
	if _, err := packageDirIn(fixture, "ws"); err == nil || !strings.Contains(err.Error(), `unknown workflow package "ws"`) {
		t.Fatalf("name absent from index accepted: %v", err)
	}
	for _, data := range []string{"{", `{"fixture":"../escape"}`, `{"fixture":""}`} {
		fixture["generated/PLACEHOLDER"].Data = []byte(data)
		if _, err := packageDirIn(fixture, "fixture"); err == nil {
			t.Fatalf("invalid package index accepted: %s", data)
		}
	}
	delete(fixture, "generated/PLACEHOLDER")
	if _, err := packageDirIn(fixture, "fixture"); err == nil {
		t.Fatal("missing package index accepted")
	}
}

func TestBuildOwnedAssets(t *testing.T) {
	t.Setenv(EnvDevCheckout, "")
	root, err := Embedded("ws")
	if _, absent := fs.Stat(assets, "generated/inventory.json"); absent != nil {
		if err == nil {
			t.Fatal("bare build accepted placeholder")
		}
		if _, err := Open("ws"); err == nil {
			t.Fatal("Open accepted placeholder")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"runtime.json", ".claude-plugin/plugin.json", "rsrc/manifest.json", "skills/lead-bootstrap/AGENTS.template.md", "bin/ws-mcp-launcher.py", "bin/ws-pi-payload.py", "native-pi/closure.zip"} {
		if _, err := fs.Stat(root, name); err != nil {
			t.Fatal(err)
		}
	}
	flow, err := Embedded("wsflow")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(flow, "native-pi"); !os.IsNotExist(err) {
		t.Fatalf("native payload duplicated: %v", err)
	}
}
