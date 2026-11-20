// Package wsdoctest holds test helpers shared by other packages' tests,
// starting with the ticket map's synthetic board fixture. It imports only the
// standard library so any package's tests can use it without an import cycle.
package wsdoctest

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The ticket map fixture is an invented, project-neutral board: every stem,
// domain, word and code path below is synthetic.
const (
	// TicketMapDefaultOpen is the default number of open tickets.
	TicketMapDefaultOpen = 170
	// TicketMapMinOpen is the smallest open-ticket count the topic layout
	// supports (the orchard topic needs room for every epic relation).
	TicketMapMinOpen = 160

	// TicketMapEpicStem is the planted epic: 11 children, a parent and a
	// grandparent, 6 related, 2 blocked-by, and 3 tickets blocked by it.
	TicketMapEpicStem            = "260101-epic-orchard-platform"
	TicketMapEpicParentStem      = "260101-epic-orchard-portfolio"
	TicketMapEpicGrandparentStem = "260101-epic-orchard-charter"

	// TicketMapEveryTicketTerm appears in every ticket, so the df cut must
	// remove it.
	TicketMapEveryTicketTerm = "widget"

	// TicketMapDanglingA and TicketMapDanglingB are referenced by open tickets
	// but exist in no status directory.
	TicketMapDanglingA = "260131-bug-billing-vanished"
	TicketMapDanglingB = "260131-feat-optics-withdrawn"
)

// TicketMapModTime is the mtime every fixture file gets: fixed (so two
// fixture copies fingerprint alike) and far older than the ticket cache's
// racy window (so warm builds memoize).
var TicketMapModTime = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

// Script values of TicketMapTopic.Script.
const (
	TicketMapScriptLatin    = "latin"
	TicketMapScriptHangul   = "hangul"
	TicketMapScriptJapanese = "japanese"
)

// TicketMapTopic is one planted topic: open tickets sharing a vocabulary and
// one code path, which the cluster engine should co-cluster.
type TicketMapTopic struct {
	Name       string   // slug used in the topic's stems (Latin topics) or a tag
	Script     string   // TicketMapScript*
	TitleWords []string // the topic vocabulary; every title draws from it
	CodePath   string   // the topic's one invented code path
	Stems      []string // open topic tickets, in creation order
}

// TicketMapLayout describes what WriteTicketMapFixture plants for a given n.
type TicketMapLayout struct {
	Open   int              // open tickets (ready + todo + idea)
	Topics []TicketMapTopic // 8 Latin topics, then the Korean and Japanese topics

	EpicChildren  []string // 11 open children of TicketMapEpicStem
	EpicAncestors []string // parent, then grandparent
	EpicRelated   []string // 6: 4 open, 2 closed (.done)
	EpicBlockedBy []string // 2: 1 open, 1 closed (.done)
	EpicBlocks    []string // 3 open tickets whose blocked-by names the epic

	RefDone      []string // 6 closed tickets in .done/ referenced by open tickets
	RefDropped   []string // 3 closed tickets in .dropped/ referenced by open tickets
	UnrefDone    []string // 30 closed tickets in .done/ nothing references
	UnrefDropped []string // 10 closed tickets in .dropped/ nothing references

	Dangling          []string    // TicketMapDanglingA, TicketMapDanglingB
	ParentCycle       [2]string   // A.parent = B, B.parent = A
	BlockedByCycle    [3]string   // A -> B -> C -> A
	SelfRef           string      // parent names itself
	Phase2From        string      // blocked-by: Phase2To#phase2
	Phase2To          string      // has at least two phases
	MentionPairs      [][2]string // [0]'s body names [1]'s stem; no frontmatter relation
	BrokenFrontmatter string      // frontmatter without a closing fence (title from H1)
	NoTitle           string      // no title key and no H1 (title falls back to the stem)
	Isolated          []string    // 5 open tickets with unique vocabulary, no relations

	// Dirs maps every written stem to its status directory: ready, todo,
	// idea, .done or .dropped.
	Dirs map[string]string
}

