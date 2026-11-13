package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
)

func TestPersistHolderExitSurvivesCleanup(t *testing.T) {
	dir := t.TempDir()
	if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{ReplyID: "abc", Hosted: true}); err != nil {
		t.Fatal(err)
	}
	holderDir := filepath.Join(dir, wsholder.DirName, "old")
	if err := os.MkdirAll(holderDir, 0700); err != nil {
		t.Fatal(err)
	}
	desc := wsholder.Descriptor{ID: "old", PID: 99999999, Port: 1234, Token: "token", Session: json.RawMessage(`{"kind":"lead","reply_id":"abc"}`)}
	raw, _ := json.Marshal(desc)
	if err := os.WriteFile(filepath.Join(holderDir, "descriptor.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(holderDir, "exit.json"), []byte(`{"exit_code":1,"reason":"child_exited","stopped_at":"2026-01-02T00:00:00Z"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(dir, wsholder.DirName), OnExit: persistHolderExit(dir)})
	defer manager.Close()
	manager.Scan()
	if _, err := os.Stat(holderDir); !os.IsNotExist(err) {
		t.Fatalf("holder directory: %v", err)
	}
	row, err := wsdashboard.ReadRegistration(dir, "abc")
	if err != nil || row.HostExitCode == nil || *row.HostExitCode != 1 || row.HostExitReason != "child_exited" || row.HostStoppedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("row: %+v, %v", row, err)
	}
}

func TestPersistHolderExitSkipsNewerHolder(t *testing.T) {
	dir := t.TempDir()
	if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{ReplyID: "abc", Hosted: true}); err != nil {
		t.Fatal(err)
	}
	other := wsholder.Descriptor{ID: "new", PID: 42, Port: 1234, Token: "token", StartedAt: 20, Session: json.RawMessage(`{"kind":"lead","reply_id":"abc"}`)}
	path := filepath.Join(dir, wsholder.DirName, other.ID)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(other)
	if err := os.WriteFile(filepath.Join(path, "descriptor.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	old := wsholder.Descriptor{ID: "old", StartedAt: 10, Session: other.Session}
	if err := persistHolderExit(dir)(old, wsholder.Exit{Reason: "stop"}); err != nil {
		t.Fatal(err)
	}
	row, err := wsdashboard.ReadRegistration(dir, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if row.HostExitReason != "" {
		t.Fatalf("stale exit persisted: %+v", row)
	}
}

func TestPersistHolderExit(t *testing.T) {
	dir := t.TempDir()
	row := wsdashboard.Registration{ReplyID: "abc", Hosted: true, Project: "project"}
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		t.Fatal(err)
	}
	save := persistHolderExit(dir)
	desc := wsholder.Descriptor{Session: json.RawMessage(`{"kind":"lead","reply_id":"abc"}`)}
	zero := 0
	exit := wsholder.Exit{ExitCode: &zero, Reason: "child_exit", StoppedAt: "2026-01-02T00:00:00Z"}
	if err := save(desc, exit); err != nil {
		t.Fatal(err)
	}
	got, err := wsdashboard.ReadRegistration(dir, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostExitCode == nil || *got.HostExitCode != 0 || got.HostExitReason != exit.Reason || got.HostStoppedAt != exit.StoppedAt || got.Project != row.Project {
		t.Fatalf("row: %+v", got)
	}
	older := wsholder.Exit{Reason: "old", StoppedAt: "2026-01-01T00:00:00Z"}
	if err := save(desc, older); err != nil {
		t.Fatal(err)
	}
	got, _ = wsdashboard.ReadRegistration(dir, "abc")
	if got.HostExitReason != exit.Reason {
		t.Fatalf("older exit replaced newer: %+v", got)
	}
	row.Hosted = false
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		t.Fatal(err)
	}
	if err := save(desc, exit); err != nil {
		t.Fatal(err)
	}
	got, _ = wsdashboard.ReadRegistration(dir, "abc")
	if got.HostExitCode != nil || got.HostExitReason != "" {
		t.Fatalf("foreign row changed: %+v", got)
	}
}
