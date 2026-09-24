package wsindex

import (
	"fmt"
	"time"
)

// Apply returns the Replayer for the record model: it applies one entry to an
// index version at time now. Every operation is idempotent, so replaying an
// entry that already landed (a crash between the remote push and the local
// clear) changes nothing and reports nothing.
func Apply(now time.Time) Replayer {
	return func(idx *Index, e PendingEntry) ([]string, string) {
		switch e.Op {
		case OpRegister:
			idx.Register(e.Stem, now)
			return nil, ""
		default:
			return nil, fmt.Sprintf("%s: unsupported ticket-index operation %q dropped", e.Stem, e.Op)
		}
	}
}
