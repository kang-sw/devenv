package wsdoc

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// The fixtures below use invented, project-neutral stems and content.
const tmTestStem = "260101-feat-sample-engine"

func tmTestDoc(t *testing.T, body string) *ticketDoc {
	t.Helper()
	return buildTicketDoc(tmTestStem, "---\ntitle: Neutral\n---\n# Neutral\n\n## Details\n"+body+"\n", false)
}

func tmHas(doc *ticketDoc, term string, ch int) bool {
	st := doc.Terms[term]
	return st != nil && st.n[ch] > 0
}

func tmRequire(t *testing.T, doc *ticketDoc, ch int, present, absent []string) {
	t.Helper()
	for _, term := range present {
		if !tmHas(doc, term, ch) {
			t.Errorf("term %q missing on channel %d; terms: %v", term, ch, tmTermKeys(doc))
		}
	}
	for _, term := range absent {
		if tmHas(doc, term, ch) {
			t.Errorf("term %q present on channel %d", term, ch)
		}
	}
}

func tmTermKeys(doc *ticketDoc) []string {
	keys := make([]string, 0, len(doc.Terms))
	for k := range doc.Terms {
		keys = append(keys, k)
	}
	return keys
}

func tmNear(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTicketMapTextEnglishStopwordsAndNoise(t *testing.T) {
	doc := tmTestDoc(t, "The parser and the renderer should handle 42 items with a1b2c3d4, x9, "+
		"v12345, deadbeef and q. Ready todo idea done dropped; see Background and Decisions.")
	tmRequire(t, doc, tmChProse,
		[]string{"parser", "renderer", "handle", "items"},
		[]string{"the", "and", "should", "with", "ready", "todo", "idea", "done", "dropped",
			"background", "decisions", "42", "a1b2c3d4", "x9", "v12345", "q"})
	// A hex word without a digit is a word, not a hash.
	tmRequire(t, doc, tmChProse, []string{"deadbeef"}, nil)
}

func TestTicketMapTextCamelCaseSplits(t *testing.T) {
	doc := tmTestDoc(t, "Call parseHTTPRequest, then utf8Decode and XMLHttpParser.")
	tmRequire(t, doc, tmChProse,
		[]string{"call", "parse", "http", "request", "utf8", "decode", "xml", "parser"},
		[]string{"parsehttprequest", "xmlhttpparser", "utf8decode"})
}

func TestTicketMapTextHangulParticles(t *testing.T) {
	doc := tmTestDoc(t, "화이트보드를 공유하는 기능은 데이터가 서버로 그리고 디스플레이 및 방법으로 저장 것을")
	tmRequire(t, doc, tmChProse,
		[]string{"화이트보드", "공유", "기능", "데이터", "서버", "디스플레이", "방법", "저장"},
		[]string{"화이트보드를", "공유하는", "기능은", "데이터가", "서버로", "그리고", "및", "디스플레", "것을", "방법으로"})
}

func TestTicketMapTextCJKBigrams(t *testing.T) {
	doc := tmTestDoc(t, "ホワイトボード 設定を変更 字")
	tmRequire(t, doc, tmChProse,
		[]string{"ホワ", "ワイ", "イト", "トボ", "ボー", "ード", "設定", "変更"},
		// Runs are per script, so the Hiragana particle splits the Kanji, and
		// single-rune runs emit nothing.
		[]string{"定を", "を変", "字", "を", "ホワイトボード"})
}

func TestTicketMapTextCodeSpans(t *testing.T) {
	doc := tmTestDoc(t, "Edit `internal/render/page_view.go:42` and `pages.go#L10-L20`; style `#mainPanel` "+
		"and `.side-bar`; call `config.Loader` via `buildIndex(alpha, beta)` on `(*Store).Flush`. "+
		"Ignore `https://example.com/docs/guide.md`, `deadbeef1234`, `260102-feat-other-thing`, "+
		"`ai-docs/tickets/todo/260102-feat-other-thing.md`, `averyveryverylongidentifiernameexceedinglimit`, "+
		"and `key:value`.")
	tmRequire(t, doc, tmChCode,
		[]string{"internal", "render", "page_view.go", "page_view", "pages.go", "pages", "#mainpanel",
			".side-bar", "config.loader", "buildindex", "store", "flush"},
		[]string{"page_view.go:42", "alpha", "beta", ".flush", "example.com", "guide.md", "guide", "docs",
			"deadbeef1234", "260102-feat-other-thing", "other", "tickets", "ai-docs",
			"averyveryverylongidentifiernameexceedinglimit", "key:value", "key", "value", "pages.go#l10-l20"})
	// The stem never reaches prose either.
	tmRequire(t, doc, tmChProse, []string{"main", "panel", "loader"}, []string{"other", "thing", "example"})
}

func TestTicketMapTextBarePaths(t *testing.T) {
	doc := tmTestDoc(t, "See lib/store/cache_entry.rs and notes.md, then src/widgets/panel. "+
		"Read/write is prose; https://example.org/docs/guide.md is a URL, "+
		"and foo.test.go is not a bare path.")
	tmRequire(t, doc, tmChCode,
		[]string{"lib", "store", "cache_entry.rs", "cache_entry", "notes.md", "notes", "src", "widgets", "panel"},
		[]string{"read", "write", "guide.md", "guide", "docs", "foo.test.go", "test.go"})
}

func TestTicketMapTextFencedCode(t *testing.T) {
	doc := tmTestDoc(t, "```go\n## not a heading\nfunc helperThing() {} // see pkg/core/util.go\n```\nclosing")
	if len(doc.Headings) != 1 || doc.Headings[0].Text != "Details" {
		t.Fatalf("headings = %#v, want only Details", doc.Headings)
	}
	tmRequire(t, doc, tmChCode, []string{"util.go", "util", "pkg", "core"}, nil)
	tmRequire(t, doc, tmChProse, []string{"closing"}, []string{"helper", "func", "heading"})
}

func TestTicketMapTextSectionWeighting(t *testing.T) {
	text := "---\ntitle: Sample Widget\nrelated:\n  260102-feat-other-thing: shares tokenizer\n---\n" +
		"# Sample Widget\n\n" +
		"## Background\nalpha `delta.go`\n\n" +
		"### Sub Topic\nfoxtrot\n\n" +
		"## Details\nbravo\n\n" +
		"## Verification\ncharlie\n\n" +
		"### Phase 2: Storage Layout\necho\n\n" +
		"### Result (abc1234) - 2026-01-02\ngolf\n"
	doc := buildTicketDoc(tmTestStem, text, false)
	cases := []struct {
		term string
		ch   int
		w    float64
	}{
		{"alpha", tmChProse, 1.4},
		{"foxtrot", tmChProse, 1.4}, // inherits Background
		{"bravo", tmChProse, 1.0},
		{"charlie", tmChProse, 0.5},
		{"echo", tmChProse, 0.5}, // inherits Verification
		{"golf", tmChProse, 0.5},
		{"delta.go", tmChCode, 3.0 * 1.4},
		{"sub", tmChHeading, 1.4},
		{"storage", tmChHeading, 0.5},
		{"tokenizer", tmChProse, 0.6},
		{"shares", tmChProse, 0.6},
		{"widget", tmChTitle, 2.4},
		{"sample", tmChTitle, 4.8}, // stem core and title
		{"engine", tmChTitle, 2.4},
	}
	for _, c := range cases {
		st := doc.Terms[c.term]
		if st == nil {
			t.Errorf("term %q missing", c.term)
			continue
		}
		if !tmNear(st.w[c.ch], c.w) {
			t.Errorf("term %q channel %d weight = %v, want %v", c.term, c.ch, st.w[c.ch], c.w)
		}
	}
	if st := doc.Terms["sample"]; st == nil || st.n[tmChTitle] != 2 {
		t.Errorf("sample title count = %+v, want 2", st)
	}
	// Heading prefixes are stripped: no phase number, result hash or date.
	tmRequire(t, doc, tmChHeading, nil, []string{"phase", "result", "abc1234", "feat", "260101"})
	if !tmOnlyChannel(doc.Terms["widget"], tmChTitle) {
		t.Errorf("widget leaked out of the title channel: %+v", doc.Terms["widget"])
	}
	wantHeadings := []mapHeading{
		{2, "Background"}, {3, "Sub Topic"}, {2, "Details"}, {2, "Verification"},
		{3, "Phase 2: Storage Layout"}, {3, "Result (abc1234) - 2026-01-02"},
	}
	if !reflect.DeepEqual(doc.Headings, wantHeadings) {
		t.Errorf("headings = %#v", doc.Headings)
	}
}

func tmOnlyChannel(st *termStat, ch int) bool {
	for i, n := range st.n {
		if (i == ch) != (n > 0) {
			return false
		}
	}
	return true
}

func TestTicketMapTextTitleFallbacks(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"frontmatter", "---\ntitle: \"Quoted Title\"\n---\n# Heading One\n", "Quoted Title"},
		{"h1", "---\nparent: x\n---\n\n```\n# fenced\n```\n# Heading One\n## Two\n", "Heading One"},
		{"broken frontmatter", "---\ntitle: Lost\n# Heading One\nbody\n", "Heading One"},
		{"stem", "plain body only\n", tmTestStem},
		{"empty", "", tmTestStem},
	}
	for _, c := range cases {
		doc := buildTicketDoc(tmTestStem, c.text, false)
		if doc.Title != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, doc.Title, c.want)
		}
	}
	// Broken frontmatter lines are body text.
	doc := buildTicketDoc(tmTestStem, cases[2].text, false)
	tmRequire(t, doc, tmChProse, []string{"lost", "body"}, nil)
	if doc.Lines != 5 {
		t.Errorf("lines = %d, want 5", doc.Lines)
	}
}

