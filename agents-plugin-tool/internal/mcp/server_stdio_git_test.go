package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wskey"
)

func TestActiveImplTicketAndGitStatusFormatting(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	stem := "260911-feat-active-impl-owner"
	suffix := wskey.Derive(stem, 3)
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", stem+".md"), "---\ntitle: Active owner\n---\n")

	active, err := activeImplTicket(root, "impl/feature/base/"+suffix, nil)
	if err != nil {
		t.Fatalf("activeImplTicket active: %v", err)
	}
	if active == nil || active.State != "active" || active.Stem != stem || active.Status != "ready" {
		t.Fatalf("activeImplTicket = %#v, want ready %q", active, stem)
	}
	status := wsgit.StatusResult{Branch: wsgit.BranchStatus{Head: "impl/feature/base/" + suffix}, Clean: true}
	if got := formatGitStatusWithImplTicket(status, active); !strings.Contains(got, "active ticket: "+stem+" (ready)\n") {
		t.Fatalf("active text = %q", got)
	}

	missing, err := activeImplTicket(root, "impl/feature/base/no-match", nil)
	if err != nil || missing == nil || missing.State != "missing" {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
	if got := formatGitStatusWithImplTicket(status, missing); !strings.Contains(got, "no active ticket matches") {
		t.Fatalf("missing text = %q", got)
	}

	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: Duplicate owner\n---\n")
	ambiguous, err := activeImplTicket(root, "impl/feature/base/"+suffix, nil)
	if err != nil || ambiguous == nil || ambiguous.State != "ambiguous" {
		t.Fatalf("ambiguous = %#v, %v", ambiguous, err)
	}
	if got := formatGitStatusWithImplTicket(status, ambiguous); !strings.Contains(got, "multiple active tickets match") {
		t.Fatalf("ambiguous text = %q", got)
	}

	nonImpl, err := activeImplTicket(root, "feature/base", nil)
	if err != nil || nonImpl != nil {
		t.Fatalf("non-impl = %#v, %v", nonImpl, err)
	}
	if got := formatGitStatusWithImplTicket(status, nil); strings.Contains(got, "ticket:") || strings.Contains(got, "nudge:") {
		t.Fatalf("non-impl format changed: %q", got)
	}
}

func TestActiveImplTicketFailsClosedOnUnreadableInventory(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	if _, err := activeImplTicket(root, "impl/feature/base/no-match", nil); err == nil {
		t.Fatal("activeImplTicket unexpectedly treated a missing ticket inventory as missing ownership")
	}
}

func TestServeStdioGitStatusImplTicketContract(t *testing.T) {
	useLeadProfile(t)
	for _, status := range []string{"idea", "todo", "ready"} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			initGit(t, root)
			stem := "260911-feat-impl-status-" + status
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", status, stem+".md"), "---\ntitle: Owner\n---\n")
			runGit(t, root, "add", ".")
			runGit(t, root, "commit", "-m", "ticket")
			runGit(t, root, "checkout", "-b", "impl/base/"+wskey.Derive(stem, 3))
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
			text := callToolWithKey(t, server, 2, key, "git.status", nil)
			jsonText := callToolWithKey(t, server, 3, key, "git.status", map[string]any{"format": "json"})
			if !strings.Contains(text, "active ticket: "+stem+" ("+status+")") {
				t.Fatalf("text status missing active owner: %s", text)
			}
			for _, want := range []string{`"impl_ticket"`, `"state":"active"`, `"stem":"` + stem + `"`, `"status":"` + status + `"`} {
				if !strings.Contains(jsonText, want) {
					t.Fatalf("JSON status missing %s: %s", want, jsonText)
				}
			}
		})
	}
}

