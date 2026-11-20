// Peer view projection merges and formats peer rows.

package wsdashboard

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// peerGroup merges contacts that resolve to one reply-id into one row and
// applies the retention rule: every live peer and the last endedPeerCap
// ended ones. A served peer's liveness is its own registration; an unserved
// one is live while its presence is (Contact.PresenceLive). project is the
// viewed session's root: only a served peer registered under the same path
// (compared after filepath.Clean, no symlink resolution) keeps its full live
// state; a live served peer elsewhere is a Compact row and its subagent tree
// is never read (only its running count). Live peers list same-worktree first.
func peerGroup(contacts []Contact, project string, lookup sessionLookup, now time.Time) PeerGroup {
	type merged struct {
		row          PeerRow
		named        bool
		presenceLive bool
	}
	var order []string
	rows := map[string]*merged{}
	for _, c := range contacts { // newest first
		key := c.ReplyID
		if key == "" {
			key = "addr:" + c.Address
		}
		m, ok := rows[key]
		if !ok {
			m = &merged{row: PeerRow{Address: c.Address, ReplyID: c.ReplyID}}
			rows[key] = m
			order = append(order, key)
		}
		isNamed := !hasReplyIDPrefix(c.Address)
		if isNamed && !m.named {
			// The most recent named address labels the row.
			m.row.Address, m.named = c.Address, true
		}
		if at := c.LastExchange.UnixMilli(); at > m.row.LastExchange {
			m.row.LastExchange = at
		}
		m.presenceLive = m.presenceLive || c.PresenceLive
		if m.row.Path == "" {
			m.row.Path = c.Cwd
		}
	}
	var group PeerGroup
	for _, key := range order {
		m := rows[key]
		row := m.row
		if row.ReplyID != "" {
			if s, ok := lookup.get(row.ReplyID); ok {
				row.Served = true
				row.Live = s.Live
				if !m.named {
					row.Label = s.Label
				}
				row.Harness = s.Harness
				if s.Project != "" {
					row.Path = s.Project
				}
				row.SameWorktree = s.Project != "" && filepath.Clean(s.Project) == filepath.Clean(project)
				row.Compact = s.Live && !row.SameWorktree
				snap := lookup.snap(row.ReplyID)
				if snap != nil && snap.Repo != nil {
					row.Branch, row.Detached = snap.Repo.Branch, snap.Repo.Detached
				}
				if adapterPresent(snap, now) {
					row.Adapter = true
					if row.Live {
						for _, a := range snap.Agents {
							if (AgentNode{Agent: a}).IsRunning() {
								row.RunningAgents++
							}
						}
					}
					if row.Compact {
						row.Lead = LeadView{Lead: snap.Lead}
					} else {
						if observed := lookup.observedSnap(row.ReplyID); observed != nil {
							snap = observed
						}
						row.Lead = leadView(snap.Lead)
						// One level deep: the peer's own agents, never its peers.
						row.Agents = agentTree(snap.Agents, lookup)
						row.AgentCount = len(snap.Agents)
					}
				}
			}
		}
		if !row.Served {
			row.Live = m.presenceLive
		}
		row.WorktreeMain = worktreeMain(row.Path)
		if row.Live {
			group.Live = append(group.Live, row)
		} else {
			group.Ended = append(group.Ended, row)
		}
	}
	byLast := func(list []PeerRow) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].LastExchange > list[j].LastExchange })
	}
	byLast(group.Live)
	byLast(group.Ended)
	sort.SliceStable(group.Live, func(i, j int) bool { return group.Live[i].SameWorktree && !group.Live[j].SameWorktree })
	if len(group.Ended) > endedPeerCap {
		group.HiddenEnded = len(group.Ended) - endedPeerCap
		group.Ended = group.Ended[:endedPeerCap]
	}
	return group
}

// Name is the row's name: Label when set, else Address.
func (r PeerRow) Name() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Address
}

// Title is the row name's tooltip: the id: Address when Label stands in for
// it, then Path, one per line; empty when neither applies.
func (r PeerRow) Title() string {
	var lines []string
	if r.Label != "" {
		lines = append(lines, r.Address)
	}
	if r.Path != "" {
		lines = append(lines, r.Path)
	}
	return strings.Join(lines, "\n")
}

// BranchHandle is the row's branch chip label, as the session page's git
// handle draws a branch (branchHandle); the zero label when Branch is empty.
func (r PeerRow) BranchHandle() GitHandle {
	if r.Branch == "" {
		return GitHandle{}
	}
	return branchHandle(r.Branch, r.Detached)
}

// Folder is the row's label prefix: Path's last element, as
// "<main>(<folder>)" when Path is a linked worktree of the repository at
// WorktreeMain (peerFolderLabel). Empty when Path is.
func (r PeerRow) Folder() string { return peerFolderLabel(r.Path, r.WorktreeMain) }

// peerFolderLabel is "<main>(<folder>)" when main, a worktree's main
// repository root, is known, else path's last element (lastElem).
func peerFolderLabel(path, main string) string {
	folder := lastElem(path)
	if m := lastElem(main); m != "" && folder != "" {
		return m + "(" + folder + ")"
	}
	return folder
}

// lastElem is p's last element, split on "/" and "\" alike (a Windows cwd
// can reach a daemon on another OS), trailing separators ignored.
func lastElem(p string) string {
	p = strings.TrimRight(p, `/\`)
	return p[strings.LastIndexAny(p, `/\`)+1:]
}

func hasReplyIDPrefix(address string) bool {
	return len(address) >= len(wsmailbox.ReplyIDPrefix) && address[:len(wsmailbox.ReplyIDPrefix)] == wsmailbox.ReplyIDPrefix
}