// Count returns how many written tickets sit in status directory dir.
func (l TicketMapLayout) Count(dir string) int {
	n := 0
	for _, d := range l.Dirs {
		if d == dir {
			n++
		}
	}
	return n
}

// TopicOf returns the name of the topic stem belongs to, or "".
func (l TicketMapLayout) TopicOf(stem string) string {
	for _, topic := range l.Topics {
		for _, s := range topic.Stems {
			if s == stem {
				return topic.Name
			}
		}
	}
	return ""
}

// TicketMapFixtureLayout returns the layout WriteTicketMapFixture writes for
// n open tickets, without touching the filesystem.
func TicketMapFixtureLayout(n int) (TicketMapLayout, error) {
	layout, _, err := planTicketMap(n)
	return layout, err
}

// WriteTicketMapFixture writes the synthetic board under
// <dir>/ai-docs/tickets/{ready,todo,idea,.done,.dropped}/ with n open tickets
// (TicketMapDefaultOpen by default; at least TicketMapMinOpen). Content is
// deterministic, and every file's mtime is TicketMapModTime.
func WriteTicketMapFixture(t testing.TB, dir string, n int) {
	t.Helper()
	_, tickets, err := planTicketMap(n)
	if err != nil {
		t.Fatal(err)
	}
	board := filepath.Join(dir, "ai-docs", "tickets")
	for _, status := range []string{"ready", "todo", "idea", ".done", ".dropped"} {
		if err := os.MkdirAll(filepath.Join(board, status), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tk := range tickets {
		path := filepath.Join(board, tk.dir, tk.stem+".md")
		if err := os.WriteFile(path, []byte(tk.render()), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, TicketMapModTime, TicketMapModTime); err != nil {
			t.Fatal(err)
		}
	}
}

type fixtureTicket struct {
	stem, dir string
	title     string // "" omits the title key
	parent    string
	related   [][2]string // stem, note
	blockedBy []string
	broken    bool // no closing frontmatter fence
	body      string
	mention   string // topic words a body mention of another stem is phrased with
}

func (tk fixtureTicket) render() string {
	var b strings.Builder
	b.WriteString("---\n")
	if tk.title != "" {
		fmt.Fprintf(&b, "title: %s\n", tk.title)
	}
	if tk.parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", tk.parent)
	}
	if len(tk.related) > 0 {
		b.WriteString("related:\n")
		for _, r := range tk.related {
			fmt.Fprintf(&b, "  %s: %s\n", r[0], r[1])
		}
	}
	if len(tk.blockedBy) > 0 {
		b.WriteString("blocked-by:\n")
		for _, spec := range tk.blockedBy {
			fmt.Fprintf(&b, "  - %s\n", spec)
		}
	}
	if tk.title == "" && tk.parent == "" && len(tk.related) == 0 && len(tk.blockedBy) == 0 {
		b.WriteString("assignee: []\n")
	}
	if !tk.broken {
		b.WriteString("---\n")
	}
	b.WriteString("\n")
	b.WriteString(tk.body)
	return b.String()
}

type topicSpec struct {
	name, script, path string
	words              []string
	base               int // size at TicketMapDefaultOpen
}

// Topic vocabularies are disjoint, avoid the text engine's stoplists, and
// share no lemma, so each topic is separable on its own terms.
var ticketMapTopics = []topicSpec{
	{"orchard", TicketMapScriptLatin, "orchard/harvest.go", []string{"orchard", "pruning", "harvest", "sapling", "graft", "rootstock", "blossom", "pollinator", "cider", "espalier", "canopy", "bushel"}, 19},
	{"billing", TicketMapScriptLatin, "billing/invoice.go", []string{"invoice", "posting", "reconcile", "journal", "accrual", "debit", "creditor", "settlement", "voucher", "payable", "auditor", "remittance"}, 19},
	{"garden", TicketMapScriptLatin, "garden/moisture.py", []string{"garden", "moisture", "soil", "sprinkler", "seedling", "compost", "mulch", "trellis", "greenhouse", "germination", "watering", "hydrometer"}, 19},
	{"optics", TicketMapScriptLatin, "optics/eyepiece.rs", []string{"telescope", "eyepiece", "nebula", "observatory", "refractor", "aperture", "collimation", "comet", "starfield", "parallax", "zenith", "astrograph"}, 18},
	{"kitchen", TicketMapScriptLatin, "kitchen/recipe.ts", []string{"recipe", "pantry", "oven", "ingredient", "simmer", "braise", "saffron", "menu", "skillet", "marinade", "broth", "ladle"}, 17},
	{"railway", TicketMapScriptLatin, "rail/timetable.sql", []string{"railway", "locomotive", "junction", "timetable", "carriage", "depot", "freight", "switchyard", "signalman", "gauge", "sleeper", "turntable"}, 16},
	{"weather", TicketMapScriptLatin, "meteo/barometer.py", []string{"forecast", "barometer", "rainfall", "humidity", "cyclone", "isobar", "drizzle", "thunderstorm", "anemometer", "monsoon", "dewpoint", "hailstone"}, 15},
	{"archive", TicketMapScriptLatin, "archive/catalog.js", []string{"archive", "catalog", "manuscript", "librarian", "bookshelf", "binding", "folio", "vellum", "codex", "provenance", "accession", "curator"}, 14},
	{"kr", TicketMapScriptHangul, "shop/cart.go", []string{"주문", "배송", "추적", "결제", "장바구니", "상품", "재고", "반품", "쿠폰", "택배", "영수증", "환불"}, 13},
	{"jp", TicketMapScriptJapanese, "music/score.go", []string{"楽譜", "調律", "録音", "演奏", "練習", "ギター", "ピアノ", "バイオリン", "メトロノーム", "合奏", "指揮者", "音階"}, 12},
}

// ticketMapSpecials counts the open tickets outside every topic: the epic,
// its two ancestors, and the five isolated tickets.
const ticketMapSpecials = 8

// Isolated tickets: each word appears in exactly one ticket.
var ticketMapIsolated = []struct {
	stem  string
	words [5]string
}{
	{"260121-research-quokka-lantern", [5]string{"quokka", "lantern", "marzipan", "dune", "glows"}},
	{"260122-research-obsidian-tambourine", [5]string{"obsidian", "tambourine", "walrus", "fjord", "hums"}},
	{"260123-research-zeppelin-origami", [5]string{"zeppelin", "origami", "cactus", "lagoon", "sways"}},
	{"260124-research-kaleidoscope-narwhal", [5]string{"kaleidoscope", "narwhal", "tundra", "velvet", "chimes"}},
	{"260125-research-harmonica-iguana", [5]string{"harmonica", "iguana", "quarry", "tapestry", "whistles"}},
}

var ticketMapCats = []string{"feat", "bug", "refactor", "chore", "research"}

// topicSizes scales the base sizes to n open tickets, keeping the total exact.
func topicSizes(n int) ([]int, error) {
	if n < TicketMapMinOpen {
		return nil, fmt.Errorf("ticket map fixture: n=%d is below the minimum %d", n, TicketMapMinOpen)
	}
	// The orchard topic never drops under orchardMinTopic (its tickets carry
	// every epic relation); the rounding correction adjusts the other topics.
	const orchardMinTopic = 19
	sizes := make([]int, len(ticketMapTopics))
	sum := 0
	for i, tp := range ticketMapTopics {
		sizes[i] = int(math.Round(float64(tp.base) * float64(n) / TicketMapDefaultOpen))
		if i == 0 {
			sizes[i] = max(sizes[i], orchardMinTopic)
		}
		sum += sizes[i]
	}
	for i := 1; sum != n-ticketMapSpecials; i = i%(len(sizes)-1) + 1 {
		if sum < n-ticketMapSpecials {
			sizes[i]++
			sum++
		} else {
			sizes[i]--
			sum--
		}
	}
	for i, size := range sizes {
		if size < 8 {
			return nil, fmt.Errorf("ticket map fixture: topic %s has %d tickets, need 8", ticketMapTopics[i].name, size)
		}
	}
	return sizes, nil
}

func topicStem(ti, j int, tp topicSpec) string {
	day := (j+ti*3)%28 + 1
	return fmt.Sprintf("2601%02d-%s-%s-%02d", day, ticketMapCats[(j+ti)%len(ticketMapCats)], tp.name, j)
}

// planTicketMap builds the layout and every ticket to write.
func planTicketMap(n int) (TicketMapLayout, []fixtureTicket, error) {
	sizes, err := topicSizes(n)
	if err != nil {
		return TicketMapLayout{}, nil, err
	}
	layout := TicketMapLayout{Open: n, Dirs: map[string]string{}}
	byTopic := make([][]*fixtureTicket, len(ticketMapTopics))
	var open []*fixtureTicket
	for ti, tp := range ticketMapTopics {
		topic := TicketMapTopic{Name: tp.name, Script: tp.script, TitleWords: append([]string(nil), tp.words...), CodePath: tp.path}
		for j := 0; j < sizes[ti]; j++ {
			tk := topicTicket(tp, j, topicStem(ti, j, tp))
			topic.Stems = append(topic.Stems, tk.stem)
			byTopic[ti] = append(byTopic[ti], tk)
			open = append(open, tk)
		}
		layout.Topics = append(layout.Topics, topic)
	}

	orchard := ticketMapTopics[0]
	epic := topicTicket(orchard, 100, TicketMapEpicStem)
	epic.title = "Orchard platform for the harvest canopy"
	epic.body = strings.Replace(epic.body, "# "+titleOf(orchard, 100), "# "+epic.title, 1)
	parent := topicTicket(orchard, 101, TicketMapEpicParentStem)
	grand := topicTicket(orchard, 102, TicketMapEpicGrandparentStem)
	open = append(open, epic, parent, grand)

	var closed []*fixtureTicket
	closedRef := func(ti, k int, dir string) *fixtureTicket {
		tp := ticketMapTopics[ti]
		stem := fmt.Sprintf("2601%02d-feat-%s-%d", 20+k, tp.name, 90+k)
		tk := topicTicket(tp, 90+k, stem)
		tk.dir = dir
		closed = append(closed, tk)
		if dir == ".done" {
			layout.RefDone = append(layout.RefDone, stem)
		} else {
			layout.RefDropped = append(layout.RefDropped, stem)
		}
		return tk
	}
	refDone := []*fixtureTicket{closedRef(0, 0, ".done"), closedRef(0, 1, ".done"), closedRef(0, 2, ".done"),
		closedRef(1, 3, ".done"), closedRef(2, 4, ".done"), closedRef(4, 5, ".done")}
	refDropped := []*fixtureTicket{closedRef(5, 6, ".dropped"), closedRef(6, 7, ".dropped"), closedRef(7, 8, ".dropped")}

	// The epic and its ancestry.
	o := byTopic[0]
	epic.parent = parent.stem
	parent.parent = grand.stem
	layout.EpicAncestors = []string{parent.stem, grand.stem}
	for _, child := range o[:11] {
		child.parent = epic.stem
		layout.EpicChildren = append(layout.EpicChildren, child.stem)
	}
	for _, rel := range []*fixtureTicket{o[11], o[12], o[13], o[14], refDone[0], refDone[1]} {
		epic.related = append(epic.related, [2]string{rel.stem, "shares the harvest canopy"})
		layout.EpicRelated = append(layout.EpicRelated, rel.stem)
	}
	for _, dep := range []*fixtureTicket{o[18], refDone[2]} {
		epic.blockedBy = append(epic.blockedBy, dep.stem)
		layout.EpicBlockedBy = append(layout.EpicBlockedBy, dep.stem)
	}
	for _, blocked := range o[15:18] {
		blocked.blockedBy = append(blocked.blockedBy, epic.stem)
		layout.EpicBlocks = append(layout.EpicBlocks, blocked.stem)
	}

	billing, garden, optics, kitchen := byTopic[1], byTopic[2], byTopic[3], byTopic[4]
	railway, weather, archive := byTopic[5], byTopic[6], byTopic[7]

	billing[0].parent, billing[1].parent = billing[1].stem, billing[0].stem
	layout.ParentCycle = [2]string{billing[0].stem, billing[1].stem}
	billing[2].blockedBy = append(billing[2].blockedBy, TicketMapDanglingA)
	billing[3].related = append(billing[3].related, [2]string{refDone[3].stem, "replaces the voucher journal"})

	garden[0].blockedBy = append(garden[0].blockedBy, garden[1].stem)
	garden[1].blockedBy = append(garden[1].blockedBy, garden[2].stem)
	garden[2].blockedBy = append(garden[2].blockedBy, garden[0].stem)
	layout.BlockedByCycle = [3]string{garden[0].stem, garden[1].stem, garden[2].stem}
	garden[3].parent = refDone[4].stem

	optics[0].parent = optics[0].stem
	layout.SelfRef = optics[0].stem
	optics[1].related = append(optics[1].related, [2]string{TicketMapDanglingB, "superseded eyepiece draft"})
	layout.Dangling = []string{TicketMapDanglingA, TicketMapDanglingB}

	kitchen[1].blockedBy = append(kitchen[1].blockedBy, kitchen[0].stem+"#phase2")
	layout.Phase2From, layout.Phase2To = kitchen[1].stem, kitchen[0].stem
	kitchen[2].related = append(kitchen[2].related, [2]string{refDone[5].stem, "the pantry recipe it grew from"})

	for _, pair := range [][]*fixtureTicket{railway, weather, archive} {
		from, to := pair[0], pair[1]
		from.body = strings.Replace(from.body, "\n## Decisions\n", "\nThe same "+from.mention+" is in "+to.stem+" too.\n\n## Decisions\n", 1)
		layout.MentionPairs = append(layout.MentionPairs, [2]string{from.stem, to.stem})
	}
	railway[2].related = append(railway[2].related, [2]string{refDropped[0].stem, "the abandoned depot timetable"})
	weather[3].blockedBy = append(weather[3].blockedBy, refDropped[1].stem)
	archive[3].related = append(archive[3].related, [2]string{refDropped[2].stem, "earlier catalog accession pass"})

	weather[2].broken = true
	layout.BrokenFrontmatter = weather[2].stem
	archive[2].title = ""
	archive[2].body = archive[2].body[strings.Index(archive[2].body, "## Background"):]
	layout.NoTitle = archive[2].stem

	for _, iso := range ticketMapIsolated {
		tk := isolatedTicket(iso.stem, iso.words)
		open = append(open, tk)
		layout.Isolated = append(layout.Isolated, tk.stem)
	}

	// Statuses: a fixed rotation over creation order; the epic and its
	// ancestors are todo, the isolated tickets idea.
	for k, tk := range open {
		switch r := k % 17; {
		case r < 2:
			tk.dir = "ready"
		case r < 9:
			tk.dir = "todo"
		default:
			tk.dir = "idea"
		}
	}
	epic.dir, parent.dir, grand.dir = "todo", "todo", "todo"
	for _, tk := range open[len(open)-len(ticketMapIsolated):] {
		tk.dir = "idea"
	}

	// Unreferenced closed tickets: never listed by a map build.
	for k := 0; k < 40; k++ {
		ti := k % 8
		tp := ticketMapTopics[ti]
		dir := ".done"
		if k%4 == 3 {
			dir = ".dropped"
		}
		stem := fmt.Sprintf("2601%02d-chore-%s-retired-%02d", k%28+1, tp.name, k)
		tk := topicTicket(tp, 200+k, stem)
		tk.dir = dir
		closed = append(closed, tk)
		if dir == ".done" {
			layout.UnrefDone = append(layout.UnrefDone, stem)
		} else {
			layout.UnrefDropped = append(layout.UnrefDropped, stem)
		}
	}

	all := make([]fixtureTicket, 0, len(open)+len(closed))
	for _, tk := range append(open, closed...) {
		if _, dup := layout.Dirs[tk.stem]; dup {
			return TicketMapLayout{}, nil, fmt.Errorf("ticket map fixture: duplicate stem %s", tk.stem)
		}
		layout.Dirs[tk.stem] = tk.dir
		all = append(all, *tk)
	}
	if len(open) != n {
		return TicketMapLayout{}, nil, fmt.Errorf("ticket map fixture: planned %d open tickets, want %d", len(open), n)
	}
	return layout, all, nil
}

// pick returns the topic word at offset m for ticket j: entry m of a
// deterministic pseudo-random permutation of the vocabulary seeded by the
// topic and j, so offsets are distinct within one ticket. A contiguous window
// over the vocabulary would make two tickets' similarity a function of their
// windows' cyclic distance, giving each topic's neighbor graph a ring that
// clustering cuts into arcs.
func pick(tp topicSpec, j, m int) string {
	var perm [16]int
	n := len(tp.words)
	for i := 0; i < n; i++ {
		perm[i] = i
	}
	seed := uint64(14695981039346656037)
	for i := 0; i < len(tp.name); i++ {
		seed = (seed ^ uint64(tp.name[i])) * 1099511628211
	}
	seed ^= uint64(j+1) * 0x9E3779B97F4A7C15
	for i := n - 1; i > 0; i-- {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		k := int(seed % uint64(i+1))
		perm[i], perm[k] = perm[k], perm[i]
	}
	return tp.words[perm[m]]
}

func capitalize(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

func titleOf(tp topicSpec, j int) string {
	w := func(m int) string { return pick(tp, j, m) }
	switch tp.script {
	case TicketMapScriptHangul:
		return w(0) + " " + w(1) + " " + w(2) + " 정리"
	case TicketMapScriptJapanese:
		return w(0) + "と" + w(1) + "の" + w(2)
	}
	return capitalize(w(0)) + " " + w(1) + " for the " + w(2)
}

// topicTicket renders one topic ticket's title and body; the caller sets its
// directory and relations.
func topicTicket(tp topicSpec, j int, stem string) *fixtureTicket {
	title := titleOf(tp, j)
	phases := 1 + (j+1)%3
	var body string
	switch tp.script {
	case TicketMapScriptHangul:
		body = hangulBody(tp, j, title, phases)
	case TicketMapScriptJapanese:
		body = japaneseBody(tp, j, title, phases)
	default:
		body = latinBody(tp, j, title, phases)
	}
	return &fixtureTicket{stem: stem, title: title, body: body, mention: pick(tp, j, 2) + " " + pick(tp, j, 3)}
}

func latinBody(tp topicSpec, j int, title string, phases int) string {
	w := func(m int) string { return pick(tp, j, m) }
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Background\n\n", title)
	fmt.Fprintf(&b, "The %s %s widget in `%s` loses the %s when the %s %s is late.\n", w(0), w(1), tp.path, w(2), w(3), w(4))
	fmt.Fprintf(&b, "Each %s keeps a %s list, and the %s %s reads it twice.\n\n", w(5), w(0), w(6), w(7))
	b.WriteString("## Decisions\n\n")
	fmt.Fprintf(&b, "- Move the %s %s into `%s` so the %s %s owns it.\n", w(1), w(2), tp.path, w(4), w(5))
	fmt.Fprintf(&b, "- The %s widget keeps the %s %s order stable.\n\n", w(6), w(7), w(0))
	b.WriteString("## Phases\n")
	for p := 1; p <= phases; p++ {
		a, c := w(p), w(p+3)
		fmt.Fprintf(&b, "\n### Phase %d: %s %s\n\n", p, capitalize(a), c)
		fmt.Fprintf(&b, "Wire the %s %s through `%s` and check the %s.\n", a, c, tp.path, w(p+5))
		if (j+p)%2 == 0 {
			fmt.Fprintf(&b, "\n### Result\n\nThe %s %s landed with the %s intact.\n", a, w(p+4), c)
		}
	}
	return b.String()
}

// koFinal reports whether word's last Hangul syllable has a final consonant.
func koFinal(word string) bool {
	r := []rune(word)
	last := r[len(r)-1]
	return last >= 0xAC00 && last <= 0xD7A3 && (last-0xAC00)%28 != 0
}

// ko attaches the allomorph of a particle pair: consonant form, vowel form.
func ko(word, afterConsonant, afterVowel string) string {
	if koFinal(word) {
		return word + afterConsonant
	}
	return word + afterVowel
}

func hangulBody(tp topicSpec, j int, title string, phases int) string {
	w := func(m int) string { return pick(tp, j, m) }
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Background\n\n", title)
	fmt.Fprintf(&b, "%s `%s`에서 %s 놓친다. widget 목록이 %s %s 함께 읽는다.\n", ko(w(0), "은", "는"), tp.path, ko(w(1), "을", "를"), ko(w(2), "과", "와"), ko(w(3), "을", "를"))
	fmt.Fprintf(&b, "%s 늦으면 %s %s 다시 계산한다.\n\n", ko(w(4), "이", "가"), w(5), ko(w(6), "을", "를"))
	b.WriteString("## Decisions\n\n")
	fmt.Fprintf(&b, "- %s `%s`로 옮겨 %s %s 소유한다.\n", ko(w(1), "을", "를"), tp.path, ko(w(4), "이", "가"), ko(w(7), "을", "를"))
	fmt.Fprintf(&b, "- widget %s %s 순서를 유지한다.\n\n", w(6), w(0))
	b.WriteString("## Phases\n")
	for p := 1; p <= phases; p++ {
		a, c := w(p), w(p+3)
		fmt.Fprintf(&b, "\n### Phase %d: %s %s\n\n", p, a, c)
		fmt.Fprintf(&b, "%s `%s`에 연결하고 %s 점검한다.\n", ko(a, "을", "를"), tp.path, ko(w(p+5), "을", "를"))
		if (j+p)%2 == 0 {
			fmt.Fprintf(&b, "\n### Result\n\n%s %s 반영했다.\n", w(p+4), ko(c, "을", "를"))
		}
	}
	return b.String()
}

func japaneseBody(tp topicSpec, j int, title string, phases int) string {
	w := func(m int) string { return pick(tp, j, m) }
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Background\n\n", title)
	fmt.Fprintf(&b, "%sは `%s` で%sを失う。widget の一覧が%sと%sを二度読む。\n", w(0), tp.path, w(1), w(2), w(3))
	fmt.Fprintf(&b, "%sが遅れると%sの%sを再計算する。\n\n", w(4), w(5), w(6))
	b.WriteString("## Decisions\n\n")
	fmt.Fprintf(&b, "- %sを `%s` へ移し、%sが%sを持つ。\n", w(1), tp.path, w(4), w(7))
	fmt.Fprintf(&b, "- widget の%sは%sの順序を保つ。\n\n", w(6), w(0))
	b.WriteString("## Phases\n")
	for p := 1; p <= phases; p++ {
		a, c := w(p), w(p+3)
		fmt.Fprintf(&b, "\n### Phase %d: %sと%s\n\n", p, a, c)
		fmt.Fprintf(&b, "%sを `%s` に繋ぎ、%sを確かめる。\n", a, tp.path, w(p+5))
		if (j+p)%2 == 0 {
			fmt.Fprintf(&b, "\n### Result\n\n%sと%sを反映した。\n", w(p+4), c)
		}
	}
	return b.String()
}

func isolatedTicket(stem string, words [5]string) *fixtureTicket {
	title := capitalize(words[0]) + " " + words[1]
	body := fmt.Sprintf("# %s\n\n## Background\n\nThe %s %s widget %s by the %s %s.\n",
		title, words[0], words[1], words[4], words[2], words[3])
	return &fixtureTicket{stem: stem, title: title, body: body}
}
