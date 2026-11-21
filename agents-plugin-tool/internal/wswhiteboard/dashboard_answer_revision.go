package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// dashboardAnswerRevisionPrefix distinguishes an owner answer arrival from
// ordinary system activity. The suffix is a JSON-encoded durable inbox ID.
const dashboardAnswerRevisionPrefix = "Dashboard answer received: "

// RecordDashboardAnswer records an inbox answer once in the board's immutable
// stream. A missing or empty board needs no marker: it has no topics to nudge.
// The inbox ID must be stable across retries and is scoped to this board.
func (f FolderStore) RecordDashboardAnswer(inboxID string, now time.Time) error {
	if strings.TrimSpace(inboxID) == "" {
		return fmt.Errorf("whiteboard: empty dashboard answer ID")
	}
	present := false
	for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
		if _, err := wsstate.Stat(filepath.Join(f.Dir, name)); err == nil {
			present = true
			break
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if !present {
		return nil
	}
	encoded, _ := json.Marshal(inboxID)
	marker := dashboardAnswerRevisionPrefix + string(encoded)
	return f.withLock(false, func() error {
		before, meta, err := f.load(nil)
		if err != nil || before == nil {
			return err
		}
		work := before.clone()
		if !recordDashboardAnswer(work, marker, now) {
			return nil
		}
		intent, err := f.plan(meta, before, work)
		if err != nil {
			return err
		}
		return f.commit(intent)
	})
}

func (b *BoardDB) RecordDashboardAnswer(inboxID string, now time.Time) error {
	if strings.TrimSpace(inboxID) == "" {
		return fmt.Errorf("whiteboard: empty dashboard answer ID")
	}
	encoded, _ := json.Marshal(inboxID)
	marker := dashboardAnswerRevisionPrefix + string(encoded)
	return b.Update(func(work *State) error { recordDashboardAnswer(work, marker, now); return nil })
}

func recordDashboardAnswer(work *State, marker string, now time.Time) bool {
	if loaded, err := time.Parse(time.RFC3339Nano, work.LoadedAt); err == nil && now.UnixMilli() < loaded.UnixMilli() {
		return false
	}
	for _, doc := range work.Topics {
		for _, entry := range doc.Entries {
			if entry.Kind == EntrySystem && entry.Text == marker {
				return false
			}
		}
	}
	if len(work.Topics) == 0 {
		return false
	}
	work.appendEntry(work.Topics[0].Name, Entry{Kind: EntrySystem, Text: marker}, now)
	return true
}
