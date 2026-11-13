package wswhiteboard

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const queryEntryTime = "2026-11-13T12:00:00Z"

func queryEntryBoard() *State {
	a := TopicDoc{Name: "a", Body: "CURRENT CONTENT token", ChangedAt: queryEntryTime, Entries: []Entry{
		{ID: "e1", Kind: EntryAgent, Text: "imported baseline", At: "2026-11-01T09:00:00Z", Imported: true},
		{ID: "e2", Kind: EntryPanel, PanelID: "p1", Recap: "old recap token", Text: " \t<wb:x>old payload token &amp;</wb:x>\r\n", At: queryEntryTime},
		{ID: "e3", Kind: EntrySystem, ItemID: 1, Text: "system event", At: queryEntryTime},
		{ID: "e4", Kind: EntryPanel, PanelID: "p1", Recap: "latest panel recap", Text: "\n<script>latest payload secret</script>\n\n", At: queryEntryTime},
	}}
	for n := 5; n <= 16; n++ {
		kind := EntryAgent
		if n%2 == 0 {
			kind = EntrySystem
		}
		a.Entries = append(a.Entries, Entry{ID: fmt.Sprintf("e%d", n), Kind: kind, Text: fmt.Sprintf("line %d", n), At: queryEntryTime})
	}
	return &State{Counter: 2, EntrySeq: 18, PanelCounter: 2,
		Items: []Item{
			{ID: 1, Topic: "a", Title: "title-marker", Status: StatusOpen, Class: ClassPolicy, Options: []string{"option-marker"}, Recommendation: "recommend-marker", Rationale: "why-marker", Answer: "answer-marker", AnswerQuote: "quoted owner words", Links: []string{"https://link-marker.example"}},
			{ID: 2, Topic: "b", Title: "second item", Status: StatusAnswered, Class: ClassTechnical},
		},
		Topics: []TopicDoc{a,
			{Name: "b", Entries: []Entry{{ID: "e17", Kind: EntryPanel, PanelID: "p2", Recap: "recap only", At: queryEntryTime}}},
			{Name: "closed", Entries: []Entry{{ID: "e18", Kind: EntryAgent, Text: "closed agent", At: queryEntryTime}}},
		},
		Labels: map[string]string{"b": "Second"}, ClosedTopics: []string{"closed"}, CloseResults: map[string]string{"closed": "done"},
	}
}

func queryResult(t *testing.T, st *State, raw string) QueryResult {
	t.Helper()
	r, problems := Query(st, queryPayload(t, raw))
	if len(problems) > 0 {
		t.Fatalf("Query(%s): %v", raw, problems)
	}
	return r
}

