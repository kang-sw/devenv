package wsdashboard

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
