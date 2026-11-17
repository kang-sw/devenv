package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		if _, err := os.Stat(filepath.Join(f.Dir, name)); err == nil {
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
		for _, doc := range before.Topics {
			for _, entry := range doc.Entries {
				if entry.Kind == EntrySystem && entry.Text == marker {
					return nil
				}
			}
		}
		if len(before.Topics) == 0 {
			return nil
		}
		work := before.clone()
		work.appendEntry(work.Topics[0].Name, Entry{Kind: EntrySystem, Text: marker}, now)
		intent, err := f.plan(meta, before, work)
		if err != nil {
			return err
		}
		return f.commit(intent)
	})
}