func TestServeStdioGitToolCalls(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	runGit(t, root, "add", "file.txt", "ai-docs/tickets/todo/260503-feat-demo.md")
	runGit(t, root, "commit", "-m", "initial", "-m", "body text")
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	mustWrite(t, root, "file.txt", "one\ntwo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo\n---\n# Demo\n\n### Result (abc123) - 2026-05-04\n\nImplemented.\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.diff","arguments":{"mode":"name_only","paths":["file.txt"],"format":"json"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.log","arguments":{"limit":1,"include_body":true,"format":"json"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git.merge_base","arguments":{"base":"HEAD","head":"HEAD","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"git.diff","arguments":{"mode":"name_only","paths":["file.txt"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"git.log","arguments":{"limit":1,"include_body":true}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"git.merge_base","arguments":{"base":"HEAD","head":"HEAD"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	var diff struct {
		Mode   string   `json:"mode"`
		Paths  []string `json:"paths"`
		Output string   `json:"output"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &diff); err != nil {
		t.Fatal(err)
	}
	if diff.Mode != "name_only" || !strings.Contains(diff.Output, "file.txt") || len(diff.Paths) != 1 || diff.Paths[0] != "file.txt" {
		t.Fatalf("diff response = %#v", diff)
	}

	var log struct {
		Limit       int  `json:"limit"`
		IncludeBody bool `json:"include_body"`
		Commits     []struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["2"])), &log); err != nil {
		t.Fatal(err)
	}
	if log.Limit != 1 || !log.IncludeBody || len(log.Commits) != 1 || log.Commits[0].Subject != "initial" || log.Commits[0].Body != "body text" {
		t.Fatalf("log response = %#v", log)
	}

	var mergeBase struct {
		Base      string `json:"base"`
		Head      string `json:"head"`
		MergeBase string `json:"merge_base"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &mergeBase); err != nil {
		t.Fatal(err)
	}
	if mergeBase.Base != "HEAD" || mergeBase.Head != "HEAD" || mergeBase.MergeBase != head {
		t.Fatalf("merge-base response = %#v, want hash %s", mergeBase, head)
	}
	if got := toolText(t, byID["4"]); got != "file.txt\n" {
		t.Fatalf("git.diff text response = %q", got)
	}
	logText := toolText(t, byID["5"])
	if !strings.Contains(logText, "commit "+head) || !strings.Contains(logText, "subject: initial") || !strings.Contains(logText, "body text") || strings.Contains(logText, `\"body text\"`) {
		t.Fatalf("git.log text response = %q", logText)
	}
	if got := toolText(t, byID["6"]); !strings.Contains(got, "merge_base: "+head) || !strings.Contains(got, "(HEAD HEAD)") {
		t.Fatalf("git.merge_base text response = %q", got)
	}

	out.Reset()
	branch := headBranch(t, root)
	commitInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt","ai-docs/tickets/todo/260503-feat-demo.md"],"title":"test: mcp commit","ai_context":["User intent: verify git.commit.","Verification: server test."],"expected_branch":%q}}}`, branch)
	if err := serveStdioWithSession(t, server, root, commitInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	commitLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(commitLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(commitLines), out.String())
	}
	commitByID := responseLinesByID(t, commitLines)

	commitText := toolText(t, commitByID["7"])
	if !strings.Contains(commitText, "commit: ") ||
		!strings.Contains(commitText, "title: test: mcp commit") ||
		!strings.Contains(commitText, "paths:") ||
		!strings.Contains(commitText, "ticket_changes:") ||
		strings.Contains(commitText, `"ticket_changes"`) {
		t.Fatalf("git.commit text response = %q", commitText)
	}
	commitBody := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
	if !strings.Contains(commitBody, "## AI Context\n- User intent: verify git.commit.\n- Verification: server test.\n\n## Ticket Updates\n") {
		t.Fatalf("git.commit message is not AI Context followed directly by Ticket Updates:\n%s", commitBody)
	}

	mustWrite(t, root, "file.txt", "one\ntwo\nthree\n")
	out.Reset()
	jsonCommitInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: mcp commit json","ai_context":["User intent: verify git.commit JSON.","Verification: server test."],"format":"json","expected_branch":%q}}}`, branch)
	if err := serveStdioWithSession(t, server, root, jsonCommitInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonCommitLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonCommitLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonCommitLines), out.String())
	}
	jsonCommitByID := responseLinesByID(t, jsonCommitLines)

	var commit struct {
		Hash          string `json:"hash"`
		Title         string `json:"title"`
		TicketChanges []struct {
			Stem        string `json:"stem"`
			ResultAdded bool   `json:"result_added"`
		} `json:"ticket_changes"`
	}
	if err := json.Unmarshal([]byte(toolText(t, jsonCommitByID["8"])), &commit); err != nil {
		t.Fatal(err)
	}
	if commit.Hash == "" || commit.Title != "test: mcp commit json" || len(commit.TicketChanges) != 0 {
		t.Fatalf("commit response = %#v", commit)
	}
}

