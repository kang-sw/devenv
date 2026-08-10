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

func TestSearchInvalidGlobFails(t *testing.T) {
	records := map[string]Record{"a": {Key: "a"}}

	if _, err := Search(records, "[", "", ""); err == nil {
		t.Fatalf("Search(malformed glob) = nil error, want failure")
	}
}
