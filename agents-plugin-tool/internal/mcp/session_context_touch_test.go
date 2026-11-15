package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveSessionLookupRetainsSecondReadAndTouch(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	key, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dirs.keyDir(key), "key.json")
	old := time.Now().Add(-2 * touchGuardWindow)
	reads := 0
	s.sessions.readFile = func(path string, newValue func() any) ([]byte, any, error) {
		reads++
		if reads == 2 {
			// Another writer may replace/backdate the activity mark after lookup's
			// first touch. The serialized read must still perform its own touch.
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
		return readRecordFile(path, newValue)
	}
	got, err := s.resolveSession(sessionAccess{mode: sessionAccessLookup, key: key})
	if err != nil || !got.found || reads != 2 {
		t.Fatalf("lookup = %+v, %v; reads %d, want 2", got, err, reads)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(old) {
		t.Fatalf("second read left activity mark at %v", info.ModTime())
	}
}
