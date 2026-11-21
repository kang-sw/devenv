// Peer view tests cover merging, worktrees, and row display.

package wsdashboard

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestPeerGroupMergesResolvesAndRetains(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	peerSnap := liveSnap(now, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool}},
		wslive.Agent{ID: "p1", Status: wslive.StatusRunning})
	sessions := map[string]Session{
		"servedpeer": liveSession("servedpeer", "/p", now),
		"endedpeer":  endedSession("endedpeer", "/q", now),
	}
	snaps := map[string]*wslive.Snapshot{"servedpeer": peerSnap}
	contacts := []Contact{
		// One session reached by its id: address and by a named address:
		// one row, labelled by the name, with the latest exchange.
		{Address: "id:servedpeer", ReplyID: "servedpeer", LastExchange: at(30)},
		{Address: "lead@machine", ReplyID: "servedpeer", LastExchange: at(2)},
		// The session itself never lists.
		{Address: "id:self", ReplyID: "self", LastExchange: at(1)},
		// A served peer whose registration ended reads ended whatever its
		// presence says.
		{Address: "id:endedpeer", ReplyID: "endedpeer", LastExchange: at(3), PresenceLive: true},
		// Unserved peers follow their presence.
		{Address: "other@machine", ReplyID: "unservedlive", LastExchange: at(4), PresenceLive: true},
		{Address: "id:unservedgone", ReplyID: "unservedgone", LastExchange: at(5)},
		{Address: "noowner@machine", LastExchange: at(6)},
	}
	for i := 0; i < 6; i++ {
		contacts = append(contacts, Contact{Address: fmt.Sprintf("id:old%d", i), ReplyID: fmt.Sprintf("old%d", i), LastExchange: at(100 + i)})
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	g := v.Tree.Peers
	if len(g.Live) != 2 {
		t.Fatalf("live peers = %+v", g.Live)
	}
	first := g.Live[0]
	if first.Address != "lead@machine" || first.ReplyID != "servedpeer" || !first.Served || !first.Adapter ||
		first.LastExchange != at(2).UnixMilli() || first.AgentCount != 1 || len(first.Agents) != 1 || first.Lead.Phase != wslive.PhaseTool {
		t.Errorf("merged served peer = %+v", first)
	}
	if g.Live[1].ReplyID != "unservedlive" || g.Live[1].Served {
		t.Errorf("second live peer = %+v", g.Live[1])
	}
	// Ended: endedpeer (3), unservedgone (5), noowner (6), old0, old1 - then
	// four more hidden.
	if len(g.Ended) != endedPeerCap || g.HiddenEnded != 4 {
		t.Fatalf("ended = %d (hidden %d): %+v", len(g.Ended), g.HiddenEnded, g.Ended)
	}
	if g.Ended[0].ReplyID != "endedpeer" || g.Ended[1].ReplyID != "unservedgone" || g.Ended[2].Address != "noowner@machine" || g.Ended[3].ReplyID != "old0" {
		t.Errorf("ended order = %+v", g.Ended)
	}
	for _, row := range g.Rows() {
		if row.ReplyID == "self" {
			t.Errorf("the session lists itself as a peer")
		}
	}
}

