package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// A load appends one recovery record through pending.json's existing log
// protocol. v0.50.7 rewrites State without LoadedAt, but only writes e1/e2
// revision panels in this reserved directory; it never reads or removes this
// log. Old replay also preserves the exact append bytes. Export and backup
// carry the reserved directory, including this log, unchanged.
const folderLoadFloorLog = folderRevisionDir + "/log.jsonl"

type folderLoadFloor struct {
	Generation int    `json:"generation"`
	LoadedAt   string `json:"loaded_at"`
}

// loadFloor reads only on an uncached board open (also the writer's open).
// Interactive reads use the LoadedAt retained in FolderCache's State header.
func (f FolderStore) loadFloor(generation int) (string, int64, error) {
	data, _, err := folderReadOptional(filepath.Join(f.Dir, filepath.FromSlash(folderLoadFloorLog)))
	if err != nil {
		return "", 0, err
	}
	var latest folderLoadFloor
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var record folderLoadFloor
		if err := json.Unmarshal(line, &record); err != nil {
			return "", 0, fmt.Errorf("whiteboard: invalid load floor: %w", err)
		}
		if _, err := time.Parse(time.RFC3339Nano, record.LoadedAt); err != nil || record.Generation <= 0 {
			return "", 0, fmt.Errorf("whiteboard: invalid load floor record")
		}
		if record.Generation <= generation && record.Generation > latest.Generation {
			latest = record
		}
	}
	return latest.LoadedAt, int64(len(data)), nil
}

// archiveLoadNow is separate from LoadArchive's backup-name time: extraction
// and the lock wait may take arbitrarily long. Tests can control this clock.
var archiveLoadNow = time.Now

func (f FolderStore) planLoadFloor(intent *folderIntent) error {
	if intent.Session.NextTopic >= folderRevisionDirNumber {
		return fmt.Errorf("whiteboard: reserved revision directory conflicts with topic allocation")
	}
	_, offset, err := f.loadFloor(intent.BaseGeneration)
	if err != nil {
		return err
	}
	// Called at the swap, under the lock, after backup and allocation cleanup.
	// Inbox times have millisecond precision: the next millisecond excludes
	// even an old-board answer sent in the same millisecond as this capture.
	at := time.UnixMilli(archiveLoadNow().UnixMilli() + 1).UTC().Format(reaskLayout)
	intent.Session.State.LoadedAt = at
	intent.Logs = append(intent.Logs, folderAppend{
		Path: folderLoadFloorLog, Offset: offset,
		Data: folderJSON(folderLoadFloor{Generation: intent.Session.Generation, LoadedAt: at}),
	})
	return nil
}
