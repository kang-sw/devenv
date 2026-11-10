package wsdashboard

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The session rail: the card every session page shows at its left (from a
// wide viewport always, else as an overlay the top-left arrow opens), listing
// the sessions this daemon serves grouped by workspace, then worktree. It is
// one shared render for every session page, keyed by language like the
// index: nothing in it depends on the viewed session, and the page marks its
// own row on the client. Like the session view it is built by a pure
// function from what the daemon read (buildRailView), so every rule is
// testable without a server; the reads live in railReader below, the
// worktree lists in railworktrees.go, and the last messages in
// railmessages.go.
//
// A workspace is a repository (its root worktree) or a plain non-git
// folder. Under a repository every worktree git lists is a row, a worktree
// with no session dimmed, except the ws pooled worktrees (under the
// repository's worktree_pool roots), which show only while they hold a
// listed session. The rail shows nothing the index and the session pages do
// not already serve to the same viewers.

// railMessageRunes bounds a row's last message; the row shows one line.
const railMessageRunes = 160

// RailView is what the rail region renders.
type RailView struct {
	Workspaces []RailWorkspace
	// Live counts the live sessions; CountPolicy sums their open policy
	// items.
	Live, CountPolicy int
}

// RailWorkspace is one repository or plain folder.
type RailWorkspace struct {
	// Name is Path's last element; Path the repository root or folder.
	Name, Path string
	// Repo marks a repository: its sessions sit under Worktrees. A plain
	// folder lists them directly in Sessions and Ended.
	Repo bool
	// Live counts the workspace's live sessions.
	Live      int
	Worktrees []RailWorktree
	// Sessions are the live sessions, Ended the ended ones still inside
	// endedSessionWindow, of a plain folder.
	Sessions, Ended []RailRow
}

// RailWorktree is one worktree of a repository.
type RailWorktree struct {
	Path string
	// Root marks the repository's root worktree; Folder is Path's last
	// element (the page shows "root" for the root worktree instead).
	Root   bool
	Folder string
	// Branch is the checked-out branch, short form; Detached marks a
	// detached HEAD (Branch then empty).
	Branch   string
	Detached bool
	// Changes is the working-tree entry count (modified, staged, and
	// untracked) of the newest snapshot of a session in the worktree;
	// HasChanges is false when no such snapshot exists, and the page then
	// shows no count. The rail never reads a working tree itself.
	Changes    int
	HasChanges bool
	// Sessions are the live sessions, Ended the ended ones still inside
	// endedSessionWindow.
	Sessions, Ended []RailRow
}

// Empty reports a worktree that holds no listed session; the page dims it.
func (w RailWorktree) Empty() bool { return len(w.Sessions) == 0 && len(w.Ended) == 0 }

// RailRow is one session.
type RailRow struct {
	ReplyID, Harness string
	Live             bool
	// Working reports a lead at work: a live adapter and a phase other than
	// idle and permission (a permission wait is the lead waiting on the
	// user). The index's working total counts permission waits; the rail
	// does not.
	Working bool
	// Subagents counts the running agents the lead itself spawned (level 1),
	// zero without a live adapter.
	Subagents int
	// CountPolicy is the board's open policy items.
	CountPolicy int
	// LastTool is the time of the lead's last tool call (epoch ms; 0 when
	// unknown). An ended session's stale snapshot still supplies it.
	LastTool int64
	// Message is the session's last user message or lead reply, one line;
	// User marks a user message.
	Message string
	User    bool
}

// railInput is one listed session with what the rail shows of it.
type railInput struct {
	Session Session
	// Snapshot is the session's snapshot file, stale or not; nil when it has
	// none.
	Snapshot *wslive.Snapshot
	// Repo is the session's repository root (railRepoRoot), empty for a
	// plain folder. Linked reports a session root that is a linked worktree
	// (its .git is a file), which a missing worktree list cannot place
	// under the root worktree.
	Repo    string
	Linked  bool
	Message railMessage
}

// railMessage is a session's last conversation message as the rail shows it.
type railMessage struct {
	Text string
	User bool
}

// railRepo is what the worktree lister holds of one repository.
type railRepo struct {
	// Read reports a successful list; Entries are its worktrees, the root
	// worktree first.
	Read    bool
	Entries []railWorktreeEntry
	// Pools are the repository's worktree pool roots.
	Pools []string
}

// railWorktreeEntry is one worktree git lists.
type railWorktreeEntry struct {
	Path string
	// Branch is the short branch name; empty with Detached, or a bare entry.
	Branch   string
	Detached bool
	Prunable bool
}

// buildRailView groups the listed sessions by workspace and worktree. A
// session is listed while live or while it was last seen inside
// endedSessionWindow. repos holds the worktree lists read so far, by
// repository root; a repository without one shows the worktrees its
// sessions are rooted in.
func buildRailView(sessions []railInput, repos map[string]railRepo, now time.Time) RailView {
	var view RailView
	type member struct {
		in  railInput
		row RailRow
	}
	byWS := map[string][]member{}
	repoOf := map[string]bool{}
	for _, in := range sessions {
		reg := in.Session.Registration
		live := reg.Live(now)
		if !live && now.Sub(reg.LastSeen()) > endedSessionWindow {
			continue
		}
		row := railRow(in, live, now)
		key := filepath.Clean(reg.Project)
		if in.Repo != "" {
			key = filepath.Clean(in.Repo)
			repoOf[key] = true
		}
		byWS[key] = append(byWS[key], member{in: in, row: row})
		if live {
			view.Live++
			view.CountPolicy += row.CountPolicy
		}
	}
	keys := make([]string, 0, len(byWS))
	for k := range byWS {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		members := byWS[key]
		ws := RailWorkspace{Name: lastElem(key), Path: key, Repo: repoOf[key]}
		for _, m := range members {
			if m.row.Live {
				ws.Live++
			}
		}
		if !ws.Repo {
			regs := map[string]Registration{}
			for _, m := range members {
				regs[m.row.ReplyID] = m.in.Session.Registration
				if m.row.Live {
					ws.Sessions = append(ws.Sessions, m.row)
				} else {
					ws.Ended = append(ws.Ended, m.row)
				}
			}
			sortRailRows(ws.Sessions, regs)
			sortRailEnded(ws.Ended, regs)
			view.Workspaces = append(view.Workspaces, ws)
			continue
		}
		repo := repos[key]
		entries := repo.Entries
		if !repo.Read {
			entries = nil
		}
		// Each session sits under the listed worktree with the longest path
		// prefix of its root. A root no listed worktree holds (no list read
		// yet, or a worktree added since) sits under the root worktree when
		// it is a folder inside it, else is a worktree of its own.
		type wtAcc struct {
			wt   RailWorktree
			snap *wslive.Snapshot
			regs map[string]Registration
		}
		accs := map[string]*wtAcc{}
		var order []string
		addWT := func(e railWorktreeEntry) *wtAcc {
			p := filepath.Clean(e.Path)
			if a, ok := accs[p]; ok {
				return a
			}
			a := &wtAcc{wt: RailWorktree{Path: p, Root: p == key, Folder: lastElem(p), Branch: e.Branch, Detached: e.Detached}, regs: map[string]Registration{}}
			accs[p] = a
			order = append(order, p)
			return a
		}
		for _, m := range members {
			project := filepath.Clean(m.in.Session.Project)
			best := ""
			for _, e := range entries {
				p := filepath.Clean(e.Path)
				if railPathUnder(p, project) && len(p) > len(best) {
					best = p
				}
			}
			var a *wtAcc
			if best != "" {
				for _, e := range entries {
					if filepath.Clean(e.Path) == best {
						a = addWT(e)
						break
					}
				}
			} else if railPathUnder(key, project) && !m.in.Linked {
				a = addWT(railWorktreeEntry{Path: key})
			} else {
				a = addWT(railWorktreeEntry{Path: project})
			}
			if m.row.Live {
				a.wt.Sessions = append(a.wt.Sessions, m.row)
			} else {
				a.wt.Ended = append(a.wt.Ended, m.row)
			}
			a.regs[m.row.ReplyID] = m.in.Session.Registration
			if s := m.in.Snapshot; s != nil && s.Repo != nil && (a.snap == nil || s.UpdatedAt > a.snap.UpdatedAt) {
				a.snap = s
			}
		}
		// Every other listed worktree shows too, dimmed, except a pooled
		// one (it shows only while it holds a session, above) and one git
		// marks prunable (its folder is gone).
		for i, e := range entries {
			p := filepath.Clean(e.Path)
			if _, ok := accs[p]; ok || e.Prunable {
				continue
			}
			if i > 0 && p != key && railUnderAny(repo.Pools, p) {
				continue
			}
			addWT(e)
		}
		for _, p := range order {
			a := accs[p]
			if a.snap != nil {
				r := a.snap.Repo
				a.wt.Changes, a.wt.HasChanges = r.Modified+r.Staged+r.Untracked, true
				if a.wt.Branch == "" && !a.wt.Detached {
					// A worktree no list names yet takes its branch from the
					// snapshot.
					a.wt.Branch, a.wt.Detached = r.Branch, r.Detached
				}
			}
			sortRailRows(a.wt.Sessions, a.regs)
			sortRailEnded(a.wt.Ended, a.regs)
			ws.Worktrees = append(ws.Worktrees, a.wt)
		}
		sort.SliceStable(ws.Worktrees, func(i, j int) bool {
			a, b := ws.Worktrees[i], ws.Worktrees[j]
			if a.Root != b.Root {
				return a.Root
			}
			return a.Path < b.Path
		})
		view.Workspaces = append(view.Workspaces, ws)
	}
	return view
}

// sortRailRows orders live sessions by registration, newest first: the
// order does not move with heartbeats or activity, so a row stays where the
// user last saw it (starred rows are pinned on the client).
func sortRailRows(rows []RailRow, regs map[string]Registration) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := regs[rows[i].ReplyID].RegisteredAt, regs[rows[j].ReplyID].RegisteredAt
		if a != b {
			return a > b
		}
		return rows[i].ReplyID < rows[j].ReplyID
	})
}

