package wsrsrc

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kang-sw/devenv/internal/wspackage"
)

func resourceMap(t *testing.T, bodies map[string]string) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	manifest := Manifest{SchemaVersion: SupportedSchemaVersion, Files: map[string]string{}}
	for name, body := range bodies {
		files[name] = &fstest.MapFile{Data: []byte(body)}
		manifest.Files[name] = hashHex([]byte(body))
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = &fstest.MapFile{Data: data}
	return files
}

func TestLoadFSVariantsIncludesAndIntegrity(t *testing.T) {
	files := resourceMap(t, map[string]string{
		"sample/sample.md":             "base",
		"sample/sample.orchestrate.md": "---\nincludes:\n  - local\n  - shared\nvariables:\n  - Value\n---\nprofile {{.Value}}\r\n",
		"sample/local.orchestrate.md":  "profile include",
		"sample/local.md":              "base include",
		"shared.md":                    "shared include",
	})
	got, err := LoadFS(files, "sample", "", "orchestrate", map[string]string{"Value": "text"})
	if err != nil || got.Profile != "orchestrate" || got.Body != "profile text\n\nprofile include\n\nshared include" {
		t.Fatalf("profile/includes = %#v, %v", got, err)
	}
	files["sample/local.orchestrate.md"].Data = []byte("tampered")
	_, err = LoadFS(files, "sample", "", "orchestrate", nil)
	var mismatch ErrHashMismatch
	if !errors.As(err, &mismatch) || mismatch.RelPath != "sample/local.orchestrate.md" {
		t.Fatalf("integrity = %v", err)
	}
	delete(files, "sample/local.orchestrate.md")
	got, err = LoadFS(files, "sample", "", "orchestrate", map[string]string{"Value": "text"})
	if err != nil || !strings.Contains(got.Body, "base include") {
		t.Fatalf("include fallback = %#v, %v", got, err)
	}
	files["manifest.json"].Data = []byte(`{"schema_version":999,"files":{}}`)
	_, err = LoadFS(files, "sample", "", "", nil)
	var schema ErrSchemaMismatch
	if !errors.As(err, &schema) {
		t.Fatalf("schema = %v", err)
	}
}

func TestBuildOwnedResourceLoader(t *testing.T) {
	// Embedded-only acceptance is enabled after generation; the separate build
	// verification lane sets this flag. Bare go test exercises the placeholder
	// via wspackage's fixture test and uses an explicit checkout elsewhere.
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") != "1" {
		return
	}
	t.Setenv(wspackage.EnvDevCheckout, "")
	t.Setenv(EnvRsrcRoot, t.TempDir())
	for _, name := range []string{"ws", "wsflow"} {
		root, err := PackageResources(name)
		if err != nil {
			t.Fatal(err)
		}
		pb, err := LoadFS(root, "lead-workflow-manual", "", "", nil)
		if err != nil || pb.Body == "" {
			t.Fatalf("%s embedded playbook: %#v, %v", name, pb, err)
		}
		if _, err := LoadPartFS(root, "lead-workflow-manual", "command-syntax"); err != nil {
			t.Fatal(err)
		}
		pkg, err := wspackage.Embedded(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"runtime.json", "config-manifest.json", "agy-config-manifest.json", "dashboard-config-manifest.json", ".claude-plugin/plugin.json", "bin/ws-mcp-launcher.py", "skills/lead-bootstrap/AGENTS.template.md"} {
			if _, err := fs.ReadFile(pkg, file); err != nil {
				t.Fatalf("%s/%s: %v", name, file, err)
			}
		}
		if name == "ws" {
			if _, err := fs.ReadFile(pkg, "native-pi/closure.zip"); err != nil {
				t.Fatal(err)
			}
		} else if _, err := fs.Stat(pkg, "native-pi"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("payload must be embedded once: %v", err)
		}
	}
}

func TestDevCheckoutResourceContentIsLive(t *testing.T) {
	checkout := t.TempDir()
	root := filepath.Join(checkout, "agents-plugin", "rsrc")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		t.Helper()
		files := resourceMap(t, map[string]string{"sample.md": body})
		for name, file := range files {
			if err := os.WriteFile(filepath.Join(root, name), file.Data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv(wspackage.EnvDevCheckout, checkout)
	resources, err := PackageResources("")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"first", "edited without rebuild"} {
		write(body)
		pb, err := LoadFS(resources, "sample", "", "", nil)
		if err != nil || pb.Body != body {
			t.Fatalf("live checkout: %#v, %v", pb, err)
		}
	}
}
