package mcp

import (
	"os"
	"testing"
	"time"
)

func TestSessionAccessCachedReadsKeepTouchAndRevocationBounds(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	key, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	path := d.partPath(key, partKey)
	old := time.Now().Add(-2 * touchGuardWindow)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	advance := intakeClock(s)
	reads := countKeyReads(t, s, key)
	if entry, found, transient := s.activeSession(key); !found || transient || entry.root != root {
		t.Fatalf("cold timer inspection = %+v, %v, %v", entry, found, transient)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(old) {
		t.Fatalf("timer inspection touched activity: %+v, %v", info, err)
	}
	if entry, found := s.intakeSession(key); !found || entry.root != root {
		t.Fatalf("warm intake = %+v, %v", entry, found)
	}
	info, err = os.Stat(path)
	if err != nil || !info.ModTime().After(old) {
		t.Fatalf("intake did not touch old activity: %+v, %v", info, err)
	}
	for i := 0; i < 5; i++ {
		s.activeSession(key)
		s.intakeSession(key)
	}
	if got := reads.Load(); got != 1 {
		t.Fatalf("warm cache re-read key.json: %d reads, want cold read only", got)
	}
	if err := os.RemoveAll(d.keyDir(key)); err != nil {
		t.Fatal(err)
	}
	if _, found := s.lookupSession(key); found {
		t.Fatal("ordinary keyed access delayed revocation")
	}
	if _, found, transient := s.activeSession(key); !found || transient {
		t.Fatal("cached timer view changed before the existing recheck bound")
	}
	advance(intakeRecheck + time.Second)
	if _, found, transient := s.activeSession(key); found || transient {
		t.Fatalf("timer after revocation bound = found %v, transient %v", found, transient)
	}
	if _, found := s.intakeSession(key); found {
		t.Fatal("intake kept a revoked key beyond the existing recheck bound")
	}
}

func TestSessionAccessTimerRetainsTransientReadDistinction(t *testing.T) {
	root, s := hostedLoginEnv(t)
	key, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	s.sessions.readFile = func(string, func() any) ([]byte, any, error) {
		return nil, nil, os.ErrPermission
	}
	if _, found, transient := s.activeSession(key); found || !transient {
		t.Fatalf("unreadable timer inspection = found %v, transient %v", found, transient)
	}
	if _, found := s.intakeSession(key); found {
		t.Fatal("cold intake treated an unreadable identity as present")
	}
	s.sessions.readFile = nil
	if _, found, transient := s.activeSession(key); !found || transient {
		t.Fatalf("recovered timer inspection = found %v, transient %v", found, transient)
	}
}
