package wsdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
)

// The boards below are invented and project-neutral. Tests in this file and
// tickets_map_touch_test.go reset package state and swap package seams, so
// none of them may run in parallel.

// resetTicketMapState drops every root's memo and every cached ticket parse
// and document, so the next build is cold.
func resetTicketMapState(t testing.TB) {
	t.Helper()
	ticketMapState.mu.Lock()
	ticketMapState.roots = map[string]*ticketMapRoot{}
	ticketMapState.mu.Unlock()
	clearTicketMapCaches()
}

// clearTicketMapCaches drops the cached parses and documents but keeps memos.
func clearTicketMapCaches() {
	ticketCache.Lock()
	ticketCache.dirs = map[string]map[string]ticketCacheEntry{}
	ticketCache.Unlock()
	ticketRefCache.Lock()
	ticketRefCache.files = map[string]ticketCacheEntry{}
	ticketRefCache.Unlock()
}

// writeMapBoard writes stem -> text tickets under the status directories the
// keys name ("todo/<stem>"), backdated past the racy window.
func writeMapBoard(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for _, status := range []string{"ready", "todo", "idea"} {
		if err := os.MkdirAll(filepath.Join(root, TicketsDir, status), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for key, text := range files {
		writeOldTicket(t, root, TicketsDir+"/"+key+".md", text, time.Hour)
	}
}

func mapTicket(title string, fm ...string) string {
	head := "---\n"
	if title != "" {
		head += "title: " + title + "\n"
	}
	for _, line := range fm {
		head += line + "\n"
	}
	return head + "---\n\n# " + title + "\n\n## Background\n\nThe sample widget.\n"
}

func buildTicketMap(t testing.TB, root string) *TicketMapResult {
	t.Helper()
	result, err := TicketMap(root)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func buildTicketMapJSON(t testing.TB, root string) []byte {
	t.Helper()
	raw, err := TicketMapJSON(root)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mapNodeIndex(t *testing.T, r *TicketMapResult, stem string) int {
	t.Helper()
	for i, node := range r.Nodes {
		if node.Stem == stem {
			return i
		}
	}
	t.Fatalf("no node %s in %v", stem, mapStems(r))
	return -1
}

func mapStems(r *TicketMapResult) []string {
	out := make([]string, len(r.Nodes))
	for i, node := range r.Nodes {
		out[i] = node.Stem
	}
	return out
}

func mapEdgeRows(r *TicketMapResult) [][5]any {
	out := make([][5]any, len(r.Edges))
	for i, e := range r.Edges {
		out[i] = [5]any{e.From, e.To, e.Rel, e.Note, e.Fragment}
	}
	return out
}

func TestTicketMapEmptyBoard(t *testing.T) {
	resetTicketMapState(t)
	for name, dirs := range map[string][]string{
		"status dirs": {"ready", "todo", "idea", ".done", ".dropped"},
		"bare board":  {""},
	} {
		root := t.TempDir()
		for _, d := range dirs {
			if err := os.MkdirAll(filepath.Join(root, TicketsDir, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		raw := buildTicketMapJSON(t, root)
		for _, want := range []string{
			`"open":0,`, `"ready":0,`, `"todo":0,`, `"idea":0,`, `"refs":0,`, `"dangling":0,`,
			`"dfCutTop":[]`, `"clusters":[]`, `"clusterKeywords":{}`, `"nodes":[]`, `"edges":[]`, `"sim":[]`,
			`"v":1,`,
		} {
			if !bytes.Contains(raw, []byte(want)) {
				t.Errorf("%s: payload lacks %s: %s", name, want, raw)
			}
		}
		if bytes.Contains(raw, []byte("null")) || bytes.HasSuffix(raw, []byte("\n")) {
			t.Errorf("%s: payload has null or a trailing newline: %s", name, raw)
		}
		if empty, err := EmptyTicketMapJSON(); err != nil || !bytes.Equal(empty, raw) {
			t.Errorf("%s: EmptyTicketMapJSON = %s, %v; want the built payload %s", name, empty, err, raw)
		}
	}
}

func TestTicketMapMissingBoard(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	if _, err := TicketMap(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("TicketMap on a missing board: %v, want ErrNotExist", err)
	}
	if _, err := TicketMapJSON(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("TicketMapJSON on a missing board: %v, want ErrNotExist", err)
	}
}

func TestTicketMapListingErrorIsReturned(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{"todo/260101-feat-alpha": mapTicket("Alpha")})
	boom := errors.New("volume unreadable")
	orig := readTicketDir
	readTicketDir = func(dir string) ([]os.DirEntry, error) {
		if filepath.Base(dir) == "todo" {
			return nil, boom
		}
		return orig(dir)
	}
	t.Cleanup(func() { readTicketDir = orig })
	if _, err := TicketMapJSON(root); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the listing error", err)
	}
}

func TestTicketMapSingleTicket(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{"ready/260101-feat-alpha": mapTicket("Alpha")})
	r := buildTicketMap(t, root)
	if r.Meta.Open != 1 || r.Meta.Ready != 1 || r.Meta.Refs != 0 || len(r.Nodes) != 1 || len(r.Edges) != 0 {
		t.Fatalf("meta %+v nodes %v edges %v", r.Meta, mapStems(r), r.Edges)
	}
	n := r.Nodes[0]
	if n.I != 0 || n.Stem != "260101-feat-alpha" || n.Path != TicketsDir+"/ready/260101-feat-alpha.md" ||
		n.Title != "Alpha" || n.Status != "ready" || n.Cat != "feat" || n.Missing != nil || n.Assignee == nil {
		t.Fatalf("node %+v", n)
	}
	if r.Meta.V != 1 || len(r.Meta.Fingerprint) != 16 {
		t.Fatalf("meta v/fingerprint %+v", r.Meta)
	}
}

func TestTicketMapSevenTicketsShareOneCluster(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	files := map[string]string{}
	dirs := []string{"ready", "todo", "todo", "idea", "idea", "idea", "ready"}
	var stems []string
	for i, dir := range dirs {
		stem := "26010" + string(rune('1'+i)) + "-feat-lantern-relay"
		stems = append(stems, stem)
		var fm []string
		if i > 0 {
			fm = append(fm, "related:", "  "+stems[i-1]+": shares the lantern relay")
		}
		files[dir+"/"+stem] = mapTicket("Lantern relay beacon", fm...)
	}
	writeMapBoard(t, root, files)
	r := buildTicketMap(t, root)
	if r.Meta.Open != 7 || r.Meta.Ready != 2 || r.Meta.Todo != 2 || r.Meta.Idea != 3 {
		t.Fatalf("meta %+v", r.Meta)
	}
	for _, n := range r.Nodes {
		if n.Cluster != r.Nodes[0].Cluster {
			t.Fatalf("7 related tickets split across clusters: %+v", r.Nodes)
		}
	}
	if len(r.Edges) != 6 {
		t.Fatalf("edges %v", mapEdgeRows(r))
	}
}

func TestTicketMapCyclesAreFlatEdges(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const a, b = "260101-feat-cycle-a", "260102-feat-cycle-b"
	const x, y, z = "260103-bug-ring-x", "260104-bug-ring-y", "260105-bug-ring-z"
	writeMapBoard(t, root, map[string]string{
		"todo/" + a: mapTicket("A", "parent: "+b),
		"todo/" + b: mapTicket("B", "parent: "+a),
		"idea/" + x: mapTicket("X", "blocked-by:", "  - "+y),
		"idea/" + y: mapTicket("Y", "blocked-by:", "  - "+z),
		"idea/" + z: mapTicket("Z", "blocked-by:", "  - "+x),
	})
	r := buildTicketMap(t, root)
	ia, ib := mapNodeIndex(t, r, a), mapNodeIndex(t, r, b)
	ix, iy, iz := mapNodeIndex(t, r, x), mapNodeIndex(t, r, y), mapNodeIndex(t, r, z)
	want := [][5]any{
		{ia, ib, "parent", "", ""},
		{ib, ia, "parent", "", ""},
		{ix, iy, "blocked-by", "", ""},
		{iy, iz, "blocked-by", "", ""},
		{iz, ix, "blocked-by", "", ""},
	}
	if got := mapEdgeRows(r); !slices.Equal(got, want) {
		t.Fatalf("edges %v, want %v", got, want)
	}
}

func TestTicketMapSelfReferenceDropped(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const s = "260101-feat-mirror"
	writeMapBoard(t, root, map[string]string{
		"todo/" + s: mapTicket("Mirror", "parent: "+s, "related:", "  "+s+": itself", "blocked-by:", "  - "+s+"#phase1"),
	})
	r := buildTicketMap(t, root)
	if len(r.Edges) != 0 || len(r.Nodes) != 1 || r.Meta.Dangling != 0 || r.Nodes[0].Missing != nil {
		t.Fatalf("self reference leaked: edges %v nodes %v meta %+v", r.Edges, mapStems(r), r.Meta)
	}
}

func TestTicketMapDanglingRefs(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const ghostA, ghostB = "260199-feat-gone", "260198-bug-vanished"
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-holder": mapTicket("Holder", "parent: "+ghostA, "related:", "  "+ghostB+": lost", "blocked-by:", "  - "+ghostA+"#phase2"),
		"idea/260102-feat-other":  mapTicket("Other", "related:", "  "+ghostA+": also lost", "  not-a-stem: ignored"),
	})
	r := buildTicketMap(t, root)
	if r.Meta.Dangling != 2 || r.Meta.Refs != 0 || len(r.Nodes) != 2 || len(r.Edges) != 0 {
		t.Fatalf("meta %+v nodes %v edges %v", r.Meta, mapStems(r), r.Edges)
	}
	holder := r.Nodes[mapNodeIndex(t, r, "260101-feat-holder")]
	other := r.Nodes[mapNodeIndex(t, r, "260102-feat-other")]
	if !slices.Equal(holder.Missing, []string{ghostB, ghostA}) || !slices.Equal(other.Missing, []string{ghostA}) {
		t.Fatalf("missing: holder %v other %v", holder.Missing, other.Missing)
	}
}

func TestTicketMapBrokenFrontmatterTitleFallback(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-unfenced":   "---\ntitle: Never Parsed\nparent: 260102-feat-untitled\n\n# Heading Title\n\nBody widget.\n",
		"todo/260102-feat-untitled":   "---\ntitle: Also Unparsed\n\n## Background\n\nNo heading here.\n",
		"todo/260103-feat-no-heading": "---\nassignee: []\n---\n\n## Background\n\nPlain.\n",
	})
	r := buildTicketMap(t, root)
	want := map[string]string{
		"260101-feat-unfenced":   "Heading Title",
		"260102-feat-untitled":   "260102-feat-untitled",
		"260103-feat-no-heading": "260103-feat-no-heading",
	}
	for stem, title := range want {
		if got := r.Nodes[mapNodeIndex(t, r, stem)].Title; got != title {
			t.Errorf("%s title %q, want %q", stem, got, title)
		}
	}
	if len(r.Edges) != 0 {
		t.Fatalf("unfenced frontmatter produced edges %v", r.Edges)
	}
}

