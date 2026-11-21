package wsdashboard

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceAdmissionFilePath(t *testing.T) {
	if got := WorkspaceAdmissionFilePath(""); got != "" {
		t.Fatalf("empty workspacesFile = %q, want empty", got)
	}
	wsFile := filepath.Join("/tmp", "cfg", "dashboard", "workspaces.json")
	want := filepath.Join("/tmp", "cfg", "dashboard", "workspace-admission.json")
	if got := WorkspaceAdmissionFilePath(wsFile); got != want {
		t.Fatalf("WorkspaceAdmissionFilePath = %q, want %q", got, want)
	}
}

func TestWorkspaceAdmissionStoreRoundTripExtraAndNewerVersion(t *testing.T) {
	if openWorkspaceAdmissionStore("") != nil {
		t.Fatal("openWorkspaceAdmissionStore(\"\") != nil")
	}
	base := t.TempDir()
	file := filepath.Join(base, "dashboard", "workspace-admission.json")
	dirA := mkDir(t, filepath.Join(base, "proj-a"))
	dirB := mkDir(t, filepath.Join(base, "proj-b"))

	s := openWorkspaceAdmissionStore(file)
	if got := s.rejected(); len(got) != 0 {
		t.Fatalf("empty store rejected = %v", got)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("opening missing store created file: %v", err)
	}
	// No-op clear/undo must not create the file.
	if _, err := s.clearRejected([]string{dirA}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no-op clear created file: %v", err)
	}

	gen0 := s.generation()
	gen1, err := s.reject([]string{dirA, dirB})
	if err != nil || gen1 <= gen0 {
		t.Fatalf("reject = (%d, %v), want gen > %d", gen1, err, gen0)
	}
	if info, err := os.Stat(file); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		t.Fatalf("file mode = %v, %v; want owner-only", info, err)
	}
	// Idempotent reject leaves file bytes and generation unchanged.
	beforeBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	genIdem, err := s.reject([]string{dirA})
	if err != nil || genIdem != gen1 {
		t.Fatalf("idempotent reject = (%d, %v), want gen=%d", genIdem, err, gen1)
	}
	if afterBytes, _ := os.ReadFile(file); string(afterBytes) != string(beforeBytes) {
		t.Fatalf("idempotent reject rewrote file:\n%s\n!=\n%s", afterBytes, beforeBytes)
	}

	reopened := openWorkspaceAdmissionStore(file)
	if !reopened.isRejected(dirA) || !reopened.isRejected(dirB) {
		t.Fatalf("reopened lost rejected paths: %v", reopened.rejected())
	}

	// Unknown fields on root and record survive reject/undo.
	rawWithExtra := `{"version":1,"future_root":{"keep":true},"rejected":[{"path":` + mustJSONString(t, dirA) + `,"reason":"noisy","meta":{"k":1}}]}` + "\n"
	if err := os.WriteFile(file, []byte(rawWithExtra), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.reject([]string{dirB}); err != nil {
		t.Fatal(err)
	}
	afterExtra, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"future_root"`, `"reason": "noisy"`, `"meta"`} {
		if !strings.Contains(string(afterExtra), want) {
			t.Fatalf("unknown field %s dropped:\n%s", want, afterExtra)
		}
	}
	if _, err := reopened.undo([]string{dirB}); err != nil {
		t.Fatal(err)
	}
	if reopened.isRejected(dirB) || !reopened.isRejected(dirA) {
		t.Fatalf("after undo: %v", reopened.rejected())
	}
	if afterUndo, _ := os.ReadFile(file); !strings.Contains(string(afterUndo), `"future_root"`) || !strings.Contains(string(afterUndo), `"reason": "noisy"`) {
		t.Fatalf("undo dropped unknown fields:\n%s", afterUndo)
	}

	// version > 1 refuses writes without overwriting.
	newer := `{"version":2,"rejected":[{"path":` + mustJSONString(t, dirA) + `}]}` + "\n"
	if err := os.WriteFile(file, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.reject([]string{dirB}); !errors.Is(err, errWorkspaceAdmissionNewer) {
		t.Fatalf("version 2 reject err = %v, want errWorkspaceAdmissionNewer", err)
	}
	if _, err := reopened.undo([]string{dirA}); !errors.Is(err, errWorkspaceAdmissionNewer) {
		t.Fatalf("version 2 undo err = %v, want errWorkspaceAdmissionNewer", err)
	}
	if got, _ := os.ReadFile(file); string(got) != newer {
		t.Fatalf("version 2 file overwritten:\n%s", got)
	}

	// Malformed JSON refuses writes without overwriting.
	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.reject([]string{dirB}); err == nil {
		t.Fatal("malformed JSON accepted reject")
	}
	if got, _ := os.ReadFile(file); string(got) != "{broken" {
		t.Fatalf("malformed file overwritten: %s", got)
	}
}

func mustJSONString(t *testing.T, v string) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWorkspaceAdmissionSweepAndZeroIOReads(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "dashboard", "workspace-admission.json")
	keepDir := mkDir(t, filepath.Join(base, "keep"))
	if err := os.WriteFile(filepath.Join(keepDir, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	goneDir := mkDir(t, filepath.Join(base, "gone"))
	lockedParent := mkDir(t, filepath.Join(base, "locked-parent"))
	lockedChild := mkDir(t, filepath.Join(lockedParent, "child"))

	s := openWorkspaceAdmissionStore(file)
	if _, err := s.reject([]string{keepDir, goneDir, lockedChild}); err != nil {
		t.Fatal(err)
	}
	if s.sweep() {
		t.Fatal("sweep with nothing changed returned true")
	}

	// Remove goneDir -> pruned on sweep.
	if err := os.RemoveAll(goneDir); err != nil {
		t.Fatal(err)
	}
	// Deny search permission on lockedParent so Lstat(lockedChild) returns EACCES, not ENOENT.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.Chmod(lockedParent, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(lockedParent, 0o755) })
	}
	genBefore := s.generation()
	if !s.sweep() {
		t.Fatal("sweep after removing directory returned false")
	}
	if s.generation() <= genBefore {
		t.Fatalf("sweep did not bump generation: %d <= %d", s.generation(), genBefore)
	}
	if s.isRejected(goneDir) {
		t.Fatal("goneDir was not pruned")
	}
	if !s.isRejected(keepDir) {
		t.Fatal("non-empty keepDir was pruned")
	}
	if !s.isRejected(lockedChild) {
		t.Fatal("EACCES lockedChild was pruned")
	}
	if s.sweep() {
		t.Fatal("second sweep with nothing new returned true")
	}

	// Foreign write on disk is picked up on sweep (even with zero-change wait via mtime/size change).
	other := openWorkspaceAdmissionStore(file)
	foreignDir := mkDir(t, filepath.Join(base, "foreign"))
	if _, err := other.reject([]string{foreignDir}); err != nil {
		t.Fatal(err)
	}
	if !s.sweep() || !s.isRejected(foreignDir) {
		t.Fatalf("sweep missed foreign write: %v", s.rejected())
	}

	// Verify rejected(), isRejected(), generation(), and railView/stamp read memory only
	// even if the backing file is deleted on disk without sweep/mutate.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if got := s.rejected(); !reflect.DeepEqual(got, []string{keepDir, lockedChild, foreignDir}) {
		t.Fatalf("memory rejected after file delete = %v", got)
	}
	if !s.isRejected(keepDir) || s.generation() == 0 {
		t.Fatal("memory accessors failed after file delete")
	}

	now := time.Now()
	stats := &streamStats{}
	d := &Daemon{
		dir:         base,
		log:         io.Discard,
		stats:       stats,
		broker:      newBroker(stats),
		index:       NewIndex(IndexOptions{Dir: base, Now: func() time.Time { return now }}),
		railMsgs:    newRailMessages(),
		snapshots:   &SnapshotCache{},
		workspaces:  openWorkspaceStore(filepath.Join(base, "workspaces.json")),
		wsAdmission: s,
	}
	d.rail = newRailLister(nil, nil, make(chan struct{}))
	d.index.sessions["s1"] = &indexEntry{session: Session{Registration: Registration{
		ReplyID: "s1", Project: keepDir, Harness: "claude", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now),
	}}}
	pages := pageReader{dir: base, index: d.index, snapshots: d.snapshots, now: func() time.Time { return now }}
	view := d.railView(pages)
	if len(view.Workspaces) != 0 || len(view.RejectedWorkspaces) != 3 || view.RejectedWorkspaces[0].Path == "" {
		t.Fatalf("railView after file delete = %+v", view)
	}
	if stamp, _ := d.railSource(pages, "en", "").stamp(refreshInputs{}); !strings.HasSuffix(stamp, "/"+strconvUint(s.generation())) {
		t.Fatalf("stamp %q does not include admission gen %d", stamp, s.generation())
	}

	var nilStore *workspaceAdmissionStore
	if nilStore.rejected() != nil || nilStore.isRejected(keepDir) || nilStore.generation() != 0 || nilStore.sweep() {
		t.Fatal("nil store failed nil-safety check")
	}
}

func strconvUint(v uint64) string {
	return string(mustMarshalJSON(v))
}

func mustMarshalJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
