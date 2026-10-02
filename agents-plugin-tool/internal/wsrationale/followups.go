package wsrationale

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// FollowupOptions selects open follow-ups. An empty Range walks the full
// history from HEAD; an empty Category or MinLevel applies no filter.
type FollowupOptions struct {
	Range    string
	Category string
	MinLevel string
}

// OpenFollowup is one unresolved follow-up and the commit that carries it.
type OpenFollowup struct {
	wsgit.Followup
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// FollowupResult is the git.followups result: open follow-ups, newest
// carrying commit first.
type FollowupResult struct {
	Range          string         `json:"range,omitempty"`
	Category       string         `json:"category,omitempty"`
	MinLevel       string         `json:"min_level,omitempty"`
	ScannedCommits int            `json:"scanned_commits"`
	Followups      []OpenFollowup `json:"followups"`
}

// followupLineRe parses a "## Follow-ups" bullet as wsgit.FollowupLine writes
// it: "<level>/<category> <id>: <content>".
var followupLineRe = regexp.MustCompile(`^([a-z]+)/([a-z]+) ([^\s:]+): (.+)$`)

// parseFollowupLine parses one follow-up record. A level outside the closed set
// is not a follow-up this runtime can order, so it is skipped; the category is
// kept as written so an older reader still lists a newer category unfiltered.
func parseFollowupLine(text string) (wsgit.Followup, bool) {
	m := followupLineRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil || wsgit.FollowupLevelRank(m[1]) < 0 {
		return wsgit.Followup{}, false
	}
	return wsgit.Followup{Level: m[1], Category: m[2], ID: m[3], Content: strings.TrimSpace(m[4])}, true
}

// commitFollowups extracts a commit body's follow-ups and resolved ids through
// the shared section parser.
func commitFollowups(body string) (followups []wsgit.Followup, resolves []string) {
	for _, block := range collectSections(body, exactOrPrefixHeading(wsgit.FollowupsHeading), "## ") {
		for _, text := range splitRecords(block.lines) {
			if f, ok := parseFollowupLine(text); ok {
				followups = append(followups, f)
			}
		}
	}
	for _, block := range collectSections(body, exactOrPrefixHeading(wsgit.ResolvesHeading), "## ") {
		for _, text := range splitRecords(block.lines) {
			if fields := strings.Fields(text); len(fields) > 0 {
				resolves = append(resolves, fields[0])
			}
		}
	}
	return followups, resolves
}

// scanBodies walks commit bodies (no --name-only) for rev, newest first. A
// repository with no commits yields nothing when rev is the implicit HEAD.
func scanBodies(ctx context.Context, runner wsgit.Runner, root, rev string) ([]rawCommit, error) {
	if rev == "" {
		if _, err := runner.RunGit(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
			return nil, nil
		}
		rev = "HEAD"
	}
	out, err := runner.RunGit(ctx, root, "log", "--date=short", logFormat(), rev)
	if err != nil {
		return nil, err
	}
	return parseCommitStream(out), nil
}

// OpenFollowups returns the follow-ups carried by commits in opts.Range (full
// history from HEAD when empty) that no commit in the same walk resolves.
// Openness is therefore evaluated as of the range's end: a resolve after it, or
// on a branch it cannot reach, does not count. Ids match by id alone, so a
// rewritten (different-SHA) commit keeps its follow-ups and resolutions
// matched. A follow-up carried by several commits (a cherry-pick) is reported
// once, at its newest carrying commit.
func OpenFollowups(ctx context.Context, runner wsgit.Runner, root string, opts FollowupOptions) (FollowupResult, error) {
	opts.Range = strings.TrimSpace(opts.Range)
	opts.Category = strings.TrimSpace(opts.Category)
	opts.MinLevel = strings.TrimSpace(opts.MinLevel)
	if strings.HasPrefix(opts.Range, "-") {
		return FollowupResult{}, fmt.Errorf("range must be a revision or range, not a git option")
	}
	if opts.Category != "" && !wsgit.ValidFollowupCategory(opts.Category) {
		return FollowupResult{}, fmt.Errorf("category %q is not one of %s", opts.Category, strings.Join(wsgit.FollowupCategories, ", "))
	}
	minRank := 0
	if opts.MinLevel != "" {
		minRank = wsgit.FollowupLevelRank(opts.MinLevel)
		if minRank < 0 {
			return FollowupResult{}, fmt.Errorf("min_level %q is not one of %s", opts.MinLevel, strings.Join(wsgit.FollowupLevels, ", "))
		}
	}
	commits, err := scanBodies(ctx, runner, root, opts.Range)
	if err != nil {
		return FollowupResult{}, err
	}
	resolved := map[string]bool{}
	type carried struct {
		f wsgit.Followup
		c rawCommit
	}
	var found []carried
	for _, c := range commits {
		followups, resolves := commitFollowups(c.body)
		for _, id := range resolves {
			resolved[id] = true
		}
		for _, f := range followups {
			found = append(found, carried{f: f, c: c})
		}
	}
	result := FollowupResult{Range: opts.Range, Category: opts.Category, MinLevel: opts.MinLevel, ScannedCommits: len(commits), Followups: []OpenFollowup{}}
	seen := map[string]bool{}
	for _, item := range found {
		f := item.f
		if resolved[f.ID] || seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		if opts.Category != "" && f.Category != opts.Category {
			continue
		}
		if wsgit.FollowupLevelRank(f.Level) < minRank {
			continue
		}
		result.Followups = append(result.Followups, OpenFollowup{Followup: f, Commit: item.c.hash, Date: item.c.date})
	}
	return result, nil
}

// ResolveWarnings checks resolve ids against the full history from HEAD and
// returns one warning per id that no commit records as a follow-up, or that a
// commit already resolves. Warnings never block a commit.
func ResolveWarnings(ctx context.Context, runner wsgit.Runner, root string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	commits, err := scanBodies(ctx, runner, root, "")
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	resolved := map[string]bool{}
	for _, c := range commits {
		followups, resolves := commitFollowups(c.body)
		for _, f := range followups {
			known[f.ID] = true
		}
		for _, id := range resolves {
			resolved[id] = true
		}
	}
	var warnings []string
	for _, id := range ids {
		switch {
		case !known[id]:
			warnings = append(warnings, fmt.Sprintf("resolves %s: no follow-up with this id in history from HEAD; the resolve was recorded anyway", id))
		case resolved[id]:
			warnings = append(warnings, fmt.Sprintf("resolves %s: already resolved in history from HEAD; the resolve was recorded anyway", id))
		}
	}
	return warnings, nil
}

// FormatFollowups renders the compact text form of a FollowupResult.
func FormatFollowups(r FollowupResult) string {
	var b strings.Builder
	scope := "full history from HEAD"
	if r.Range != "" {
		scope = "range " + r.Range
	}
	fmt.Fprintf(&b, "open follow-ups: %d (%s, %d commits scanned)\n", len(r.Followups), scope, r.ScannedCommits)
	for _, f := range r.Followups {
		short := f.Commit
		if len(short) > 9 {
			short = short[:9]
		}
		fmt.Fprintf(&b, "- %s %s/%s (%s %s): %s\n", f.ID, f.Level, f.Category, short, f.Date, f.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}
