package wsdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// The session rail's layout: the order of each worktree group's sessions,
// the depth-1 session tree (a parent's children), the starred sessions, and
// the order of the workspaces and their worktree groups, kept by the daemon so every browser shows
// the same rail. It is one JSON file in the daemon directory, beside the
// registry:
//
//	{"schema_version": 1,
//	 "workspace_order": [<workspace path>...],
//	 "workspaces": {"<workspace path>": {
//	   "worktree_order": [<worktree path>...], optional group sequence
//	   "order": [<reply-id>...],              top-level rows, every group's
//	   "children": {"<parent>": [<reply-id>...]},
//	   "stars": [<reply-id>...]}}}
//
// A worktree group's rows never meet another group's in one order, so one
// "order" list holds every group of the workspace; only the relative order
// of a group's own ids means anything. The file changes only by ops
// (applyRailLayoutOp), never by a whole-array write, and every op reads the
// file again first, so two browsers' edits merge. Each op is validated
// against the rail as the daemon renders it (railFacts): the same grouping
// the rows show under, not a second rule.
//
// A reply-id the rail does not list is ignored on read. A write drops it
// only once the index has read the whole registry (Index.Loaded); before
// that it may only be unread yet, so a daemon start never wipes the layout.
// A link to a parent the rail does not list is likewise ignored, and dropped
// for good by such a write: forgetting the parent ends the link.
//
// A child whose parent has ended is an orphan: it shows top-level, in its
// parent's slot when they share a worktree group, while the stored link
// stays so it nests again when the parent comes back. Any op on an orphan,
// and an attach under one, first drops that dormant link (settle) and then
// treats it as the top-level row it shows as.

const (
	railLayoutFileName      = "rail-layout.json"
	railLayoutSchemaVersion = 1
)

// The layout route's answer reasons: a stable machine set, which rail.js
// maps to its js.rail.layout.<code> strings and its own drag and keyboard
// checks name too. Every one but layoutNotApplied is a refusal.
const (
	// layoutUnknown: the rail lists no such session or workspace.
	layoutUnknown = "unknown"
	// layoutEnded: the session moved, or the parent or place named, has
	// ended.
	layoutEnded = "ended"
	// layoutSelf: a session or workspace named itself as its place or
	// parent.
	layoutSelf = "self"
	// layoutDepth: a child would become a parent, or a parent a child (the
	// tree is one level deep).
	layoutDepth = "depth"
	// layoutWorkspace: the place or parent is in another workspace.
	layoutWorkspace = "workspace"
	// layoutGroup: a top-level place in another worktree group.
	layoutGroup = "group"
	// layoutLevel: the place is not at the session's level (a top-level
	// row's place is a child, or a child's place is not its sibling, or an
	// attach's place is not one of the parent's children).
	layoutLevel = "level"
	// layoutStars: a reorder across the starred sessions' boundary.
	layoutStars = "stars"
	// layoutChild: a star on a child.
	layoutChild = "child"
	// layoutTopLevel: a detach of a session under no parent.
	layoutTopLevel = "toplevel"
	// layoutInvalid: a malformed request (an unknown op, a field it needs
	// missing, both before and after).
	layoutInvalid = "invalid"
	// layoutRetry: an import before the index has read the registry; the
	// browser keeps its keys and tries on a later load.
	layoutRetry = "retry"
	// layoutStore: the layout file could not be read or written (malformed,
	// a newer schema, an I/O error).
	layoutStore = "store"
	// layoutNotApplied: an import over a layout that holds an entry; it
	// changed nothing, and the browser drops its keys all the same.
	layoutNotApplied = "not_applied"
)

// errRailLayoutNewer is a layout file of a newer schema than this daemon
// writes: it is never written over.
var errRailLayoutNewer = errors.New("rail layout file has a newer schema_version")

// railLayoutDoc is the layout file. Fields this version does not know are
// kept through a read-modify-write (extra), so an older daemon's write
// never drops a newer one's additive field.
type railLayoutDoc struct {
	SchemaVersion  int
	WorkspaceOrder []string
	Workspaces     map[string]*railLayoutWS
	extra          map[string]json.RawMessage
}

// railLayoutWS is one workspace's layout.
type railLayoutWS struct {
	WorktreeOrder []string
	Order         []string
	Children      map[string][]string
	Stars         []string
	extra         map[string]json.RawMessage
}

