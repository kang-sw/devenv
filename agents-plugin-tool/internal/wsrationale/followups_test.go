package wsrationale

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// followupMessage builds a commit message through the production section
// writer, so every fixture body has exactly the shape git.commit writes.
func followupMessage(title string, followups []wsgit.Followup, resolves []string) string {
	return wsgit.CommitMessage(wsgit.CommitOptions{Title: title, AIContext: []string{"context for " + title}, Followups: followups, Resolves: resolves})
}

func openFollowups(t *testing.T, root string, opts FollowupOptions) FollowupResult {
	t.Helper()
	res, err := OpenFollowups(context.Background(), wsgit.ExecRunner{}, root, opts)
	if err != nil {
		t.Fatalf("OpenFollowups error: %v", err)
	}
	return res
}

func openIDs(res FollowupResult) []string {
	ids := []string{}
	for _, f := range res.Followups {
		ids = append(ids, f.ID)
	}
	return ids
}

func TestFollowupSectionsRoundTripThroughWriterAndParser(t *testing.T) {
	in := []wsgit.Followup{
		{ID: "amber-latch-dwelled", Level: "minor", Category: "review", Content: "agents-plugin-pi/src/bridge.ts:485 resolvedSessionKeyArg re-implements key resolution"},
		{ID: "cobalt-rift-oaken", Level: "critical", Category: "review", Content: "a.go:1 uses: colons, and punctuation"},
	}
	message := followupMessage("chore(review): stamp", in, []string{"older-fixed-item", "second-fixed-item"})
	followups, resolves := commitFollowups(message)
	if !reflect.DeepEqual(followups, in) {
		t.Fatalf("followups round trip = %#v, want %#v", followups, in)
	}
	if !reflect.DeepEqual(resolves, []string{"older-fixed-item", "second-fixed-item"}) {
		t.Fatalf("resolves round trip = %#v", resolves)
	}
	// Follow-ups are not rationale: rationale.query must never return them.
	for _, rec := range commitRecords(rawCommit{hash: "h", body: message}) {
		if strings.Contains(rec.Text, "amber-latch-dwelled") || strings.Contains(rec.Text, "older-fixed-item") {
			t.Fatalf("follow-up leaked into rationale record: %q", rec.Text)
		}
	}
}

func TestOpenFollowupsDropsResolvedAndCarriesCommit(t *testing.T) {
	root := initRepo(t)
	fHash := commitFixture(t, root, "2026-10-01", map[string]string{"a.txt": "1"}, followupMessage("chore(review): stamp", []wsgit.Followup{
		{ID: "amber-latch-dwelled", Level: "minor", Category: "review", Content: "a.go:1 minor thing"},
		{ID: "cobalt-rift-oaken", Level: "important", Category: "review", Content: "b.go:2 important thing"},
	}, nil))
	commitFixture(t, root, "2026-10-02", map[string]string{"a.txt": "2"}, followupMessage("fix: minor thing", nil, []string{"amber-latch-dwelled"}))

	res := openFollowups(t, root, FollowupOptions{})
	if len(res.Followups) != 1 {
		t.Fatalf("open = %#v, want only cobalt-rift-oaken", res.Followups)
	}
	got := res.Followups[0]
	if got.ID != "cobalt-rift-oaken" || got.Level != "important" || got.Category != "review" || got.Content != "b.go:2 important thing" || got.Commit != fHash || got.Date != "2026-10-01" {
		t.Fatalf("open row = %#v", got)
	}
	if res.ScannedCommits != 2 {
		t.Fatalf("scanned = %d", res.ScannedCommits)
	}
	text := FormatFollowups(res)
	if !strings.Contains(text, "open follow-ups: 1") || !strings.Contains(text, "cobalt-rift-oaken important/review") || !strings.Contains(text, fHash[:9]) {
		t.Fatalf("text = %s", text)
	}
}

func TestOpenFollowupsFiltersCategoryAndMinLevel(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-10-01", map[string]string{"a.txt": "1"}, followupMessage("chore(review): stamp", []wsgit.Followup{
		{ID: "one-minor-item", Level: "minor", Category: "review", Content: "a"},
		{ID: "two-important-item", Level: "important", Category: "review", Content: "b"},
		{ID: "three-critical-item", Level: "critical", Category: "review", Content: "c"},
	}, nil))
	if got := openIDs(openFollowups(t, root, FollowupOptions{})); len(got) != 3 {
		t.Fatalf("unfiltered = %v", got)
	}
	if got := openIDs(openFollowups(t, root, FollowupOptions{MinLevel: "important"})); !reflect.DeepEqual(got, []string{"two-important-item", "three-critical-item"}) {
		t.Fatalf("min_level important = %v", got)
	}
	if got := openIDs(openFollowups(t, root, FollowupOptions{MinLevel: "critical", Category: "review"})); !reflect.DeepEqual(got, []string{"three-critical-item"}) {
		t.Fatalf("min_level critical = %v", got)
	}
	if _, err := OpenFollowups(context.Background(), wsgit.ExecRunner{}, root, FollowupOptions{Category: "perf"}); err == nil || !strings.Contains(err.Error(), "category") {
		t.Fatalf("unknown category error = %v", err)
	}
	if _, err := OpenFollowups(context.Background(), wsgit.ExecRunner{}, root, FollowupOptions{MinLevel: "warn"}); err == nil || !strings.Contains(err.Error(), "min_level") {
		t.Fatalf("unknown min_level error = %v", err)
	}
	if _, err := OpenFollowups(context.Background(), wsgit.ExecRunner{}, root, FollowupOptions{Range: "--all"}); err == nil {
		t.Fatal("option-shaped range accepted")
	}
}