// sortRailEnded orders ended sessions by last seen, newest first.
func sortRailEnded(rows []RailRow, regs map[string]Registration) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := regs[rows[i].ReplyID].LastSeen(), regs[rows[j].ReplyID].LastSeen()
		if !a.Equal(b) {
			return a.After(b)
		}
		return rows[i].ReplyID < rows[j].ReplyID
	})
}

// railRow is one session's row.
func railRow(in railInput, live bool, now time.Time) RailRow {
	reg := in.Session.Registration
	row := RailRow{
		ReplyID:     reg.ReplyID,
		Harness:     reg.Harness,
		Live:        live,
		CountPolicy: in.Session.CountPolicy,
		Message:     in.Message.Text,
		User:        in.Message.User,
	}
	snap := in.Snapshot
	if snap == nil {
		return row
	}
	row.LastTool = lastToolAt(snap.Lead)
	if adapterPresent(snap, now) {
		row.Working = leadWorking(snap.Lead.Phase)
		row.Subagents = levelOneRunning(snap.Agents)
	}
	return row
}

// leadWorking reports a phase in which the lead works: anything but idle
// (or none) and a permission wait.
func leadWorking(phase string) bool {
	return phase != "" && phase != wslive.PhaseIdle && phase != wslive.PhasePermission
}