// TestPeerRowFolder pins the row label's folder: Path's last element, split
// on either separator, trailing separators ignored.
func TestPeerRowFolder(t *testing.T) {
	for path, want := range map[string]string{
		"/home/u/devenv":      "devenv",
		"/home/u/devenv/":     "devenv",
		`C:\Users\u\proj`:     "proj",
		`C:\Users\u\proj\`:    "proj",
		`D:/mixed\sep/leaf//`: "leaf",
		"plain":               "plain",
		"":                    "",
		"/":                   "",
	} {
		if got := (PeerRow{Path: path}).Folder(); got != want {
			t.Errorf("Folder(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestPeerFolderLabel pins the linked-worktree prefix: "<main>(<folder>)"
// from both paths' last elements under either separator, the plain folder
// when no main root is known or the path is empty.
func TestPeerFolderLabel(t *testing.T) {
	for _, c := range []struct{ path, main, want string }{
		{"/home/u/.wt/devenv/gamma", "/home/u/devenv", "devenv(gamma)"},
		{"/home/u/.wt/devenv/gamma/", "/home/u/devenv/", "devenv(gamma)"},
		{`C:\wt\gamma\`, `C:\src\devenv`, "devenv(gamma)"},
		{"/home/u/devenv", "", "devenv"},
		{"", "/home/u/devenv", ""},
		{"/home/u/x", "/", "x"},
	} {
		if got := (PeerRow{Path: c.path, WorktreeMain: c.main}).Folder(); got != c.want {
			t.Errorf("Folder(%q, %q) = %q, want %q", c.path, c.main, got, c.want)
		}
	}
}

// TestWorktreeMain pins linked-worktree detection from fake .git entries: a
// .git file pointing into <root>/.git/worktrees/<name> yields root (via
// commondir when present, relative gitdir included); a plain repo, a
// submodule, a bare-repo worktree, a missing path, and a Windows path seen
// here yield "". Trailing separators resolve like the clean path.
func TestWorktreeMain(t *testing.T) {
	base := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{base}, parts...)...)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main := mk("devenv")
	mk("devenv", ".git", "modules", "sub")
	wtGit := mk("devenv", ".git", "worktrees", "gamma")
	write(filepath.Join(wtGit, "commondir"), "../..\n")
	linked := mk("wt", "gamma")
	write(filepath.Join(linked, ".git"), "gitdir: "+wtGit+"\n")

	// No commondir, relative gitdir.
	mk("devenv", ".git", "worktrees", "delta")
	rel := mk("wt", "delta")
	write(filepath.Join(rel, ".git"), "gitdir: ../../devenv/.git/worktrees/delta\n")

	sub := mk("devenv", "sub")
	write(filepath.Join(sub, ".git"), "gitdir: ../.git/modules/sub\n")

	bareWt := mk("repo.git", "worktrees", "eps")
	write(filepath.Join(bareWt, "commondir"), "../..\n")
	bare := mk("wt", "eps")
	write(filepath.Join(bare, ".git"), "gitdir: "+bareWt+"\n")

	for path, want := range map[string]string{
		linked:                              main,
		linked + string(filepath.Separator): main,
		rel:                                 main,
		main:                                "",
		sub:                                 "",
		bare:                                "",
		filepath.Join(base, "missing"):      "",
		`C:\Users\u\proj`:                   "",
		"":                                  "",
	} {
		if got := readWorktreeMain(path); path != "" && got != want {
			t.Errorf("readWorktreeMain(%q) = %q, want %q", path, got, want)
		}
		if got := worktreeMain(path); got != want {
			t.Errorf("worktreeMain(%q) = %q, want %q", path, got, want)
		}
	}

	// Peer rows carry the prefix for a linked-worktree cwd.
	now := testNow
	contacts := []Contact{{Address: "id:wt", ReplyID: "wt", LastExchange: now, PresenceLive: true, Cwd: linked}}
	g := peerGroup(contacts, "/p", lookupOf(map[string]Session{}, nil), now)
	if len(g.Live) != 1 || g.Live[0].Folder() != "devenv(gamma)" {
		t.Errorf("peer row folder = %+v, want devenv(gamma)", g.Live)
	}
}

// TestPeerRowPath pins where a peer row's Path comes from: a served peer's
// registration Project over any contact cwd; an unserved id: or named peer's
// Contact.Cwd (the newest non-empty over merged addresses); none when no cwd
// is known. The row renders the folder prefix muted inside .pname, the full
// path as its title, and the bare address when no folder is known.
func TestPeerRowPath(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	sessions := map[string]Session{"served": liveSession("served", "/srv/work/served-wt", now)}
	contacts := []Contact{
		{Address: "id:served", ReplyID: "served", LastExchange: at(1), Cwd: "/elsewhere/ignored"},
		{Address: "id:unserved", ReplyID: "unserved", LastExchange: at(2), PresenceLive: true, Cwd: "/home/u/idpeer"},
		{Address: "named@machine", ReplyID: "namedpeer", LastExchange: at(3), PresenceLive: true},
		{Address: "id:namedpeer", ReplyID: "namedpeer", LastExchange: at(4), Cwd: "/home/u/namedwt/"},
		{Address: "win@machine", LastExchange: at(5), PresenceLive: true, Cwd: `C:\Users\u\winproj`},
		{Address: "id:nocwd", ReplyID: "nocwd", LastExchange: at(6), PresenceLive: true},
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, nil)})
	got := map[string][2]string{}
	for _, r := range v.Tree.Peers.Rows() {
		got[r.Address] = [2]string{r.Path, r.Folder()}
	}
	want := map[string][2]string{
		"id:served":     {"/srv/work/served-wt", "served-wt"},
		"id:unserved":   {"/home/u/idpeer", "idpeer"},
		"named@machine": {"/home/u/namedwt/", "namedwt"},
		"win@machine":   {`C:\Users\u\winproj`, "winproj"},
		"id:nocwd":      {"", ""},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows (path, folder) = %v, want %v", got, want)
	}
	html := renderTree(t, v)
	for _, w := range []string{
		`<a class="pname" href="/s/served" title="/srv/work/served-wt"><span class="pfolder">served-wt/</span>id:served</a>`,
		`<span class="pname" title="/home/u/idpeer"><span class="pfolder">idpeer/</span>id:unserved</span>`,
		`<span class="pname">id:nocwd</span>`,
	} {
		if !strings.Contains(html, w) {
			t.Errorf("tree lacks %q: %s", w, html)
		}
	}
}

func TestPeerTreeIsOneLevelDeep(t *testing.T) {
	now := testNow
	// The peer's snapshot carries agents; its own contacts are never read,
	// so its peer group cannot appear inside this page.
	peerSnap := liveSnap(now, wslive.Lead{}, wslive.Agent{ID: "x", Status: wslive.StatusRunning})
	sessions := map[string]Session{"peer": liveSession("peer", "/p", now)}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
		Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": peerSnap})})
	row := v.Tree.Peers.Live[0]
	if len(row.Agents) != 1 || len(row.Agents[0].Children) != 0 {
		t.Fatalf("peer tree = %+v", row.Agents)
	}
	// A served peer without a live adapter shows no phase.
	stale := *peerSnap
	stale.UpdatedAt = now.Add(-time.Hour).UnixMilli()
	v = buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
		Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": &stale})})
	if row := v.Tree.Peers.Live[0]; row.Adapter || len(row.Agents) != 0 {
		t.Fatalf("a peer without an adapter carries live state: %+v", row)
	}
}

// renderTree renders the Agents region of a view through the page template.
func renderTree(t *testing.T, v SessionView) string {
	t.Helper()
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, "r-tree", v); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPeerWorktreeCompactAndOrder(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	snap := func() *wslive.Snapshot {
		return liveSnap(now, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{{Seq: 1, EndedAt: 1}, {Seq: 2, Running: &wslive.RunningView{ToolName: "Bash"}}}}},
			wslive.Agent{ID: "a", Status: wslive.StatusRunning})
	}
	sessions := map[string]Session{
		"samewt":   liveSession("samewt", "/p/", now), // Clean makes it equal to /p
		"otherwt":  liveSession("otherwt", "/q", now),
		"otherwt2": liveSession("otherwt2", "/q", now),
		"gone":     endedSession("gone", "/q", now),
	}
	snaps := map[string]*wslive.Snapshot{"samewt": snap(), "otherwt": snap(), "otherwt2": snap(), "gone": snap()}
	contacts := []Contact{
		{Address: "id:otherwt", ReplyID: "otherwt", LastExchange: at(1)},
		{Address: "id:samewt", ReplyID: "samewt", LastExchange: at(5)},
		{Address: "id:otherwt2", ReplyID: "otherwt2", LastExchange: at(2)},
		{Address: "id:unserved", ReplyID: "unserved", LastExchange: at(3), PresenceLive: true},
		{Address: "id:gone", ReplyID: "gone", LastExchange: at(4)},
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	g := v.Tree.Peers
	var order []string
	for _, r := range g.Live {
		order = append(order, r.ReplyID)
	}
	if want := []string{"samewt", "otherwt", "otherwt2", "unserved"}; fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("live order = %v, want %v", order, want)
	}
	same, other := g.Live[0], g.Live[1]
	if !same.SameWorktree || same.Compact || !same.Adapter || len(same.Agents) != 1 || same.AgentCount != 1 || same.Lead.Phase != wslive.PhaseTool || len(same.Lead.Finished) != 1 || same.Lead.Running == nil {
		t.Errorf("same-worktree peer = %+v", same)
	}
	if other.SameWorktree || !other.Compact || !other.Adapter || other.Lead.Phase != wslive.PhaseTool ||
		len(other.Agents) != 0 || other.AgentCount != 0 || len(other.Lead.Finished) != 0 || other.Lead.Running != nil {
		t.Errorf("other-worktree peer = %+v", other)
	}
	if len(g.Ended) != 1 || g.Ended[0].Compact || len(g.Ended[0].Agents) != 1 {
		t.Errorf("ended peer = %+v", g.Ended)
	}
	html := renderTree(t, v)
	if got := strings.Count(html, `<details class="peer live"`); got != 1 {
		t.Errorf("expanding live peers = %d: %s", got, html)
	}
	if got := strings.Count(html, `class="peer live compact"`); got != 2 {
		t.Errorf("compact peers = %d: %s", got, html)
	}
	// The header chip counts every live peer: 2 compact + 1 same + 1 unserved.
	if !strings.Contains(html, `<span class="chip">`+"\u003c"[:0]) && !strings.Contains(html, `4</span></span></div>`) {
		t.Errorf("mail chip does not count every live peer: %s", html)
	}
	// The compact row has the phase mark (a pulsing dot) and last mail but no
	// tool, count, or tree.
	i := strings.Index(html, `class="peer live compact"`)
	row := html[i : i+strings.Index(html[i:], "</div>")]
	for _, want := range []string{`href="/s/otherwt"`, `class="pulse"`, `last mail exchange`} {
		if !strings.Contains(row, want) {
			t.Errorf("compact row lacks %q: %s", want, row)
		}
	}
	// Its one running subagent shows as the running label, not the total; an
	// agent mark, so it carries no spinner.
	if !strings.Contains(row, `class="prun"`) || !strings.Contains(row, `title="1 subagent"`) || strings.Contains(row, `spin`) {
		t.Errorf("compact row lacks the running-subagent label: %s", row)
	}
	for _, not := range []string{"ptool", "ptree", "pline"} {
		if strings.Contains(row, not) {
			t.Errorf("compact row carries %q: %s", not, row)
		}
	}
}

// TestPeerRowLabeledClass pins the markup the stylesheet's collapse order
// keys on: a row whose name is a manual label carries the labeled class on
// its peer element, in both the expanding and the compact shape, while a row
// showing its id: address does not; and the harness badge sits in its phost
// wrapper, which collapses whole.
func TestPeerRowLabeledClass(t *testing.T) {
	render := func(row PeerRow) string {
		v := SessionView{}
		v.Tree.Peers.Live = []PeerRow{row}
		return renderTree(t, v)
	}
	base := PeerRow{Address: "id:peer", ReplyID: "peer", Served: true, Live: true, Adapter: true, Harness: "claude"}
	labeled := base
	labeled.Label = "main work"
	if html := render(labeled); !strings.Contains(html, `<details class="peer live labeled"`) || !strings.Contains(html, `<span class="phost"><span class="host sm">claude</span></span>`) {
		t.Errorf("labeled row lacks the labeled class or the harness wrapper: %s", html)
	}
	compact := labeled
	compact.Compact = true
	if html := render(compact); !strings.Contains(html, `class="peer live compact labeled"`) {
		t.Errorf("labeled compact row lacks the labeled class: %s", html)
	}
	if html := render(base); strings.Contains(html, "labeled") || !strings.Contains(html, `<details class="peer live"`) {
		t.Errorf("id-named row carries the labeled class: %s", html)
	}
}

func TestPeerRunningSubagentSpinner(t *testing.T) {
	now := testNow
	agents := []wslive.Agent{
		{ID: "a", Status: wslive.StatusRunning},
		{ID: "b", Status: wslive.StatusRunning, ParentID: "a"}, // nested counts too
		{ID: "c", Status: wslive.StatusDone},
		{ID: "d", Status: wslive.StatusFailed},
	}
	build := func(project string, as ...wslive.Agent) PeerRow {
		sessions := map[string]Session{"peer": liveSession("peer", project, now)}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
			Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
			Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": liveSnap(now, wslive.Lead{}, as...)})})
		return v.Tree.Peers.Live[0]
	}
	render := func(row PeerRow) string {
		v := SessionView{}
		v.Tree.Peers.Live = []PeerRow{row}
		return renderTree(t, v)
	}
	for name, project := range map[string]string{"compact": "/q", "same-worktree": "/p"} {
		row := build(project, agents...)
		if row.Compact != (name == "compact") {
			t.Fatalf("%s: Compact = %v", name, row.Compact)
		}
		if row.RunningAgents != 2 {
			t.Errorf("%s: RunningAgents = %d, want 2 (finished agents excluded)", name, row.RunningAgents)
		}
		if html := render(row); !strings.Contains(html, `class="prun"`) || !strings.Contains(html, `title="2 subagents"`) || !strings.Contains(html, "</svg>2</span>") || strings.Contains(html, `class="spin"`) {
			t.Errorf("%s: no running label: %s", name, html)
		}
		// Zero running: finished agents leave no spinner label.
		row = build(project, agents[2:]...)
		if row.RunningAgents != 0 {
			t.Errorf("%s: RunningAgents = %d, want 0", name, row.RunningAgents)
		}
		if html := render(row); strings.Contains(html, "prun") {
			t.Errorf("%s: spinner label with nothing running: %s", name, html)
		}
	}
	if html := render(build("/p", agents[0])); !strings.Contains(html, `title="1 subagent"`) || strings.Contains(html, "1 subagents") {
		t.Errorf("singular form: %s", html)
	}
}

// TestPeerRowBranch pins the peer row's branch chip: a served peer carries
// its snapshot's branch (with or without a live adapter) and an unserved one
// none; the chip draws a branch as the session page's git handle does (goal
// and impl marks and the last segment, a detached HEAD by its commit under
// the detached title), and a root that is no repository draws no chip.
func TestPeerRowBranch(t *testing.T) {
	now := testNow
	withRepo := func(repo *wslive.Repo, adapter bool) *wslive.Snapshot {
		snap := liveSnap(now, wslive.Lead{})
		if !adapter {
			snap.Adapter = ""
		}
		snap.Repo = repo
		return snap
	}
	sessions := map[string]Session{
		"impl":   liveSession("impl", "/p", now),
		"plain":  liveSession("plain", "/p", now),
		"detach": liveSession("detach", "/p", now),
		"norepo": liveSession("norepo", "/p", now),
	}
	snaps := map[string]*wslive.Snapshot{
		"impl":   withRepo(&wslive.Repo{Folder: "p", Branch: "impl/goal/develop/epic/leaf-stem"}, true),
		"plain":  withRepo(&wslive.Repo{Folder: "p", Branch: "develop"}, false),
		"detach": withRepo(&wslive.Repo{Folder: "p", Branch: "1a2b3c4d5e", Detached: true}, true),
		"norepo": withRepo(&wslive.Repo{Folder: "p"}, true),
	}
	var contacts []Contact
	for _, id := range []string{"impl", "plain", "detach", "norepo", "unserved"} {
		contacts = append(contacts, Contact{Address: "id:" + id, ReplyID: id, LastExchange: now, PresenceLive: true})
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	rows := map[string]PeerRow{}
	for _, r := range v.Tree.Peers.Rows() {
		rows[r.ReplyID] = r
	}
	if r := rows["impl"]; r.Branch != "impl/goal/develop/epic/leaf-stem" || r.Detached {
		t.Errorf("served peer branch = %q detached %v", r.Branch, r.Detached)
	}
	if r := rows["plain"]; r.Branch != "develop" {
		t.Errorf("a served peer without an adapter lost its branch: %+v", r)
	}
	if r := rows["unserved"]; r.Branch != "" || r.Served {
		t.Errorf("an unserved peer carries a branch: %+v", r)
	}

	chip := func(id string) string {
		html := renderNamed(t, "peerrow", rows[id])
		i := strings.Index(html, `<span class="pbr"`)
		if i < 0 {
			return ""
		}
		j := strings.Index(html[i:], `</span></span>`)
		return html[i : i+j]
	}
	if c := chip("impl"); !strings.Contains(c, `title="impl/goal/develop/epic/leaf-stem"`) || !strings.Contains(c, `class="gh-goal"`) ||
		!strings.Contains(c, `class="gh-impl"`) || !strings.Contains(c, `<span class="pbt">leaf-stem`) {
		t.Errorf("impl chip = %s", c)
	}
	if c := chip("plain"); !strings.Contains(c, `#i-branch`) || !strings.Contains(c, `<span class="pbt">develop`) || strings.Contains(c, "gh-") {
		t.Errorf("plain chip = %s", c)
	}
	if c := chip("detach"); !strings.Contains(c, `title="detached HEAD at 1a2b3c4d5e"`) || !strings.Contains(c, `<span class="pbt">1a2b3c4d5e`) {
		t.Errorf("detached chip = %s", c)
	}
	for _, id := range []string{"norepo", "unserved"} {
		if c := chip(id); c != "" {
			t.Errorf("%s draws a branch chip: %s", id, c)
		}
	}
}
