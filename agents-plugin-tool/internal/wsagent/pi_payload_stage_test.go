package wsagent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestPiInputStageLostPublishRace(t *testing.T) {
	for _, winner := range []string{"valid", "corrupt", "missing-file", "not-directory", "absent"} {
		t.Run(winner, func(t *testing.T) {
			state := t.TempDir()
			files := map[string][]byte{"bin/helper.py": []byte("expected")}
			publishErr := errors.New("lost publication race")
			var temporary, target string
			root, err := stagePiFilesWithRename(state, files, func(old, next string) error {
				temporary, target = old, next
				if winner == "absent" {
					return publishErr
				}
				if winner == "not-directory" {
					if err := os.WriteFile(next, []byte("not a directory"), 0o600); err != nil {
						t.Fatal(err)
					}
					return publishErr
				}
				if err := os.MkdirAll(filepath.Join(next, "bin"), 0o700); err != nil {
					t.Fatal(err)
				}
				if winner != "missing-file" {
					data := files["bin/helper.py"]
					if winner == "corrupt" {
						data = []byte("corrupt")
					}
					if err := os.WriteFile(filepath.Join(next, "bin/helper.py"), data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return publishErr
			})
			if winner == "valid" {
				if err != nil || root != target {
					t.Fatalf("valid winner = %q, %v; want %q", root, err, target)
				}
			} else if root != "" || !errors.Is(err, publishErr) {
				t.Fatalf("invalid winner = %q, %v; want publication error", root, err)
			}
			if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
				t.Fatalf("own temporary directory retained: %v", err)
			}
			// Simulate a new daemon to exercise the exact same winner checks.
			piPayloadStages.Lock()
			delete(piPayloadStages.paths, target)
			piPayloadStages.Unlock()
			if winner != "absent" {
				_, warmErr := stagePiFiles(state, files)
				if (warmErr == nil) != (winner == "valid") {
					t.Fatalf("warm validation of %s winner: %v", winner, warmErr)
				}
			}
		})
	}
}

func TestPiInputStageReclaimsOnlyOldTemporaryDirectories(t *testing.T) {
	state := t.TempDir()
	parent := filepath.Join(state, "native-pi-inputs")
	old := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{".stage-old", ".stage-recent", "settled-hash"} {
		path := filepath.Join(parent, name)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if name != ".stage-recent" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	files := map[string][]byte{"helper.py": []byte("expected")}
	root, err := stagePiFiles(state, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, ".stage-old")); !os.IsNotExist(err) {
		t.Fatalf("old temporary directory retained: %v", err)
	}
	for _, name := range []string{".stage-recent", "settled-hash"} {
		if _, err := os.Stat(filepath.Join(parent, name)); err != nil {
			t.Fatalf("retained directory %s: %v", name, err)
		}
	}
	// Remove the entire staging tree: cached reuse must neither inspect it nor
	// recreate anything, including the best-effort cleanup's directory scan.
	if err := os.RemoveAll(parent); err != nil {
		t.Fatal(err)
	}
	again, err := stagePiFiles(state, files)
	if err != nil || again != root {
		t.Fatalf("cached stage = %q, %v", again, err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("cached reuse touched staging tree: %v", err)
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