func TestTicketMapPhaseFragmentKept(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-consumer": mapTicket("Consumer", "blocked-by:", "  - 260102-feat-producer#phase2", "  - 260103-feat-closed#Phase3"),
		"todo/260102-feat-producer": mapTicket("Producer"),
	})
	writeOldTicket(t, root, TicketsDir+"/.done/260103-feat-closed.md", mapTicket("Closed"), time.Hour)
	r := buildTicketMap(t, root)
	want := [][5]any{
		{0, 1, "blocked-by", "", "#phase2"},
		{0, 2, "blocked-by", "", "#phase3"},
	}
	if got := mapEdgeRows(r); !slices.Equal(got, want) {
		t.Fatalf("edges %v, want %v", got, want)
	}
	raw := buildTicketMapJSON(t, root)
	if !bytes.Contains(raw, []byte(`[0,1,"blocked-by","","#phase2"]`)) {
		t.Fatalf("edge row not encoded as an array: %s", raw)
	}
}

func TestTicketMapDuplicateStemOpenWins(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const dup, dup2 = "260101-feat-twice", "260102-feat-thrice"
	writeMapBoard(t, root, map[string]string{
		"todo/" + dup:              mapTicket("Open copy"),
		"ready/" + dup2:            mapTicket("Ready copy"),
		"todo/" + dup2:             mapTicket("Todo copy"),
		"idea/260103-feat-pointer": mapTicket("Pointer", "related:", "  "+dup+": points at it", "  "+dup2+": and at it"),
	})
	writeOldTicket(t, root, TicketsDir+"/.done/"+dup+".md", mapTicket("Closed copy"), time.Hour)
	var lstats []string
	orig := lstatTicketFile
	lstatTicketFile = func(path string) (fs.FileInfo, error) {
		lstats = append(lstats, path)
		return orig(path)
	}
	t.Cleanup(func() { lstatTicketFile = orig })

	r := buildTicketMap(t, root)
	if len(lstats) != 0 {
		t.Fatalf("open stems were looked up as closed refs: %v", lstats)
	}
	if r.Meta.Refs != 0 || r.Meta.Open != 3 || len(r.Nodes) != 3 {
		t.Fatalf("meta %+v nodes %v", r.Meta, mapStems(r))
	}
	if n := r.Nodes[mapNodeIndex(t, r, dup)]; n.Status != "todo" || n.Title != "Open copy" {
		t.Fatalf("todo/.done duplicate: %+v", n)
	}
	if n := r.Nodes[mapNodeIndex(t, r, dup2)]; n.Status != "ready" || n.Title != "Ready copy" || n.Path != TicketsDir+"/ready/"+dup2+".md" {
		t.Fatalf("ready/todo duplicate: %+v", n)
	}
	if r.Meta.Ready != 1 || r.Meta.Todo != 1 || r.Meta.Idea != 1 {
		t.Fatalf("duplicate counted twice: %+v", r.Meta)
	}
}

