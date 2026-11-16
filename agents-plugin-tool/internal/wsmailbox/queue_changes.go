package wsmailbox

import (
	"maps"
	"slices"

	"github.com/kang-sw/devenv/internal/wswake"
)

// Snapshot only the queues already read under the writer lock. Presence and
// contact heartbeats must not schedule mailbox maintenance or new disk reads.
func copyQueues(queues map[string][]Envelope) map[string][]Envelope {
	copy := make(map[string][]Envelope, len(queues))
	for name, queue := range queues {
		copy[name] = slices.Clone(queue)
	}
	return copy
}

func publishQueueChange(path string, before, after map[string][]Envelope) {
	if !maps.EqualFunc(before, after, func(a, b []Envelope) bool { return slices.Equal(a, b) }) {
		wswake.Mailbox.StoreChanged(path)
	}
}
