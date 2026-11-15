package wsdashboard

import "strings"

// Classification partitions presentation only. The full inventory stays in
// Worktrees for the existing layout facts, ordering and session-tree contracts.
func (w RailWorkspace) PendingWorktrees() []RailWorktree  { return w.admissionWorktrees("pending") }
func (w RailWorkspace) RejectedWorktrees() []RailWorktree { return w.admissionWorktrees("rejected") }
func (w RailWorkspace) AdmittedWorktrees() []RailWorktree { return w.admissionWorktrees("admitted") }
func (w RailWorkspace) admissionWorktrees(state string) []RailWorktree {
	var out []RailWorktree
	for _, wt := range w.Worktrees {
		if wt.Admission == state || (state == "admitted" && wt.Admission == "") {
			out = append(out, wt)
		}
	}
	return out
}

// RejectedPrompt is the cumulative investigation prompt for the rejected rows
// this render displays; the rejected-list header copies it, and the admission
// route answers an Apply with the same text.
func (v RailView) RejectedPrompt() string {
	var visible []railAdmissionRecord
	for _, ws := range v.Workspaces {
		for _, wt := range ws.RejectedWorktrees() {
			root, path := wt.AdmissionRepository, wt.AdmissionPath
			if root == "" {
				root = ws.Path
			}
			if path == "" {
				path = wt.Path
			}
			visible = append(visible, railAdmissionRecord{Repository: root, Path: path, Origin: wt.Origin, State: "rejected", Branch: wt.Branch})
		}
	}
	return railRejectedPrompt(visible)
}

// ShortPath is Path with its middle folders elided, keeping the first two and
// the last ("/var/folders/…/tmp.Xk2"): a detected worktree often sits deep in a
// temporary folder, and the row's title keeps the full path.
func (w RailWorktree) ShortPath() string {
	sep := "/"
	if !strings.Contains(w.Path, "/") && strings.Contains(w.Path, `\`) {
		sep = `\`
	}
	parts := strings.Split(w.Path, sep)
	lead := 2
	if parts[0] == "" {
		lead = 3 // an absolute path's empty first element
	}
	if len(parts) <= lead+2 {
		return w.Path
	}
	return strings.Join(parts[:lead], sep) + sep + "…" + sep + parts[len(parts)-1]
}
