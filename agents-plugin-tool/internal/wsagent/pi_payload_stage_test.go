package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wspackage"
)

func TestPiInputStagesAreContentAddressedAndRetained(t *testing.T) {
	state := t.TempDir()
	files := map[string][]byte{"bin/helper.py": []byte("first"), "native-pi/closure.zip": []byte("archive")}
	first, err := stagePiFiles(state, files)
	if err != nil {
		t.Fatal(err)
	}
	again, err := stagePiFiles(state, files)
	if err != nil || again != first {
		t.Fatalf("unchanged stage = %q, %v", again, err)
	}
	files["bin/helper.py"] = []byte("second")
	second, err := stagePiFiles(state, files)
	if err != nil || second == first {
		t.Fatalf("changed stage = %q, %v", second, err)
	}
	data, err := os.ReadFile(filepath.Join(first, "bin", "helper.py"))
	if err != nil || string(data) != "first" {
		t.Fatalf("settled stage overwritten: %q, %v", data, err)
	}
	for _, name := range []string{"../escape", `bin\escape`, "C:/escape"} {
		if _, err := stagePiFiles(state, map[string][]byte{name: nil}); err == nil {
			t.Fatalf("accepted unsafe input %q", name)
		}
	}
}

func TestPiEmbeddedPayloadWithoutPluginCopy(t *testing.T) {
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") != "1" {
		return
	}
	t.Setenv(wspackage.EnvDevCheckout, "")
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_RSRC_ROOT", filepath.Join(t.TempDir(), "nonexistent-plugin", "rsrc"))
	for _, name := range []string{"ws", "wsflow"} {
		root, err := piPreparedPayload(name)
		if err != nil {
			t.Fatalf("%s embedded payload: %v", name, err)
		}
		if !strings.Contains(root, "native-pi") {
			t.Fatalf("not materialized from build: %s", root)
		}
		raw, err := os.ReadFile(filepath.Join(root, "workflow-owner.json"))
		var owner struct {
			Plugin string `json:"plugin"`
		}
		if err != nil || json.Unmarshal(raw, &owner) != nil || owner.Plugin != name {
			t.Fatalf("%s owner = %s, %v", name, raw, err)
		}
		entries, err := piEntriesAt(root)
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s prepared extension: %v, %v", name, entries, err)
		}
	}
}
