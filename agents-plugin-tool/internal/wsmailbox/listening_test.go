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

	if err := ClearListeningMarker(marker.SessionKey); err != nil {
		t.Fatalf("ClearListeningMarker: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker file still present after clear: err=%v", err)
	}
	if _, ok, err := ReadListeningMarker(marker.SessionKey); err != nil || ok {
		t.Fatalf("ReadListeningMarker after clear: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestClearListeningMarkerIsNoOpWhenNeverArmed(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	if err := ClearListeningMarker("never-armed-key"); err != nil {
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