func TestServeStdioGitCommitUpdatedTicketsPresence(t *testing.T) {
	useLeadProfile(t)
	for _, tc := range []struct {
		name  string
		field string
	}{
		{"omitted", ""},
		{"empty", `,"updated_tickets":[]`},
		{"explicit", `,"updated_tickets":["owner supplied summary"]`},
	} {
		for _, tickets := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/tickets=%t", tc.name, tickets), func(t *testing.T) {
				root := t.TempDir()
				initGit(t, root)
				mustWrite(t, root, "file.txt", "before\n")
				const moveStem = "260503-feat-move"
				const resultStem = "260503-feat-result"
				const editionStem = "260503-feat-edition"
				oldPath := "ai-docs/tickets/todo/" + moveStem + ".md"
				newPath := "ai-docs/tickets/idea/" + moveStem + ".md"
				resultPath := "ai-docs/tickets/todo/" + resultStem + ".md"
				editionPath := "ai-docs/tickets/todo/" + editionStem + ".md"
				const body = "---\ntitle: Demo\n---\n# Demo\n\n## Phases\n\n### Phase 1: Implement\n\nWork.\n"
				const result = "### Result (abc123) - 2026-05-04"
				const edition = "#### Edition (def456) - 2026-05-05"
				if tickets {
					mustWrite(t, root, oldPath, body)
					mustWrite(t, root, resultPath, body)
					mustWrite(t, root, editionPath, body+"\n"+result+"\n\nDone.\n")
				}
				runGit(t, root, "add", ".")
				runGit(t, root, "commit", "-m", "initial")
				paths := []string{"file.txt"}
				mustWrite(t, root, "file.txt", "after\n")
				if tickets {
					mustWrite(t, root, newPath, body)
					if err := os.Remove(filepath.Join(root, filepath.FromSlash(oldPath))); err != nil {
						t.Fatal(err)
					}
					mustWrite(t, root, resultPath, body+"\n"+result+"\n\nDone.\n")
					mustWrite(t, root, editionPath, body+"\n"+result+"\n\nDone.\n\n"+edition+"\n\nAdjusted.\n")
					paths = append(paths, oldPath, newPath, resultPath, editionPath)
				}
				pathsJSON, err := json.Marshal(paths)
				if err != nil {
					t.Fatal(err)
				}
				input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":%s,"title":"test: ticket summaries","ai_context":["Preserve caller intent."],"expected_branch":%q,"format":"json"%s}}}`, pathsJSON, headBranch(t, root), tc.field)
				var out bytes.Buffer
				if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
					t.Fatal(err)
				}
				byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
				if toolIsError(t, byID["1"]) {
					t.Fatalf("git.commit failed: %s", toolText(t, byID["1"]))
				}
				var got wsgit.CommitResult
				if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &got); err != nil {
					t.Fatal(err)
				}
				message := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
				if got.Hash != strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD"))) || got.Title != "test: ticket summaries" || !strings.Contains(message, "## AI Context\n- Preserve caller intent.") {
					t.Fatalf("commit record = %#v; message:\n%s", got, message)
				}
				var wantChanges []wsgit.TicketChange
				if tickets {
					wantChanges = []wsgit.TicketChange{
						{Stem: editionStem, Path: editionPath, ToStatus: "todo", ResultAdded: true, ResultHeading: edition},
						{Stem: moveStem, Path: newPath, OldPath: oldPath, FromStatus: "todo", ToStatus: "idea"},
						{Stem: resultStem, Path: resultPath, ToStatus: "todo", ResultAdded: true, ResultHeading: result},
					}
				}
				if len(got.TicketChanges) != len(wantChanges) {
					t.Fatalf("ticket_changes = %#v, want %#v", got.TicketChanges, wantChanges)
				}
				for i, want := range wantChanges {
					if got.TicketChanges[i] != want {
						t.Fatalf("ticket_changes[%d] = %#v, want %#v", i, got.TicketChanges[i], want)
					}
				}
				wantUpdates := ""
				if tc.name == "explicit" {
					wantUpdates = "## Ticket Updates\n- owner supplied summary"
				} else if tc.name == "omitted" && tickets {
					wantUpdates = "## Ticket Updates\n- " + editionStem + ": added " + edition + "\n- " + moveStem + ": moved todo -> idea\n- " + resultStem + ": added " + result
				}
				if index := strings.Index(message, "## Ticket Updates"); index >= 0 {
					if strings.TrimSpace(message[index:]) != wantUpdates {
						t.Fatalf("Ticket Updates = %q, want %q", message[index:], wantUpdates)
					}
				} else if wantUpdates != "" {
					t.Fatalf("missing Ticket Updates %q in:\n%s", wantUpdates, message)
				}
			})
		}
	}
}

// unresolvedPhaseTicketBody mirrors
// wsdoc.TestTicketVerifyUnresolvedPhaseIsSoftWarnOnly's fixture: a closed
// ticket with a resolved Phase 1 and an unresolved (non-`[dropped]`) Phase 2,
// which TicketVerify reports as a soft "unresolved-phases" warning, never a
// hard finding.
const unresolvedPhaseTicketBody = "---\ntitle: Closed with open phase\ncompleted: 2026-07-23\n---\n\n" +
	"## Phases\n\n" +
	"### Phase 1: First\n\n" +
	"### Result (abc123) - 2026-07-23\n\n" +
	"Done.\n\n" +
	"### Phase 2: Second\n\n" +
	"Never resolved.\n"

