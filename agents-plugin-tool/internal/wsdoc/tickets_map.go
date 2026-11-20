package wsdoc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ticketMapVersion is the payload version (meta.v) and part of the memo
// fingerprint. Bump it whenever the payload shape or the cluster output for an
// unchanged board changes (stoplists, weights, thresholds).
const ticketMapVersion = 1

// ticketMapRootCap bounds how many project roots keep map documents and a
// memoized result in a long-lived daemon; the least recently built root past
// the cap drops both.
const ticketMapRootCap = 16

// ticketMapOpenStatuses are the status directories a map build lists, in
// ticketStatusRank order. Closed tickets are never listed; they are found by a
// targeted lstat per referenced stem (ticketMapClosedDirs order).
var (
	ticketMapOpenStatuses = []string{"ready", "todo", "idea"}
	ticketMapClosedDirs   = []string{".done", ".dropped"}
)

var ticketMapCatRE = regexp.MustCompile(`^\d{6}-([a-z]+)-`)

// TicketMapResult is the ticket map payload served by the dashboard's
// GET /s/<rid>/tickets/graph. Treat a returned value as read-only: it is the
// memoized result shared with later calls.
type TicketMapResult struct {
	Meta            TicketMapMeta                 `json:"meta"`
	Clusters        []TicketMapCluster            `json:"clusters"`
	ClusterKeywords map[string][]TicketMapKeyword `json:"clusterKeywords"`
	Nodes           []TicketMapNode               `json:"nodes"`
	Edges           []TicketMapEdge               `json:"edges"`
	Sim             []TicketMapSim                `json:"sim"`
}

type TicketMapMeta struct {
	V            int             `json:"v"`
	Fingerprint  string          `json:"fingerprint"`
	Open         int             `json:"open"`
	Ready        int             `json:"ready"`
	Todo         int             `json:"todo"`
	Idea         int             `json:"idea"`
	Refs         int             `json:"refs"`
	RefDone      int             `json:"refDone"`
	RefDropped   int             `json:"refDropped"`
	Dangling     int             `json:"dangling"`
	MentionPairs int             `json:"mentionPairs"`
	Gamma        MapFloat        `json:"gamma"`
	Modularity   MapFloat        `json:"modularity"`
	DFCut        int             `json:"dfCut"`
	DFCutTop     []string        `json:"dfCutTop"`
	Params       TicketMapParams `json:"params"`
}

type TicketMapParams struct {
	WTitle      MapFloat `json:"wTitle"`
	WCode       MapFloat `json:"wCode"`
	WProse      MapFloat `json:"wProse"`
	DFCutoff    MapFloat `json:"dfCutoff"`
	TopK        int      `json:"topK"`
	SimNeighbor MapFloat `json:"simNeighbor"`
	SimGraph    MapFloat `json:"simGraph"`
	MinComm     int      `json:"minComm"`
	MaxComm     int      `json:"maxComm"`
	TargetK     [2]int   `json:"targetK"`
}

type TicketMapCluster struct {
	ID        int             `json:"id"`
	Label     string          `json:"label"`
	NameTerms []string        `json:"nameTerms"`
	Terms     []TicketMapTerm `json:"terms"`
	Hue       int             `json:"hue"`
	Open      int             `json:"open"`
	Ref       int             `json:"ref"`
	Ready     int             `json:"ready"`
	Misc      bool            `json:"misc"`
}

type TicketMapKeyword struct {
	Term      string     `json:"term"`
	Weight    MapFloat   `json:"weight"`
	Code      int        `json:"code"`
	TicketIDs []int      `json:"ticketIds"`
	TW        []MapFloat `json:"tw"`
	Open      int        `json:"open"`
}

