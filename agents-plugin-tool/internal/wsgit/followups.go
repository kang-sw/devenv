package wsgit

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wskey"
)

// Commit-body section headings for commit-anchored follow-ups. A follow-up is
// forward-looking, ownerless work with an open -> resolved lifecycle; it rides
// the commit that recorded it and is resolved by id from any later commit.
// These sections are deliberately not rationale (wsrationale's commitSections
// never lists them); wsrationale's follow-up query parses them instead.
const (
	FollowupsHeading = "## Follow-ups"
	ResolvesHeading  = "## Resolves"
)

// FollowupLevels is the closed follow-up level set, lowest first. It reuses the
// reviewer severity vocabulary so a review finding maps without translation.
var FollowupLevels = []string{"minor", "important", "critical"}

// FollowupCategories is the closed follow-up category enum. Adding a value is a
// deliberate code change: admit one only when its items have no existing home
// (ticket, forward note, ws note, code comment) and an item left unattended for
// one release cycle may acceptably drop out of view.
var FollowupCategories = []string{"review"}

// Followup is one commit-anchored follow-up. ID is minted by Commit; callers
// supply Level, Category, and Content.
type Followup struct {
	ID       string `json:"id"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

// FollowupLevelRank returns the position of level in FollowupLevels, or -1 for
// a level outside the closed set.
func FollowupLevelRank(level string) int {
	for i, known := range FollowupLevels {
		if level == known {
			return i
		}
	}
	return -1
}

// ValidFollowupCategory reports whether category is in FollowupCategories.
func ValidFollowupCategory(category string) bool {
	for _, known := range FollowupCategories {
		if category == known {
			return true
		}
	}
	return false
}

// FollowupLine renders the bullet text (without the leading "- ") written
// under FollowupsHeading: "<level>/<category> <id>: <content>".
func FollowupLine(f Followup) string {
	return fmt.Sprintf("%s/%s %s: %s", f.Level, f.Category, f.ID, f.Content)
}

// normalizeFollowups trims and validates caller-supplied follow-ups. Content
// is a single line; level and category must be in their closed sets.
func normalizeFollowups(in []Followup) ([]Followup, error) {
	out := make([]Followup, 0, len(in))
	for i, f := range in {
		f.Level = strings.TrimSpace(f.Level)
		f.Category = strings.TrimSpace(f.Category)
		f.Content = strings.TrimSpace(f.Content)
		if FollowupLevelRank(f.Level) < 0 {
			return nil, fmt.Errorf("followups[%d].level %q is not one of %s", i, f.Level, strings.Join(FollowupLevels, ", "))
		}
		if !ValidFollowupCategory(f.Category) {
			return nil, fmt.Errorf("followups[%d].category %q is not one of %s", i, f.Category, strings.Join(FollowupCategories, ", "))
		}
		if f.Content == "" {
			return nil, fmt.Errorf("followups[%d].content is required", i)
		}
		if strings.ContainsAny(f.Content, "\r\n") {
			return nil, fmt.Errorf("followups[%d].content must be a single line", i)
		}
		f.ID = ""
		out = append(out, f)
	}
	return out, nil
}

// normalizeResolves trims resolve ids, drops blanks, and rejects an id that is
// not a single token.
func normalizeResolves(in []string) ([]string, error) {
	out := trimStrings(in)
	for _, id := range out {
		if strings.ContainsAny(id, " \t\r\n:") {
			return nil, fmt.Errorf("resolves entry %q must be a single follow-up id", id)
		}
	}
	return out, nil
}

// mintFollowupIDs assigns each follow-up a fresh word-codename id, distinct
// within the commit. Ids are independent of the commit SHA so squash, rebase,
// and cherry-pick keep follow-ups and their resolutions matched.
func mintFollowupIDs(followups []Followup, generate func() (string, error)) error {
	if generate == nil {
		generate = wskey.Generate
	}
	seen := map[string]bool{}
	for i := range followups {
		for attempt := 0; ; attempt++ {
			id, err := generate()
			if err != nil {
				return err
			}
			if !seen[id] {
				seen[id] = true
				followups[i].ID = id
				break
			}
			if attempt >= 16 {
				return fmt.Errorf("could not mint a distinct follow-up id")
			}
		}
	}
	return nil
}
