package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	got, err := s.resolveSession(sessionAccess{mode: "lookup", key: " " + child + " ", root: t.TempDir(), parent: "display-parent"})
	if err != nil || !got.found || got.key != child || got.scope != roleDelegate || got.parent != lead || got.root != canonical || got.canonicalRoot != canonical || got.hosted || got.harness != "pi" || got.harnessSession != "/historical/session.jsonl" || got.harnessCwd != root {
		t.Fatalf("CLI context = %+v, %v", got, err)
	}

	hostedSessionEnv(t, lead)
	s = NewServer(root, "test")
	before := sessionKeyFileCount(t, s)
	for _, tt := range []struct {
		name        string
		in          sessionAccess
		key, parent string
		scope       toolRole
		hosted      bool
	}{
		{"hosted bootstrap", sessionAccess{mode: "bootstrap", root: canonical, scope: roleLead}, lead, "", roleLead, true},
		{"hosted relogin", sessionAccess{mode: "relogin", key: lead, root: canonical, scope: roleLead}, lead, "", roleLead, true},
		{"explicit child", sessionAccess{mode: "manual", key: child}, child, lead, roleDelegate, false},
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
		{"sentinel", sessionAccess{mode: "manual", key: freshBootstrapKey, root: canonical}, "workflow_manual:"},
		{"relogin mismatch", sessionAccess{mode: "relogin", key: child, root: canonical, scope: roleLeaf, parent: lead}, "session bootstrap:"},
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
	for _, mode := range []string{"bootstrap", "mint"} {
		got, err := s.resolveSession(sessionAccess{mode: mode, root: canonical, scope: roleLead, parent: lead})
		if err != nil || !got.found || got.key == lead || got.key == child || got.parent != lead || got.scope != roleLead || got.hosted {
			t.Fatalf("%s child = %+v, %v", mode, got, err)
		}
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
		got, err := s.resolveSession(sessionAccess{mode: "lookup", key: name})
		if err != nil || !got.found || got.key != name || got.scope != roleLead || got.parent != "historical-parent" || got.root != canonical || got.harness != "claude" || got.harnessSession != "old-native-session" || got.harnessCwd != root {
			t.Fatalf("%s = %+v, %v", name, got, err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, after) {
			t.Fatalf("lookup rewrote historical record: %s, %v", after, err)
		}
		if legacy {
			if _, err := os.Stat(dirs.keyDir(name)); !os.IsNotExist(err) {
				t.Fatalf("read migrated legacy record: %v", err)
			}
		}
	}
}
