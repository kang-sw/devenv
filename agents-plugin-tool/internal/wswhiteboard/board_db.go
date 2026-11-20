package wswhiteboard

// The board DB is the recovery side of the memory-first whiteboard store. It
// is deliberately not wired to MCP callers here: BoardDB keeps one complete
// State in memory and writes only row deltas to one held SQLite connection.

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	boardDBName                = ".board.sqlite"
	boardDBSchemaVersion       = 1
	boardSentinelSchemaVersion = 3

	// BoardDBCheckpointIdle and BoardDBCheckpointWALBytes keep checkpoints
	// away from an update response. A board must be idle for this long and its
	// WAL must exceed this size before the background PASSIVE checkpoint runs.
	BoardDBCheckpointIdle     = 30 * time.Second
	BoardDBCheckpointWALBytes = 4 << 20
	// BoardDBOpenRetryBound covers the brief overlap while a draining daemon
	// still owns the database in locking_mode=EXCLUSIVE.
	BoardDBOpenRetryBound = 2 * time.Second
)

// BoardDBOptions supplies observability and deterministic test seams.
type BoardDBOptions struct {
	Logf    func(format string, args ...any)
	Observe func(stage string)
	Fault   func(stage string) error
}

// BoardDB owns one board's held SQLite handle and complete in-memory state.
// Call Close when the board ages out or the daemon drains.
type BoardDB struct {
	dir  string
	path string
	db   *sql.DB
	opts BoardDBOptions

	mu                 sync.Mutex
	state              *State
	generation         int
	importedGeneration int
	revisionSlot       int
	nextTopic          int
	topicDirs          map[string]string
	checkpointTimer    *time.Timer
	checkpointDue      time.Time
	closed             bool
}

type boardDBTopic struct {
	Doc         *TopicDoc `json:"doc,omitempty"`
	Items       []Item    `json:"items,omitempty"`
	Label       *string   `json:"label,omitempty"`
	Closed      bool      `json:"closed,omitempty"`
	CloseResult *string   `json:"close_result,omitempty"`
}

type boardDBTopicRow struct {
	Dir      string
	Key      string
	Position int
	Data     []byte
	Entries  []boardDBEntryRow
}

type boardDBEntryRow struct {
	Seq       int
	ID        string
	TopicDir  string
	Data      []byte
	PanelBody *string
}

type boardDBRows struct {
	State     []byte
	ItemOrder []byte
	Topics    map[string]boardDBTopicRow
	Dirs      map[string]string
	NextTopic int
}

type boardSentinel struct {
	SchemaVersion int `json:"schema_version"`
}

var boardDBSchema = []string{
	`CREATE TABLE IF NOT EXISTS board_meta (
		id INTEGER PRIMARY KEY CHECK(id = 1),
		schema_version INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		imported_generation INTEGER NOT NULL,
		next_topic INTEGER NOT NULL,
		state_json BLOB NOT NULL,
		item_order_json BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS topics (
		topic_dir TEXT PRIMARY KEY,
		topic_key TEXT NOT NULL UNIQUE,
		position INTEGER NOT NULL,
		topic_json BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS entries (
		entry_id TEXT PRIMARY KEY,
		entry_seq INTEGER NOT NULL UNIQUE,
		topic_dir TEXT NOT NULL REFERENCES topics(topic_dir),
		entry_json BLOB NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS entries_topic_seq ON entries(topic_dir, entry_seq)`,
	`CREATE TABLE IF NOT EXISTS panels (
		entry_id TEXT PRIMARY KEY REFERENCES entries(entry_id),
		body TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS revision_receipts (
		slot INTEGER PRIMARY KEY CHECK(slot IN (1, 2)),
		generation INTEGER NOT NULL,
		metadata_revision INTEGER NOT NULL,
		stale_nudge_revision TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS load_floors (
		generation INTEGER PRIMARY KEY,
		loaded_at TEXT NOT NULL
	)`,
}

