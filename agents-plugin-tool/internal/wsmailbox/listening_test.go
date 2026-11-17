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

	// The marker lives in the store: no per-marker file is written.
	if _, err := os.Stat(filepath.Join(os.Getenv("WS_CACHE_HOME"), "mailbox-listening")); !os.IsNotExist(err) {
		t.Fatalf("a listening marker file dir was created: err=%v", err)
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

func TestClearListeningMarkerIsNoOpWhenNeverArmed(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	if err := ClearListeningMarker("never-armed-key", os.Getpid()); err != nil {
		t.Fatalf("ClearListeningMarker on never-armed key: %v", err)
	}
}

// TestListeningMarkerRejectsInvalidSessionKey pins the session_key guard,
// once the marker file name's path-safety check (a round-1 review found
// "../../escaped/pwned" escaping the listening directory) and kept as the
// key's validation now that markers live in the store.
func TestListeningMarkerRejectsInvalidSessionKey(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	for _, bad := range []string{
		"../../escaped/pwned",
		"nested/path",
		"UPPER-CASE",
		"trailing.dot.",
		"",
		"has a space",
	} {
		if _, _, err := ReadListeningMarker(bad); err == nil {
			t.Fatalf("ReadListeningMarker(%q) succeeded, want a rejection", bad)
		}
		if err := WriteListeningMarker(ListeningMarker{SessionKey: bad}); err == nil {
			t.Fatalf("WriteListeningMarker with session_key %q succeeded, want a rejection", bad)
		}
	}
}
