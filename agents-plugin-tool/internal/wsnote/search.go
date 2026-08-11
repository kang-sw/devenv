package wsnote

import (
	"path"
	"regexp"
	"sort"
)

// dateOnlyBound matches a bare "YYYY-MM-DD" date, distinguishing it from a
// full RFC3339 timestamp for the from/then bound-widening logic below.
var dateOnlyBound = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Search returns records whose Key matches glob (stdlib path.Match — no new
// dependency, only key-glob matching is required) and whose WrittenAt falls
// within the inclusive [from, then] range when those bounds are non-empty.
// An empty or "*" glob matches every key, including keys containing "/"
// (path.Match's "*" does not cross "/", so it is bypassed entirely for the
// match-all case rather than relied on to behave like one).
//
// Bounds accept either a full RFC3339 timestamp or a bare date prefix (e.g.
// "2026-08-01"), compared as strings. The two bounds are NOT symmetric under
// a bare date: a date-only "from" already sorts below every RFC3339 timestamp
// on that day (e.g. "2026-08-01" < "2026-08-01T00:00:00Z"), so it needs no
// adjustment as an inclusive lower bound. A date-only "then" does NOT sort
// above every timestamp on that day — e.g. "2026-08-09T09:00:00Z" >
// "2026-08-09" is true — so taken literally it would exclude the entire
// target day from an inclusive upper bound. A bare-date "then" is therefore
// widened to the last instant of that day before comparison.
func Search(records map[string]Record, glob string, from, then string) ([]Record, error) {
	matchAll := glob == "" || glob == "*"

	thenBound := then
	if dateOnlyBound.MatchString(then) {
		thenBound = then + "T23:59:59Z"
	}

	out := []Record{}
	for _, rec := range records {
		if !matchAll {
			matched, err := path.Match(glob, rec.Key)
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
		}
		if from != "" && rec.WrittenAt < from {
			continue
		}
		if thenBound != "" && rec.WrittenAt > thenBound {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