// takeJSON decodes m[key] into into and removes it from m; a missing key
// leaves into as it is.
func takeJSON(m map[string]json.RawMessage, key string, into any) error {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	delete(m, key)
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

func (d *railLayoutDoc) UnmarshalJSON(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	*d = railLayoutDoc{}
	if err := takeJSON(m, "schema_version", &d.SchemaVersion); err != nil {
		return err
	}
	if err := takeJSON(m, "workspace_order", &d.WorkspaceOrder); err != nil {
		return err
	}
	if err := takeJSON(m, "workspaces", &d.Workspaces); err != nil {
		return err
	}
	if len(m) > 0 {
		d.extra = m
	}
	return nil
}

func (d railLayoutDoc) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range d.extra {
		m[k] = v
	}
	m["schema_version"] = railLayoutSchemaVersion
	m["workspace_order"] = nonNilStrings(d.WorkspaceOrder)
	ws := map[string]*railLayoutWS{}
	for p, w := range d.Workspaces {
		if w != nil {
			ws[p] = w
		}
	}
	m["workspaces"] = ws
	return json.Marshal(m)
}

func (w *railLayoutWS) UnmarshalJSON(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	*w = railLayoutWS{}
	if err := takeJSON(m, "worktree_order", &w.WorktreeOrder); err != nil {
		return err
	}
	if err := takeJSON(m, "order", &w.Order); err != nil {
		return err
	}
	if err := takeJSON(m, "children", &w.Children); err != nil {
		return err
	}
	if err := takeJSON(m, "stars", &w.Stars); err != nil {
		return err
	}
	if len(m) > 0 {
		w.extra = m
	}
	return nil
}

func (w railLayoutWS) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range w.extra {
		m[k] = v
	}
	if len(w.WorktreeOrder) > 0 {
		m["worktree_order"] = w.WorktreeOrder
	}
	m["order"] = nonNilStrings(w.Order)
	kids := map[string][]string{}
	for p, c := range w.Children {
		if len(c) > 0 {
			kids[p] = c
		}
	}
	m["children"] = kids
	m["stars"] = nonNilStrings(w.Stars)
	return json.Marshal(m)
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// normalize drops empty and repeated ids from every list, keeping the
// first of each, so the ops can assume a set in order.
func (d *railLayoutDoc) normalize() {
	d.WorkspaceOrder = uniqueIDs(d.WorkspaceOrder)
	for p, w := range d.Workspaces {
		if w == nil {
			delete(d.Workspaces, p)
			continue
		}
		w.WorktreeOrder = cleanWorktreeOrder(w.WorktreeOrder)
		w.Order, w.Stars = uniqueIDs(w.Order), uniqueIDs(w.Stars)
		for parent, kids := range w.Children {
			if kids = uniqueIDs(kids); parent == "" || len(kids) == 0 {
				delete(w.Children, parent)
			} else {
				w.Children[parent] = kids
			}
		}
	}
}

func uniqueIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// clone is a deep copy, which an op changes while the copy the rail reads
// stays as it was.
func (d *railLayoutDoc) clone() *railLayoutDoc {
	c := &railLayoutDoc{SchemaVersion: d.SchemaVersion, WorkspaceOrder: slices.Clone(d.WorkspaceOrder), extra: d.extra, Workspaces: map[string]*railLayoutWS{}}
	for p, w := range d.Workspaces {
		cw := &railLayoutWS{WorktreeOrder: slices.Clone(w.WorktreeOrder), Order: slices.Clone(w.Order), Stars: slices.Clone(w.Stars), extra: w.extra}
		if w.Children != nil {
			cw.Children = map[string][]string{}
			for parent, kids := range w.Children {
				cw.Children[parent] = slices.Clone(kids)
			}
		}
		c.Workspaces[p] = cw
	}
	return c
}

