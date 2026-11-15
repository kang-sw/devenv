package mcp

import "testing"

func TestSessionAccessIdentityMintKeepsStoreOnlyBehavior(t *testing.T) {
	root, seed := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	lead, err := seed.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	hostedSessionEnv(t, lead)
	s := NewServer(root, "test")
	before := sessionKeyFileCount(t, s)
	reads := 0
	s.sessions.readFile = func(path string, newValue func() any) ([]byte, any, error) {
		reads++
		return readRecordFile(path, newValue)
	}
	childRoot := t.TempDir()
	got, err := s.resolveSession(sessionAccess{mode: sessionAccessMintIdentity, root: childRoot, scope: roleLead, parent: lead})
	if err != nil || !got.found || got.key == "" || got.key == lead || got.root != childRoot || got.scope != roleLead || got.parent != lead || got.hosted {
		t.Fatalf("identity mint = %+v, %v", got, err)
	}
	if reads != 0 || s.hostedKey != "" {
		t.Fatalf("store-only mint added a record read or hosted adoption: reads %d, hosted key %q", reads, s.hostedKey)
	}
	if entry, found := s.lookupSession(got.key); !found || entry != got.entry() || entry.isTopLevelLead() {
		t.Fatalf("minted identity = %+v, %v; projected %+v", entry, found, got.entry())
	}
	if after := sessionKeyFileCount(t, s); after != before+1 {
		t.Fatalf("identity mint created %d records, want one", after-before)
	}
}
