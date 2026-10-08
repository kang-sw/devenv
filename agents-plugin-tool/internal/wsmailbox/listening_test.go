package wsmailbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListeningMarkerWriteReadClear(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	marker := ListeningMarker{SessionKey: "amber-tide-fox", Slug: "alice@machine", PID: 4242, StartedAt: "2026-09-13T00:00:00Z"}
	if err := WriteListeningMarker(marker); err != nil {
		t.Fatalf("WriteListeningMarker: %v", err)
	}

	path, err := ListeningMarkerPath(marker.SessionKey)
	if err != nil {
		t.Fatalf("ListeningMarkerPath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("marker file not written at %s: %v", path, err)
	}

	got, ok, err := ReadListeningMarker(marker.SessionKey)
	if err != nil {
		t.Fatalf("ReadListeningMarker: %v", err)
	}
	if !ok {
		t.Fatalf("ReadListeningMarker ok = false, want true while armed")
	}
	if got != marker {
		t.Fatalf("ReadListeningMarker = %#v, want %#v", got, marker)
	}

	if err := ClearListeningMarker(marker.SessionKey, marker.PID); err != nil {
		t.Fatalf("ClearListeningMarker: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker file still present after clear: err=%v", err)
	}
	if _, ok, err := ReadListeningMarker(marker.SessionKey); err != nil || ok {
		t.Fatalf("ReadListeningMarker after clear: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

// TestClearListeningMarkerLeavesAnotherOwnersMarker pins the owner check: when
// two waits share a session_key (a reload arms a replacement before the old
// wait exits), the exiting wait's clear must not delete the replacement's
// marker.
func TestClearListeningMarkerLeavesAnotherOwnersMarker(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	live := ListeningMarker{SessionKey: "amber-tide-fox", PID: 4242, StartedAt: "2026-09-13T00:00:00Z"}
	if err := WriteListeningMarker(live); err != nil {
		t.Fatalf("WriteListeningMarker: %v", err)
	}

	if err := ClearListeningMarker(live.SessionKey, 1111); err != nil {
		t.Fatalf("ClearListeningMarker by non-owner: %v", err)
	}
	got, ok, err := ReadListeningMarker(live.SessionKey)
	if err != nil || !ok {
		t.Fatalf("ReadListeningMarker after non-owner clear: ok=%v err=%v, want the live marker kept", ok, err)
	}
	if got != live {
		t.Fatalf("marker after non-owner clear = %#v, want %#v", got, live)
	}
}

// TestClearListeningMarkerLeavesUnparseableMarker pins that a marker whose
// owner cannot be established is not removed: the clear reports the parse
// error and the file stays for the next WriteListeningMarker to replace.
func TestClearListeningMarkerLeavesUnparseableMarker(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := ListeningMarkerPath("amber-tide-fox")
	if err != nil {
		t.Fatalf("ListeningMarkerPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearListeningMarker("amber-tide-fox", os.Getpid()); err == nil {
		t.Fatalf("ClearListeningMarker on an unparseable marker succeeded, want the parse error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unparseable marker removed by clear: err=%v", err)
	}
}

func TestClearListeningMarkerIsNoOpWhenNeverArmed(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	if err := ClearListeningMarker("never-armed-key", os.Getpid()); err != nil {
		t.Fatalf("ClearListeningMarker on never-armed key: %v", err)
	}
}

// TestListeningMarkerPathRejectsPathTraversal pins the path-safety guard a
// round-1 correctness review flagged Critical: an unvalidated session_key
// interpolated into a filename lets a caller escape the listening directory
// (verified live: "../../escaped/pwned" wrote a marker one level above the
// cache root before this guard existed).
func TestListeningMarkerPathRejectsPathTraversal(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	for _, bad := range []string{
		"../../escaped/pwned",
		"nested/path",
		"UPPER-CASE",
		"trailing.dot.",
		"",
		"has a space",
	} {
		if _, err := ListeningMarkerPath(bad); err == nil {
			t.Fatalf("ListeningMarkerPath(%q) succeeded, want a path-safety rejection", bad)
		}
		if err := WriteListeningMarker(ListeningMarker{SessionKey: bad}); err == nil {
			t.Fatalf("WriteListeningMarker with session_key %q succeeded, want a path-safety rejection", bad)
		}
	}
}