func TestTicketMapNodeOrderCatAndPhases(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	phased := "---\ntitle: Phased\nrelated:\n  260110-feat-ref-b: b\n  260109-spike-ref-a: a\n---\n\n## Phases\n\n" +
		"### Phase 1: One\n\n### Result\n\nok\n\n### Phase 2: Two\n\n### Phase 3: Three\n\n### Result\n\nok\n"
	writeMapBoard(t, root, map[string]string{
		"idea/260101-chore-idea-early": mapTicket("Idea early"),
		"idea/260108-x9-odd-category":  mapTicket("Odd"),
		"todo/260105-epic-todo-late":   phased,
		"todo/260102-bug-todo-early":   mapTicket("Todo early"),
		"ready/260107-refactor-ready":  mapTicket("Ready late"),
		"ready/260103-research-ready":  mapTicket("Ready early"),
	})
	writeOldTicket(t, root, TicketsDir+"/.dropped/260109-spike-ref-a.md", mapTicket("Ref A"), time.Hour)
	writeOldTicket(t, root, TicketsDir+"/.done/260110-feat-ref-b.md", mapTicket("Ref B"), time.Hour)
	r := buildTicketMap(t, root)
	wantStems := []string{
		"260103-research-ready", "260107-refactor-ready",
		"260102-bug-todo-early", "260105-epic-todo-late",
		"260101-chore-idea-early", "260108-x9-odd-category",
		"260109-spike-ref-a", "260110-feat-ref-b",
	}
	if got := mapStems(r); !slices.Equal(got, wantStems) {
		t.Fatalf("node order %v, want %v", got, wantStems)
	}
	wantCat := []string{"research", "refactor", "bug", "epic", "chore", "other", "spike", "feat"}
	wantStatus := []string{"ready", "ready", "todo", "todo", "idea", "idea", "dropped", "done"}
	for i, n := range r.Nodes {
		if n.I != i || n.Cat != wantCat[i] || n.Status != wantStatus[i] {
			t.Errorf("node %d: i=%d cat=%q status=%q, want cat %q status %q", i, n.I, n.Cat, n.Status, wantCat[i], wantStatus[i])
		}
	}
	if p := r.Nodes[3].Phases; p != [2]int{2, 3} {
		t.Errorf("phases %v, want [2 3]", p)
	}
	if p := r.Nodes[0].Phases; p != [2]int{0, 0} {
		t.Errorf("phaseless ticket phases %v", p)
	}
	if r.Meta.Refs != 2 || r.Meta.RefDone != 1 || r.Meta.RefDropped != 1 || r.Meta.Open != 6 {
		t.Errorf("meta %+v", r.Meta)
	}
	if !strings.Contains(string(buildTicketMapJSON(t, root)), `"phases":[2,3]`) {
		t.Errorf("phases not encoded as a pair")
	}
}

