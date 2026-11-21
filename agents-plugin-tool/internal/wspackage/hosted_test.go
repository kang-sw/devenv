package wspackage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func hostedFixture() fstest.MapFS {
	files := fstest.MapFS{}
	for _, path := range hostedFiles {
		files[path] = &fstest.MapFile{Data: []byte("input:" + path)}
	}
	files["rsrc/not-staged.md"] = &fstest.MapFile{Data: []byte("not a hosted input")}
	return files
}

func checkHostedFiles(t *testing.T, root string, source fs.FS) {
	t.Helper()
	count := 0
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		want, err := fs.ReadFile(source, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if string(data) != string(want) {
			t.Fatalf("wrong bytes: %s", rel)
		}
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("stage has %d files, want exactly 6", count)
	}
}

func TestHostedStageContentAddressAndRetention(t *testing.T) {
	dir := t.TempDir()
	files := hostedFixture()
	first, err := stageHosted("ws", dir, files, os.Rename)
	if err != nil {
		t.Fatal(err)
	}
	checkHostedFiles(t, first, files)
	again, err := stageHosted("ws", dir, files, os.Rename)
	if err != nil || again != first {
		t.Fatalf("unstable stage: %q %v", again, err)
	}
	other, err := stageHosted("wsflow", dir, files, os.Rename)
	if err != nil || other == first {
		t.Fatalf("package identity missing: %q %v", other, err)
	}
	files["runtime.json"] = &fstest.MapFile{Data: []byte("new build")}
	next, err := stageHosted("ws", dir, files, os.Rename)
	if err != nil || next == first {
		t.Fatalf("content change not addressed: %q %v", next, err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("old build not retained:", err)
	}
	files["rsrc/not-staged.md"].Data = []byte("out-of-scope edit")
	same, err := stageHosted("ws", dir, files, os.Rename)
	if err != nil || same != next {
		t.Fatalf("unrelated bytes changed hash: %q %v", same, err)
	}
}

func TestHostedStageRefusesCorruptionAndUnexpectedFiles(t *testing.T) {
	for _, kind := range []string{"corrupt", "missing", "extra", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir, files := t.TempDir(), hostedFixture()
			root, err := stageHosted("ws", dir, files, os.Rename)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "corrupt":
				err = os.WriteFile(filepath.Join(root, "runtime.json"), []byte("broken"), 0o600)
			case "missing":
				err = os.Remove(filepath.Join(root, "runtime.json"))
			case "extra":
				err = os.WriteFile(filepath.Join(root, "extra"), nil, 0o600)
			case "symlink":
				path := filepath.Join(root, "runtime.json")
				if err = os.Remove(path); err == nil {
					err = os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
				}
			}
			if err != nil {
				if kind == "symlink" && runtime.GOOS == "windows" {
					t.Skipf("Windows symlink permission unavailable: %v", err)
				}
				t.Fatal(err)
			}
			if _, err := stageHosted("ws", dir, files, os.Rename); err == nil {
				t.Fatal("accepted invalid retained stage")
			}
		})
	}
}

func TestHostedStageConcurrentPublicationValidatesWinner(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrupt", true: "valid"}[valid], func(t *testing.T) {
			files := hostedFixture()
			rename := func(tmp, target string) error {
				if valid {
					if err := os.Rename(tmp, target); err != nil {
						return err
					}
				} else {
					if err := os.MkdirAll(target, 0o700); err != nil {
						return err
					}
				}
				return errors.New("concurrent publication")
			}
			root, err := stageHosted("ws", t.TempDir(), files, rename)
			if valid {
				if err != nil {
					t.Fatal(err)
				}
				checkHostedFiles(t, root, files)
			} else if err == nil || !strings.Contains(err.Error(), "target validation") {
				t.Fatalf("corrupt winner accepted: %q %v", root, err)
			}
		})
	}
}

func TestHostedStageWarmCacheDoesNotTouchFilesystem(t *testing.T) {
	dir, source := t.TempDir(), hostedFixture()
	opens := 0
	open := func(string) (fs.FS, error) {
		opens++
		if opens != 1 {
			t.Fatal("warm production launch reopened source assets")
		}
		return source, nil
	}
	root, err := stageHostedPackage("ws", dir, "", open)
	if err != nil {
		t.Fatal(err)
	}
	checkHostedFiles(t, root, source)
	// A warm production call must use only memory, not reopen source assets,
	// validate the stage or recreate a removed destination.
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	again, err := stageHostedPackage("ws", dir, "", open)
	if err != nil || again != root {
		t.Fatalf("warm stage %q %v", again, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("warm call recreated the stage")
	}
}

func TestHostedDevCheckoutEditsTakeEffectAtNextLaunch(t *testing.T) {
	checkout, dir := t.TempDir(), t.TempDir()
	for path, file := range hostedFixture() {
		dest := filepath.Join(checkout, "agents-plugin", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(EnvDevCheckout, checkout)
	first, err := StageHosted("ws", dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "agents-plugin", "runtime.json"), []byte("live edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := StageHosted("ws", dir)
	if err != nil || next == first {
		t.Fatalf("live dev input not restaged: %s %v", next, err)
	}
	if data, err := os.ReadFile(filepath.Join(next, "runtime.json")); err != nil || string(data) != "live edit" {
		t.Fatalf("staged dev input = %q %v", data, err)
	}
}

func TestHostedEmbeddedStageBothPackages(t *testing.T) {
	t.Setenv(EnvDevCheckout, "")
	if _, err := fs.Stat(assets, "generated/inventory.json"); err != nil {
		if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") == "1" {
			t.Fatal("embedded verification requires generated assets:", err)
		}
		if _, err := StageHosted("ws", t.TempDir()); err == nil {
			t.Fatal("placeholder-only build must refuse hosted staging")
		}
		return
	}
	for _, name := range []string{"ws", "wsflow"} {
		source, err := Embedded(name)
		if err != nil {
			t.Fatal(err)
		}
		root, err := StageHosted(name, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		checkHostedFiles(t, root, source)
	}
}