// lastToolAt is the time of the lead's last tool call: over its batches
// that called a tool, the running call's start, else a finished batch's end
// (its start when no end is known). A message still generating is no call.
func lastToolAt(lead wslive.Lead) int64 {
	var last int64
	for _, b := range lead.Batches {
		var t int64
		switch {
		case b.Running != nil:
			t = b.Running.StartedAt
			if t == 0 {
				t = b.StartedAt
			}
		case b.Generating != nil || b.CallCount == 0:
			continue
		default:
			t = b.EndedAt
			if t == 0 {
				t = b.StartedAt
			}
		}
		last = max(last, t)
	}
	return last
}

// levelOneRunning counts the running agents the lead spawned itself. Both
// adapters (Claude and Pi) send a lead-spawned agent without a parent id, so
// that is level 1; an agent whose parent left the list (retention keeps
// every running agent, so only a malformed snapshot does this) counts as
// level 1 too, as subagentCounts counts it.
func levelOneRunning(agents []wslive.Agent) int {
	return subagentCounts("", agents, sessionLookup{}, time.Time{}).Direct
}

// railPathUnder reports whether child is at or below parent, at directory
// boundaries.
func railPathUnder(parent, child string) bool {
	if parent == child {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(child, strings.TrimSuffix(parent, sep)+sep)
}

// railUnderAny reports whether path is at or below one of roots.
func railUnderAny(roots []string, path string) bool {
	for _, r := range roots {
		if r != "" && railPathUnder(filepath.Clean(r), path) {
			return true
		}
	}
	return false
}

// railRepoRoot is the repository root of a session: from its registered git
// common directory when it has one (a non-bare repository's common
// directory is <root>/.git; a bare repository's or a submodule's is not,
// and such a session shows as a plain folder), else from its root's .git
// read without running git (railFallbackRoot). Empty for a plain folder.
func railRepoRoot(reg Registration) string {
	if c := reg.GitCommonDir; c != "" {
		c = filepath.Clean(c)
		if filepath.Base(c) != ".git" {
			return ""
		}
		return filepath.Dir(c)
	}
	return railFallbackRoot(reg.Project)
}

// railFallbacks memoizes railFallbackRoot per path, as worktreeMains does.
var railFallbacks sync.Map // path -> string

// railFallbackRoot is the repository root of path for a registration
// without a common directory: its main repository when path is a linked
// worktree (worktreeMain), path itself when it holds a .git directory, else
// empty. Memoized; git is never run.
func railFallbackRoot(path string) string {
	if path == "" {
		return ""
	}
	if v, ok := railFallbacks.Load(path); ok {
		return v.(string)
	}
	root := worktreeMain(path)
	if root == "" {
		if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
			root = filepath.Clean(path)
		}
	}
	railFallbacks.Store(path, root)
	return root
}