func (d *railLayoutDoc) encode() ([]byte, error) {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// readRailLayout reads the layout file: a missing one is an empty layout;
// a malformed one, or one of a newer schema, is an error.
func readRailLayout(file string) (*railLayoutDoc, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return &railLayoutDoc{Workspaces: map[string]*railLayoutWS{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc railLayoutDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion > railLayoutSchemaVersion {
		return nil, fmt.Errorf("%w (%d, this daemon writes %d)", errRailLayoutNewer, doc.SchemaVersion, railLayoutSchemaVersion)
	}
	if doc.Workspaces == nil {
		doc.Workspaces = map[string]*railLayoutWS{}
	}
	doc.normalize()
	return &doc, nil
}

// railFileStamp is a file's state as last read: whether it existed, its
// modification time, and its size.
type railFileStamp struct {
	ok        bool
	mod, size int64
}

func statRailFile(file string) railFileStamp {
	info, err := os.Stat(file)
	if err != nil {
		return railFileStamp{}
	}
	return railFileStamp{ok: true, mod: info.ModTime().UnixNano(), size: info.Size()}
}

// railLayoutStore holds the layout file. The rail reads the copy in memory,
// read again whenever the file's state moved (another process wrote it);
// every op reads the file again first, under mu, so a change another
// process wrote is never lost. A file that does not parse, or is of a newer
// schema, is never written over: the rail keeps the copy last read, and
// every op fails.
type railLayoutStore struct {
	file string

	mu   sync.Mutex
	doc  *railLayoutDoc
	seen railFileStamp
	// gen rises with every change of doc. It starts at the store's opening
	// time in microseconds, so a newer daemon's generations come after an
	// older one's, and a browser comparing them across a daemon restart
	// (rail.js staleRail) still sees them rise; it stays a safe JavaScript
	// integer for centuries.
	gen uint64
}

// openRailLayoutStore opens the layout in the daemon directory dir; an
// unreadable file reads as an empty layout (and refuses every op).
func openRailLayoutStore(dir string) *railLayoutStore {
	s := &railLayoutStore{file: filepath.Join(dir, railLayoutFileName), doc: &railLayoutDoc{Workspaces: map[string]*railLayoutWS{}}, gen: uint64(time.Now().UnixMicro())}
	s.mu.Lock()
	s.refreshLocked()
	s.mu.Unlock()
	return s
}

// refreshLocked reads the file again when its state moved since the last
// read; mu is held. A read that fails keeps the copy in memory.
func (s *railLayoutStore) refreshLocked() {
	st := statRailFile(s.file)
	if st == s.seen {
		return
	}
	s.seen = st
	if doc, err := readRailLayout(s.file); err == nil {
		s.doc = doc
		s.gen++
	}
}

// current is the layout the rail shows and its generation. The returned
// layout is never changed afterwards (an op replaces it), so it is read
// without the lock. A nil store reads as an empty layout.
func (s *railLayoutStore) current() (*railLayoutDoc, uint64) {
	if s == nil {
		return &railLayoutDoc{}, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshLocked()
	return s.doc, s.gen
}

// railLayoutAnswer is an op's outcome: Reason empty when it applied,
// Changed when it changed the file, Gen the layout's generation after it.
type railLayoutAnswer struct {
	Reason  string
	Changed bool
	Gen     uint64
}

// apply runs req on the file as it is on disk now, validated against f,
// and writes the result when it changed anything; loaded reports an index
// that has read the whole registry, which lets the write drop the entries
// f does not list. err is a store failure (Reason layoutStore).
func (s *railLayoutStore) apply(req railLayoutRequest, f railFacts, loaded bool) (railLayoutAnswer, error) {
	if s == nil {
		return railLayoutAnswer{Reason: layoutStore}, errors.New("no rail layout store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := statRailFile(s.file)
	doc, err := readRailLayout(s.file)
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore, Gen: s.gen}, err
	}
	if st != s.seen {
		s.seen, s.doc = st, doc
		s.gen++
	}
	next := doc.clone()
	if reason := applyRailLayoutOp(next, req, f, loaded); reason != "" {
		return railLayoutAnswer{Reason: reason, Gen: s.gen}, nil
	}
	if loaded {
		pruneRailLayout(next, f)
	}
	before, err := doc.encode()
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore, Gen: s.gen}, err
	}
	after, err := next.encode()
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore, Gen: s.gen}, err
	}
	if bytes.Equal(before, after) {
		return railLayoutAnswer{Gen: s.gen}, nil
	}
	if err := writePrivateFile(filepath.Dir(s.file), filepath.Base(s.file), after); err != nil {
		return railLayoutAnswer{Reason: layoutStore, Gen: s.gen}, err
	}
	s.doc, s.seen = next, statRailFile(s.file)
	s.gen++
	return railLayoutAnswer{Changed: true, Gen: s.gen}, nil
}

// railFact is what the rail shows of one session.
type railFact struct {
	// WS is the workspace's path; Group names the worktree group the row
	// renders under (its RailWorktree, or a plain folder's own list), keyed
	// with the workspace so two workspaces' groups never share a key.
	WS, Group string
	Live      bool
	// Pos is the row's place in its group as the rail lists it before the
	// layout orders it: the live rows (oldest registration first), then
	// the ended ones.
	Pos int
}

// railFacts is the rail as the layout's ops and order read it.
type railFacts struct {
	rows map[string]railFact
	// groups holds each group's reply-ids in Pos order.
	groups map[string][]string
	// worktrees is each repository's complete displayed group inventory in
	// fallback order, including empty groups (not derivable from rows).
	worktrees map[string][]string
	// wsList is the listed workspaces in the order the rail lists them
	// before the layout orders them; ws the same as a set.
	wsList []string
	ws     map[string]bool
}

