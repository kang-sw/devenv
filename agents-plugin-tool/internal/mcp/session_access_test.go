package mcp

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// Keyed access is not a fresh adoption: both entry modes keep the caller's
// exact key, scope and lineage even when the server's root or hosted key differs.
func TestSessionAccessIdentityParity(t *testing.T) {
	root, seed := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	canonical := canonicalRootForTest(t, root)
	lead, err := seed.sessions.mint(canonical, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := seed.sessions.mint(canonical, roleLead, lead)
	if err != nil {
		t.Fatal(err)
	}
	delegate, err := seed.sessions.mint(canonical, roleDelegate, lead)
	if err != nil {
		t.Fatal(err)
	}
	for _, hosted := range []bool{false, true} {
		name := "cli-plugin"
		if hosted {
			name = "hosted"
		}
		t.Run(name, func(t *testing.T) {
			if hosted {
				hostedSessionEnv(t, lead)
			}
			s := NewServer(t.TempDir(), "test")
			before := sessionKeyFileCount(t, s)
			for _, tt := range []struct {
				key, parent string
				scope       toolRole
				top         bool
			}{
				{lead, "", roleLead, true},
				{child, lead, roleLead, false},
				{delegate, lead, roleDelegate, false},
			} {
				reads := countKeyReads(t, s, tt.key)
				entry, found := s.lookupSession(tt.key)
				if !found || entry.root != canonical || entry.scope != tt.scope || entry.parent != tt.parent || entry.isTopLevelLead() != tt.top {
					t.Fatalf("%s identity = %+v, %v", tt.key, entry, found)
				}
				if got := reads.Load(); got != 1 {
					t.Fatalf("identity read key.json %d times, want one", got)
				}
				gotRoot, err := s.resolveToolRoot(map[string]any{"session_key": tt.key, "root": s.root}, nil)
				if err != nil || gotRoot != canonical || s.sessionConfigOptions(tt.key).RepoRoot != canonical {
					t.Fatalf("key root = %q, %v", gotRoot, err)
				}
			}
			for _, key := range []string{"", freshBootstrapKey, "unknown-session", " " + lead + " ", "../" + lead} {
				if entry, found := s.lookupSession(key); found || entry != (sessionEntry{}) {
					t.Fatalf("key %q unexpectedly resolved/adopted: %+v", key, entry)
				}
				if _, err := s.resolveToolRoot(map[string]any{"session_key": key}, nil); err == nil {
					t.Fatalf("root access accepted %q", key)
				}
			}
			if _, err := s.requireLeadSessionKey("fixture", map[string]any{"session_key": " " + child + " "}); err != nil {
				t.Fatalf("lead-only consumer lost its existing trim/child behavior: %v", err)
			}
			if err := s.requireRootLeadForFollowups(map[string]any{"session_key": child}); err == nil || !strings.Contains(err.Error(), "child or non-lead") {
				t.Fatalf("child lead gained root authority: %v", err)
			}
			if after := sessionKeyFileCount(t, s); after != before {
				t.Fatalf("keyed access minted records: %d -> %d", before, after)
			}
		})
	}
}

func TestSessionAccessHistoricalIdentityDoesNotMigrate(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	d, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		key := "split-access"
		if legacy {
			key = "legacy-access"
			writeLegacyRecordForTest(t, s.sessions, key, fullLegacyRecord(root, "old-parent"))
		} else {
			key, err = s.sessions.mint(root, roleLead, "old-parent")
			if err != nil {
				t.Fatal(err)
			}
		}
		path := d.partPath(key, partKey)
		if legacy {
			path = d.legacyPath(key)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * touchGuardWindow)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		entry, found := s.lookupSession(key)
		if !found || entry.root != root || entry.parent != "old-parent" || entry.scope != roleLead {
			t.Fatalf("historical identity = %+v, %v", entry, found)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("identity lookup changed stored bytes: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().After(old) {
			t.Fatalf("identity lookup lost its activity touch: %v", err)
		}
		if legacy {
			if _, err := os.Stat(d.keyDir(key)); !os.IsNotExist(err) {
				t.Fatalf("historical read migrated session metadata: %v", err)
			}
		}
	}
}
