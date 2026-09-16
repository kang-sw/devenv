package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRationaleQueryListedAndSessionKeyed is Phase 1 acceptance test 12: the
// tool is advertised in tools/list and requires a session_key like git.log.
func TestRationaleQueryListedAndSessionKeyed(t *testing.T) {
	if !toolSchemaRequiresSessionKey("rationale.query") {
		t.Fatal("toolSchemaRequiresSessionKey(\"rationale.query\") = false, want true")
	}

	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	listResp := callToolsList(t, server)
	if !toolNameListed(t, listResp, "rationale.query") {
		t.Fatalf("tools/list missing rationale.query: %s", listResp)
	}
	props := toolPropertiesByName(t, listResp, "rationale.query")
	if _, ok := props["session_key"]; !ok {
		t.Fatalf("rationale.query schema missing session_key property: %v", props)
	}
	if _, ok := props["site"]; !ok {
		t.Fatalf("rationale.query schema missing site property: %v", props)
	}
}

// TestRationaleQueryDispatchMapsArguments exercises the tools/call dispatch and
// the JSON-argument-to-Options mapping in server.go (which wsrationale's own
// tests bypass), so a typo'd or dropped argument key would be caught.
func TestRationaleQueryDispatchMapsArguments(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustWrite(t, root, "svc/handler.go", "package svc\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "--cleanup=verbatim", "-m",
		"seed\n\n## AI Context\n- wired the widget for 260701-feat-widget\n")

	server := NewServer(root, "test")
	key, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatalf("mint key: %v", err)
	}

	// query + format=json map through the dispatch.
	jsonResp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
		"session_key": key, "query": "widget", "format": "json",
	}))
	if !strings.Contains(jsonResp, "wired the widget") || !strings.Contains(jsonResp, "scanned_commits") {
		t.Fatalf("query/format args did not map through dispatch: %s", jsonResp)
	}

	// paths (array) maps: a non-matching glob excludes the record, a matching one keeps it.
	miss := toolText(t, callToolOnce(t, server, 3, "rationale.query", map[string]any{
		"session_key": key, "paths": []any{"no/such/dir"},
	}))
	if strings.Contains(miss, "wired the widget") {
		t.Fatalf("paths arg did not map (non-matching glob still returned the record): %s", miss)
	}
	hit := toolText(t, callToolOnce(t, server, 4, "rationale.query", map[string]any{
		"session_key": key, "paths": []any{"svc"},
	}))
	if !strings.Contains(hit, "wired the widget") {
		t.Fatalf("paths arg did not map (matching glob missed the record): %s", hit)
	}

	// exclude_stem (string) maps through.
	excl := toolText(t, callToolOnce(t, server, 5, "rationale.query", map[string]any{
		"session_key": key, "exclude_stem": "260701-feat-widget",
	}))
	if strings.Contains(excl, "wired the widget") {
		t.Fatalf("exclude_stem arg did not map (record not dropped): %s", excl)
	}
}

// commitDated stages files and commits them at a given author/committer date
// (YYYY-MM-DD), returning the new commit's short hash. wsrationale's own
// commitFixture helper lives in an unexported package, so the dispatch tests
// below need their own minimal equivalent.
func commitDated(t *testing.T, root, date, message string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		mustWrite(t, root, rel, content)
	}
	runGit(t, root, "add", "-A")
	t.Setenv("GIT_AUTHOR_DATE", date+"T00:00:00")
	t.Setenv("GIT_COMMITTER_DATE", date+"T00:00:00")
	runGit(t, root, "commit", "--cleanup=verbatim", "-m", message)
	out := runGitOutput(t, root, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(string(out))
}

