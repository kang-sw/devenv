// Board database snapshots and updates.
package wswhiteboard

import (
	"bytes"
	"database/sql"
	"fmt"
	"reflect"
	"slices"
)

// State returns a detached copy of the complete in-memory board.
func (b *BoardDB) Snapshot() (*State, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == nil {
		return nil, uint64(b.generation)
	}
	return b.state.clone(), uint64(b.generation)
}

func (b *BoardDB) Revision() uint64 { b.mu.Lock(); defer b.mu.Unlock(); return uint64(b.generation) }

func (b *BoardDB) State() *State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == nil {
		return nil
	}
	return b.state.clone()
}

// Update applies one in-memory mutation and commits only changed rows in one
// transaction. A byte-for-byte unchanged State opens no transaction.
func (b *BoardDB) Update(fn func(*State) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("whiteboard board DB is closed")
	}
	return b.updateLocked(fn)
}

func (b *BoardDB) updateLocked(fn func(*State) error) error {
	work := b.state.clone()
	if err := fn(work); err != nil {
		return err
	}
	work.MetadataRevision = max(work.MetadataRevision, b.state.MetadataRevision)
	if work.StaleNudgeRevision == "" {
		work.StaleNudgeRevision = b.state.StaleNudgeRevision
	}
	if reflect.DeepEqual(b.state, work) {
		return nil
	}
	beforeRows, err := boardRows(b.state, b.topicDirs, b.nextTopic, nil)
	if err != nil {
		return err
	}
	afterRows, err := boardRows(work, b.topicDirs, b.nextTopic, b.state)
	if err != nil {
		return err
	}
	if err := b.hit("transaction-begin"); err != nil {
		return err
	}
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	nextRevisionSlot := 1
	if b.revisionSlot == 1 {
		nextRevisionSlot = 2
	}
	if err := boardWriteDelta(tx, beforeRows, afterRows, b.generation+1, b.importedGeneration, nextRevisionSlot, b.state, work); err != nil {
		return err
	}
	if err := b.hit("transaction-before-commit"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if work.MetadataRevision != b.state.MetadataRevision || work.StaleNudgeRevision != b.state.StaleNudgeRevision {
		b.revisionSlot = nextRevisionSlot
	}
	b.generation++
	b.nextTopic = afterRows.NextTopic
	b.topicDirs = afterRows.Dirs
	b.state = work.clone()
	b.armCheckpointLocked()
	return nil
}

func boardWriteDelta(tx *sql.Tx, before, after boardDBRows, generation, imported, revisionSlot int, beforeState, afterState *State) error {
	for dir, old := range before.Topics {
		if _, ok := after.Topics[dir]; !ok {
			if len(old.Entries) != 0 {
				return fmt.Errorf("whiteboard: cannot remove topic entry history")
			}
			if _, err := tx.Exec(`DELETE FROM topics WHERE topic_dir=?`, dir); err != nil {
				return err
			}
		}
	}
	oldEntries := map[string]boardDBEntryRow{}
	for _, topic := range before.Topics {
		for _, entry := range topic.Entries {
			oldEntries[entry.ID] = entry
		}
	}
	afterEntries := map[string]bool{}
	for dir, row := range after.Topics {
		old, exists := before.Topics[dir]
		if !exists || old.Key != row.Key || old.Position != row.Position || !bytes.Equal(old.Data, row.Data) {
			if _, err := tx.Exec(`INSERT INTO topics(topic_dir,topic_key,position,topic_json) VALUES(?,?,?,?) ON CONFLICT(topic_dir) DO UPDATE SET topic_key=excluded.topic_key,position=excluded.position,topic_json=excluded.topic_json`, row.Dir, row.Key, row.Position, row.Data); err != nil {
				return err
			}
		}
		for _, entry := range row.Entries {
			afterEntries[entry.ID] = true
			prior, exists := oldEntries[entry.ID]
			if exists {
				if prior.TopicDir != row.Dir || !bytes.Equal(prior.Data, entry.Data) || (prior.PanelBody == nil) != (entry.PanelBody == nil) || prior.PanelBody != nil && *prior.PanelBody != *entry.PanelBody {
					return fmt.Errorf("whiteboard topic %q: entries are append-only", row.Key)
				}
				continue
			}
			if entry.Seq <= beforeState.EntrySeq {
				return fmt.Errorf("whiteboard: new entry reuses sequence %q", entry.ID)
			}
			if _, err := tx.Exec(`INSERT INTO entries(entry_id,entry_seq,topic_dir,entry_json) VALUES(?,?,?,?)`, entry.ID, entry.Seq, row.Dir, entry.Data); err != nil {
				return err
			}
			if entry.PanelBody != nil {
				if _, err := tx.Exec(`INSERT INTO panels(entry_id,body) VALUES(?,?)`, entry.ID, *entry.PanelBody); err != nil {
					return err
				}
			}
		}
	}
	for id := range oldEntries {
		if !afterEntries[id] {
			return fmt.Errorf("whiteboard: entries are append-only; missing %s", id)
		}
	}
	if afterState.EntrySeq < beforeState.EntrySeq || afterState.PanelCounter < beforeState.PanelCounter {
		return fmt.Errorf("whiteboard: entry/panel counters cannot decrease")
	}
	if _, err := tx.Exec(`UPDATE board_meta SET generation=?,imported_generation=?,next_topic=?,state_json=?,item_order_json=? WHERE id=1`, generation, imported, after.NextTopic, after.State, after.ItemOrder); err != nil {
		return err
	}
	if afterState.MetadataRevision != beforeState.MetadataRevision || afterState.StaleNudgeRevision != beforeState.StaleNudgeRevision {
		if _, err := tx.Exec(`INSERT INTO revision_receipts(slot,generation,metadata_revision,stale_nudge_revision) VALUES(?,?,?,?) ON CONFLICT(slot) DO UPDATE SET generation=excluded.generation,metadata_revision=excluded.metadata_revision,stale_nudge_revision=excluded.stale_nudge_revision`, revisionSlot, generation, afterState.MetadataRevision, afterState.StaleNudgeRevision); err != nil {
			return err
		}
	}
	if afterState.LoadedAt != "" && afterState.LoadedAt != beforeState.LoadedAt {
		if _, err := tx.Exec(`INSERT INTO load_floors(generation,loaded_at) VALUES(?,?)`, generation, afterState.LoadedAt); err != nil {
			return err
		}
	}
	return nil
}

func boardRows(state *State, known map[string]string, nextTopic int, before *State) (boardDBRows, error) {
	rows := boardDBRows{Topics: map[string]boardDBTopicRow{}, Dirs: map[string]string{}, NextTopic: nextTopic}
	global := state.clone()
	global.Items, global.Topics, global.Labels, global.CloseResults = nil, nil, nil, nil
	rows.State = folderJSON(global)
	order := make([]int, 0, len(state.Items))
	for _, item := range state.Items {
		order = append(order, item.ID)
	}
	rows.ItemOrder = folderJSON(order)
	keys := folderKeys(state)
	used := map[string]bool{}
	entryIDs := map[string]bool{}
	panelOwners := map[string]string{}
	for pos, key := range keys {
		dir := ""
		if before != nil {
			for oldKey, oldDir := range known {
				if !used[oldDir] && !slices.Contains(keys, oldKey) && before.topicKnown(oldKey) && folderSameTopic(before, state, oldKey, key) {
					dir = oldDir
					break
				}
			}
		}
		if dir == "" {
			dir = known[key]
		}
		if dir == "" {
			rows.NextTopic++
			dir = fmt.Sprintf("t%d", rows.NextTopic)
		}
		if !folderDirID(dir) || used[dir] {
			return rows, fmt.Errorf("whiteboard: invalid or duplicate topic mapping")
		}
		used[dir] = true
		rows.Dirs[key] = dir
		topic := boardDBTopic{Closed: slices.Contains(state.ClosedTopics, key)}
		if label, ok := state.Labels[key]; ok {
			topic.Label = &label
		}
		if result, ok := state.CloseResults[key]; ok {
			topic.CloseResult = &result
		}
		for _, item := range state.Items {
			if item.Topic == key {
				topic.Items = append(topic.Items, item)
			}
		}
		var entries []Entry
		if doc := folderDoc(state, key); doc != nil {
			copy := *doc
			entries = copy.Entries
			copy.Entries = nil
			topic.Doc = &copy
		}
		row := boardDBTopicRow{Dir: dir, Key: key, Position: pos, Data: folderJSON(topic)}
		last := 0
		for _, entry := range entries {
			seq := folderNumber(entry.ID, "e")
			if seq <= last || seq > state.EntrySeq || entryIDs[entry.ID] {
				return rows, fmt.Errorf("whiteboard: invalid entry sequence %q", entry.ID)
			}
			last = seq
			entryIDs[entry.ID] = true
			if err := folderValidEntry(entry, state.PanelCounter); err != nil {
				return rows, err
			}
			if entry.Kind == EntryPanel {
				if owner := panelOwners[entry.PanelID]; owner != "" && owner != key {
					return rows, fmt.Errorf("whiteboard: panel %s belongs to multiple topics", entry.PanelID)
				}
				panelOwners[entry.PanelID] = key
			}
			er := boardDBEntryRow{Seq: seq, ID: entry.ID, TopicDir: dir}
			if entry.Kind == EntryPanel {
				body := entry.Text
				er.PanelBody = &body
				entry.Text = ""
			}
			er.Data = folderJSON(entry)
			row.Entries = append(row.Entries, er)
		}
		rows.Topics[dir] = row
	}
	return rows, nil
}