// TestServeStdioGitCommitRefusesBranchMismatch is the end-to-end evidence for
// the expected_branch guard: the required argument flows through the git.commit
// dispatch case into wsgit.Commit, a believed/actual mismatch is refused with a
// believed-vs-actual message, and no commit lands.
func TestServeStdioGitCommitRefusesBranchMismatch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	actualBranch := headBranch(t, root)
	mustWrite(t, root, "file.txt", "one\ntwo\n")

	server := NewServer(root, "test")
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: mismatch","ai_context":["User intent: prove the guard refuses a believed/actual mismatch."],"expected_branch":"definitely-not-the-checked-out-branch"}}}`
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) {
		t.Fatalf("git.commit did not refuse a believed/actual branch mismatch: %s", byID["1"])
	}
	text := toolText(t, byID["1"])
	for _, want := range []string{"definitely-not-the-checked-out-branch", actualBranch, "believed", "actual"} {
		if !strings.Contains(text, want) {
			t.Fatalf("git.commit refusal = %q, want it to mention %q", text, want)
		}
	}
	headAfter := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	if headAfter != head {
		t.Fatalf("HEAD moved to %s despite a refused commit (want unchanged %s)", headAfter, head)
	}
}

// TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories is the
// concrete instance of {#260720-wsdoc-commit-boundary}'s Phase 1 verification
// boundary: a real ticket that trips wsdoc.TicketVerify's soft
// unresolved-phases warning must still commit (Warnings never block OK), and
// the warning text must surface through verifyAdapter into
// wsgit.CommitResult.Advisories and formatGitCommit's text-mode "advisories:"
// block — but never through format:"json", since CommitResult.Advisories
// carries a `json:"-"` tag (see {#260626-git-commit-todo-reinjection}
// precedent).
func TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	branch := headBranch(t, root)

	server := NewServer(root, "test")

	textTicketPath := "ai-docs/tickets/.done/260726-feat-openphase-text.md"
	mustWrite(t, root, textTicketPath, unresolvedPhaseTicketBody)

	var out bytes.Buffer
	textInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close ticket with open phase","updated_tickets":[],"ai_context":["User intent: prove unresolved-phases warnings surface as git.commit advisories."],"expected_branch":%q}}}`, textTicketPath, branch)
	if err := serveStdioWithSession(t, server, root, textInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	textLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(textLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(textLines), out.String())
	}
	textByID := responseLinesByID(t, textLines)
	if toolIsError(t, textByID["1"]) {
		t.Fatalf("git.commit reported an error for a soft-warning-only ticket: %s", toolText(t, textByID["1"]))
	}
	text := toolText(t, textByID["1"])
	if !strings.Contains(text, "commit: ") {
		t.Fatalf("git.commit text response missing commit hash: %q", text)
	}
	if !strings.Contains(text, "advisories:\n") || !strings.Contains(text, "WARN [unresolved-phases]") {
		t.Fatalf("git.commit text response missing advisories block: %q", text)
	}

	if message := string(runGitOutput(t, root, "log", "-1", "--format=%B")); strings.Contains(message, "## Ticket Updates") || !strings.Contains(message, "## AI Context") {
		t.Fatalf("empty updates did not preserve message boundary:\n%s", message)
	}

	jsonTicketPath := "ai-docs/tickets/.done/260726-feat-openphase-json.md"
	mustWrite(t, root, jsonTicketPath, unresolvedPhaseTicketBody)

	out.Reset()
	jsonInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close ticket with open phase json","updated_tickets":[],"ai_context":["User intent: prove the json:\"-\" tag holds for advisories."],"format":"json","expected_branch":%q}}}`, jsonTicketPath, branch)
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonLines), out.String())
	}
	jsonByID := responseLinesByID(t, jsonLines)
	if toolIsError(t, jsonByID["2"]) {
		t.Fatalf("git.commit reported an error for a soft-warning-only ticket in JSON mode: %s", toolText(t, jsonByID["2"]))
	}
	jsonText := toolText(t, jsonByID["2"])
	if strings.Contains(jsonText, "advisories") || strings.Contains(jsonText, "WARN") {
		t.Fatalf("git.commit JSON response leaked advisories despite the json:\"-\" tag: %q", jsonText)
	}
	var jsonCommit struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(jsonText), &jsonCommit); err != nil {
		t.Fatal(err)
	}
	if jsonCommit.Hash == "" {
		t.Fatalf("JSON commit response missing hash: %q", jsonText)
	}
}

// ticketGraphAdvisoryFixture builds the smallest board that exercises all
// three advisory kinds at once: a .done child of a non-epic parent whose every
// child is now closed, carrying a dangling related: key. It yields a board
// block, one fix advisory, and one check advisory, which is what lets the
// commit-path and standalone renderers be compared over identical input.
func ticketGraphAdvisoryFixture(t *testing.T) (string, []string) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260726-refactor-graph-parent.md",
		"---\ntitle: Graph parent\n---\n\n# Graph parent\n")
	child := "ai-docs/tickets/.done/260726-feat-graph-child.md"
	mustWrite(t, root, child,
		"---\ntitle: Graph child\ncompleted: 2026-07-27\nparent: 260726-refactor-graph-parent\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# Graph child\n")
	return root, []string{child}
}

// TestVerifyAdapterAppendsAmendRecipeToFixAdvisoriesOnly pins the one
// presentation difference {#260723-tickets-verify-tool}'s identical-verdict
// guarantee permits: the commit path knows a commit already exists, so a
// mechanical remedy gets the amend recipe. A check or board advisory carries no
// mechanical remedy, so appending it there would be a false instruction.
func TestVerifyAdapterAppendsAmendRecipeToFixAdvisoriesOnly(t *testing.T) {
	root, paths := ticketGraphAdvisoryFixture(t)

	advisories, err := verifyAdapter(root, paths)
	if err != nil {
		t.Fatalf("verifyAdapter returned an error for a non-blocking fixture: %v", err)
	}
	if len(advisories) != 3 {
		t.Fatalf("advisories = %#v, want a board block plus one fix and one check", advisories)
	}

	const recipe = "Then git commit --amend --no-edit."
	var sawFix, sawCheck bool
	for _, advisory := range advisories {
		switch {
		case strings.Contains(advisory, "FIX:"):
			sawFix = true
			if !strings.HasSuffix(advisory, "\n       "+recipe) {
				t.Fatalf("fix advisory missing the amend recipe:\n%s", advisory)
			}
		case strings.Contains(advisory, "CHECK:"):
			sawCheck = true
			if strings.Contains(advisory, "--amend") {
				t.Fatalf("check advisory must not carry the amend recipe:\n%s", advisory)
			}
		default:
			if !strings.HasPrefix(advisory, "## Parent Board") {
				t.Fatalf("unexpected advisory:\n%s", advisory)
			}
			if strings.Contains(advisory, "--amend") {
				t.Fatalf("board advisory must not carry the amend recipe:\n%s", advisory)
			}
		}
	}
	if !sawFix || !sawCheck {
		t.Fatalf("fixture did not produce both a fix and a check advisory: %#v", advisories)
	}
}

// TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe is the standalone
// half of the same comparison: same verdict, same advisories, minus the recipe
// sentence — and advisories must never fire the "should be addressed or
// explicitly accepted" next_instruction, which is wrong for a no-action-needed
// ancestor note.
func TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe(t *testing.T) {
	root, paths := ticketGraphAdvisoryFixture(t)

	result, err := wsdoc.TicketVerify(root, paths)
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true; findings = %#v", result.Findings)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %#v, want none so the next_instruction assertion is non-vacuous", result.Warnings)
	}

	text := formatTicketVerify(result)
	for _, want := range []string{"verify: PASS", "## Parent Board", "Parent [1]: 260726-refactor-graph-parent", "FIX:", "CHECK:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatTicketVerify output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "--amend") {
		t.Fatalf("standalone verify output leaked the commit-path amend recipe:\n%s", text)
	}
	if strings.Contains(text, "should be addressed or explicitly accepted") {
		t.Fatalf("advisories wrongly fired the warning next_instruction:\n%s", text)
	}
}

// TestVerifyAdapterDegradesToSilenceOnGraphLoadFailure closes the commit-side
// half of the degrade-to-silence invariant. The wsdoc tests prove no advisories
// are produced; this proves the commit still lands, since wsgit.Commit vetoes
// on a non-nil verifier error and nothing else.
func TestVerifyAdapterDegradesToSilenceOnGraphLoadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not make a file unreadable on Windows")
	}
	root := t.TempDir()
	// The dangling related: would otherwise produce a FIX:, which makes the
	// silence non-vacuous; the unreadable sibling is what fails the board scan.
	path := "ai-docs/tickets/.done/260726-feat-graph-load-failure.md"
	mustWrite(t, root, path,
		"---\ntitle: Load failure\ncompleted: 2026-07-27\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# Load failure\n")
	broken := "ai-docs/tickets/todo/260726-feat-unreadable.md"
	mustWrite(t, root, broken, "---\ntitle: Unreadable\n---\n\n# Unreadable\n")
	brokenAbs := filepath.Join(root, filepath.FromSlash(broken))
	if err := os.Chmod(brokenAbs, 0o000); err != nil {
		t.Skipf("cannot make a file unreadable here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(brokenAbs, 0o644) })
	if _, err := os.ReadFile(brokenAbs); err == nil {
		t.Skip("mode 000 is still readable (running as root?)")
	}

	advisories, err := verifyAdapter(root, []string{path})
	if err != nil {
		t.Fatalf("a graph-load failure vetoed the commit: %v", err)
	}
	if len(advisories) != 0 {
		t.Fatalf("advisories = %#v, want none after a graph-load failure", advisories)
	}
}

// TestServeStdioGitCommitSurfacesTicketGraphParentBoard drives the real MCP
// dispatch: a commit closing the last child of an epic must land, keep OK
// unchanged, and carry the ## Parent Board block in the text response — while
// format:"json" stays advisory-free, since CommitResult.Advisories is tagged
// json:"-".
func TestServeStdioGitCommitSurfacesTicketGraphParentBoard(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	branch := headBranch(t, root)

	mustWrite(t, root, "ai-docs/tickets/todo/260726-epic-graph-e2e.md",
		"---\ntitle: E2E epic\n---\n\n# E2E epic\n")

	server := NewServer(root, "test")

	textChild := "ai-docs/tickets/.done/260726-feat-graph-e2e-text.md"
	// The dangling related: makes this commit carry both a multi-line board
	// block and a single-block FIX:, which is the adjacency that needs a blank
	// separator in formatGitCommit.
	mustWrite(t, root, textChild,
		"---\ntitle: E2E child\ncompleted: 2026-07-27\nparent: 260726-epic-graph-e2e\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# E2E child\n")

	var out bytes.Buffer
	textInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close the last child of an epic","ai_context":["User intent: prove the parent board reaches the git.commit response."],"expected_branch":%q}}}`, textChild, branch)
	if err := serveStdioWithSession(t, server, root, textInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	textLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(textLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(textLines), out.String())
	}
	textByID := responseLinesByID(t, textLines)
	if toolIsError(t, textByID["1"]) {
		t.Fatalf("git.commit vetoed a commit whose only output is advisory: %s", toolText(t, textByID["1"]))
	}
	text := toolText(t, textByID["1"])
	for _, want := range []string{"commit: ", "advisories:\n", "## Parent Board", "Parent [1]: 260726-epic-graph-e2e", "(just now)", "ACTION:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("git.commit text response missing %q: %q", want, text)
		}
	}
	// The board block and the FIX: advisory must not run together.
	if !strings.Contains(text, "\n\n  FIX:") {
		t.Fatalf("git.commit text response missing the blank separator before the FIX: advisory: %q", text)
	}
	if !strings.Contains(text, "Then git commit --amend --no-edit.") {
		t.Fatalf("git.commit text response missing the amend recipe: %q", text)
	}

	jsonChild := "ai-docs/tickets/.done/260726-feat-graph-e2e-json.md"
	mustWrite(t, root, jsonChild,
		"---\ntitle: E2E child json\ncompleted: 2026-07-27\nparent: 260726-epic-graph-e2e\n---\n\n# E2E child json\n")

	out.Reset()
	jsonInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close another child of the epic","ai_context":["User intent: prove the json:\"-\" tag holds for graph advisories."],"format":"json","expected_branch":%q}}}`, jsonChild, branch)
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonLines), out.String())
	}
	jsonByID := responseLinesByID(t, jsonLines)
	if toolIsError(t, jsonByID["2"]) {
		t.Fatalf("git.commit reported an error in JSON mode: %s", toolText(t, jsonByID["2"]))
	}
	jsonText := toolText(t, jsonByID["2"])
	if strings.Contains(jsonText, "Parent Board") || strings.Contains(jsonText, "advisories") {
		t.Fatalf("git.commit JSON response leaked advisories despite the json:\"-\" tag: %q", jsonText)
	}
}

// aiContextDebugEvents fetches runtime.debug_events (a process-wide ring
// buffer shared by every test in this package's binary — see appendDebugEvent
// in server.go) via a standalone ServeStdio call and returns every
// "git.commit.ai_context_received" event currently in the buffer, in
// insertion order. It is called both before and after the calls under test so
// the test can diff the two snapshots instead of asserting an absolute count,
// which would be polluted both by other tests' git.commit calls and by
// repeated invocations of this same test under `go test -count=N` (all
// sharing the one process-wide buffer).
func aiContextDebugEvents(t *testing.T, server *Server, root string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":"debug","method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":256}}}`
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio debug_events returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 debug_events response, got %d\n%s", len(lines), out.String())
	}
	text := toolText(t, responseLinesByID(t, lines)["debug"])
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse debug event line %q: %v", line, err)
		}
		if event["event"] == "git.commit.ai_context_received" {
			events = append(events, event)
		}
	}
	return events
}

