// Board database import and state loading.
package wswhiteboard

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
)

func (b *BoardDB) importState(state *State, meta *folderSession, legacy FolderStore) error {
	known := make(map[string]string, len(meta.Topics))
	for _, ref := range meta.Topics {
		known[ref.Key] = ref.Dir
	}
	rows, err := boardRows(state, known, meta.NextTopic, nil)
	if err != nil {
		return err
	}
	// The board row mirrors meta.json, while load_floors carries the recovery
	// override separately. Recovery applies the committed receipt and floor rows.
	metaState := meta.State.clone()
	metaState.Items, metaState.Topics, metaState.Labels, metaState.CloseResults = nil, nil, nil, nil
	rows.State = folderJSON(metaState)
	rows.ItemOrder = folderJSON(meta.ItemOrder)
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := boardInsertAll(tx, rows, meta.Generation, meta.Generation); err != nil {
		return err
	}
	latestRevisionGeneration := 0
	for slot := 1; slot <= 2; slot++ {
		var record folderRevisionFile
		exists, err := folderDecode(filepath.Join(legacy.Dir, folderRevisionDir, "panels", fmt.Sprintf("e%d.json", slot)), &record)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO revision_receipts(slot,generation,metadata_revision,stale_nudge_revision) VALUES(?,?,?,?)`, slot, record.Generation, record.Revision.MetadataRevision, record.Revision.StaleNudgeRevision); err != nil {
			return err
		}
		if record.Generation <= meta.Generation && record.Generation > latestRevisionGeneration {
			latestRevisionGeneration = record.Generation
			b.revisionSlot = slot
		}
	}
	floors, err := boardReadLoadFloors(legacy.Dir)
	if err != nil {
		return err
	}
	for _, floor := range floors {
		// FolderStore chooses the first record at a generation. Duplicate
		// generations are valid legacy input, including identical replay bytes.
		if _, err := tx.Exec(`INSERT INTO load_floors(generation,loaded_at) VALUES(?,?) ON CONFLICT(generation) DO NOTHING`, floor.Generation, floor.LoadedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.state = state.clone()
	b.generation = meta.Generation
	b.importedGeneration = meta.Generation
	b.nextTopic = rows.NextTopic
	b.topicDirs = rows.Dirs
	return nil
}

func boardReadLoadFloors(dir string) ([]folderLoadFloor, error) {
	data, _, err := folderReadOptional(filepath.Join(dir, filepath.FromSlash(folderLoadFloorLog)))
	if err != nil {
		return nil, err
	}
	var out []folderLoadFloor
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var floor folderLoadFloor
		if err := json.Unmarshal(line, &floor); err != nil {
			return nil, err
		}
		out = append(out, floor)
	}
	return out, nil
}

func boardInsertAll(tx *sql.Tx, rows boardDBRows, generation, imported int) error {
	if _, err := tx.Exec(`INSERT INTO board_meta(id,schema_version,generation,imported_generation,next_topic,state_json,item_order_json) VALUES(1,?,?,?,?,?,?)`, boardDBSchemaVersion, generation, imported, rows.NextTopic, rows.State, rows.ItemOrder); err != nil {
		return err
	}
	dirs := make([]string, 0, len(rows.Topics))
	for dir := range rows.Topics {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool { return rows.Topics[dirs[i]].Position < rows.Topics[dirs[j]].Position })
	for _, dir := range dirs {
		row := rows.Topics[dir]
		if _, err := tx.Exec(`INSERT INTO topics(topic_dir,topic_key,position,topic_json) VALUES(?,?,?,?)`, row.Dir, row.Key, row.Position, row.Data); err != nil {
			return err
		}
		for _, entry := range row.Entries {
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
	return nil
}

func (b *BoardDB) load() error {
	var schema int
	var stateJSON, itemOrderJSON []byte
	if err := b.db.QueryRow(`SELECT schema_version,generation,imported_generation,next_topic,state_json,item_order_json FROM board_meta WHERE id=1`).Scan(&schema, &b.generation, &b.importedGeneration, &b.nextTopic, &stateJSON, &itemOrderJSON); err != nil {
		return err
	}
	if schema != boardDBSchemaVersion {
		return fmt.Errorf("unsupported board DB schema_version %d", schema)
	}
	var state State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return err
	}
	var itemOrder []int
	if err := json.Unmarshal(itemOrderJSON, &itemOrder); err != nil {
		return err
	}
	rows, err := b.db.Query(`SELECT topic_dir,topic_key,topic_json FROM topics ORDER BY position`)
	if err != nil {
		return err
	}
	type storedTopic struct {
		dir  string
		key  string
		data []byte
	}
	var storedTopics []storedTopic
	for rows.Next() {
		var topic storedTopic
		if err := rows.Scan(&topic.dir, &topic.key, &topic.data); err != nil {
			rows.Close()
			return err
		}
		storedTopics = append(storedTopics, topic)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	items := map[int]Item{}
	for _, stored := range storedTopics {
		dir, key, data := stored.dir, stored.key, stored.data
		var topic boardDBTopic
		if err := json.Unmarshal(data, &topic); err != nil {
			return err
		}
		if topic.Doc != nil && topic.Doc.Name != key {
			return fmt.Errorf("whiteboard board DB topic %s: key mismatch", dir)
		}
		entryRows, err := b.db.Query(`SELECT e.entry_json,p.body FROM entries e LEFT JOIN panels p ON p.entry_id=e.entry_id WHERE e.topic_dir=? ORDER BY e.entry_seq`, dir)
		if err != nil {
			return err
		}
		var entries []Entry
		for entryRows.Next() {
			var data []byte
			var panel sql.NullString
			if err := entryRows.Scan(&data, &panel); err != nil {
				entryRows.Close()
				return err
			}
			var entry Entry
			if err := json.Unmarshal(data, &entry); err != nil {
				entryRows.Close()
				return err
			}
			if entry.Kind == EntryPanel {
				if !panel.Valid {
					entryRows.Close()
					return fmt.Errorf("whiteboard board DB panel %s: body missing", entry.ID)
				}
				entry.Text = panel.String
			}
			entries = append(entries, entry)
		}
		if err := errors.Join(entryRows.Err(), entryRows.Close()); err != nil {
			return err
		}
		if topic.Doc != nil {
			topic.Doc.Entries = entries
			state.Topics = append(state.Topics, *topic.Doc)
		} else if len(entries) != 0 {
			return fmt.Errorf("whiteboard board DB topic %s: entries without content mapping", dir)
		}
		for _, item := range topic.Items {
			if item.Topic != key || items[item.ID].ID != 0 {
				return fmt.Errorf("whiteboard board DB: invalid or duplicate item %d", item.ID)
			}
			items[item.ID] = item
		}
		if topic.Label != nil {
			if state.Labels == nil {
				state.Labels = map[string]string{}
			}
			state.Labels[key] = *topic.Label
		}
		if topic.CloseResult != nil {
			if state.CloseResults == nil {
				state.CloseResults = map[string]string{}
			}
			state.CloseResults[key] = *topic.CloseResult
		}
		if topic.Closed != slices.Contains(state.ClosedTopics, key) {
			return fmt.Errorf("whiteboard board DB topic %s: close state mismatch", dir)
		}
		b.topicDirs[key] = dir
	}
	for _, id := range itemOrder {
		item, ok := items[id]
		if !ok {
			return fmt.Errorf("whiteboard board DB: incomplete item order")
		}
		state.Items = append(state.Items, item)
		delete(items, id)
	}
	if len(items) != 0 {
		return fmt.Errorf("whiteboard board DB: incomplete item order")
	}
	var metadata uint64
	var stale string
	if err := b.db.QueryRow(`SELECT slot,metadata_revision,stale_nudge_revision FROM revision_receipts WHERE generation<=? ORDER BY generation DESC,slot LIMIT 1`, b.generation).Scan(&b.revisionSlot, &metadata, &stale); err == nil {
		state.MetadataRevision = max(state.MetadataRevision, metadata)
		if stale != "" {
			state.StaleNudgeRevision = stale
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var loadedAt string
	if err := b.db.QueryRow(`SELECT loaded_at FROM load_floors WHERE generation<=? ORDER BY generation DESC LIMIT 1`, b.generation).Scan(&loadedAt); err == nil {
		state.LoadedAt = loadedAt
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	b.state = state.clone()
	return nil
}
