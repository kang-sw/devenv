package wslive

// ForDisk returns the mutable recovery-cache projection. Live reasoning summaries
// belong only to RAM/live delivery until their natural immutable RingTurn commit.
// Keep ordinary JSON encoding unchanged for in-memory HostedSlot publication.
func (s Snapshot) ForDisk() Snapshot {
	s.Lead.ReasoningSummary = nil
	return s
}
