package wsnote

import (
	"path"
	"sort"
)

// Search returns records whose Key matches glob (stdlib path.Match — no new
// dependency, only key-glob matching is required) and whose WrittenAt falls
// within [from, then] when those bounds are non-empty. An empty glob matches
// every key. Bounds accept RFC3339 timestamps or a bare date prefix (e.g.
// "2026-08-01"); comparison is lexicographic string comparison, which is
// correct both for RFC3339-vs-RFC3339 and for a shorter date-prefix bound
// against a full RFC3339 value (a date prefix sorts as a lower/upper bound of
// every timestamp on that day). Results are sorted by Priority descending,
// then Key ascending, matching the injection ordering in inject.go.
func Search(records map[string]Record, glob string, from, then string) ([]Record, error) {
	if glob == "" {
		glob = "*"
	}
	out := []Record{}
	for _, rec := range records {
		matched, err := path.Match(glob, rec.Key)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if from != "" && rec.WrittenAt < from {
			continue
		}
		if then != "" && rec.WrittenAt > then {
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
