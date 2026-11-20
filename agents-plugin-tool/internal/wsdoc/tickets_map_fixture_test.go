package wsdoc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
)

// writeTicketMapFixture writes the synthetic, project-neutral ticket map board
// (wsdoctest.WriteTicketMapFixture) under dir and returns its layout.
func writeTicketMapFixture(t testing.TB, dir string, n int) wsdoctest.TicketMapLayout {
	t.Helper()
	layout, err := wsdoctest.TicketMapFixtureLayout(n)
	if err != nil {
		t.Fatal(err)
	}
	wsdoctest.WriteTicketMapFixture(t, dir, n)
	return layout
}

func TestTicketMapFixtureCounts(t *testing.T) {
	for _, n := range []int{wsdoctest.TicketMapDefaultOpen, 300} {
		dir := t.TempDir()
		layout := writeTicketMapFixture(t, dir, n)
		onDisk := map[string]int{}
		for _, status := range []string{"ready", "todo", "idea", ".done", ".dropped"} {
			entries, err := os.ReadDir(filepath.Join(dir, "ai-docs", "tickets", status))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				stem := strings.TrimSuffix(e.Name(), ".md")
				if !ValidTicketStem(stem) {
					t.Errorf("n=%d: %s/%s is not a ticket stem", n, status, e.Name())
				}
				if layout.Dirs[stem] != status {
					t.Errorf("n=%d: %s found in %s, layout says %q", n, stem, status, layout.Dirs[stem])
				}
				info, err := e.Info()
				if err != nil {
					t.Fatal(err)
				}
				if !info.ModTime().Equal(wsdoctest.TicketMapModTime) {
					t.Errorf("n=%d: %s mtime %v", n, e.Name(), info.ModTime())
				}
			}
			onDisk[status] = len(entries)
			if want := layout.Count(status); len(entries) != want {
				t.Errorf("n=%d: %s holds %d tickets, layout says %d", n, status, len(entries), want)
			}
		}
		if got := onDisk["ready"] + onDisk["todo"] + onDisk["idea"]; got != n {
			t.Errorf("n=%d: %d open tickets", n, got)
		}
		if onDisk[".done"] != 36 || onDisk[".dropped"] != 13 {
			t.Errorf("n=%d: closed = %d done, %d dropped; want 36, 13", n, onDisk[".done"], onDisk[".dropped"])
		}

		latin, topicTotal := 0, 0
		for _, topic := range layout.Topics {
			topicTotal += len(topic.Stems)
			if topic.Script == wsdoctest.TicketMapScriptLatin {
				latin++
				if n == wsdoctest.TicketMapDefaultOpen && (len(topic.Stems) < 12 || len(topic.Stems) > 24) {
					t.Errorf("topic %s has %d tickets, want 12-24", topic.Name, len(topic.Stems))
				}
			}
		}
		if latin != 8 || len(layout.Topics) != 10 {
			t.Errorf("n=%d: %d topics, %d Latin", n, len(layout.Topics), latin)
		}
		if topicTotal+3+len(layout.Isolated) != n {
			t.Errorf("n=%d: topics %d + epic family 3 + isolated %d != %d", n, topicTotal, len(layout.Isolated), n)
		}
		counts := map[string]int{
			"children": len(layout.EpicChildren), "ancestors": len(layout.EpicAncestors),
			"related": len(layout.EpicRelated), "blockedBy": len(layout.EpicBlockedBy),
			"blocks": len(layout.EpicBlocks), "refDone": len(layout.RefDone),
			"refDropped": len(layout.RefDropped), "unrefDone": len(layout.UnrefDone),
			"unrefDropped": len(layout.UnrefDropped), "dangling": len(layout.Dangling),
			"mentions": len(layout.MentionPairs), "isolated": len(layout.Isolated),
		}
		want := map[string]int{
			"children": 11, "ancestors": 2, "related": 6, "blockedBy": 2, "blocks": 3,
			"refDone": 6, "refDropped": 3, "unrefDone": 30, "unrefDropped": 10,
			"dangling": 2, "mentions": 3, "isolated": 5,
		}
		for k, v := range want {
			if counts[k] != v {
				t.Errorf("n=%d: %s = %d, want %d", n, k, counts[k], v)
			}
		}
		for _, stem := range layout.Dangling {
			if _, ok := layout.Dirs[stem]; ok {
				t.Errorf("dangling stem %s was written", stem)
			}
		}

		read := func(stem string) string {
			raw, err := os.ReadFile(filepath.Join(dir, "ai-docs", "tickets", layout.Dirs[stem], stem+".md"))
			if err != nil {
				t.Fatal(err)
			}
			return string(raw)
		}
		for stem := range layout.Dirs {
			if !strings.Contains(read(stem), wsdoctest.TicketMapEveryTicketTerm) {
				t.Errorf("%s lacks %q", stem, wsdoctest.TicketMapEveryTicketTerm)
			}
		}
		for _, pair := range layout.MentionPairs {
			from, to := read(pair[0]), read(pair[1])
			if !strings.Contains(from, pair[1]) || strings.Contains(to, pair[0]) {
				t.Errorf("mention pair %v not body-only one-way", pair)
			}
			if head := from[:strings.Index(from, "\n---\n")]; strings.Contains(head, pair[1]) {
				t.Errorf("mention pair %v is in frontmatter", pair)
			}
		}
		if text := read(layout.BrokenFrontmatter); strings.Count(text, "\n---\n") != 0 {
			t.Errorf("broken frontmatter ticket has a closing fence")
		}
		if text := read(layout.NoTitle); strings.Contains(text, "title:") || strings.Contains(text, "\n# ") {
			t.Errorf("no-title ticket carries a title")
		}
		if text := read(layout.Phase2From); !strings.Contains(text, layout.Phase2To+"#phase2") {
			t.Errorf("phase2 edge missing")
		}
	}
}

func TestTicketMapFixtureIsDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTicketMapFixture(t, a, wsdoctest.TicketMapDefaultOpen)
	writeTicketMapFixture(t, b, wsdoctest.TicketMapDefaultOpen)
	err := filepath.WalkDir(a, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(a, path)
		x, _ := os.ReadFile(path)
		y, err := os.ReadFile(filepath.Join(b, rel))
		if err != nil || string(x) != string(y) {
			t.Errorf("%s differs between two writes", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestTicketMapFixtureSpine checks the load and edge facts the fixture plants,
// independent of clustering.
func TestTicketMapFixtureSpine(t *testing.T) {
	root := t.TempDir()
	layout := writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	resetTicketMapState(t)
	r := buildTicketMap(t, root)
	m := r.Meta
	if m.Open != layout.Open || m.Ready != layout.Count("ready") || m.Todo != layout.Count("todo") || m.Idea != layout.Count("idea") ||
		m.Refs != 9 || m.RefDone != 6 || m.RefDropped != 3 || m.Dangling != 2 {
		t.Fatalf("meta %+v", m)
	}
	if len(r.Nodes) != layout.Open+9 {
		t.Fatalf("%d nodes", len(r.Nodes))
	}
	refs := append(slices.Clone(layout.RefDone), layout.RefDropped...)
	slices.Sort(refs)
	if got := mapStems(r)[layout.Open:]; !slices.Equal(got, refs) {
		t.Fatalf("ref nodes %v, want %v", got, refs)
	}

	idx := func(stem string) int { return mapNodeIndex(t, r, stem) }
	has := func(from, to, rel, frag string) bool {
		for _, e := range r.Edges {
			if e.From == idx(from) && e.To == idx(to) && e.Rel == rel && e.Fragment == frag {
				return true
			}
		}
		return false
	}
	epic := wsdoctest.TicketMapEpicStem
	for _, child := range layout.EpicChildren {
		if !has(child, epic, "parent", "") {
			t.Errorf("child %s lacks its parent edge", child)
		}
	}
	if !has(epic, layout.EpicAncestors[0], "parent", "") || !has(layout.EpicAncestors[0], layout.EpicAncestors[1], "parent", "") {
		t.Errorf("epic ancestry edges missing")
	}
	for _, rel := range layout.EpicRelated {
		if !has(epic, rel, "related", "") {
			t.Errorf("epic related %s missing", rel)
		}
	}
	for _, dep := range layout.EpicBlockedBy {
		if !has(epic, dep, "blocked-by", "") {
			t.Errorf("epic blocked-by %s missing", dep)
		}
	}
	for _, blocked := range layout.EpicBlocks {
		if !has(blocked, epic, "blocked-by", "") {
			t.Errorf("%s not blocked by the epic", blocked)
		}
	}
	a, b := layout.ParentCycle[0], layout.ParentCycle[1]
	if !has(a, b, "parent", "") || !has(b, a, "parent", "") {
		t.Errorf("parent 2-cycle edges missing")
	}
	c := layout.BlockedByCycle
	if !has(c[0], c[1], "blocked-by", "") || !has(c[1], c[2], "blocked-by", "") || !has(c[2], c[0], "blocked-by", "") {
		t.Errorf("blocked-by 3-cycle edges missing")
	}
	if !has(layout.Phase2From, layout.Phase2To, "blocked-by", "#phase2") {
		t.Errorf("#phase2 edge missing")
	}
	for _, e := range r.Edges {
		if e.From == e.To {
			t.Errorf("self edge %+v", e)
		}
	}
	for _, pair := range layout.MentionPairs {
		for _, e := range r.Edges {
			if (e.From == idx(pair[0]) && e.To == idx(pair[1])) || (e.From == idx(pair[1]) && e.To == idx(pair[0])) {
				t.Errorf("mention pair %v has a frontmatter edge", pair)
			}
		}
	}
	var missing []string
	for _, n := range r.Nodes {
		missing = append(missing, n.Missing...)
	}
	slices.Sort(missing)
	if !slices.Equal(missing, []string{wsdoctest.TicketMapDanglingA, wsdoctest.TicketMapDanglingB}) {
		t.Errorf("node.missing across the board: %v", missing)
	}
	if got := r.Nodes[idx(layout.NoTitle)].Title; got != layout.NoTitle {
		t.Errorf("no-title ticket title %q", got)
	}
	if got := r.Nodes[idx(layout.BrokenFrontmatter)].Title; strings.HasPrefix(got, "26") || got == "" {
		t.Errorf("broken-frontmatter title %q, want its H1", got)
	}
	for _, n := range r.Nodes {
		if n.Preview == "" || n.Lines < 2 {
			t.Errorf("%s: empty preview or lines %d", n.Stem, n.Lines)
		}
	}
}
