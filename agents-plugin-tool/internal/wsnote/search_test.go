package wsnote

import "testing"

func TestSearchGlobMatchesKeys(t *testing.T) {
	records := map[string]Record{
		"ticket.260807.status": {Key: "ticket.260807.status", Value: "ready", Priority: 1, WrittenAt: "2026-08-01T00:00:00Z"},
		"ticket.260523.status": {Key: "ticket.260523.status", Value: "blocked", Priority: 2, WrittenAt: "2026-08-02T00:00:00Z"},
		"other":                {Key: "other", Value: "x", Priority: 0, WrittenAt: "2026-08-03T00:00:00Z"},
	}

	got, err := Search(records, "ticket.*", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Search(ticket.*) = %d results, want 2: %#v", len(got), got)
	}
	// Priority descending: 260523 (priority 2) before 260807 (priority 1).
	if got[0].Key != "ticket.260523.status" || got[1].Key != "ticket.260807.status" {
		t.Fatalf("Search(ticket.*) order = %#v, want priority-descending", got)
	}
}

func TestSearchEmptyGlobMatchesAll(t *testing.T) {
	records := map[string]Record{
		"a": {Key: "a", WrittenAt: "t"},
		"b": {Key: "b", WrittenAt: "t"},
	}

	got, err := Search(records, "", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Search(\"\") = %d results, want 2 (match-all)", len(got))
	}
}

// TestSearchMatchAllCrossesSlashInKey verifies the "*"/"" match-all case
// bypasses path.Match entirely rather than relying on it to behave like a
// true wildcard: path.Match's "*" does not cross "/", so a slash-bearing key
// would otherwise be silently dropped from a "matches every key" search.
func TestSearchMatchAllCrossesSlashInKey(t *testing.T) {
	records := map[string]Record{
		"a/b/c": {Key: "a/b/c", WrittenAt: "t"},
	}

	got, err := Search(records, "*", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Key != "a/b/c" {
		t.Fatalf("Search(\"*\") = %#v, want the slash-bearing key included", got)
	}

	got, err = Search(records, "", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Key != "a/b/c" {
		t.Fatalf("Search(\"\") = %#v, want the slash-bearing key included", got)
	}
}

func TestSearchDateRangeFilters(t *testing.T) {
	records := map[string]Record{
		"early": {Key: "early", WrittenAt: "2026-08-01T00:00:00Z"},
		"mid":   {Key: "mid", WrittenAt: "2026-08-05T00:00:00Z"},
		"late":  {Key: "late", WrittenAt: "2026-08-10T00:00:00Z"},
	}

	got, err := Search(records, "*", "2026-08-02", "2026-08-09")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Key != "mid" {
		t.Fatalf("Search(from/then) = %#v, want only 'mid'", got)
	}
}

// TestSearchDateOnlyThenBoundIncludesWholeDay is a regression test for the
// lexicographic-compare bug where a bare date-only "then" bound (e.g.
// "2026-08-09") excluded every real RFC3339 timestamp ON that day, because
// "2026-08-09T09:00:00Z" > "2026-08-09" is true under plain string
// comparison. The documented contract is an INCLUSIVE [from, then] range, so
// a record timestamped anywhere on the "then" day must be included. Unlike
// TestSearchDateRangeFilters (whose fixture dates never land exactly on the
// "then" day, masking this bug), this places a record on 2026-08-09 itself.
func TestSearchDateOnlyThenBoundIncludesWholeDay(t *testing.T) {
	records := map[string]Record{
		"before":   {Key: "before", WrittenAt: "2026-08-08T23:59:59Z"},
		"on-day":   {Key: "on-day", WrittenAt: "2026-08-09T09:00:00Z"},
		"late-day": {Key: "late-day", WrittenAt: "2026-08-09T23:59:58Z"},
		"after":    {Key: "after", WrittenAt: "2026-08-10T00:00:00Z"},
	}

	got, err := Search(records, "*", "", "2026-08-09")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	gotKeys := map[string]bool{}
	for _, rec := range got {
		gotKeys[rec.Key] = true
	}
	if !gotKeys["on-day"] || !gotKeys["late-day"] {
		t.Fatalf("Search(then:2026-08-09) = %#v, want both records timestamped ON 2026-08-09 included", got)
	}
	if !gotKeys["before"] {
		t.Fatalf("Search(then:2026-08-09) = %#v, want the earlier 2026-08-08 record included (no 'from' bound set)", got)
	}
	if gotKeys["after"] {
		t.Fatalf("Search(then:2026-08-09) = %#v, want 2026-08-10's record excluded", got)
	}
	if len(got) != 3 {
		t.Fatalf("Search(then:2026-08-09) = %d results, want exactly 3 (before, on-day, late-day)", len(got))
	}
}

// TestSearchOrdersByPriorityThenWrittenAtThenKey verifies Search's result
// order matches Compute's 3-key comparator (priority desc -> written_at desc
// -> key asc) exactly, not just the priority-desc/key-asc 2-key order it used
// before this change. Neither this file nor note_tools_test.go previously
// asserted order across records that tie on priority but differ on
// written_at, so this is new coverage, not a regression check.
func TestSearchOrdersByPriorityThenWrittenAtThenKey(t *testing.T) {
	records := map[string]Record{
		// Distinct priority: "high" must sort first regardless of the others.
		"high": {Key: "high", Priority: 5, WrittenAt: "2026-08-01T00:00:00Z"},
		// Equal priority (1), distinct written_at: newer written_at first.
		"newer": {Key: "newer", Priority: 1, WrittenAt: "2026-08-10T00:00:00Z"},
		"older": {Key: "older", Priority: 1, WrittenAt: "2026-08-06T00:00:00Z"},
		// Equal priority (1) AND equal written_at: key ascending breaks the tie.
		"tie-b": {Key: "tie-b", Priority: 1, WrittenAt: "2026-08-05T00:00:00Z"},
		"tie-a": {Key: "tie-a", Priority: 1, WrittenAt: "2026-08-05T00:00:00Z"},
	}

	got, err := Search(records, "*", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	wantOrder := []string{"high", "newer", "older", "tie-a", "tie-b"}
	if len(got) != len(wantOrder) {
		t.Fatalf("Search order = %#v, want %d records", got, len(wantOrder))
	}
	for i, wantKey := range wantOrder {
		if got[i].Key != wantKey {
			t.Fatalf("Search order[%d] = %q, want %q; full order: %#v", i, got[i].Key, wantKey, got)
		}
	}
}

func TestSearchInvalidGlobFails(t *testing.T) {
	records := map[string]Record{"a": {Key: "a"}}

	if _, err := Search(records, "[", "", ""); err == nil {
		t.Fatalf("Search(malformed glob) = nil error, want failure")
	}
}
