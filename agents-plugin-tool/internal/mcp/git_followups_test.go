package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// followupsFixture is a one-commit repo with a pending change to src.txt and a
// server whose session store the caller mints keys into.
func followupsFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustWrite(t, root, "src.txt", "base\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "seed")
	mustWrite(t, root, "src.txt", "changed\n")
	return NewServer(root, "test"), root, headBranch(t, root)
}

func followupCommitArgs(key, branch string, extra map[string]any) map[string]any {
	args := map[string]any{
		"paths":           []string{"src.txt"},
		"title":           "chore(review): stamp",
		"ai_context":      []string{"review stamped"},
		"expected_branch": branch,
	}
	if key != "" {
		args["session_key"] = key
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

var oneFollowup = []any{map[string]any{"level": "minor", "category": "review", "content": "src.txt:1 duplicated literal"}}

func headSubject(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(string(runGitOutput(t, root, "log", "-1", "--format=%s")))
}

func TestGitCommitFollowupsRequireRootLeadSession(t *testing.T) {
	server, root, branch := followupsFixture(t)
	rootLead, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	childLead, err := server.sessions.mint(root, roleLead, rootLead)
	if err != nil {
		t.Fatal(err)
	}
	delegate, err := server.sessions.mint(root, roleDelegate, rootLead)
	if err != nil {
		t.Fatal(err)
	}

	for i, tc := range []struct{ label, key string }{
		{"no session key", ""},
		{"child lead (worker) key", childLead},
		{"delegate key", delegate},
	} {
		resp := callToolOnce(t, server, 10+i, "git.commit", followupCommitArgs(tc.key, branch, map[string]any{"followups": oneFollowup}))
		if !toolIsError(t, resp) {
			t.Fatalf("%s: followups commit accepted: %s", tc.label, resp)
		}
		if got := headSubject(t, root); got != "seed" {
			t.Fatalf("%s: a refused followups commit still landed %q", tc.label, got)
		}
		if staged := strings.TrimSpace(string(runGitOutput(t, root, "diff", "--cached", "--name-only"))); staged != "" {
			t.Fatalf("%s: refused followups commit staged %q", tc.label, staged)
		}
	}
	// An empty-but-present followups argument is still gated.
	resp := callToolOnce(t, server, 20, "git.commit", followupCommitArgs(childLead, branch, map[string]any{"followups": []any{}}))
	if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "root lead") {
		t.Fatalf("empty followups from a child key = %s", resp)
	}

	// The root lead records them and gets the minted id back.
	resp = callToolOnce(t, server, 21, "git.commit", followupCommitArgs(rootLead, branch, map[string]any{
		"followups": []any{
			map[string]any{"level": "minor", "category": "review", "content": "src.txt:1 duplicated literal"},
			map[string]any{"level": "important", "category": "review", "content": "src.txt:2 untested edge"},
		},
		"format": "json",
	}))
	if toolIsError(t, resp) {
		t.Fatalf("root lead followups commit refused: %s", resp)
	}
	var result struct {
		Hash      string `json:"hash"`
		Followups []struct {
			ID, Level, Category, Content string
		} `json:"followups"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &result); err != nil {
		t.Fatalf("parse result: %v\n%s", err, resp)
	}
	if len(result.Followups) != 2 || result.Followups[0].Level != "minor" || result.Followups[1].Level != "important" ||
		len(strings.Split(result.Followups[0].ID, "-")) != 3 || result.Followups[0].ID == result.Followups[1].ID {
		t.Fatalf("minted followups = %#v", result.Followups)
	}
	body := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
	if !strings.Contains(body, "## Follow-ups\n- minor/review "+result.Followups[0].ID+": src.txt:1 duplicated literal") {
		t.Fatalf("commit body missing follow-up section:\n%s", body)
	}

	// git.followups lists both as open, and min_level maps through dispatch.
	listed := toolText(t, callToolOnce(t, server, 22, "git.followups", map[string]any{"session_key": delegate, "min_level": "important", "category": "review"}))
	if !strings.Contains(listed, "open follow-ups: 1") || !strings.Contains(listed, result.Followups[1].ID) || strings.Contains(listed, result.Followups[0].ID) {
		t.Fatalf("git.followups filtered listing = %s", listed)
	}
}

func TestGitCommitResolvesAcceptedFromNonLeadWithWarnings(t *testing.T) {
	server, root, branch := followupsFixture(t)
	rootLead, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	resp := callToolOnce(t, server, 1, "git.commit", followupCommitArgs(rootLead, branch, map[string]any{"followups": oneFollowup, "format": "json"}))
	var stamped struct {
		Followups []struct{ ID string } `json:"followups"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &stamped); err != nil || len(stamped.Followups) != 1 {
		t.Fatalf("stamp commit = %s (%v)", resp, err)
	}
	id := stamped.Followups[0].ID

	worker, err := server.sessions.mint(root, roleLead, rootLead)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "src.txt", "fixed\n")
	resp = callToolOnce(t, server, 2, "git.commit", followupCommitArgs(worker, branch, map[string]any{
		"title":    "fix: duplicated literal",
		"resolves": []any{id, "never-recorded-id"},
	}))
	if toolIsError(t, resp) {
		t.Fatalf("worker resolves commit refused: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "warnings:") || !strings.Contains(text, "never-recorded-id") || strings.Contains(text, "resolves "+id+":") {
		t.Fatalf("resolve warnings = %s", text)
	}
	if got := headSubject(t, root); got != "fix: duplicated literal" {
		t.Fatalf("resolve commit did not land: %q", got)
	}
	listed := toolText(t, callToolOnce(t, server, 3, "git.followups", map[string]any{"session_key": worker}))
	if !strings.Contains(listed, "open follow-ups: 0") {
		t.Fatalf("resolved follow-up still open: %s", listed)
	}

	// Resolving it again warns as already resolved; the commit still lands.
	mustWrite(t, root, "src.txt", "fixed twice\n")
	resp = callToolOnce(t, server, 4, "git.commit", followupCommitArgs(worker, branch, map[string]any{"title": "fix: again", "resolves": []any{id}}))
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "already resolved") {
		t.Fatalf("already-resolved warning = %s", resp)
	}
}

func TestGitFollowupsListedAndSessionKeyed(t *testing.T) {
	if !toolSchemaRequiresSessionKey("git.followups") {
		t.Fatal("git.followups must require a session_key")
	}
	server, _, _ := followupsFixture(t)
	listResp := callToolsList(t, server)
	if !toolNameListed(t, listResp, "git.followups") {
		t.Fatalf("tools/list missing git.followups: %s", listResp)
	}
	props := toolPropertiesByName(t, listResp, "git.followups")
	for _, name := range []string{"session_key", "range", "category", "min_level"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("git.followups schema missing %s: %v", name, props)
		}
	}
	commitProps := toolPropertiesByName(t, listResp, "git.commit")
	for _, name := range []string{"followups", "resolves"} {
		if _, ok := commitProps[name]; !ok {
			t.Fatalf("git.commit schema missing %s: %v", name, commitProps)
		}
	}
	for _, scope := range []toolRole{roleLead, roleDelegate, roleLeaf} {
		if !roleAllowsTool(scope, "git.followups") {
			t.Fatalf("read-only git.followups denied to %s", scope)
		}
	}
}