func TestOpenFollowupsRangeBoundsVisibilityAndOpenness(t *testing.T) {
	root := initRepo(t)
	c1 := commitFixture(t, root, "2026-09-01", map[string]string{"a.txt": "1"}, followupMessage("stamp one", []wsgit.Followup{{ID: "old-before-range", Level: "important", Category: "review", Content: "a"}}, nil))
	c2 := commitFixture(t, root, "2026-09-02", map[string]string{"a.txt": "2"}, followupMessage("stamp two", []wsgit.Followup{{ID: "inside-the-range", Level: "important", Category: "review", Content: "b"}}, nil))
	c3 := commitFixture(t, root, "2026-09-03", map[string]string{"a.txt": "3"}, followupMessage("fix", nil, []string{"inside-the-range"}))

	if got := openIDs(openFollowups(t, root, FollowupOptions{})); !reflect.DeepEqual(got, []string{"old-before-range"}) {
		t.Fatalf("full history = %v", got)
	}
	// c1 sits before the range, so its follow-up is out of view.
	if got := openIDs(openFollowups(t, root, FollowupOptions{Range: c1 + ".." + c3})); len(got) != 0 {
		t.Fatalf("range c1..c3 = %v, want none", got)
	}
	// Openness is as of the range's end: c3's resolve lies after c2.
	res := openFollowups(t, root, FollowupOptions{Range: c1 + ".." + c2})
	if got := openIDs(res); !reflect.DeepEqual(got, []string{"inside-the-range"}) || res.ScannedCommits != 1 {
		t.Fatalf("range c1..c2 = %v (scanned %d), want inside-the-range open", got, res.ScannedCommits)
	}
}

func TestOpenFollowupsMatchIDsAcrossRewrittenCommits(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-09-01", map[string]string{"base.txt": "base"}, "base")
	runGit(t, root, "branch", "-M", "main")
	runGit(t, root, "checkout", "-b", "feature")
	origF := commitFixture(t, root, "2026-09-02", map[string]string{"f.txt": "f"}, followupMessage("stamp", []wsgit.Followup{{ID: "rewritten-follow-up", Level: "important", Category: "review", Content: "f.go:1 thing"}}, nil))
	origR := commitFixture(t, root, "2026-09-03", map[string]string{"r.txt": "r"}, followupMessage("fix", nil, []string{"rewritten-follow-up"}))
	runGit(t, root, "checkout", "main")
	commitFixture(t, root, "2026-09-04", map[string]string{"m.txt": "m"}, "diverge main")

	// Rewrite only the follow-up onto main: it is open at its new SHA.
	cherryPick(t, root, origF)
	res := openFollowups(t, root, FollowupOptions{})
	if len(res.Followups) != 1 || res.Followups[0].ID != "rewritten-follow-up" || res.Followups[0].Commit == origF {
		t.Fatalf("rewritten follow-up = %#v, want open at a new SHA", res.Followups)
	}
	// Rewrite its resolve too: the id still matches, so nothing is open.
	cherryPick(t, root, origR)
	if got := openIDs(openFollowups(t, root, FollowupOptions{})); len(got) != 0 {
		t.Fatalf("after rewritten resolve, open = %v", got)
	}
	head := strings.TrimSpace(gitOut(t, root, "rev-parse", "HEAD"))
	if head == origR {
		t.Fatal("cherry-pick did not rewrite the resolve commit")
	}
}

func TestResolveWarningsFlagsUnknownAndAlreadyResolved(t *testing.T) {
	root := initRepo(t)
	if warnings, err := ResolveWarnings(context.Background(), wsgit.ExecRunner{}, root, []string{"any-id-here"}); err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "no follow-up with this id") {
		t.Fatalf("empty repository warnings = %v, %v", warnings, err)
	}
	commitFixture(t, root, "2026-10-01", map[string]string{"a.txt": "1"}, followupMessage("stamp", []wsgit.Followup{
		{ID: "still-open-item", Level: "minor", Category: "review", Content: "a"},
		{ID: "done-already-item", Level: "minor", Category: "review", Content: "b"},
	}, nil))
	commitFixture(t, root, "2026-10-02", map[string]string{"a.txt": "2"}, followupMessage("fix", nil, []string{"done-already-item"}))

	warnings, err := ResolveWarnings(context.Background(), wsgit.ExecRunner{}, root, []string{"still-open-item", "done-already-item", "never-seen-item"})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "done-already-item") || !strings.Contains(warnings[0], "already resolved") ||
		!strings.Contains(warnings[1], "never-seen-item") || !strings.Contains(warnings[1], "no follow-up with this id") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func cherryPick(t *testing.T, root, hash string) {
	t.Helper()
	cmd := exec.Command("git", "cherry-pick", hash)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2026-09-05T12:00:00")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cherry-pick %s: %v\n%s", hash, err, out)
	}
}

func gitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