func TestTicketMapRefPrefersDoneOverDropped(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const ref = "260102-feat-closed-twice"
	writeMapBoard(t, root, map[string]string{"todo/260101-feat-holder": mapTicket("Holder", "parent: "+ref)})
	writeOldTicket(t, root, TicketsDir+"/.done/"+ref+".md", mapTicket("Done copy"), time.Hour)
	writeOldTicket(t, root, TicketsDir+"/.dropped/"+ref+".md", mapTicket("Dropped copy"), time.Hour)
	r := buildTicketMap(t, root)
	if len(r.Nodes) != 2 || r.Nodes[1].Status != "done" || r.Nodes[1].Title != "Done copy" || r.Meta.RefDropped != 0 {
		t.Fatalf("nodes %+v meta %+v", r.Nodes, r.Meta)
	}
}

func TestTicketMapEdgeRowsSortedAndDeduplicated(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	const a, b, c = "260101-feat-a", "260102-feat-b", "260103-feat-c"
	writeMapBoard(t, root, map[string]string{
		"todo/" + c: mapTicket("C", "parent: "+a, "related:", "  "+b+": sibling", "  "+a+": same parent",
			"blocked-by:", "  - "+b, "  - "+b+"#phase2", "  - "+a+"#phase1"),
		"todo/" + a: mapTicket("A", "related:", "  "+c+": back"),
		"todo/" + b: mapTicket("B"),
	})
	r := buildTicketMap(t, root)
	want := [][5]any{
		{0, 2, "related", "back", ""},
		{2, 0, "blocked-by", "", "#phase1"},
		{2, 0, "parent", "", ""},
		{2, 0, "related", "same parent", ""},
		{2, 1, "blocked-by", "", ""},
		{2, 1, "related", "sibling", ""},
	}
	if got := mapEdgeRows(r); !slices.Equal(got, want) {
		t.Fatalf("edges\n%v\nwant\n%v", got, want)
	}
}

