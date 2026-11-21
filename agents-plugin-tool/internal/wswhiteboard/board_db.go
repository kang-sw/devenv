package wswhiteboard

// The board DB is the recovery side of the memory-first whiteboard store. It
// is deliberately not wired to MCP callers here: BoardDB keeps one complete
// State in memory and writes only row deltas to one held SQLite connection.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	return OpenBoardDBLazy(dir, func() (*State, error) { return legacy, nil }, opts)
}

// OpenBoardDBLazy consults the legacy snapshot only when no folder or DB authority exists.
func OpenBoardDBLazy(dir string, legacy func() (*State, error), opts BoardDBOptions) (*BoardDB, error) {
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
		(FolderStore{Dir: dir}).rememberIncarnation(board.state.LoadedAt)
		return board, nil
	}

	legacyStore := FolderStore{Dir: dir}
	present, err := boardAuthorityPresent(dir)
	if err != nil {
		return nil, err
	}
	var legacyState *State
	if !present {
		if legacy != nil {
			legacyState, err = legacy()
			if err != nil {
				return nil, err
			}
		}
		if legacyState == nil {
			return nil, os.ErrNotExist
		}
	}
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
		state, meta, err := legacyStore.load(legacyState)
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
		return OpenBoardDBLazy(dir, legacy, opts)
	}
	board.armCheckpointLocked()
	(FolderStore{Dir: dir}).rememberIncarnation(board.state.LoadedAt)
	return board, nil
}

// boardAuthorityPresent checks only authoritative files; a directory or lock
// alone does not constitute a board.
func boardAuthorityPresent(dir string) (bool, error) {
	for _, name := range []string{boardDBName, "meta.json", "pending.json", "committed.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

// CreateBoardDB installs initial state directly, refusing existing authority.
func CreateBoardDB(dir string, initial *State, opts BoardDBOptions) (*BoardDB, error) {
	if dir == "" || initial == nil {
		return nil, fmt.Errorf("whiteboard board DB: directory and initial state are required")
	}
	f := FolderStore{Dir: dir}
	var b *BoardDB
	err := f.withGuard(true, func() error {
		present, err := boardAuthorityPresent(dir)
		if err != nil {
			return err
		}
		if present {
			return fmt.Errorf("whiteboard board already exists: %w", os.ErrExist)
		}
		return nil
	}, func() error {
		rows, err := boardRows(initial, nil, 0, nil)
		if err != nil {
			return err
		}
		cleanup := func() {
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				_ = os.Remove(filepath.Join(dir, boardDBName+suffix))
			}
		}
		b, err = openBoardDBHandle(dir, opts)
		if err != nil {
			cleanup()
			return err
		}
		tx, err := b.db.Begin()
		if err != nil {
			_ = b.db.Close()
			cleanup()
			return err
		}
		defer tx.Rollback()
		if err = boardInsertAll(tx, rows, 1, 0); err == nil && (initial.MetadataRevision != 0 || initial.StaleNudgeRevision != "") {
			_, err = tx.Exec(`INSERT INTO revision_receipts(slot,generation,metadata_revision,stale_nudge_revision) VALUES(1,1,?,?)`, initial.MetadataRevision, initial.StaleNudgeRevision)
			b.revisionSlot = 1
		}
		if err == nil && initial.LoadedAt != "" {
			_, err = tx.Exec(`INSERT INTO load_floors(generation,loaded_at) VALUES(1,?)`, initial.LoadedAt)
		}
		if err == nil {
			err = b.hit("transaction-before-commit")
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			_ = tx.Rollback()
			_ = b.db.Close()
			cleanup()
			return err
		}
		b.state, b.generation, b.nextTopic, b.topicDirs = initial.clone(), 1, rows.NextTopic, rows.Dirs
		if err := b.writeSentinel(f); err != nil {
			b.logf("whiteboard board DB %s: write migration sentinel: %v", dir, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	f.rememberIncarnation(b.state.LoadedAt)
	b.armCheckpointLocked()
	return b, nil
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