// railFactsOf reads the facts off a rail view, as buildRailView built it.
func railFactsOf(view RailView) railFacts {
	f := railFacts{rows: map[string]railFact{}, groups: map[string][]string{}, worktrees: map[string][]string{}, ws: map[string]bool{}}
	add := func(ws, path string, live, ended []RailRow) {
		group := ws + "\x00" + path
		for _, list := range [][]RailRow{live, ended} {
			for _, r := range list {
				f.rows[r.ReplyID] = railFact{WS: ws, Group: group, Live: r.Live, Pos: len(f.groups[group])}
				f.groups[group] = append(f.groups[group], r.ReplyID)
			}
		}
	}
	for _, w := range view.Workspaces {
		f.wsList = append(f.wsList, w.Path)
		f.ws[w.Path] = true
		if !w.Repo {
			add(w.Path, w.Path, w.Sessions, w.Ended)
			continue
		}
		for _, wt := range w.Worktrees {
			f.worktrees[w.Path] = append(f.worktrees[w.Path], wt.Path)
			add(w.Path, wt.Path, wt.Sessions, wt.Ended)
		}
	}
	return f
}

// railLayoutRequest is the layout route's body: an op and what it names.
type railLayoutRequest struct {
	Op        string `json:"op"`
	RID       string `json:"rid,omitempty"`
	Parent    string `json:"parent,omitempty"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Path      string `json:"path,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	// Order, Stars, and WSOrder are an import's: a browser's retired local
	// session order, starred sessions, and workspace order.
	Order   []string `json:"order,omitempty"`
	Stars   []string `json:"stars,omitempty"`
	WSOrder []string `json:"wsorder,omitempty"`
}

// place is the request's before or after: ref, whether it is after, and
// whether one is given; ok is false when both are.
func (r railLayoutRequest) place() (ref string, after, given, ok bool) {
	switch {
	case r.Before != "" && r.After != "":
		return "", false, false, false
	case r.After != "":
		return r.After, true, true, true
	case r.Before != "":
		return r.Before, false, true, true
	}
	return "", false, false, true
}

// layoutScope is one workspace's layout read against the rail's facts. w
// is nil for a workspace the layout holds nothing of (read only).
type layoutScope struct {
	ws string
	w  *railLayoutWS
	f  railFacts
}

// scope is ws's layout; with create it makes an empty entry for ws.
func (d *railLayoutDoc) scope(ws string, f railFacts, create bool) layoutScope {
	var w *railLayoutWS
	if d != nil {
		w = d.Workspaces[ws]
		if w == nil && create {
			w = &railLayoutWS{}
			if d.Workspaces == nil {
				d.Workspaces = map[string]*railLayoutWS{}
			}
			d.Workspaces[ws] = w
		}
	}
	return layoutScope{ws: ws, w: w, f: f}
}

// mine reports a session the rail lists in this workspace.
func (s layoutScope) mine(id string) bool {
	r, ok := s.f.rows[id]
	return ok && r.WS == s.ws
}

// parent is id's stored parent among the sessions the rail lists in this
// workspace, "" for none (a link to any other is ignored); of two, the
// first by name, so a hand-edited file still reads one way.
func (s layoutScope) parent(id string) string {
	if s.w == nil {
		return ""
	}
	best := ""
	for p, kids := range s.w.Children {
		if p != id && s.mine(p) && slices.Contains(kids, id) && (best == "" || p < best) {
			best = p
		}
	}
	return best
}

// child reports a session under a live parent: it shows as a child.
func (s layoutScope) child(id string) bool {
	p := s.parent(id)
	return p != "" && s.f.rows[p].Live
}

// orphan reports a session whose stored parent has ended: it shows
// top-level.
func (s layoutScope) orphan(id string) bool {
	p := s.parent(id)
	return p != "" && !s.f.rows[p].Live
}

// hasChildren reports a session that is any listed session's stored
// parent, live or ended.
func (s layoutScope) hasChildren(id string) bool {
	if s.w == nil {
		return false
	}
	for _, c := range s.w.Children[id] {
		if c != id && s.mine(c) {
			return true
		}
	}
	return false
}

// starOn reports a starred session whose star counts: a child's stays
// stored but inert until it is top-level again.
func (s layoutScope) starOn(id string) bool {
	return s.w != nil && slices.Contains(s.w.Stars, id) && !s.child(id)
}