func TestTicketMapTextPreview(t *testing.T) {
	short := buildTicketDoc(tmTestStem, "# T\nintro\n## Background\n\nshort text\n\n## Next\nmore\n## Empty\n", false)
	if want := "## Background\nshort text\n\n## Next\nmore\n\n## Empty"; short.Preview != want {
		t.Errorf("preview = %q, want %q", short.Preview, want)
	}
	long := "## Background\n" + strings.Repeat("화이트보드 내용 ", 400) + "\n## Later\nnever shown\n"
	for _, c := range []struct {
		closed bool
		limit  int
	}{{false, 1500}, {true, 700}} {
		doc := buildTicketDoc(tmTestStem, long, c.closed)
		n := utf8.RuneCountInString(doc.Preview)
		if n > c.limit || n < c.limit-10 {
			t.Errorf("closed=%v: preview runes = %d, want about %d", c.closed, n, c.limit)
		}
		if !strings.HasPrefix(doc.Preview, "## Background\n화이트보드") || !strings.HasSuffix(doc.Preview, "\n…") {
			t.Errorf("closed=%v: preview shape = %q…%q", c.closed, doc.Preview[:30], doc.Preview[len(doc.Preview)-10:])
		}
		if !utf8.ValidString(doc.Preview) {
			t.Errorf("closed=%v: preview is not valid UTF-8", c.closed)
		}
	}
}

