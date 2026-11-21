// Board database sentinel and lifecycle operations.
package wswhiteboard

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

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