func queryEntryIDs(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

func TestQueryDiscoveryMetadataOnly(t *testing.T) {
	st := queryEntryBoard()
	r := queryResult(t, st, `{}`)
	if r.Mode != "discovery" || len(r.Topics) != 2 || r.FoldedTopics != 1 {
		t.Fatalf("discovery = %+v", r)
	}
	a := r.Topics[0]
	if a.Body != "" || a.Items != nil || a.LatestAgent == nil || a.LatestAgent.ID != "e15" {
		t.Fatalf("discovery leaked current content/items or selected a system summary: %+v", a)
	}
	if len(a.Entries) != 10 || a.Entries[0].ID != "e7" || a.Entries[9].ID != "e16" || a.OmittedEntries != 4 {
		t.Fatalf("default log tail = %+v", a)
	}
	if len(a.Panels) != 1 || a.Panels[0].ID != "e4" || a.Panels[0].Text != "" || a.OmittedPanels != 1 {
		t.Fatalf("latest panel recaps = %+v", a.Panels)
	}
	text := FormatQuery(r)
	wantText(t, "metadata discovery", text, []string{"current e15 " + queryEntryTime + ": line 15", "panel p1 (e4)", "omitted: 4 matching entries (last: 10); 1 older matching panel versions", "1 closed topic"}, []string{"CURRENT CONTENT", "payload secret", "old payload token", "topic body:", "title-marker"})
	for _, raw := range []string{`{"topic":"a"}`, `{"topic":"a","depth":"full"}`, `{"topic":"a","depth":"summary"}`} {
		if got := queryText(t, st, raw); strings.Contains(got, "CURRENT CONTENT") || strings.Contains(got, "payload secret") {
			t.Fatalf("implicit body leak for %s:\n%s", raw, got)
		}
	}
	if got := queryText(t, st, `{"topic":"a","body":true}`); !strings.Contains(got, "CURRENT CONTENT token") || strings.Contains(got, "payload secret") {
		t.Fatalf("explicit content retrieval:\n%s", got)
	}
	if got := queryText(t, st, `{"closed":true}`); !strings.Contains(got, "closed agent") {
		t.Fatal("closed discovery lost agent line")
	}
	if got := queryText(t, st, `{"topic":"closed","closed":false}`); !strings.Contains(got, "omitted: 1 closed topic") {
		t.Fatal("explicit closed exclusion lost omission")
	}
}

func TestQueryDiscoveryEmptyAndImported(t *testing.T) {
	for _, st := range []*State{nil, {}} {
		if got := queryText(t, st, `{}`); got != "no matching topics\n" {
			t.Fatalf("empty discovery = %q", got)
		}
	}
	st := &State{Topics: []TopicDoc{{Name: "content", Body: "not implicit"}, {Name: "log", Entries: []Entry{{ID: "e1", Kind: EntryAgent, Text: "imported line", Imported: true, At: queryEntryTime}}}}, EntrySeq: 1}
	text := queryText(t, st, `{}`)
	wantText(t, "zero and one entry", text, []string{`topic "content"`, `current e1 [imported]`, `e1 [agent] [imported]`}, []string{"not implicit"})
	direct := queryText(t, st, `{"entry":"e1"}`)
	if !strings.HasPrefix(direct, `entry e1 topic "log" [agent] `+queryEntryTime+" [imported]\n") || !strings.HasSuffix(direct, "\nimported line") {
		t.Fatalf("direct import provenance: %q", direct)
	}
}

func TestQueryStreamFiltersBeforeLastAndChronological(t *testing.T) {
	st := queryEntryBoard()
	for _, tc := range []struct {
		raw string
		ids []string
	}{
		{`{"topic":"a","entries":"all","last":3}`, []string{"e14", "e15", "e16"}},
		{`{"topic":"a","entries":"all","kind":"agent","last":2}`, []string{"e13", "e15"}},
		{`{"topic":"a","entries":"log","kind":"system","last":2}`, []string{"e14", "e16"}},
		{`{"topic":"a","entries":"panel","last":1}`, []string{"e4"}},
		{`{"topic":"a","entries":"all","text":"old payload","last":1}`, []string{"e2"}},
		{`{"topic":"a","entries":"all","since":"e14"}`, []string{"e15", "e16"}},
		{`{"topic":"a","entries":"all","since":"e17"}`, nil},
		{`{"topic":"a","entries":"all","since":"2026-11-13T12:00:00Z"}`, nil},
		{`{"topic":"a","entries":"panel","since":"2026-11-13T11:59:59.999999999Z"}`, []string{"e2", "e4"}},
	} {
		r := queryResult(t, st, tc.raw)
		if len(r.Topics) != 1 || !reflect.DeepEqual(queryEntryIDs(r.Topics[0].Entries), tc.ids) {
			t.Errorf("%s: %+v, want %v", tc.raw, r.Topics, tc.ids)
		}
		for _, e := range r.Topics[0].Entries {
			if e.Kind == EntryPanel && e.Text != "" {
				t.Fatal("stream exposed panel body")
			}
		}
	}
	r := queryResult(t, st, `{"topic":"a","entries":"panel","last":1}`)
	if r.Topics[0].OmittedEntries != 1 || len(r.Topics[0].Panels) != 0 {
		t.Fatal("stream panel omission/window incorrect")
	}
	if r := queryResult(t, st, `{"topic":"a","kind":"agent","last":2}`); len(r.Topics[0].Panels) != 0 || r.Topics[0].Entries[0].ID != "e13" {
		t.Fatal("kind filter ignored in discovery")
	}
}

func TestQueryDiscoveryTextSearchesAllStoredTextWithoutBodies(t *testing.T) {
	st := queryEntryBoard()
	for _, token := range []string{"CURRENT CONTENT", "title-marker", "option-marker", "recommend-marker", "why-marker", "answer-marker", "quoted owner words", "link-marker", "old recap token", "old payload token", "latest payload secret", "system event"} {
		r := queryResult(t, st, fmt.Sprintf(`{"text":%q}`, token))
		if len(r.Topics) != 1 || r.Topics[0].Name != "a" {
			t.Errorf("text %q did not discover the topic: %+v", token, r.Topics)
		}
		if strings.Contains(FormatQuery(r), "<script>") || strings.Contains(FormatQuery(r), "CURRENT CONTENT token") {
			t.Errorf("text %q dumped body", token)
		}
	}
	// A historical-body match exposes that exact version, not a misleading
	// recap of the newer version whose body did not match.
	r := queryResult(t, st, `{"text":"old payload token"}`)
	if len(r.Topics[0].Panels) != 1 || r.Topics[0].Panels[0].ID != "e2" || r.Topics[0].Panels[0].Text != "" {
		t.Fatal("historical body match lost exact locator")
	}
	if got := queryText(t, st, `{"text":"not present"}`); got != "no matching topics\n" {
		t.Fatalf("empty search = %q", got)
	}
}

func TestQueryDirectPayloadsAreVerbatim(t *testing.T) {
	st := queryEntryBoard()
	for _, tc := range []struct{ raw, id, body string }{
		{`{"panel":"p1"}`, "e4", st.Topics[0].Entries[3].Text},
		{`{"panel":"e2"}`, "e2", st.Topics[0].Entries[1].Text},
		{`{"panel":"p2","topic":"Second"}`, "e17", ""},
		{`{"entry":"e3"}`, "e3", "system event"},
		{`{"entry":"e18"}`, "e18", "closed agent"},
	} {
		r := queryResult(t, st, tc.raw)
		out := strings.SplitN(FormatQuery(r), "\n", 2)
		if r.Direct == nil || r.Direct.Entry.ID != tc.id || len(out) != 2 || out[1] != tc.body {
			t.Errorf("%s changed payload bytes: %q", tc.raw, FormatQuery(r))
		}
		if len(r.Topics) != 0 || strings.Contains(out[0], "unrelated") {
			t.Fatal("direct lookup included discovery")
		}
	}
	st.Topics[0].Entries[2].Text = " \n<wb:mention>same &amp; exact log\t\n"
	if out := strings.SplitN(queryText(t, st, `{"entry":"e3"}`), "\n", 2); out[1] != st.Topics[0].Entries[2].Text {
		t.Fatal("direct log whitespace/XML changed")
	}
}

func TestQueryEntrySelectorValidation(t *testing.T) {
	st := queryEntryBoard()
	cases := map[string]string{
		`{"entries":"message"}`: "payload.entries", `{"kind":"panel"}`: "payload.kind",
		`{"entries":"panel","kind":"agent"}`: "cannot be combined",
		`{"entries":"all","id":1}`:           "cannot be combined", `{"entries":"log","body":false}`: "cannot be combined",
		`{"last":1,"depth":"items"}`: "cannot be combined", `{"since":"e1","fields":"answer"}`: "cannot be combined",
		`{"last":0}`: "must be positive", `{"last":-1}`: "must be positive", `{"last":1.5}`: "whole number", `{"last":"2"}`: "must be a number",
		`{"since":"yesterday"}`: "RFC3339", `{"since":"e01"}`: "canonical eN", `{"since":"e99"}`: "stale or missing",
		`{"panel":"p0"}`: "expected pN", `{"panel":"p01"}`: "expected pN", `{"entry":"p1"}`: "expected eN", `{"entry":"e0"}`: "expected eN",
		`{"entry":"e99"}`: "stale or missing", `{"panel":"p99"}`: "stale or missing", `{"panel":"e99"}`: "stale or missing",
		`{"entry":"e2"}`: `use panel:"e2"`, `{"panel":"e3"}`: `use entry:"e3"`,
		`{"panel":"p1","entry":"e1"}`: "cannot be combined", `{"panel":"p1","body":true}`: "cannot be combined",
		`{"panel":"p1","text":"x"}`: "cannot be combined", `{"entry":"e1","closed":true}`: "cannot be combined",
		`{"entry":"e1","last":1}`: "cannot be combined", `{"panel":"e2","since":"e1"}`: "cannot be combined",
		`{"panel":"p1","entries":"panel"}`: "cannot be combined", `{"entry":"e1","status":"open"}`: "cannot be combined",
		`{"panel":"p1","topic":"b"}`: "stale or missing", `{"entry":"e3","topic":"b"}`: "stale or missing",
	}
	for raw, message := range cases {
		if _, problems := Query(st, queryPayload(t, raw)); !hasProblem(problems, message) {
			t.Errorf("%s: %v, want %q", raw, problems, message)
		}
	}
}

func TestQueryIndependentValidationProblemsAggregate(t *testing.T) {
	for raw, wants := range map[string][]string{
		`{"entries":"bad","last":0,"since":"yesterday","extra":1}`: {"payload.entries", "payload.last", "payload.since", "payload.extra"},
		`{"entry":"bad","topic":1,"extra":1}`:                      {"payload.entry", "payload.topic", "payload.extra"},
		`{"depth":"bad","fields":"bad","extra":1}`:                 {"payload.depth", "payload.fields", "payload.extra"},
	} {
		_, problems := Query(queryEntryBoard(), queryPayload(t, raw))
		wantProblems(t, problems, wants...)
	}
}

func TestQueryZeroTimeBoundaryAndImportedContinuation(t *testing.T) {
	st := &State{EntrySeq: 2, Topics: []TopicDoc{{Name: "a", Entries: []Entry{
		{ID: "e1", Kind: EntryAgent, Imported: true, Text: "imported\nnot a new topic heading"},
		{ID: "e2", Kind: EntrySystem, Text: "dated", At: queryEntryTime},
	}}}}
	r := queryResult(t, st, `{"entries":"log","since":"0001-01-01T00:00:00Z"}`)
	if got := queryEntryIDs(r.Topics[0].Entries); !reflect.DeepEqual(got, []string{"e2"}) {
		t.Fatalf("zero-time cursor was treated as absent: %v", got)
	}
	if text := FormatQuery(r); !strings.Contains(text, "current e1 [imported] unknown-time: imported\n  not a new topic heading\n") {
		t.Fatalf("import continuation/provenance: %q", text)
	}
}

func TestQueryDurableLocatorsAcrossRestartAndRepost(t *testing.T) {
	dir := t.TempDir()
	before, err := (FolderStore{Dir: dir}).Open(queryEntryBoard())
	if err != nil {
		t.Fatal(err)
	}
	if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
		s.EntrySeq++
		s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: "e19", Kind: EntryPanel, PanelID: "p1", Recap: "repost", Text: "new full payload", At: queryEntryTime})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := (FolderStore{Dir: dir}).Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := queryResult(t, after, `{"panel":"e2"}`).Direct.Entry; got != before.Topics[0].Entries[1] {
		t.Fatal("old panel mention redirected after repost/restart")
	}
	if got := queryResult(t, after, `{"panel":"p1"}`).Direct.Entry; got.ID != "e19" || got.Text != "new full payload" {
		t.Fatal("stable panel target did not resolve latest")
	}
	if got := queryResult(t, after, `{"entry":"e3"}`).Direct.Entry; got != before.Topics[0].Entries[2] {
		t.Fatal("log locator changed at equal timestamps")
	}
	if got := queryResult(t, after, `{"topic":"a","entries":"all","since":"e16"}`).Topics[0].Entries; len(got) != 1 || got[0].ID != "e19" {
		t.Fatalf("since boundary changed after restart: %+v", got)
	}
}