func TestTicketMapJSONShape(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-a": mapTicket("A <&> \"quoted\"", "related:", "  260102-feat-b: note"),
		"todo/260102-feat-b": mapTicket("B"),
	})
	raw := buildTicketMapJSON(t, root)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("payload is not JSON: %v\n%s", err, raw)
	}
	for _, key := range []string{"meta", "clusters", "clusterKeywords", "nodes", "edges", "sim"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("payload lacks %q", key)
		}
	}
	edges := doc["edges"].([]any)
	if row, ok := edges[0].([]any); !ok || len(row) != 5 {
		t.Fatalf("edge row %v is not a 5-array", edges[0])
	}
	if !bytes.Contains(raw, []byte(`"title":"A <&> \"quoted\""`)) {
		t.Errorf("HTML escaping applied or title lost: %s", raw)
	}
	params := doc["meta"].(map[string]any)["params"].(map[string]any)
	for _, key := range []string{"wTitle", "wCode", "wProse", "dfCutoff", "topK", "simNeighbor", "simGraph", "minComm", "maxComm", "targetK"} {
		if _, ok := params[key]; !ok {
			t.Errorf("meta.params lacks %q", key)
		}
	}

	sim := TicketMapSim{I: 1, J: 4, Score: MapFloat(roundTo(0.41249, 3)), Mention: true,
		Shared: []TicketMapTerm{{Term: "orbit", Code: false}, {Term: "orbit/path.go", Code: true}}}
	got, err := json.Marshal([]TicketMapSim{sim, {I: 0, J: 2, Score: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `[[1,4,0.412,[["orbit",0],["orbit/path.go",1]],1],[0,2,1,[],0]]`; string(got) != want {
		t.Errorf("sim rows %s, want %s", got, want)
	}
	for x, want := range map[float64]string{
		roundTo(0.1+0.2, 3): "0.3", 1: "1", 0: "0", roundTo(1.7000000001, 1): "1.7",
		math.NaN(): "0", math.Inf(1): "0", -0.25: "-0.25", 1e-7: "0.0000001",
	} {
		if got, _ := MapFloat(x).MarshalJSON(); string(got) != want {
			t.Errorf("MapFloat(%v) = %s, want %s", x, got, want)
		}
	}
}

// copyBoardReversed copies src's files to dst creating directories and files
// in reverse lexical order, with each file's mtime copied.
func copyBoardReversed(t *testing.T, src, dst string) {
	t.Helper()
	var dirs, files []string
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			dirs = append(dirs, rel)
		} else {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(dirs)
	slices.Reverse(files)
	for _, rel := range dirs {
		if err := os.MkdirAll(filepath.Join(dst, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, rel), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dst, rel), info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTicketMapDeterminism(t *testing.T) {
	root := t.TempDir()
	writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)

	resetTicketMapState(t)
	first := bytes.Clone(buildTicketMapJSON(t, root))
	resetTicketMapState(t)
	if second := buildTicketMapJSON(t, root); !bytes.Equal(first, second) {
		t.Fatal("two cold builds differ")
	}

	prev := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
	resetTicketMapState(t)
	one := bytes.Clone(buildTicketMapJSON(t, root))
	runtime.GOMAXPROCS(8)
	resetTicketMapState(t)
	if eight := buildTicketMapJSON(t, root); !bytes.Equal(one, eight) || !bytes.Equal(one, first) {
		t.Fatal("GOMAXPROCS 1 and 8 builds differ")
	}
	runtime.GOMAXPROCS(prev)

	reversed := t.TempDir()
	copyBoardReversed(t, root, reversed)
	resetTicketMapState(t)
	if got := buildTicketMapJSON(t, reversed); !bytes.Equal(first, got) {
		t.Fatal("a reversed-creation-order copy builds different JSON")
	}
}

func TestTicketMapConcurrentCallsShareOneResult(t *testing.T) {
	root := t.TempDir()
	writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	resetTicketMapState(t)
	const workers = 16
	out := make([][]byte, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				out[i], errs[i] = TicketMapJSON(root)
				return
			}
			var r *TicketMapResult
			if r, errs[i] = TicketMap(root); errs[i] == nil {
				out[i], errs[i] = encodeTicketMap(r)
			}
		}()
	}
	wg.Wait()
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if !bytes.Equal(out[i], out[0]) {
			t.Fatalf("worker %d got different bytes", i)
		}
	}
}

