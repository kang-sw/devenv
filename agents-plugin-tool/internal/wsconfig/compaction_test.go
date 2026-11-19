package wsconfig

import (
	"encoding/json"
	"os"
	"testing"
)

type compactionSession map[string]string

func (s compactionSession) GetOverride(_, key string) (string, bool) { v, ok := s[key]; return v, ok }

func TestCompactionGrammar(t *testing.T) {
	for _, v := range []string{"50", "50%", "1", "100%", "150k", "1.5m", "0.5k", "1.0004k", "007k", "50%|150k", "150k|50%", "100|1m"} {
		if _, err := ParseThreshold(v); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	for _, v := range []string{"0", "101%", "050", "50.5", "-5", "", "0k", "0.0m", "150K", ".5k", "5.k", "50|60", "1k|2m", "50%|1k|2m"} {
		if _, err := ParseThreshold(v); err == nil {
			t.Errorf("accepted %q", v)
		}
	}
	th, _ := ParseThreshold("50%|150k")
	if th.Point(200000) != 100000 || th.Point(400000) != 150000 {
		t.Fatal(th)
	}
}

func TestCompactionLayerMajorAndLegacyRoundTrip(t *testing.T) {
	opts := Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir(), RepoRoot: t.TempDir()}
	// These are the unchanged old global/project writers: only one map entry
	// is mutated, so new scalar buckets survive an intervening old-key write.
	r := NewResolver(opts, nil, compactionSession{}, nil)
	key := "compaction.lead_advisory"
	legacy := "pi.compaction_advisory_percent"
	set := func(k, v string, scope Scope) {
		t.Helper()
		if err := r.Set(k, v, SetOptions{ExplicitScope: scope}); err != nil {
			t.Fatal(err)
		}
	}
	set(key+".pi", "88%", ScopeGlobal)
	set(legacy, "99%", ScopeGlobal)
	if got, err := r.Get("", key+".pi"); err != nil || got.Value != "88%" {
		t.Fatalf("old global write lost new scalar: %+v %v", got, err)
	}
	set(key, "40%", ScopeProject)
	check := func(want string, scope Scope) {
		t.Helper()
		got, _, err := r.Compaction("session", key, "pi", legacy)
		if err != nil || got.Value != want || got.Scope != scope {
			t.Fatalf("got %+v %v, want %s %s", got, err, want, scope)
		}
	}
	check("40%", ScopeProject)
	set(legacy, "35%", ScopeProject)
	check("40%", ScopeProject)
	if err := r.Unset(key, SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	check("35%", ScopeProject)
	set(key+".pi", "30%", ScopeProject)
	check("30%", ScopeProject)
	set(legacy, "25%", ScopeProject)
	check("30%", ScopeProject)
	// Decode/re-encode the old string map, then reopen the new resolver.
	for _, scope := range []Scope{ScopeGlobal, ScopeProject} {
		var path string
		var err error
		if scope == ScopeGlobal {
			path, err = GlobalPath(opts)
		} else {
			path, err = Path(opts)
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var old Config
		if err = json.Unmarshal(raw, &old); err != nil {
			t.Fatal(err)
		}
		raw, err = json.Marshal(old)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	r = NewResolver(opts, nil, compactionSession{}, nil)
	check("30%", ScopeProject)
	if err := r.Unset(key+".pi", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	check("25%", ScopeProject)
	if err := r.Unset(legacy, SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	set(legacy, "20%", ScopeRepo)
	check("20%", ScopeRepo)
	r.sessionR = compactionSession{key: "15%"}
	check("15%", ScopeSession)
	r.sessionR = compactionSession{legacy: "10%"}
	check("10%", ScopeSession)
	r.sessionR = compactionSession{key + ".pi": "5%", key: "15%", legacy: "10%"}
	check("5%", ScopeSession)
	for key, def := range CompactionDefaults {
		clean := NewResolver(Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}, nil, nil, nil)
		got, _, err := clean.Compaction("", key, "pi", "")
		if err != nil || got.Value != def || got.Scope != ScopeBuiltin {
			t.Fatalf("default %s: %+v %v", key, got, err)
		}
	}
	// Lead legacy values are not consulted for worker keys.
	got, _, err := r.Compaction("session", "compaction.worker_advisory", "pi", "")
	if err != nil || got.Value != "50%" {
		t.Fatalf("worker inherited lead legacy: %+v %v", got, err)
	}
}
