// Board database legacy archive operations.
package wswhiteboard

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

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
	next, err := readBoardArchive(zipPath)
	if err != nil {
		return "", 0, err
	}
	finish := (FolderStore{Dir: b.dir}).beginIncarnationLoad()
	b.mu.Lock()
	oldToken := b.state.LoadedAt
	b.mu.Unlock()
	committed := false
	defer func() {
		if committed {
			finish(next.LoadedAt, nil)
		} else {
			finish(oldToken, nil)
		}
	}()
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
	if err := b.hit("transaction-before-commit"); err != nil {
		return backup, 0, err
	}
	if err := tx.Commit(); err != nil {
		return backup, 0, err
	}
	b.revisionSlot = 0
	if next.MetadataRevision != 0 || next.StaleNudgeRevision != "" {
		b.revisionSlot = 1
	}
	committed = true
	b.generation++
	b.nextTopic = rows.NextTopic
	b.topicDirs = rows.Dirs
	b.state = next.clone()
	b.armCheckpointLocked()
	return backup, len(next.Topics), nil
}

func readBoardArchive(zipPath string) (*State, error) {
	tmp, err := os.MkdirTemp("", "ws-whiteboard-db-load-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := extractArchive(zipPath, tmp); err != nil {
		return nil, err
	}
	// Archive authority is always the legacy layout. A supplied recovery DB
	// (or its sidecars) must not bypass the folder's replay and schema checks.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(filepath.Join(tmp, boardDBName+suffix)); err == nil {
			return nil, fmt.Errorf("the archive contains SQLite recovery files; export a legacy-layout board archive")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	loaded, err := OpenBoardDB(tmp, nil, BoardDBOptions{})
	if err != nil {
		return nil, fmt.Errorf("the archive is not a valid board: %w", err)
	}
	next := loaded.State()
	if err := loaded.Close(); err != nil {
		return nil, err
	}
	if next == nil {
		return nil, fmt.Errorf("the archive holds no board")
	}
	return next, nil
}

// LoadNewBoardDB validates a legacy archive and installs it without a backup.
func LoadNewBoardDB(dir, zipPath string, opts BoardDBOptions) (*BoardDB, int, error) {
	next, err := readBoardArchive(zipPath)
	if err != nil {
		return nil, 0, err
	}
	next.LoadedAt = time.UnixMilli(archiveLoadNow().UnixMilli() + 1).UTC().Format(reaskLayout)
	b, err := CreateBoardDB(dir, next, opts)
	if err != nil {
		return nil, 0, err
	}
	return b, len(next.Topics), nil
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