func TestTicketMapRacyRewriteIsNotMemoized(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{"todo/260101-feat-stable": mapTicket("Stable")})
	mustWrite(t, root, TicketsDir+"/todo/260102-feat-fresh.md", mapTicket("Fresh"))
	reads := countTicketReads(t, root)

	memoized := func() bool {
		ticketMapState.mu.Lock()
		defer ticketMapState.mu.Unlock()
		state := ticketMapState.roots[root]
		return state != nil && state.result != nil
	}
	buildTicketMap(t, root)
	if n := reads.Load(); n != 2 || memoized() {
		t.Fatalf("cold racy build: %d reads, memoized %v", n, memoized())
	}
	reads.Store(0)
	buildTicketMap(t, root)
	if n := reads.Load(); n != 1 || memoized() {
		t.Fatalf("second racy build: %d reads (want the fresh ticket reread), memoized %v", n, memoized())
	}

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, TicketsDir, "todo", "260102-feat-fresh.md"), old, old); err != nil {
		t.Fatal(err)
	}
	reads.Store(0)
	buildTicketMap(t, root)
	if n := reads.Load(); n != 1 || !memoized() {
		t.Fatalf("settled build: %d reads, memoized %v", n, memoized())
	}
	reads.Store(0)
	buildTicketMap(t, root)
	if n := reads.Load(); n != 0 {
		t.Fatalf("warm build: %d reads", n)
	}
}

func TestTicketMapPayloadSize(t *testing.T) {
	root := t.TempDir()
	writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	resetTicketMapState(t)
	raw := buildTicketMapJSON(t, root)
	r := buildTicketMap(t, root)
	t.Logf("n=%d payload: %d bytes (%d nodes, %d edges, %d sim rows)",
		wsdoctest.TicketMapDefaultOpen, len(raw), len(r.Nodes), len(r.Edges), len(r.Sim))
	if len(raw) >= 600_000 {
		t.Fatalf("payload %d bytes, budget 600 KB", len(raw))
	}
}

// TestTicketMapSourcesNameNoRepositoryTickets keeps the shipped ticket map
// sources free of this repository's ticket stems and board paths. Test-only
// fixture code (*_test.go, wsdoctest/) is out of scope.
func TestTicketMapSourcesNameNoRepositoryTickets(t *testing.T) {
	stemRE := regexp.MustCompile(`\b\d{6}-(feat|bug|refactor|chore|research|epic)-[a-z0-9-]+`)
	pathRE := regexp.MustCompile(`ai-docs/tickets/(ready|todo|idea|\.done|\.dropped)/\d`)
	files := []string{
		"tickets_map.go",
		"tickets_map_text.go",
		"tickets_map_words.go",
		"tickets_map_cluster.go",
		"tickets_map_louvain.go",
		"tickets_map_naming.go",
		"../wsdashboard/ticket_graph.go",
		"../mcp/dashboard_ticket_graph.go",
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if m := stemRE.Find(raw); m != nil {
			t.Errorf("%s names a ticket stem: %s", name, m)
		}
		if m := pathRE.Find(raw); m != nil {
			t.Errorf("%s names a ticket board path: %s", name, m)
		}
	}
}
