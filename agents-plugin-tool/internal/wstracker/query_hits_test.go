package wstracker

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// hitsView is the fixture the hit tests run on:
//
//	ada.pc#1  task "Plan" with body "the Quick brown fox"; lena comments
//	          "fox in a comment", maya "a second FOX comment", lena hides a
//	          third "fox hidden" comment, then four more mention fox
//	ada.pc#2  task "Fox title" with no body match
//	ada.pc#3  task "Long" whose body is long, non-ASCII, and has whitespace runs
//	ada.pc#4  task "Hid" hidden, body "fox in hiding"
func hitsView(t *testing.T) *View {
	t.Helper()
	long := strings.Repeat("Zażółć gęślą jaźń. ", 8) + "ÜNÏCODE\n\n  İSTANBUL   needle" + strings.Repeat(" ünï çödé", 30)
	steps := []step{
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Plan", "body": "the Quick brown fox"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "fox in a comment"}},
		{"maya.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "a second FOX comment"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "fox hidden"}},
		{"lena.pc", "comment.hide", map[string]any{"comment": "lena.pc#2"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "unrelated"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "fox three"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "fox four"}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "fox five"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Fox title"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Long", "body": long}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Hid", "body": "fox in hiding"}},
		{"ada.pc", "item.hide", map[string]any{"item": "ada.pc#4"}},
	}
	return topicView(t, "ada.pc", steps)
}

func hitsOf(t *testing.T, v *View, query string) []SearchHit {
	t.Helper()
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", query, err)
	}
	return v.SearchHits(q)
}

func TestSearchHitsBodyAndComments(t *testing.T) {
	v := hitsView(t)
	hits := hitsOf(t, v, "fox")
	var got []string
	for _, h := range hits {
		got = append(got, h.ID+"/"+h.Field+"/"+h.Comment)
	}
	thread := v.Items["ada.pc#1"].Thread
	var shown []string
	for _, cid := range thread {
		c := v.Comments[cid]
		if c.Hidden == nil && strings.Contains(strings.ToLower(c.Body), "fox") {
			shown = append(shown, cid)
		}
	}
	if len(shown) != 5 {
		t.Fatalf("fixture: %d shown fox comments, want 5", len(shown))
	}
	want := []string{
		"ada.pc#1/body/",
		"ada.pc#1/comment/" + shown[0],
		"ada.pc#1/comment/" + shown[1],
		"ada.pc#1/comment/" + shown[2], // three comments at most; the hidden one is skipped
		"ada.pc#2/item/",               // matched on its title only
		"ada.pc#4/body/",               // hidden items are included
	}
	if !slices.Equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	if hits[0].Text != "the Quick brown fox" || hits[1].Text != "fox in a comment" || hits[2].Text != "a second FOX comment" {
		t.Errorf("excerpts = %q, %q, %q", hits[0].Text, hits[1].Text, hits[2].Text)
	}
	for _, c := range hits[1:4] {
		if strings.Contains(c.Text, "hidden") {
			t.Errorf("hidden comment surfaced: %+v", c)
		}
	}
	if h := hits[4]; h.Text != "" || h.Title != "Fox title" || h.Kind != "task" || h.Status == "" || h.Hidden {
		t.Errorf("item hit = %+v", h)
	}
	if h := hits[5]; !h.Hidden || h.Title != "Hid" {
		t.Errorf("hidden hit = %+v", h)
	}
}

func TestSearchHitsFilterOnlyQueryFallsBackToItem(t *testing.T) {
	v := hitsView(t)
	hits := hitsOf(t, v, "status:hidden")
	if len(hits) != 1 || hits[0].ID != "ada.pc#4" || hits[0].Field != "item" || hits[0].Text != "" || !hits[0].Hidden {
		t.Fatalf("hits = %+v", hits)
	}
	// A free-text term beside a filter still excerpts; the filter alone does not.
	hits = hitsOf(t, v, "kind:task fox status:hidden")
	if len(hits) != 1 || hits[0].Field != "body" || hits[0].Text != "fox in hiding" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestSearchHitsOrderMatchesSearch(t *testing.T) {
	v := hitsView(t)
	for _, query := range []string{"", "fox", "status:open", "kind:task", "long", "zzz"} {
		q, err := ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, h := range v.SearchHits(q) {
			if len(ids) == 0 || ids[len(ids)-1] != h.ID {
				ids = append(ids, h.ID)
			}
		}
		want := v.Search(q)
		if !slices.Equal(ids, want) && !(len(ids) == 0 && len(want) == 0) {
			t.Errorf("query %q: hit ids %v, Search %v", query, ids, want)
		}
	}
	if hits := hitsOf(t, v, "zzz"); hits == nil || len(hits) != 0 {
		t.Errorf("no match = %#v, want an empty non-nil slice", hits)
	}
}

func TestSearchHitsExcerptIsRuneSafe(t *testing.T) {
	v := hitsView(t)
	for _, query := range []string{"needle", "ünï", "İstanbul", "ÜNÏCODE istanbul"} {
		hits := hitsOf(t, v, query)
		if len(hits) != 1 || hits[0].ID != "ada.pc#3" || hits[0].Field != "body" {
			t.Fatalf("%q: hits = %+v", query, hits)
		}
		tx := hits[0].Text
		if !utf8.ValidString(tx) {
			t.Errorf("%q: excerpt is not valid UTF-8: %q", query, tx)
		}
		if n := utf8.RuneCountInString(strings.Trim(tx, "…")); n > excerptTotal {
			t.Errorf("%q: excerpt has %d runes, want <= %d", query, n, excerptTotal)
		}
		if strings.Contains(tx, "\n") || strings.Contains(tx, "  ") {
			t.Errorf("%q: whitespace not collapsed: %q", query, tx)
		}
		if !strings.HasPrefix(tx, "…") || !strings.HasSuffix(tx, "…") {
			t.Errorf("%q: a cut on both sides needs ellipses: %q", query, tx)
		}
	}
	// The match sits inside the window, after the collapse, with the
	// leading context cut to about 40 runes.
	tx := hitsOf(t, v, "needle")[0].Text
	if !strings.Contains(tx, "ÜNÏCODE İSTANBUL needle") {
		t.Errorf("excerpt lost the collapsed context: %q", tx)
	}
	if i := strings.Index(tx, "needle"); utf8.RuneCountInString(tx[:i]) > excerptBefore+len("…")+1 {
		t.Errorf("leading context too long: %q", tx)
	}
	// An İ lower-cases to a shorter byte sequence: the offsets must not drift.
	if tx := hitsOf(t, v, "istanbul")[0].Text; !strings.Contains(tx, "İSTANBUL") {
		t.Errorf("excerpt drifted off the match: %q", tx)
	}
}
