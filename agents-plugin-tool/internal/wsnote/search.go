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
//
// The result order matches Compute's comparator exactly (priority desc ->
// written_at desc -> key asc, via CompareRecords) so a single-layer
// note.query and a multi-layer note.query never diverge in order, only in
// whether each record carries a layer tag.
func Search(records map[string]Record, glob string, from, then string) ([]Record, error) {
	out, err := FilterRecords(records, glob, from, then)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return CompareRecords(out[i], out[j])
	})
	return out, nil
}

// FilterRecords applies Search's glob/from/then filtering without sorting.
// It is exported (rather than kept package-private) because the multi-layer
// note.query merge path lives in package mcp, not wsnote: that path filters
// each layer independently via this same function before combining and
// sorting the tagged result once, so the two paths' filtering can never
// diverge.
func FilterRecords(records map[string]Record, glob string, from, then string) ([]Record, error) {
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
	return out, nil
}

// CompareRecords reports whether a sorts before b under the shared
// note-ordering comparator: priority desc -> written_at desc -> key asc.
// This is the exact comparator Compute (inject.go) uses for the ambient
// "# Notes" block, shared here so Search's single-layer order and the
// multi-layer note.query merge order can never diverge from it.
func CompareRecords(a, b Record) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.WrittenAt != b.WrittenAt {
		return a.WrittenAt > b.WrittenAt
	}
	return a.Key < b.Key
}
