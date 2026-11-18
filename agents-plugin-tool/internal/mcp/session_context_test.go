package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestResolveSessionSelection(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	canonical := canonicalRootForTest(t, root)
	lead, err := s.sessions.mint(canonical, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.sessions.mint(canonical, roleDelegate, lead)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sessions.mutateRecord(child, partKey, func(rec *sessionRecord) error {
		rec.Harness, rec.HarnessSession, rec.HarnessCwd = "pi", "/historical/session.jsonl", root
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The requested root is not authority: lookup projects the key's root.
	got, err := s.resolveSession(sessionAccess{mode: sessionAccessLookup, key: " " + child + " ", root: t.TempDir(), parent: "display-parent"})
	if err != nil || !got.found || got.key != child || got.scope != roleDelegate || got.parent != lead || got.root != canonical || got.hosted {
		t.Fatalf("CLI context = %+v, %v", got, err)
	}

	// A key-record child alone is no descendant on a hosted connection; the
	// Runner record of an agent.spawn child is (hostedDescendant).
	orphan, err := s.sessions.mint(canonical, roleDelegate, lead)
	if err != nil {
		t.Fatal(err)
	}
	hostedSessionEnv(t, lead)
	s = NewServer(root, "test")
	recordSpawned(t, s, canonical, child, wsmailbox.ReplyID(lead), wsagent.ParentLead)
	before := sessionKeyFileCount(t, s)
	for _, tt := range []struct {
		name        string
		in          sessionAccess
		key, parent string
		scope       toolRole
		hosted      bool
	}{
		{"hosted bootstrap", sessionAccess{mode: sessionAccessBootstrap, root: canonical, scope: roleLead}, lead, "", roleLead, true},
		{"hosted relogin", sessionAccess{mode: sessionAccessRelogin, key: lead, root: canonical, scope: roleLead}, lead, "", roleLead, true},
		{"explicit child", sessionAccess{mode: sessionAccessManual, key: child}, child, lead, roleDelegate, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.resolveSession(tt.in)
			if err != nil || !got.found || got.key != tt.key || got.parent != tt.parent || got.scope != tt.scope || got.root != canonical || got.hosted != tt.hosted {
				t.Fatalf("context = %+v, %v", got, err)
			}
		})
	}
	for _, tt := range []struct {
		name string
		in   sessionAccess
		want string
	}{
		{"sentinel", sessionAccess{mode: sessionAccessManual, key: freshBootstrapKey, root: canonical}, "workflow_manual:"},
		{"key-record child", sessionAccess{mode: sessionAccessManual, key: orphan}, "workflow_manual:"},
		{"relogin mismatch", sessionAccess{mode: sessionAccessRelogin, key: child, root: canonical, scope: roleLeaf, parent: lead}, "session bootstrap:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.resolveSession(tt.in)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) || !strings.Contains(err.Error(), envHostedSessionKey) || !strings.Contains(err.Error(), lead) {
				t.Fatalf("refusal = %v", err)
			}
		})
	}
	if after := sessionKeyFileCount(t, s); after != before {
		t.Fatalf("adoption/refusal minted keys: %d -> %d", before, after)
	}
	for _, mode := range []sessionAccessMode{sessionAccessBootstrap, sessionAccessMint} {
		got, err := s.resolveSession(sessionAccess{mode: mode, root: canonical, scope: roleLead, parent: lead})
		if err != nil || !got.found || got.key == lead || got.key == child || got.parent != lead || got.scope != roleLead || got.hosted {
			t.Fatalf("%s child = %+v, %v", mode, got, err)
		}
	}
}

func TestResolveSessionMintPreservesParent(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	parent := " padded-parent "
	got, err := s.resolveSession(sessionAccess{mode: sessionAccessMint, root: canonicalRootForTest(t, root), scope: roleDelegate, parent: parent})
	if err != nil || !got.found || got.parent != parent {
		t.Fatalf("mint context = %+v, %v; want exact parent %q", got, err, parent)
	}
	rec, err := s.sessions.readStateWithError(got.key, partKey)
	if err != nil || rec.Parent != parent {
		t.Fatalf("mint record = %+v, %v; want exact parent %q", rec, err, parent)
	}
}

func TestResolveSessionHistoricalRecordsStayReadable(t *testing.T) {
	root, s := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	canonical := canonicalRootForTest(t, root)
	dirs, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		name, path, version := "split-history", "", 2
		if legacy {
			name, version = "legacy-history", 1
			path = dirs.legacyPath(name)
		} else {
			path = filepath.Join(dirs.keyDir(name), "key.json")
		}
		raw, err := json.Marshal(map[string]any{"schema_version": version, "root": canonical, "scope": "lead", "parent": "historical-parent", "harness": "claude", "harness_session": "old-native-session", "harness_cwd": root})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := s.resolveSession(sessionAccess{mode: sessionAccessLookup, key: name})
		if err != nil || !got.found || got.key != name || got.scope != roleLead || got.parent != "historical-parent" || got.root != canonical {
			t.Fatalf("%s = %+v, %v", name, got, err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, after) {
			t.Fatalf("lookup rewrote historical record: %s, %v", after, err)
		}
		rec, err := s.sessions.readStateWithError(name, partKey)
		if err != nil || rec.Harness != "claude" || rec.HarnessSession != "old-native-session" || rec.HarnessCwd != root {
			t.Fatalf("historical harness metadata = %+v, %v", rec, err)
		}
		if legacy {
			if _, err := os.Stat(dirs.keyDir(name)); !os.IsNotExist(err) {
				t.Fatalf("read migrated legacy record: %v", err)
			}
		}
	}
}
