package wsdashboard

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSessionHiddenSidecarSurvivesLegacyRewrites(t *testing.T) {
	for _, version := range []string{"v0.49.0", "v0.49.1"} {
		t.Run(version, func(t *testing.T) {
			dir, id := t.TempDir(), "archivedlead"
			if hidden, err := ReadSessionHidden(dir, id); err != nil || hidden {
				t.Fatalf("absent sidecar = %v, %v", hidden, err)
			}
			if err := WriteSessionHidden(dir, id, true); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(SessionDir(dir, id), SessionVisibilityFile)
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			// Both releases read fixed structs from these two fixed names and
			// replace only those files. Unknown fields are intentionally lost.
			for name, raw := range map[string]string{
				"agent.json":        `{"reply_id":"archivedlead","profile":"lead-claude","future":true}`,
				"registration.json": `{"reply_id":"archivedlead","project":"/repo","future":true}`,
			} {
				path := filepath.Join(SessionDir(dir, id), name)
				if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
					t.Fatal(err)
				}
				var old struct {
					ReplyID string `json:"reply_id"`
					Profile string `json:"profile,omitempty"`
					Project string `json:"project,omitempty"`
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &old); err != nil {
					t.Fatal(err)
				}
				data, err = json.Marshal(old)
				if err != nil {
					t.Fatal(err)
				}
				if err := writePrivateFile(SessionDir(dir, id), name, data); err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(data, []byte("hidden")) {
					t.Fatal("hidden leaked into a legacy record")
				}
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("legacy rewrite changed sidecar: %q, %v", after, err)
			}
			if hidden, err := ReadSessionHidden(dir, id); err != nil || !hidden {
				t.Fatalf("new reopen = %v, %v", hidden, err)
			}
			if err := WriteSessionHidden(dir, id, false); err != nil {
				t.Fatal(err)
			}
			if hidden, err := ReadSessionHidden(dir, id); err != nil || hidden {
				t.Fatalf("unhide = %v, %v", hidden, err)
			}
		})
	}
}

func TestSessionHiddenRejectsPathEscape(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"", "../escape", "/absolute", `a\b`, "a/b", ".", ".."} {
		if err := WriteSessionHidden(dir, id, true); err == nil {
			t.Fatalf("write accepted %q", id)
		}
		if _, err := ReadSessionHidden(dir, id); err == nil {
			t.Fatalf("read accepted %q", id)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid IDs wrote files: %v, %v", entries, err)
	}
}

func TestSessionHiddenFailedTempWriteKeepsLastGood(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permissions")
	}
	dir, id := t.TempDir(), "archivedlead"
	if err := WriteSessionHidden(dir, id, true); err != nil {
		t.Fatal(err)
	}
	session := SessionDir(dir, id)
	file := filepath.Join(session, SessionVisibilityFile)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(session, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(session, 0o700) })
	if err := WriteSessionHidden(dir, id, false); err == nil {
		t.Fatal("temporary write unexpectedly succeeded")
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed write replaced last good state: %q, %v", after, err)
	}
}

func TestSessionHiddenDoesNotExemptExistingPrune(t *testing.T) {
	dir, id := t.TempDir(), "archivedlead"
	now := time.Now()
	old := now.Add(-RegistryRetention - time.Hour).UTC().Format(time.RFC3339)
	if err := WriteRegistration(dir, Registration{ReplyID: id, Project: "/repo", RegisteredAt: old, HeartbeatAt: old, EndedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionHidden(dir, id, true); err != nil {
		t.Fatal(err)
	}
	if n := PruneRegistrations(dir, now, RegistryRetention); n != 1 {
		t.Fatalf("prune = %d", n)
	}
	if _, err := os.Stat(SessionDir(dir, id)); !os.IsNotExist(err) {
		t.Fatalf("eligible archived directory remains: %v", err)
	}
}
