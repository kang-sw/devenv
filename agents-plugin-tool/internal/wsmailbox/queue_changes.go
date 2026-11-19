package wsmailbox

import "slices"

// copyQueues clones queues one level deep: each queue's slice is copied, so
// a mutation of the copy never reaches the original's envelopes.
func copyQueues(queues map[string][]Envelope) map[string][]Envelope {
	copy := make(map[string][]Envelope, len(queues))
	for name, queue := range queues {
		copy[name] = slices.Clone(queue)
	}
	return copy
}