func TestTicketMapTextInvalidUTF8(t *testing.T) {
	doc := tmTestDoc(t, "valid\xffword 화이\xfe트보드를 plain\xc3")
	for term := range doc.Terms {
		if !utf8.ValidString(term) {
			t.Errorf("term %q is not valid UTF-8", term)
		}
	}
	tmRequire(t, doc, tmChProse, []string{"validword", "화이트보드", "plain"}, nil)
	if !utf8.ValidString(doc.Preview) || !utf8.ValidString(doc.Title) {
		t.Errorf("preview or title is not valid UTF-8: %q %q", doc.Preview, doc.Title)
	}
}

func TestTicketMapTextMentions(t *testing.T) {
	text := "---\nrelated:\n  260109-chore-frontmatter-only: note\n---\n# T\n" +
		"See 260102-feat-other-thing and 260102-feat-other-thing again, " +
		"ai-docs/tickets/todo/260103-bug-third-item.md, 260104-refactor-fourth#phase2, " +
		"our own " + tmTestStem + ", x260105-feat-glued, 1260106-feat-seven-digits.\n" +
		"```\n260107-epic-in-fence\n```\n"
	doc := buildTicketDoc(tmTestStem, text, false)
	want := []string{"260102-feat-other-thing", "260103-bug-third-item", "260104-refactor-fourth", "260107-epic-in-fence"}
	if !reflect.DeepEqual(doc.Mentions, want) {
		t.Errorf("mentions = %q, want %q", doc.Mentions, want)
	}
	tmRequire(t, doc, tmChProse, nil, []string{"other", "thing", "third", "fourth"})
}

func TestTicketMapTextFeatureCapAndODQ(t *testing.T) {
	filler := strings.Repeat("filler words here\n", (tmFeatureCap/18)+10)
	text := "# T\n## Open Decision Queue\n" + filler + "tailword\n"
	doc := buildTicketDoc(tmTestStem, text, false)
	if tmHas(doc, "tailword", tmChProse) {
		t.Error("a term past the 256 KiB feature cap was read")
	}
	tmRequire(t, doc, tmChProse, []string{"filler"}, nil)
	if !doc.ODQ {
		t.Error("ODQ = false, want true")
	}
	if want := strings.Count(text, "\n") + 1; doc.Lines != want {
		t.Errorf("lines = %d, want %d", doc.Lines, want)
	}
}

func BenchmarkTicketMapTextBuildDoc(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("---\ntitle: Neutral Sample Ticket\nrelated:\n  260102-feat-other-thing: shares parsing\n---\n# Neutral Sample Ticket\n\n")
	for i := 0; i < 40; i++ {
		sb.WriteString("## Background Section\n")
		sb.WriteString("The parseHTTPRequest helper in `internal/render/page_view.go:42` maps rows to columns; ")
		sb.WriteString("see lib/store/cache_entry.rs and 260103-bug-third-item. 화이트보드를 공유하는 기능은 설정を変更.\n\n")
		sb.WriteString("```go\nfunc helper() {}\n```\n")
	}
	text := sb.String()
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buildTicketDoc(tmTestStem, text, false)
	}
}