// TestRationaleQueryDispatchMapsRemainingArguments extends
// TestRationaleQueryDispatchMapsArguments to the other dispatch keys at
// server.go:981-994 that test only left implicitly covered: site, occurrence,
// pickaxe, since, until, stems, kinds, order, and limit. Each subtest isolates
// one (or a naturally paired two) argument's effect on the Options field it
// should map to, so a typo'd or dropped key at the MCP boundary shows up as a
// silently zero-valued field rather than a passing test.
func TestRationaleQueryDispatchMapsRemainingArguments(t *testing.T) {
	t.Run("site and occurrence", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "seed\n\n## AI Context\n- seeded duplicate markers\n",
			map[string]string{"dup.go": "// foo one\n// foo two\n"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		// site (regex form, two matches) maps: without occurrence, the
		// K-locations note names the default occurrence.
		noOccurrence := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "site": "dup.go:/foo/,+1",
		}))
		if !strings.Contains(noOccurrence, "site matched 2 locations; used occurrence 1") {
			t.Fatalf("site arg did not map (missing K-locations note): %s", noOccurrence)
		}

		// occurrence maps: an explicit occurrence (even the default's own
		// value would keep the note only when Occurrence < 1) suppresses the
		// note, proving the argument reached Options.Occurrence rather than
		// staying at its zero-value fallback.
		withOccurrence := toolText(t, callToolOnce(t, server, 3, "rationale.query", map[string]any{
			"session_key": key, "site": "dup.go:/foo/,+1", "occurrence": 2,
		}))
		if strings.Contains(withOccurrence, "used occurrence 1") {
			t.Fatalf("occurrence arg did not map (K-locations note still present for explicit occurrence=2): %s", withOccurrence)
		}
		if !strings.Contains(withOccurrence, "site: introduced") {
			t.Fatalf("occurrence call should still resolve a site chain summary: %s", withOccurrence)
		}
	})

	t.Run("pickaxe", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "introduce\n\n## AI Context\n- introduced the pickaxe token\n",
			map[string]string{"token.go": "const K = \"PICKAXETOKEN\"\n"})
		commitDated(t, root, "2026-01-02", "remove\n\n## AI Context\n- removed the pickaxe token\n",
			map[string]string{"token.go": "const K = \"\"\n"})
		commitDated(t, root, "2026-01-03", "unrelated\n\n## AI Context\n- an unrelated change\n",
			map[string]string{"other.go": "package other\n"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		resp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "pickaxe": "PICKAXETOKEN",
		}))
		// pickaxe mode's own `git log -S` scan sees only the 2 commits that
		// touch the token, not the 3 in the full default scan — a dropped
		// pickaxe arg would fall through to the default scan and report 3.
		if !strings.Contains(resp, "omitted: scanned 2 commits") {
			t.Fatalf("pickaxe arg did not map (expected a pickaxe-scoped 2-commit scan): %s", resp)
		}
		if !strings.Contains(resp, "introduced the pickaxe token") || !strings.Contains(resp, "removed the pickaxe token") {
			t.Fatalf("pickaxe scan missing its introducing/removing commits: %s", resp)
		}
		if strings.Contains(resp, "an unrelated change") {
			t.Fatalf("pickaxe scan should not include the unrelated commit: %s", resp)
		}
	})

	t.Run("since and until", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "old\n\n## AI Context\n- old decision recorded\n", map[string]string{"a.txt": "1"})
		commitDated(t, root, "2026-01-05", "mid\n\n## AI Context\n- mid decision recorded\n", map[string]string{"b.txt": "2"})
		commitDated(t, root, "2026-01-10", "new\n\n## AI Context\n- new decision recorded\n", map[string]string{"c.txt": "3"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		sinceResp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "since": "2026-01-05",
		}))
		if strings.Contains(sinceResp, "old decision recorded") {
			t.Fatalf("since arg did not map (a before-bound record was not excluded): %s", sinceResp)
		}
		if !strings.Contains(sinceResp, "mid decision recorded") || !strings.Contains(sinceResp, "new decision recorded") {
			t.Fatalf("since arg over-filtered on-or-after-bound records: %s", sinceResp)
		}

		untilResp := toolText(t, callToolOnce(t, server, 3, "rationale.query", map[string]any{
			"session_key": key, "until": "2026-01-05",
		}))
		if strings.Contains(untilResp, "new decision recorded") {
			t.Fatalf("until arg did not map (an after-bound record was not excluded): %s", untilResp)
		}
		if !strings.Contains(untilResp, "old decision recorded") || !strings.Contains(untilResp, "mid decision recorded") {
			t.Fatalf("until arg over-filtered on-or-before-bound records: %s", untilResp)
		}
	})

	t.Run("stems", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "seed\n\n## AI Context\n- seeded work for 260701-feat-alpha\n", map[string]string{"a.txt": "1"})
		commitDated(t, root, "2026-01-02", "other\n\n## AI Context\n- unrelated work for 260702-feat-beta\n", map[string]string{"b.txt": "2"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		resp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "stems": []any{"260701-feat-alpha"},
		}))
		if !strings.Contains(resp, "seeded work for 260701-feat-alpha") {
			t.Fatalf("stems arg did not map (matching stem's record was dropped): %s", resp)
		}
		if strings.Contains(resp, "unrelated work for 260702-feat-beta") {
			t.Fatalf("stems arg did not map (non-matching stem's record was not filtered out): %s", resp)
		}
	})

	t.Run("kinds", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		mustWrite(t, root, "ai-docs/tickets/todo/260701-feat-kinds.md",
			"---\ntitle: Kinds Ticket\n---\n# Kinds Ticket\n\n## Decisions\n- keep the kinds filter honest\n")
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "seed\n\n## AI Context\n- recorded a commit decision\n", map[string]string{"a.txt": "1"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		resp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "kinds": []any{"ticket"},
		}))
		if !strings.Contains(resp, "keep the kinds filter honest") {
			t.Fatalf("kinds arg did not map (ticket record was dropped): %s", resp)
		}
		if strings.Contains(resp, "recorded a commit decision") {
			t.Fatalf("kinds arg did not map (commit record was not filtered out): %s", resp)
		}
	})

	t.Run("order", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		// Distinct stems put each commit in its own thread, so cross-thread
		// order is what the assertions read. commit1 repeats the query term
		// to win on relevance despite being older; commit2 is newer and wins
		// on time.
		commitDated(t, root, "2026-01-01", "seed\n\n## AI Context\n- alpha term term term for 260701-feat-alpha\n", map[string]string{"a.txt": "1"})
		commitDated(t, root, "2026-01-05", "mid\n\n## AI Context\n- beta term appears once for 260702-feat-beta\n", map[string]string{"b.txt": "2"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		relevance := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "query": "term",
		}))
		alphaIdx, betaIdx := strings.Index(relevance, "260701-feat-alpha"), strings.Index(relevance, "260702-feat-beta")
		if alphaIdx < 0 || betaIdx < 0 || alphaIdx > betaIdx {
			t.Fatalf("default relevance order should rank the higher-scoring older thread first: %s", relevance)
		}

		byTime := toolText(t, callToolOnce(t, server, 3, "rationale.query", map[string]any{
			"session_key": key, "query": "term", "order": "time",
		}))
		alphaIdx, betaIdx = strings.Index(byTime, "260701-feat-alpha"), strings.Index(byTime, "260702-feat-beta")
		if alphaIdx < 0 || betaIdx < 0 || betaIdx > alphaIdx {
			t.Fatalf("order arg did not map (order=time should rank the newer thread first): %s", byTime)
		}
	})

	t.Run("limit", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		commitDated(t, root, "2026-01-01", "seed\n\n## AI Context\n- decision A\n", map[string]string{"a.txt": "1"})
		commitDated(t, root, "2026-01-02", "next\n\n## AI Context\n- decision B\n", map[string]string{"b.txt": "2"})
		commitDated(t, root, "2026-01-03", "last\n\n## AI Context\n- decision C\n", map[string]string{"c.txt": "3"})

		server := NewServer(root, "test")
		key, err := server.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatalf("mint key: %v", err)
		}

		resp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
			"session_key": key, "limit": 1,
		}))
		if !strings.Contains(resp, "2 records past limit") {
			t.Fatalf("limit arg did not map (expected a capped scan to report 2 records past limit): %s", resp)
		}
	})
}