// newDebugEvents returns the events in after that are not in before. The ring
// buffer is full once the package has run enough tests, so the calls between
// the two snapshots can evict older events: diff by full content (ts
// included) instead of by count or position.
func newDebugEvents(t *testing.T, before, after []map[string]any) []map[string]any {
	t.Helper()
	key := func(event map[string]any) string {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("encode debug event: %v", err)
		}
		return string(raw)
	}
	seen := map[string]int{}
	for _, event := range before {
		seen[key(event)]++
	}
	var fresh []map[string]any
	for _, event := range after {
		if k := key(event); seen[k] > 0 {
			seen[k]--
			continue
		}
		fresh = append(fresh, event)
	}
	return fresh
}

func TestServeStdioGitCommitAIContextConditionsAndDebugEvent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	branch := headBranch(t, root)
	mustWrite(t, root, "file.txt", "one\ntwo\n")

	server := NewServer(root, "test")

	// Snapshot the debug-event ring buffer before issuing any of this test's
	// git.commit calls (see aiContextDebugEvents doc comment for why this is
	// a diff, not an absolute-count, assertion).
	before := aiContextDebugEvents(t, server, root)

	largeAIContext := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		largeAIContext = append(largeAIContext, fmt.Sprintf("Entry %d: %s", i, strings.Repeat("context detail", 20)))
	}
	largeAIContextJSON, err := json.Marshal(largeAIContext)
	if err != nil {
		t.Fatal(err)
	}

	commitInput := strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: empty ai_context array","ai_context":[],"expected_branch":%q}}}`, branch),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: absent ai_context field","expected_branch":%q}}}`, branch),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: single empty-string entry","ai_context":[""],"expected_branch":%q}}}`, branch),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: single whitespace entry","ai_context":["  "],"expected_branch":%q}}}`, branch),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: large valid ai_context array","ai_context":%s,"expected_branch":%q}}}`, largeAIContextJSON, branch),
	}, "\n")

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, commitInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	emptyArrayText := toolText(t, byID["1"])
	if !strings.Contains(byID["1"], `"isError":true`) || !strings.Contains(emptyArrayText, "received an empty array") {
		t.Fatalf("git.commit with empty ai_context array = %s", byID["1"])
	}
	absentFieldText := toolText(t, byID["2"])
	// ai_context is a required property, so the central argument precheck
	// refuses its absence before the handler runs; its generic
	// tools.call.precheck_refused event (TestPrecheckRefusalEmitsDebugEvent)
	// replaces the handler's absent-field event over MCP.
	if !strings.Contains(byID["2"], `"isError":true`) || !strings.Contains(absentFieldText, "missing required: ai_context") {
		t.Fatalf("git.commit with absent ai_context field = %s", byID["2"])
	}
	emptyStringEntryText := toolText(t, byID["3"])
	if !strings.Contains(byID["3"], `"isError":true`) || !strings.Contains(emptyStringEntryText, "received 1 entry, all blank") {
		t.Fatalf("git.commit with [\"\"] ai_context = %s", byID["3"])
	}
	whitespaceEntryText := toolText(t, byID["4"])
	if !strings.Contains(byID["4"], `"isError":true`) || !strings.Contains(whitespaceEntryText, "received 1 entry, all blank") {
		t.Fatalf("git.commit with [\"  \"] ai_context = %s", byID["4"])
	}
	// [""] and ["  "] must classify identically ("all blank"), not one as
	// "empty array" and the other as "all blank" — this is the condition the
	// review finding flagged: stringList used to silently drop exact-empty
	// entries before wsgit ever saw them.
	if emptyStringEntryText != whitespaceEntryText {
		t.Fatalf("[\"\"] and [\"  \"] ai_context reported different conditions:\n%q\nvs\n%q", emptyStringEntryText, whitespaceEntryText)
	}
	largeArrayResultText := toolText(t, byID["5"])
	if strings.Contains(byID["5"], `"isError":true`) {
		t.Fatalf("git.commit with a large valid ai_context array was rejected: %s", byID["5"])
	}
	if !strings.Contains(largeArrayResultText, "commit: ") {
		t.Fatalf("git.commit with a large valid ai_context array = %s", largeArrayResultText)
	}

	after := aiContextDebugEvents(t, server, root)
	newEvents := newDebugEvents(t, before, after)
	if len(newEvents) != 4 {
		t.Fatalf("expected 4 new git.commit.ai_context_received events, got %d (%d before, %d after)", len(newEvents), len(before), len(after))
	}

	// The five git.commit calls above run as concurrent goroutines (see
	// ServeStdio's per-line `go func` in server.go), so their completion
	// order — and thus their append order into the ring buffer — is not
	// guaranteed. Classify the five new events by their own field values
	// instead of assuming array position.
	var emptyArrayEvent, absentFieldEvent, largeArrayEvent map[string]any
	var blankEntryEvents []map[string]any
	for _, event := range newEvents {
		switch {
		case event["present"] == false:
			absentFieldEvent = event
		case event["raw_entry_count"] == float64(0):
			emptyArrayEvent = event
		case event["raw_entry_count"] == float64(1) && event["post_trim_entry_count"] == float64(0):
			blankEntryEvents = append(blankEntryEvents, event)
		case event["raw_entry_count"] == float64(60):
			largeArrayEvent = event
		}
	}
	if emptyArrayEvent == nil || emptyArrayEvent["present"] != true || emptyArrayEvent["raw_bytes"] != float64(0) || emptyArrayEvent["post_trim_entry_count"] != float64(0) {
		t.Fatalf("empty-array debug event = %#v", emptyArrayEvent)
	}
	if absentFieldEvent != nil {
		t.Fatalf("precheck-refused absent-field call still reached the handler: %#v", absentFieldEvent)
	}
	// One blank-entry event from [""] (raw_bytes 0) and one from ["  "]
	// (raw_bytes 2); both must report post_trim_entry_count 0 — this is the
	// field-accuracy fix: a naive non-empty-string count would have reported
	// 1 here instead of 0.
	if len(blankEntryEvents) != 2 {
		t.Fatalf("expected 2 blank-entry debug events ([\"\"] and [\"  \"]), got %d: %#v", len(blankEntryEvents), blankEntryEvents)
	}
	sawEmptyStringBytes, sawWhitespaceBytes := false, false
	for _, event := range blankEntryEvents {
		if event["present"] != true {
			t.Fatalf("blank-entry debug event missing present=true: %#v", event)
		}
		switch event["raw_bytes"] {
		case float64(0):
			sawEmptyStringBytes = true
		case float64(2):
			sawWhitespaceBytes = true
		}
	}
	if !sawEmptyStringBytes || !sawWhitespaceBytes {
		t.Fatalf("blank-entry debug events missing expected raw_bytes (0 for [\"\"], 2 for [\"  \"]): %#v", blankEntryEvents)
	}
	if largeArrayEvent == nil || largeArrayEvent["present"] != true || largeArrayEvent["post_trim_entry_count"] != float64(60) {
		rawBytes, _ := largeArrayEvent["raw_bytes"].(float64)
		t.Fatalf("large-array debug event = %#v (raw_bytes=%v)", largeArrayEvent, rawBytes)
	}
	if rawBytes, _ := largeArrayEvent["raw_bytes"].(float64); rawBytes < 5000 {
		t.Fatalf("large-array debug event raw_bytes too small: %v", rawBytes)
	}
}

// TestServeStdioGitCommitRejectsNonArrayAIContext covers the ai_context
// type-coercion trap distinct from the absent/empty/all-blank diagnostics
// TestServeStdioGitCommitAIContextConditionsAndDebugEvent covers: a caller
// passing a dash-bulleted prose string (the shape a commit message's
// `## AI Context` block takes everywhere else in the workflow) must get a
// type-accurate rejection, not stringListKeepBlank's silent nil-coercion
// collapsing it into "no ai_context field was received" — indistinguishable
// from an omitted field.
func TestServeStdioGitCommitRejectsNonArrayAIContext(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	mustWrite(t, root, "file.txt", "one\ntwo\n")

	server := NewServer(root, "test")
	before := aiContextDebugEvents(t, server, root)
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: prose ai_context","ai_context":"- User intent: prose string instead of array.","expected_branch":%q}}}`, headBranch(t, root))

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	resp := strings.TrimSpace(out.String())
	text := toolText(t, resp)
	if !strings.Contains(resp, `"isError":true`) || !strings.Contains(text, "ai_context must be an array of strings") || !strings.Contains(text, "got string") {
		t.Fatalf("git.commit with prose-string ai_context = %s", resp)
	}
	if strings.Contains(text, "no ai_context field was received") {
		t.Fatalf("must not fall through to the absent-field message: %s", text)
	}

	// The debug-event ring buffer exists specifically to diagnose ai_context
	// arrival shape, so a wrong-type call must still record one (marked
	// type_mismatch) rather than short-circuiting before it is ever logged.
	after := aiContextDebugEvents(t, server, root)
	newEvents := newDebugEvents(t, before, after)
	if len(newEvents) != 1 {
		t.Fatalf("expected 1 new git.commit.ai_context_received event, got %d (%d before, %d after)", len(newEvents), len(before), len(after))
	}
	newEvent := newEvents[0]
	if newEvent["type_mismatch"] != true || newEvent["present"] != true {
		t.Fatalf("type-mismatch debug event = %#v", newEvent)
	}
}