type TicketMapNode struct {
	I              int             `json:"i"`
	Stem           string          `json:"stem"`
	Path           string          `json:"path"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Cat            string          `json:"cat"`
	Cluster        int             `json:"cluster"`
	Phases         [2]int          `json:"phases"`
	ODQ            bool            `json:"odq"`
	BlockedHeading bool            `json:"blockedHeading"`
	Lines          int             `json:"lines"`
	Assignee       []string        `json:"assignee"`
	KW             []TicketMapTerm `json:"kw"`
	Preview        string          `json:"preview"`
	Missing        []string        `json:"missing,omitempty"`
}

// TicketMapTerm encodes as ["term", 0|1], the second element the code flag.
type TicketMapTerm struct {
	Term string
	Code bool
}

func (t TicketMapTerm) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{t.Term, boolInt(t.Code)})
}

// TicketMapEdge encodes as [from, to, rel, note, fragment]; From is the ticket
// whose frontmatter declares the relation.
type TicketMapEdge struct {
	From, To            int
	Rel, Note, Fragment string
}

func (e TicketMapEdge) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{e.From, e.To, e.Rel, e.Note, e.Fragment})
}

// TicketMapSim encodes as [i, j, score, sharedTerms, mention 0|1] with I < J.
type TicketMapSim struct {
	I, J    int
	Score   MapFloat
	Shared  []TicketMapTerm
	Mention bool
}

func (s TicketMapSim) MarshalJSON() ([]byte, error) {
	shared := s.Shared
	if shared == nil {
		shared = []TicketMapTerm{}
	}
	return json.Marshal([]any{s.I, s.J, s.Score, shared, boolInt(s.Mention)})
}

// MapFloat is a payload float, already rounded by the builder, encoded with
// strconv.FormatFloat(x, 'f', -1, 64) so the bytes are stable.
type MapFloat float64

func (f MapFloat) MarshalJSON() ([]byte, error) {
	x := float64(f)
	if math.IsNaN(x) || math.IsInf(x, 0) {
		x = 0
	}
	return []byte(strconv.FormatFloat(x, 'f', -1, 64)), nil
}

// roundTo rounds x to the given number of decimals.
func roundTo(x float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(x*p) / p
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// mapNode is one included ticket handed to analyzeTicketMap, in payload node
// order: open tickets first (ready, todo, idea; then stem), then closed refs
// (stem). doc is never nil.
type mapNode struct {
	info TicketInfo
	doc  *ticketDoc
	open bool
}

// mapEdge is one explicit relation between included nodes, by node index,
// declared by from's frontmatter. rel is "parent", "blocked-by" or "related".
type mapEdge struct {
	from, to        int
	rel, note, frag string
}

// ticketRefCache holds parsed closed refs keyed by absolute file path,
// revalidated by the size and mtime of the targeted lstat.
var ticketRefCache = struct {
	sync.Mutex
	files map[string]ticketCacheEntry
}{files: map[string]ticketCacheEntry{}}

// ticketMapState serializes builds and holds each root's memo. Concurrent
// requests during a build wait on mu and then reuse the memo.
var ticketMapState = struct {
	mu    sync.Mutex
	tick  uint64
	roots map[string]*ticketMapRoot
}{roots: map[string]*ticketMapRoot{}}

type ticketMapRoot struct {
	used        uint64
	fingerprint string
	result      *TicketMapResult
	json        []byte
}

// TicketMapJSON returns the encoded ticket map of root's board, memoized by a
// corpus fingerprint. A missing ai-docs/tickets/ returns an error wrapping
// os.ErrNotExist. The returned bytes are shared; do not modify them.
func TicketMapJSON(root string) ([]byte, error) {
	entry, err := ticketMapFor(root)
	if err != nil {
		return nil, err
	}
	return entry.json, nil
}

// EmptyTicketMapJSON returns the payload TicketMapJSON builds for a board with
// no tickets, for callers that serve a missing board as an empty one. The
// bytes are freshly encoded and owned by the caller.
func EmptyTicketMapJSON() ([]byte, error) {
	load := ticketMapLoad{stable: true}
	return encodeTicketMap(assembleTicketMap(load, ticketMapFingerprint(load)))
}

// TicketMap is the typed form of TicketMapJSON. The result is shared; do not
// modify it.
func TicketMap(root string) (*TicketMapResult, error) {
	entry, err := ticketMapFor(root)
	if err != nil {
		return nil, err
	}
	return entry.result, nil
}

// ticketMapLoad is what one load pass of the board produced before analysis.
type ticketMapLoad struct {
	nodes    []mapNode
	files    []ticketMapFile
	dangling []string
	missing  map[string][]string // open stem -> its own dangling stems
	stable   bool
}

type ticketMapFile struct {
	rel     string
	size    int64
	modTime time.Time
}

func ticketMapFor(root string) (*ticketMapRoot, error) {
	ticketMapState.mu.Lock()
	defer ticketMapState.mu.Unlock()

	load, err := loadTicketMap(root)
	if err != nil {
		return nil, err
	}
	fp := ticketMapFingerprint(load)
	ticketMapState.tick++
	state := ticketMapState.roots[root]
	if state != nil && state.result != nil && state.fingerprint == fp {
		state.used = ticketMapState.tick
		return state, nil
	}
	result := assembleTicketMap(load, fp)
	encoded, err := encodeTicketMap(result)
	if err != nil {
		return nil, err
	}
	built := &ticketMapRoot{used: ticketMapState.tick, fingerprint: fp, result: result, json: encoded}
	if load.stable {
		ticketMapState.roots[root] = built
	} else if state != nil {
		// A racy build is served but never memoized; the root stays tracked
		// so its documents remain under the root cap.
		state.used, state.fingerprint, state.result, state.json = ticketMapState.tick, "", nil, nil
	} else {
		ticketMapState.roots[root] = &ticketMapRoot{used: ticketMapState.tick}
	}
	evictTicketMapRoots()
	return built, nil
}

// evictTicketMapRoots drops the least recently built roots past
// ticketMapRootCap: their memo, the documents carried in ticketCache, and
// their closed-ref cache entries. Plain listings keep their parsed TicketInfo.
func evictTicketMapRoots() {
	for len(ticketMapState.roots) > ticketMapRootCap {
		oldest, oldestUsed := "", uint64(math.MaxUint64)
		for root, state := range ticketMapState.roots {
			if state.used < oldestUsed || (state.used == oldestUsed && root < oldest) {
				oldest, oldestUsed = root, state.used
			}
		}
		delete(ticketMapState.roots, oldest)
		prefix := filepath.Join(oldest, filepath.FromSlash(TicketsDir)) + string(filepath.Separator)
		ticketCache.Lock()
		for dir, entries := range ticketCache.dirs {
			if !strings.HasPrefix(dir+string(filepath.Separator), prefix) {
				continue
			}
			for name, entry := range entries {
				if entry.doc != nil {
					entry.doc = nil
					entries[name] = entry
				}
			}
		}
		ticketCache.Unlock()
		ticketRefCache.Lock()
		for path := range ticketRefCache.files {
			if strings.HasPrefix(path, prefix) {
				delete(ticketRefCache.files, path)
			}
		}
		ticketRefCache.Unlock()
	}
}

// loadTicketMap lists ready/, todo/ and idea/ (exactly three readTicketDir
// calls), revalidates every open ticket through ticketCache, and resolves each
// stem an open ticket's parent, blocked-by or related names by at most two
// targeted lstats (.done/ first, .dropped/ only on a miss). It never lists a
// closed directory.
func loadTicketMap(root string) (ticketMapLoad, error) {
	ticketsRoot := filepath.Join(root, filepath.FromSlash(TicketsDir))
	load := ticketMapLoad{stable: true, missing: map[string][]string{}}
	openIndex := map[string]bool{}
	missingDirs := 0
	for _, status := range ticketMapOpenStatuses {
		scans, err := scanStatusDirWith(root, filepath.Join(ticketsRoot, status), status, true)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				missingDirs++
				continue
			}
			return ticketMapLoad{}, err
		}
		for _, scan := range scans {
			// First wins in ticketStatusRank order, like loadTicketGraph.
			if openIndex[scan.info.Stem] {
				continue
			}
			openIndex[scan.info.Stem] = true
			load.nodes = append(load.nodes, mapNode{info: scan.info, doc: scan.doc, open: true})
			load.files = append(load.files, ticketMapFile{rel: scan.info.Path, size: scan.size, modTime: scan.modTime})
			load.stable = load.stable && scan.stable
		}
	}
	if missingDirs == len(ticketMapOpenStatuses) {
		// Only now pay a stat to tell a missing board from an empty one.
		if _, err := os.Stat(ticketsRoot); err != nil {
			return ticketMapLoad{}, fmt.Errorf("tickets directory not found: %w", err)
		}
	}

	refs := map[string]bool{}
	for _, node := range load.nodes {
		for _, stem := range ticketMapRefStems(node.info) {
			if !openIndex[stem] {
				refs[stem] = true
			}
		}
	}
	dangling := map[string]bool{}
	for _, stem := range slices.Sorted(maps.Keys(refs)) {
		node, file, stable, found, err := loadTicketMapRef(root, ticketsRoot, stem)
		if err != nil {
			return ticketMapLoad{}, err
		}
		if !found {
			dangling[stem] = true
			continue
		}
		load.nodes = append(load.nodes, node)
		load.files = append(load.files, file)
		load.stable = load.stable && stable
	}
	load.dangling = slices.Sorted(maps.Keys(dangling))
	for _, node := range load.nodes {
		if !node.open {
			continue
		}
		var own []string
		for _, stem := range ticketMapRefStems(node.info) {
			if dangling[stem] && !slices.Contains(own, stem) {
				own = append(own, stem)
			}
		}
		if len(own) > 0 {
			slices.Sort(own)
			load.missing[node.info.Stem] = own
		}
	}
	return load, nil
}

// ticketMapRefStems lists the valid stems info's frontmatter names through
// parent, blocked-by (fragment stripped) and related, without self-references,
// in declaration order (related keys sorted).
func ticketMapRefStems(info TicketInfo) []string {
	var out []string
	add := func(stem string) {
		stem = strings.TrimSpace(stem)
		if stem == "" || stem == info.Stem || !ValidTicketStem(stem) {
			return
		}
		out = append(out, stem)
	}
	add(info.Parent)
	for _, spec := range info.BlockedBy {
		if edge, ok := parseBlockedByEdge(spec); ok {
			add(edge.Stem)
		}
	}
	for _, stem := range slices.Sorted(maps.Keys(info.Related)) {
		add(stem)
	}
	return out
}

// loadTicketMapRef resolves one closed referenced stem by lstat and reads it
// through ticketRefCache. found is false when neither closed directory holds a
// regular file (or symlink) for it: the stem is dangling.
func loadTicketMapRef(root, ticketsRoot, stem string) (mapNode, ticketMapFile, bool, bool, error) {
	for _, dir := range ticketMapClosedDirs {
		path := filepath.Join(ticketsRoot, dir, stem+ticketFileSuffix)
		stat, err := lstatTicketFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return mapNode{}, ticketMapFile{}, false, false, err
		}
		if stat.IsDir() {
			continue
		}
		symlink := stat.Mode()&fs.ModeSymlink != 0
		ticketRefCache.Lock()
		hit, ok := ticketRefCache.files[path]
		ticketRefCache.Unlock()
		if ok && !symlink && hit.size == stat.Size() && hit.modTime.Equal(stat.ModTime()) {
			file := ticketMapFile{rel: hit.info.Path, size: hit.size, modTime: hit.modTime}
			return mapNode{info: cloneTicketInfo(hit.info), doc: hit.doc}, file, true, true, nil
		}
		raw, err := readTicketFile(path)
		if err != nil {
			return mapNode{}, ticketMapFile{}, false, false, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return mapNode{}, ticketMapFile{}, false, false, err
		}
		info := readTicketFromBytes(filepath.ToSlash(rel), dir, string(raw))
		doc := buildTicketDoc(info.Stem, string(raw), true)
		file := ticketMapFile{rel: info.Path, size: int64(len(raw))}
		stable := false
		if !symlink {
			file.size, file.modTime = stat.Size(), stat.ModTime()
			if time.Since(stat.ModTime()) > ticketCacheRacyWindow {
				stable = true
				ticketRefCache.Lock()
				ticketRefCache.files[path] = ticketCacheEntry{size: stat.Size(), modTime: stat.ModTime(), info: cloneTicketInfo(info), doc: doc}
				ticketRefCache.Unlock()
			}
		}
		return mapNode{info: info, doc: doc}, file, stable, true, nil
	}
	return mapNode{}, ticketMapFile{}, false, false, nil
}

// ticketMapFingerprint hashes the sorted (relPath, size, mtime) of every
// included file, the sorted dangling stems, and ticketMapVersion.
func ticketMapFingerprint(load ticketMapLoad) string {
	files := slices.Clone(load.files)
	slices.SortFunc(files, func(a, b ticketMapFile) int { return strings.Compare(a.rel, b.rel) })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "f\x00%s\x00%d\x00%d\n", f.rel, f.size, f.modTime.UnixNano())
	}
	for _, stem := range load.dangling {
		fmt.Fprintf(h, "d\x00%s\n", stem)
	}
	fmt.Fprintf(h, "v\x00%d\n", ticketMapVersion)
	return hex.EncodeToString(h.Sum(nil))
}

// ticketMapEdges collects the explicit relations every included node declares
// whose both ends are included, without self-references, de-duplicated by
// (from, to, rel) keeping the first declaration, sorted by (from, to, rel).
func ticketMapEdges(nodes []mapNode) []mapEdge {
	index := make(map[string]int, len(nodes))
	for i, node := range nodes {
		index[node.info.Stem] = i
	}
	seen := map[[3]string]bool{}
	var out []mapEdge
	add := func(from int, stem, rel, note, frag string) {
		to, ok := index[strings.TrimSpace(stem)]
		if !ok || to == from {
			return
		}
		key := [3]string{strconv.Itoa(from), strconv.Itoa(to), rel}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, mapEdge{from: from, to: to, rel: rel, note: note, frag: frag})
	}
	for i, node := range nodes {
		add(i, node.info.Parent, "parent", "", "")
		for _, spec := range node.info.BlockedBy {
			edge, ok := parseBlockedByEdge(spec)
			if !ok {
				continue
			}
			frag := ""
			if edge.Phase > 0 {
				frag = "#phase" + strconv.Itoa(edge.Phase)
			}
			add(i, edge.Stem, "blocked-by", "", frag)
		}
		for _, stem := range slices.Sorted(maps.Keys(node.info.Related)) {
			add(i, stem, "related", strings.TrimSpace(node.info.Related[stem]), "")
		}
	}
	slices.SortFunc(out, func(a, b mapEdge) int {
		if a.from != b.from {
			return a.from - b.from
		}
		if a.to != b.to {
			return a.to - b.to
		}
		return strings.Compare(a.rel, b.rel)
	})
	return out
}

// assembleTicketMap runs the analysis over a load and builds the payload.
func assembleTicketMap(load ticketMapLoad, fingerprint string) *TicketMapResult {
	nodes := load.nodes
	edges := ticketMapEdges(nodes)
	analysis := analyzeTicketMap(nodes, edges)

	result := &TicketMapResult{
		ClusterKeywords: analysis.keywords,
		Clusters:        analysis.clusters,
		Nodes:           make([]TicketMapNode, 0, len(nodes)),
		Edges:           make([]TicketMapEdge, 0, len(edges)),
		Sim:             analysis.sim,
	}
	if result.ClusterKeywords == nil {
		result.ClusterKeywords = map[string][]TicketMapKeyword{}
	}
	if result.Clusters == nil {
		result.Clusters = []TicketMapCluster{}
	}
	if result.Sim == nil {
		result.Sim = []TicketMapSim{}
	}
	meta := TicketMapMeta{
		V:            ticketMapVersion,
		Fingerprint:  fingerprint[:16],
		Dangling:     len(load.dangling),
		MentionPairs: analysis.mentionPairs,
		Gamma:        MapFloat(roundTo(analysis.gamma, 1)),
		Modularity:   MapFloat(roundTo(analysis.modularity, 3)),
		DFCut:        analysis.dfCut,
		DFCutTop:     analysis.dfCutTop,
		Params:       analysis.params,
	}
	if meta.DFCutTop == nil {
		meta.DFCutTop = []string{}
	}
	for i, node := range nodes {
		status := strings.TrimPrefix(node.info.Status, ".")
		switch status {
		case "ready":
			meta.Ready++
		case "todo":
			meta.Todo++
		case "idea":
			meta.Idea++
		case "done":
			meta.RefDone++
		case "dropped":
			meta.RefDropped++
		}
		if node.open {
			meta.Open++
		} else {
			meta.Refs++
		}
		cat := "other"
		if m := ticketMapCatRE.FindStringSubmatch(node.info.Stem); m != nil {
			cat = m[1]
		}
		done := 0
		for _, phase := range node.info.Phases {
			if phase.ResultPresent {
				done++
			}
		}
		assignee := node.info.Assignee
		if assignee == nil {
			assignee = []string{}
		}
		out := TicketMapNode{
			I:              i,
			Stem:           node.info.Stem,
			Path:           node.info.Path,
			Title:          node.doc.Title,
			Status:         status,
			Cat:            cat,
			Phases:         [2]int{done, len(node.info.Phases)},
			ODQ:            node.doc.ODQ,
			BlockedHeading: len(node.info.BlockedHeadings) > 0,
			Lines:          node.doc.Lines,
			Assignee:       assignee,
			KW:             []TicketMapTerm{},
			Preview:        node.doc.Preview,
			Missing:        load.missing[node.info.Stem],
		}
		if i < len(analysis.cluster) {
			out.Cluster = analysis.cluster[i]
		}
		if i < len(analysis.kw) && analysis.kw[i] != nil {
			out.KW = analysis.kw[i]
		}
		result.Nodes = append(result.Nodes, out)
	}
	for _, edge := range edges {
		result.Edges = append(result.Edges, TicketMapEdge{From: edge.from, To: edge.to, Rel: edge.rel, Note: edge.note, Fragment: edge.frag})
	}
	result.Meta = meta
	return result
}

func encodeTicketMap(result *TicketMapResult) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