// OpenBoardDB opens or imports one board. The first import holds the legacy
// folder lock through the DB commit and schema-3 sentinel write. legacy is the
// old session snapshot accepted by FolderStore.Open when no folder exists.
func OpenBoardDB(dir string, legacy *State, opts BoardDBOptions) (*BoardDB, error) {
	if dir == "" {
		return nil, fmt.Errorf("whiteboard board DB: empty directory")
	}
	path := filepath.Join(dir, boardDBName)
	sentinel, err := boardHasSentinel(dir)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Stat(path)
	dbExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, boardDBRollbackError(path, statErr.Error())
	}
	if sentinel && !dbExists {
		return nil, boardDBRollbackError(path, "is missing")
	}
	if dbExists && !info.Mode().IsRegular() {
		return nil, boardDBRollbackError(path, "is unreadable")
	}
	if dbExists {
		board, err := openBoardDBHandle(dir, opts)
		if err != nil {
			if boardSQLiteBusy(err) || errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("whiteboard board DB %s is busy after %s; retry after the owning daemon releases it: %w", path, BoardDBOpenRetryBound, err)
			}
			return nil, boardDBRollbackError(path, err.Error())
		}
		if err := board.load(); err != nil {
			_ = board.db.Close()
			return nil, boardDBRollbackError(path, err.Error())
		}
		if !sentinel {
			board.finishSentinel()
		}
		board.armCheckpointLocked()
		return board, nil
	}

	legacyStore := FolderStore{Dir: dir}
	var board *BoardDB
	retryOpen := false
	err = legacyStore.withGuard(true, func() error { return boardLegacyGuard(dir) }, func() error {
		// Another importer may have committed while this opener waited for
		// the legacy lock. Re-enter the existing-DB path after releasing it.
		if _, err := os.Stat(path); err == nil {
			retryOpen = true
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return boardDBRollbackError(path, err.Error())
		}
		has, err := boardHasSentinel(dir)
		if err != nil {
			return err
		}
		if has {
			return boardDBRollbackError(path, "is missing")
		}
		state, meta, err := legacyStore.load(legacy)
		if err != nil {
			return err
		}
		if state == nil || meta == nil {
			return os.ErrNotExist
		}
		board, err = openBoardDBHandle(dir, opts)
		if err != nil {
			return err
		}
		if err := board.importState(state, meta, legacyStore); err != nil {
			_ = board.db.Close()
			return err
		}
		if err := board.hit("import-committed"); err != nil {
			_ = board.db.Close()
			return err
		}
		if err := board.writeSentinel(legacyStore); err != nil {
			board.logf("whiteboard board DB %s: write migration sentinel: %v", dir, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if retryOpen {
		return OpenBoardDB(dir, legacy, opts)
	}
	board.armCheckpointLocked()
	return board, nil
}

func openBoardDBHandle(dir string, opts BoardDBOptions) (*BoardDB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, boardDBName)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	b := &BoardDB{dir: dir, path: path, db: db, opts: opts, topicDirs: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), BoardDBOpenRetryBound)
	defer cancel()
	// locking_mode is deliberately first: WAL never opens a shared-memory
	// coordination path for this daemon-owned database.
	for _, stmt := range []string{
		`PRAGMA locking_mode=EXCLUSIVE`,
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA wal_autocheckpoint=0`,
		`PRAGMA cache_size=-256`,
		`PRAGMA busy_timeout=100`,
		`PRAGMA foreign_keys=ON`,
	} {
		if err := boardSQLiteRetry(ctx, func() error { _, err := db.ExecContext(ctx, stmt); return err }); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure board sqlite %s: %w", stmt, err)
		}
	}
	if err := boardSQLiteRetry(ctx, func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, stmt := range boardDBSchema {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return tx.Commit()
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate board sqlite: %w", err)
	}
	return b, nil
}

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

// State returns a detached copy of the complete in-memory board.
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

func (b *BoardDB) finishSentinel() {
	legacyStore := FolderStore{Dir: b.dir}
	err := legacyStore.withGuard(true, func() error { return boardLegacyGuard(b.dir) }, func() error {
		has, err := boardHasSentinel(b.dir)
		if err != nil || has {
			return err
		}
		var receipt folderReceipt
		committed, err := folderDecode(filepath.Join(b.dir, "committed.json"), &receipt)
		if err != nil {
			return err
		}
		var intent folderIntent
		pending, err := folderDecode(filepath.Join(b.dir, "pending.json"), &intent)
		if err != nil {
			return err
		}
		if committed && receipt.Generation > b.importedGeneration {
			b.logf("whiteboard board DB divergence for %s: imported generation %d, legacy committed generation %d", b.dir, b.importedGeneration, receipt.Generation)
		}
		if pending && intent.SchemaVersion == FolderSchemaVersion && intent.Session.Generation > b.importedGeneration {
			b.logf("whiteboard board DB divergence for %s: imported generation %d, legacy pending intent generation %d will be overwritten", b.dir, b.importedGeneration, intent.Session.Generation)
		}
		return b.writeSentinel(legacyStore)
	})
	if err != nil {
		b.logf("whiteboard board DB %s: write migration sentinel: %v", b.dir, err)
	}
}

func boardLegacyGuard(dir string) error {
	has, err := boardHasSentinel(dir)
	if err != nil || has {
		return err
	}
	return (FolderStore{Dir: dir}).guard()
}

func boardHasSentinel(dir string) (bool, error) {
	path := filepath.Join(dir, "pending.json")
	data, exists, err := folderReadOptional(path)
	if err != nil || !exists {
		return false, err
	}
	var head boardSentinel
	if err := folderUnmarshal(path, data, &head); err != nil {
		return false, err
	}
	return head.SchemaVersion == boardSentinelSchemaVersion, nil
}

func (b *BoardDB) writeSentinel(legacy FolderStore) error {
	if err := b.hit("sentinel-write"); err != nil {
		return err
	}
	return legacy.atomic("pending.json", folderJSON(boardSentinel{SchemaVersion: boardSentinelSchemaVersion}), "board-sentinel-staged")
}

func boardDBRollbackError(path, reason string) error {
	return fmt.Errorf("whiteboard board DB %s %s; manual rollback: delete pending.json and .board.sqlite to restore the legacy board as it stood at import", path, reason)
}

func (b *BoardDB) hit(stage string) error {
	if b.opts.Observe != nil {
		b.opts.Observe(stage)
	}
	if b.opts.Fault != nil {
		return b.opts.Fault(stage)
	}
	return nil
}

func (b *BoardDB) logf(format string, args ...any) {
	if b.opts.Logf != nil {
		b.opts.Logf(format, args...)
	}
}

func boardSQLiteRetry(ctx context.Context, fn func() error) error {
	var err error
	for {
		err = fn()
		if !boardSQLiteBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func boardSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		primary := sqliteErr.Code() & 0xff
		return primary == sqlite3.SQLITE_BUSY || primary == sqlite3.SQLITE_LOCKED
	}
	text := strings.ToUpper(err.Error())
	return strings.Contains(text, "SQLITE_BUSY") || strings.Contains(text, "SQLITE_LOCKED") || strings.Contains(text, "DATABASE IS LOCKED")
}

func (b *BoardDB) armCheckpointLocked() {
	b.checkpointDue = time.Now().Add(BoardDBCheckpointIdle)
	if b.checkpointTimer != nil {
		b.checkpointTimer.Stop()
	}
	b.checkpointTimer = time.AfterFunc(BoardDBCheckpointIdle, b.backgroundCheckpoint)
}

func (b *BoardDB) backgroundCheckpoint() {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Stop cannot cancel a callback already waiting for mu. A later update
	// moves this deadline so that callback cannot checkpoint an active board.
	if b.closed || time.Now().Before(b.checkpointDue) {
		return
	}
	info, err := wsstate.Stat(b.path + "-wal")
	if err != nil || info.Size() < BoardDBCheckpointWALBytes {
		return
	}
	if _, err := b.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
		b.logf("whiteboard board DB %s: background checkpoint: %v", b.dir, err)
	}
}

// Checkpoint runs the same PASSIVE checkpoint used by close. It is exposed so
// the daemon's idle manager can trigger it without reopening the database.
func (b *BoardDB) Checkpoint() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("whiteboard board DB is closed")
	}
	_, err := b.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	return err
}

// Close performs the settled PASSIVE checkpoint, then releases the held DB
// handle. It is idempotent.
func (b *BoardDB) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.checkpointTimer != nil {
		b.checkpointTimer.Stop()
	}
	_, checkpointErr := b.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	closeErr := b.db.Close()
	return errors.Join(checkpointErr, closeErr)
}

// ExportArchive emits the legacy folder layout. Dot-named DB files and the
// schema-3 sentinel are constructed nowhere in the archive.
func (b *BoardDB) ExportArchive() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, fmt.Errorf("whiteboard board DB is closed")
	}
	tmp, err := os.MkdirTemp("", "ws-whiteboard-db-export-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	f := FolderStore{Dir: tmp}
	intent, err := b.legacyIntentLocked(f)
	if err != nil {
		return nil, err
	}
	return boardLegacyArchive(intent)
}

// BackupLegacy writes the current board as a legacy-layout folder at dir.
// It is the Phase-1 building block used by whiteboard.load before replacement.
func (b *BoardDB) BackupLegacy(dir string) error {
	data, err := b.ExportArchive()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "ws-whiteboard-db-backup-*.zip")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	return extractArchive(name, dir)
}

// LoadArchive replaces the in-memory/DB board from a legacy-layout zip. The
// zip is first imported by a temporary BoardDB, so pending replay, v1
// conversion, and all folder consistency checks are the same as first open.
// The previous board is backed up as a legacy-layout folder.
func (b *BoardDB) LoadArchive(zipPath string, now time.Time) (backup string, topics int, err error) {
	tmp, err := os.MkdirTemp("", "ws-whiteboard-db-load-*")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmp)
	if err := extractArchive(zipPath, tmp); err != nil {
		return "", 0, err
	}
	// Archive authority is always the legacy layout. A supplied recovery DB
	// (or its sidecars) must not bypass the folder's replay and schema checks.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(filepath.Join(tmp, boardDBName+suffix)); err == nil {
			return "", 0, fmt.Errorf("the archive contains SQLite recovery files; export a legacy-layout board archive")
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", 0, err
		}
	}
	loaded, err := OpenBoardDB(tmp, nil, BoardDBOptions{})
	if err != nil {
		return "", 0, fmt.Errorf("the archive is not a valid board: %w", err)
	}
	next := loaded.State()
	if err := loaded.Close(); err != nil {
		return "", 0, err
	}
	if next == nil {
		return "", 0, fmt.Errorf("the archive holds no board")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", 0, fmt.Errorf("whiteboard board DB is closed")
	}
	next.Seq = max(next.Seq, b.state.Seq+1)
	next.MetadataRevision = max(next.MetadataRevision, b.state.MetadataRevision)
	if next.StaleNudgeRevision == "" {
		next.StaleNudgeRevision = b.state.StaleNudgeRevision
	}
	base := b.dir + ".bak-" + now.Format("060102_1504")
	for n := 1; ; n++ {
		backup = base
		if n > 1 {
			backup = fmt.Sprintf("%s-%d", base, n)
		}
		if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return "", 0, err
		}
	}
	// Export without recursively taking b.mu.
	if err := b.backupLegacyLocked(backup); err != nil {
		return "", 0, err
	}
	// The load floor excludes answers to the old board even when extraction,
	// the lock wait, or backup took time. now is only the backup-name clock.
	loadedAt := archiveLoadNow().UnixMilli() + 1
	if prior, err := time.Parse(time.RFC3339Nano, b.state.LoadedAt); err == nil {
		loadedAt = max(loadedAt, prior.UnixMilli()+1)
	}
	next.LoadedAt = time.UnixMilli(loadedAt).UTC().Format(reaskLayout)
	rows, err := boardRows(next, nil, b.nextTopic, nil)
	if err != nil {
		return backup, 0, err
	}
	tx, err := b.db.Begin()
	if err != nil {
		return backup, 0, err
	}
	defer tx.Rollback()
	for _, table := range []string{"panels", "entries", "topics", "revision_receipts", "load_floors", "board_meta"} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return backup, 0, err
		}
	}
	if err := boardInsertAll(tx, rows, b.generation+1, b.importedGeneration); err != nil {
		return backup, 0, err
	}
	if next.MetadataRevision != 0 || next.StaleNudgeRevision != "" {
		if _, err := tx.Exec(`INSERT INTO revision_receipts(slot,generation,metadata_revision,stale_nudge_revision) VALUES(1,?,?,?)`, b.generation+1, next.MetadataRevision, next.StaleNudgeRevision); err != nil {
			return backup, 0, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO load_floors(generation,loaded_at) VALUES(?,?)`, b.generation+1, next.LoadedAt); err != nil {
		return backup, 0, err
	}
	if err := tx.Commit(); err != nil {
		return backup, 0, err
	}
	b.revisionSlot = 0
	if next.MetadataRevision != 0 || next.StaleNudgeRevision != "" {
		b.revisionSlot = 1
	}
	b.generation++
	b.nextTopic = rows.NextTopic
	b.topicDirs = rows.Dirs
	b.state = next.clone()
	b.armCheckpointLocked()
	return backup, len(next.Topics), nil
}

func (b *BoardDB) backupLegacyLocked(dir string) error {
	tmp, err := os.MkdirTemp("", "ws-whiteboard-db-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	f := FolderStore{Dir: tmp}
	intent, err := b.legacyIntentLocked(f)
	if err != nil {
		return err
	}
	archive, err := boardLegacyArchive(intent)
	if err != nil {
		return err
	}
	zipFile, err := os.CreateTemp("", "ws-whiteboard-db-backup-*.zip")
	if err != nil {
		return err
	}
	name := zipFile.Name()
	defer os.Remove(name)
	if _, err := zipFile.Write(archive); err != nil {
		zipFile.Close()
		return err
	}
	if err := zipFile.Close(); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	return extractArchive(name, dir)
}

func boardLegacyArchive(intent *folderIntent) ([]byte, error) {
	files := map[string][]byte{
		"meta.json":      folderJSON(intent.Session),
		"committed.json": folderJSON(folderReceipt{SchemaVersion: FolderSchemaVersion, Generation: intent.Session.Generation}),
	}
	for _, write := range append(append([]folderWrite(nil), intent.Topics...), intent.Panels...) {
		files[write.Path] = append([]byte(nil), write.Data...)
	}
	for _, log := range intent.Logs {
		if int64(len(files[log.Path])) != log.Offset {
			return nil, fmt.Errorf("whiteboard archive log %s: offset mismatch", log.Path)
		}
		files[log.Path] = append(files[log.Path], log.Data...)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, name := range names {
		writer, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// legacyIntentLocked maps the current State to a fresh legacy layout. Load
// floors live in the reserved log so older State serializers can omit LoadedAt
// without losing it. The caller holds b.mu and f points at an empty directory.
func (b *BoardDB) legacyIntentLocked(f FolderStore) (*folderIntent, error) {
	state := b.state.clone()
	state.LoadedAt = ""
	intent, err := f.plan(nil, nil, state)
	if err != nil {
		return nil, err
	}
	if b.state.LoadedAt != "" {
		intent.Logs = append(intent.Logs, folderAppend{
			Path: folderLoadFloorLog,
			Data: folderJSON(folderLoadFloor{Generation: intent.Session.Generation, LoadedAt: b.state.LoadedAt}),
		})
	}
	return intent, nil
}
