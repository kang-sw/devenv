package wsholder

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExitCleanupScanAndStop(t *testing.T) {
	for _, mode := range []string{"scan", "stop"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			desc := Descriptor{ID: "holder1", PID: 42, Port: 1234, Token: "token", Session: json.RawMessage(`{"kind":"lead","reply_id":"abc"}`)}
			holderDir := filepath.Join(dir, desc.ID)
			if err := os.Mkdir(holderDir, 0700); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(desc)
			if err := os.WriteFile(filepath.Join(holderDir, descriptorName), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(holderDir, "exit.json"), []byte(`{"exit_code":0,"reason":"child_exit","stopped_at":"2026-01-01T00:00:00Z"}`), 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			m := New(Options{Dir: dir, alive: func(int) bool { return false }, OnExit: func(got Descriptor, exit Exit) error {
				calls++
				if got.ID != desc.ID || exit.ExitCode == nil || *exit.ExitCode != 0 || exit.Reason != "child_exit" {
					t.Fatal(got, exit)
				}
				if _, err := os.Stat(holderDir); err != nil {
					t.Fatal("removed before persistence", err)
				}
				if calls == 1 {
					return errors.New("retry")
				}
				return nil
			}})
			defer m.Close()
			if mode == "stop" {
				m.entries[desc.ID] = &entry{desc: desc}
				if m.Stop(desc.ID) == nil {
					t.Fatal("expected persistence error")
				}
			} else {
				m.Scan()
			}
			if _, err := os.Stat(holderDir); err != nil {
				t.Fatal("lost retry directory", err)
			}
			m.Scan()
			if calls != 2 {
				t.Fatalf("calls = %d", calls)
			}
			if _, err := os.Stat(holderDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("directory remains: %v", err)
			}
		})
	}
}

func TestExitCleanupWithoutExitFile(t *testing.T) {
	dir := t.TempDir()
	desc := Descriptor{ID: "old", PID: 42, Port: 1234, Token: "token"}
	holderDir := filepath.Join(dir, desc.ID)
	if err := os.Mkdir(holderDir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(desc)
	if err := os.WriteFile(filepath.Join(holderDir, descriptorName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Dir: dir, alive: func(int) bool { return false }, OnExit: func(_ Descriptor, exit Exit) error {
		if exit.Reason != "unknown" || exit.StoppedAt == "" || exit.ExitCode != nil {
			t.Fatalf("unexpected fallback: %+v", exit)
		}
		return nil
	}})
	defer m.Close()
	m.Scan()
	if _, err := os.Stat(holderDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory remains: %v", err)
	}
}