// rank is a top-level row's place in the order: its own entry, or the
// tail for an unlisted ordinary live row. Ended rows keep their prior rank
// and an orphan without an entry of its own takes its ended parent's place when
// they share a group, else -2, the head of its own group. Once a placement
// in its group stored the orphan where it shows (place), that entry ranks
// it, so a row placed before or after the orphan stays on that side of it;
// a nested child holds no entry (place drops it), so an orphan's entry is
// never older than its last time top-level.
func (s layoutScope) rank(id string) int {
	if s.w == nil {
		return -1
	}
	own := slices.Index(s.w.Order, id)
	if p := s.parent(id); p != "" && !s.f.rows[p].Live && own < 0 {
		if s.f.rows[p].Group != s.f.rows[id].Group {
			return -2
		}
		return slices.Index(s.w.Order, p)
	}
	if own < 0 && s.f.rows[id].Live && s.parent(id) == "" {
		return len(s.w.Order)
	}
	return own
}

// seq is group's top-level rows, live and ended, in the order's order
// (stars aside), rows of the same rank as the rail lists them.
func (s layoutScope) seq(group string) []string {
	var ids []string
	rank := map[string]int{}
	for _, id := range s.f.groups[group] {
		if !s.child(id) {
			ids = append(ids, id)
			rank[id] = s.rank(id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool { return rank[ids[i]] < rank[ids[j]] })
	return ids
}

// place stores seq (one group's top-level rows, in their new order) in the
// order: the group's ids give way to seq at the first of them, every other
// id (another group's, an id not listed now) keeps its place. An entry of a
// child nested under a live parent is dropped: it would outrank the
// parent's slot if the parent ended again (rank).
func (s layoutScope) place(seq []string) {
	s.w.Order = slices.DeleteFunc(placeIDs(s.w.Order, seq), s.child)
}

func placeIDs(order, seq []string) []string {
	in := map[string]bool{}
	for _, id := range seq {
		in[id] = true
	}
	var rest []string
	at := -1
	for _, id := range order {
		if in[id] {
			if at < 0 {
				at = len(rest)
			}
			continue
		}
		rest = append(rest, id)
	}
	if at < 0 {
		at = 0
	}
	return slices.Concat(rest[:at], seq, rest[at:])
}

// unlink removes id from every parent's children.
func (s layoutScope) unlink(id string) {
	for p, kids := range s.w.Children {
		if kids = slices.DeleteFunc(kids, func(x string) bool { return x == id }); len(kids) == 0 {
			delete(s.w.Children, p)
		} else {
			s.w.Children[p] = kids
		}
	}
}

// settle drops an orphan's dormant link: its group's order is stored first,
// so the orphan keeps the place it shows in.
func (s layoutScope) settle(id string) {
	if !s.orphan(id) {
		return
	}
	s.place(s.seq(s.f.rows[id].Group))
	s.unlink(id)
}

// insertAt puts id into ids before ref, or after it; at the end when ref is
// not in ids.
func insertAt(ids []string, id, ref string, after bool) []string {
	ids = slices.DeleteFunc(ids, func(x string) bool { return x == id })
	i := slices.Index(ids, ref)
	if i < 0 {
		return append(ids, id)
	}
	if after {
		i++
	}
	return slices.Insert(ids, i, id)
}

// applyRailLayoutOp applies req to doc, validated against f, and returns
// "" when it applied, else the reason (a refusal, or layoutNotApplied for
// an import over a layout that holds an entry); a refused op may leave doc
// half-changed, so the caller discards it. loaded reports an index that
// has read the whole registry, which an import needs.
func applyRailLayoutOp(doc *railLayoutDoc, req railLayoutRequest, f railFacts, loaded bool) string {
	ref, after, given, ok := req.place()
	if !ok {
		return layoutInvalid
	}
	switch req.Op {
	case "move":
		if req.RID == "" || !given {
			return layoutInvalid
		}
		return layoutMove(doc, f, req.RID, ref, after)
	case "attach":
		if req.RID == "" || req.Parent == "" {
			return layoutInvalid
		}
		return layoutAttach(doc, f, req.RID, req.Parent, ref, after)
	case "detach":
		if req.RID == "" {
			return layoutInvalid
		}
		return layoutDetach(doc, f, req.RID, ref, after)
	case "star", "unstar":
		if req.RID == "" || given {
			return layoutInvalid
		}
		return layoutStar(doc, f, req.RID, req.Op == "star")
	case "wsmove":
		if req.Path == "" || !given {
			return layoutInvalid
		}
		return layoutWSMove(doc, f, req.Path, ref, after)
	case "wtmove":
		if req.Workspace == "" || req.Path == "" || !given {
			return layoutInvalid
		}
		return layoutWTMove(doc, f, req.Workspace, req.Path, ref, after)
	case "import":
		if !loaded {
			return layoutRetry
		}
		return layoutImport(doc, f, req)
	}
	return layoutInvalid
}

// layoutMove reorders id at its level: among its group's top-level rows on
// its side of the stars, or among its parent's children.
func layoutMove(doc *railLayoutDoc, f railFacts, id, ref string, after bool) string {
	r, ok := f.rows[id]
	switch {
	case !ok:
		return layoutUnknown
	case !r.Live:
		return layoutEnded
	case ref == id:
		return layoutSelf
	}
	t, ok := f.rows[ref]
	switch {
	case !ok:
		return layoutUnknown
	case !t.Live:
		return layoutEnded
	case t.WS != r.WS:
		return layoutWorkspace
	}
	s := doc.scope(r.WS, f, true)
	if s.child(id) {
		p := s.parent(id)
		if !s.child(ref) || s.parent(ref) != p {
			return layoutLevel
		}
		s.w.Children[p] = insertAt(s.w.Children[p], id, ref, after)
		return ""
	}
	switch {
	case s.child(ref):
		return layoutLevel
	case t.Group != r.Group:
		return layoutGroup
	case s.starOn(id) != s.starOn(ref):
		return layoutStars
	}
	s.settle(id)
	s.place(insertAt(s.seq(r.Group), id, ref, after))
	return ""
}

// layoutAttach makes id a child of parent, before or after one of its
// children, else last.
func layoutAttach(doc *railLayoutDoc, f railFacts, id, parent, ref string, after bool) string {
	r, ok := f.rows[id]
	if !ok {
		return layoutUnknown
	}
	if parent == id {
		return layoutSelf
	}
	p, ok := f.rows[parent]
	switch {
	case !ok:
		return layoutUnknown
	case !p.Live:
		return layoutEnded
	case p.WS != r.WS:
		return layoutWorkspace
	}
	s := doc.scope(r.WS, f, true)
	if s.child(parent) || s.hasChildren(id) {
		return layoutDepth
	}
	if ref != "" {
		t, ok := f.rows[ref]
		switch {
		case ref == id:
			return layoutSelf
		case !ok:
			return layoutUnknown
		case !t.Live:
			return layoutEnded
		case s.parent(ref) != parent:
			return layoutLevel
		}
	}
	s.settle(parent)
	s.unlink(id)
	s.w.Order = slices.DeleteFunc(s.w.Order, func(x string) bool { return x == id })
	if s.w.Children == nil {
		s.w.Children = map[string][]string{}
	}
	s.w.Children[parent] = insertAt(s.w.Children[parent], id, ref, after)
	return ""
}

// layoutDetach makes a child (or an orphan) top-level: before or after a
// top-level row of its own group, else right after its old parent when
// that is in its group, else at the head of its group.
func layoutDetach(doc *railLayoutDoc, f railFacts, id, ref string, after bool) string {
	r, ok := f.rows[id]
	if !ok {
		return layoutUnknown
	}
	s := doc.scope(r.WS, f, true)
	p := s.parent(id)
	if p == "" {
		return layoutTopLevel
	}
	if ref != "" {
		t, ok := f.rows[ref]
		switch {
		case ref == id:
			return layoutSelf
		case !ok:
			return layoutUnknown
		case !t.Live:
			return layoutEnded
		case t.WS != r.WS:
			return layoutWorkspace
		case s.child(ref):
			return layoutLevel
		case t.Group != r.Group:
			return layoutGroup
		}
	}
	s.unlink(id)
	s.w.Order = slices.DeleteFunc(s.w.Order, func(x string) bool { return x == id })
	seq := slices.DeleteFunc(s.seq(r.Group), func(x string) bool { return x == id })
	switch {
	case ref != "":
		seq = insertAt(seq, id, ref, after)
	case f.rows[p].Group == r.Group && slices.Contains(seq, p):
		seq = insertAt(seq, id, p, true)
	default:
		seq = slices.Insert(seq, 0, id)
	}
	s.place(seq)
	return ""
}

// layoutStar stars or unstars id; a child cannot be starred.
func layoutStar(doc *railLayoutDoc, f railFacts, id string, on bool) string {
	r, ok := f.rows[id]
	if !ok {
		return layoutUnknown
	}
	s := doc.scope(r.WS, f, true)
	if on && s.child(id) {
		return layoutChild
	}
	s.settle(id)
	s.w.Stars = slices.DeleteFunc(s.w.Stars, func(x string) bool { return x == id })
	if on {
		s.w.Stars = append(s.w.Stars, id)
	}
	return ""
}

// railWSSeq is the listed workspaces in the order the rail shows them: the
// workspace order's first, the rest after as the rail lists them.
func railWSSeq(doc *railLayoutDoc, f railFacts) []string {
	seq := slices.Clone(f.wsList)
	rank := map[string]int{}
	for _, p := range seq {
		rank[p] = len(seq)
		if doc != nil {
			if i := slices.Index(doc.WorkspaceOrder, p); i >= 0 {
				rank[p] = i
			}
		}
	}
	sort.SliceStable(seq, func(i, j int) bool { return rank[seq[i]] < rank[seq[j]] })
	return seq
}

// layoutWSMove reorders the workspace at path among the listed ones.
func layoutWSMove(doc *railLayoutDoc, f railFacts, path, ref string, after bool) string {
	switch {
	case !f.ws[path] || !f.ws[ref]:
		return layoutUnknown
	case path == ref:
		return layoutSelf
	}
	doc.WorkspaceOrder = placeIDs(doc.WorkspaceOrder, insertAt(railWSSeq(doc, f), path, ref, after))
	return ""
}

// railLayoutEmpty reports a layout that holds no entry at all: no
// workspace-order entry, and no order, children, or star entry in any
// workspace, whether or not the rail lists what an entry names. Only an
// empty layout takes an import.
func railLayoutEmpty(doc *railLayoutDoc) bool {
	if doc == nil {
		return true
	}
	if len(doc.WorkspaceOrder) > 0 {
		return false
	}
	for _, w := range doc.Workspaces {
		if len(w.WorktreeOrder) > 0 || len(w.Order) > 0 || len(w.Stars) > 0 || len(w.Children) > 0 {
			return false
		}
	}
	return true
}

// layoutImport takes a browser's retired local layout into an empty one:
// each session goes to its workspace's order and stars, unlisted and
// repeated entries dropped, and no children (the local layout had no
// tree).
func layoutImport(doc *railLayoutDoc, f railFacts, req railLayoutRequest) string {
	if !railLayoutEmpty(doc) {
		return layoutNotApplied
	}
	for _, id := range uniqueIDs(req.Order) {
		if r, ok := f.rows[id]; ok {
			s := doc.scope(r.WS, f, true)
			s.w.Order = append(s.w.Order, id)
		}
	}
	for _, id := range uniqueIDs(req.Stars) {
		if r, ok := f.rows[id]; ok {
			s := doc.scope(r.WS, f, true)
			s.w.Stars = append(s.w.Stars, id)
		}
	}
	for _, p := range uniqueIDs(req.WSOrder) {
		if f.ws[p] {
			doc.WorkspaceOrder = append(doc.WorkspaceOrder, p)
		}
	}
	return ""
}

// pruneRailLayout drops every entry the rail does not list: workspaces from
// the workspace order, sessions from each workspace's lists, and a parent
// with every link to it. A workspace left with nothing (and no field this
// version does not know) goes too. Only a write against an index that has
// read the whole registry prunes.
func pruneRailLayout(doc *railLayoutDoc, f railFacts) {
	known := func(id string) bool { _, ok := f.rows[id]; return ok }
	keep := func(ids []string) []string {
		return slices.DeleteFunc(ids, func(id string) bool { return !known(id) })
	}
	doc.WorkspaceOrder = slices.DeleteFunc(doc.WorkspaceOrder, func(p string) bool { return !f.ws[p] })
	for path, w := range doc.Workspaces {
		w.Order, w.Stars = keep(w.Order), keep(w.Stars)
		for p, kids := range w.Children {
			if kids = keep(kids); !known(p) || len(kids) == 0 {
				delete(w.Children, p)
			} else {
				w.Children[p] = kids
			}
		}
		if len(w.WorktreeOrder) == 0 && len(w.Order) == 0 && len(w.Stars) == 0 && len(w.Children) == 0 && len(w.extra) == 0 {
			delete(doc.Workspaces, path)
		}
	}
}

// orderRailView orders view, as buildRailView built it, by the layout doc
// (f read off that same view): within each worktree group the starred units
// first, then the layout's order, unlisted ordinary live rows at the tail
// in registration order (oldest first). Orphans retain their established
// ranks. A live child of a live
// top-level parent leaves its own group's list for its parent's Children,
// in the parent's children order, whatever group either is in; an
// orphan, an ended child, and a child whose parent shows as a child itself
// (a hand-edited file) stay in their own group. The ended rows keep their
// own order (last seen), the starred ones first. Each row's Starred is its
// star as it counts (a child's is inert), and Group, HasChildren, and the
// roll-ups are set. The workspaces follow the workspace order, the rest
// after in the rail's order.
func orderRailView(view *RailView, doc *railLayoutDoc, f railFacts) {
	for i := range view.Workspaces {
		w := &view.Workspaces[i]
		s := doc.scope(w.Path, f, false)
		if !w.Repo {
			s.orderTree([]railGroupRows{{w.Path, "", &w.Sessions, &w.Ended}})
			continue
		}
		groups := make([]railGroupRows, len(w.Worktrees))
		for j := range w.Worktrees {
			wt := &w.Worktrees[j]
			tag := wt.Branch
			if wt.Detached || tag == "" {
				tag = wt.Folder
			}
			groups[j] = railGroupRows{wt.Path, tag, &wt.Sessions, &wt.Ended}
		}
		s.orderTree(groups)
		rank := map[string]int{}
		for j, path := range railWTSeq(doc, f, w.Path) {
			rank[path] = j
		}
		sort.SliceStable(w.Worktrees, func(i, j int) bool {
			return rank[w.Worktrees[i].Path] < rank[w.Worktrees[j].Path]
		})
	}
	rank := map[string]int{}
	for i, p := range railWSSeq(doc, f) {
		rank[p] = i
	}
	sort.SliceStable(view.Workspaces, func(i, j int) bool {
		return rank[view.Workspaces[i].Path] < rank[view.Workspaces[j].Path]
	})
	view.LayoutEmpty = railLayoutEmpty(doc)
}

// railGroupRows is one worktree group of a workspace as orderTree reads
// it: its path, its branch tag (railRow.BranchTag), and its row lists.
type railGroupRows struct {
	path, tag   string
	live, ended *[]RailRow
}

// nests reports a live child that renders under its parent: the parent is
// live and shows top-level.
func (s layoutScope) nests(id string) bool {
	return s.child(id) && !s.child(s.parent(id))
}

// orderTree orders one workspace's groups (orderRailView).
func (s layoutScope) orderTree(groups []railGroupRows) {
	kids := map[string]RailRow{}
	tags := map[string]string{}
	for _, g := range groups {
		tags[g.path] = g.tag
		top := (*g.live)[:0]
		for _, r := range *g.live {
			r.Group, r.HasChildren = g.path, s.hasChildren(r.ReplyID)
			if s.nests(r.ReplyID) {
				kids[r.ReplyID] = r
				continue
			}
			top = append(top, r)
		}
		*g.live = top
		for i := range *g.ended {
			e := &(*g.ended)[i]
			e.Group, e.HasChildren, e.StarHidden = g.path, s.hasChildren(e.ReplyID), s.child(e.ReplyID)
		}
		s.orderRows(*g.live, *g.ended)
	}
	for _, g := range groups {
		for i := range *g.live {
			p := &(*g.live)[i]
			for _, id := range s.childIDs(p.ReplyID) {
				k, ok := kids[id]
				if !ok || s.parent(id) != p.ReplyID {
					continue
				}
				delete(kids, id)
				k.Child, k.Starred, k.StarHidden = true, false, true
				if k.Group != p.Group {
					k.BranchTag = tags[k.Group]
				}
				if k.Approval != nil {
					p.KidApprovals++
				} else if k.Working {
					p.KidWorking = true
				}
				p.Children = append(p.Children, k)
			}
		}
	}
}

// childIDs is id's stored children, in their order.
func (s layoutScope) childIDs(id string) []string {
	if s.w == nil {
		return nil
	}
	return s.w.Children[id]
}

// orderRows orders one worktree group's top-level live rows and its ended
// rows (orderTree).
func (s layoutScope) orderRows(live, ended []RailRow) {
	type unitKey struct {
		star      bool
		rank, pos int
	}
	keys := map[string]unitKey{}
	for i := range live {
		id := live[i].ReplyID
		keys[id] = unitKey{star: s.starOn(id), rank: s.rank(id), pos: s.f.rows[id].Pos}
		live[i].Starred = keys[id].star
	}
	sort.SliceStable(live, func(i, j int) bool {
		a, b := keys[live[i].ReplyID], keys[live[j].ReplyID]
		switch {
		case a.star != b.star:
			return a.star
		case a.rank != b.rank:
			return a.rank < b.rank
		}
		return a.pos < b.pos
	})
	for i := range ended {
		ended[i].Starred = s.starOn(ended[i].ReplyID)
	}
	sort.SliceStable(ended, func(i, j int) bool { return ended[i].Starred && !ended[j].Starred })
}
