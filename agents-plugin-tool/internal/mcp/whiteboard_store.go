package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Every board consumer uses this boundary, including read-only opens. Legacy
// cache parts remain rollback snapshots; the folder store alone owns new writes.
func whiteboardFolder(root, key string) wswhiteboard.FolderStore {
	return wswhiteboard.FolderStore{Dir: filepath.Join(root, filepath.FromSlash(whiteboardDirRel), wsmailbox.ReplyID(key))}
}

func watchWhiteboardFiles(dir string, watch func(string)) {
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && (filepath.Ext(path) == ".json" || filepath.Ext(path) == ".jsonl") {
			watch(path)
		}
		return nil
	})
}

func (s *sessionStore) readWhiteboard(key string) (sessionRecord, error) {
	d, err := s.dirs()
	if err != nil {
		return sessionRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, _, err := s.loadRecord(d, key, partWhiteboard, nil)
	if err == nil {
		s.touch(d, key)
	}
	return record, err
}

func openWhiteboard(record *sessionRecord, key string, raw io.Writer) error {
	folder := whiteboardFolder(record.Root, key)
	if record.Whiteboard != nil {
		if _, err := os.Stat(filepath.Join(folder.Dir, "meta.json")); os.IsNotExist(err) {
			if err := registerWhiteboardExclude(wsgit.ExecRunner{}, record.Root, filepath.Dir(folder.Dir)); err != nil {
				return err
			}
		}
	}
	board, err := folder.Open(record.Whiteboard)
	if err != nil {
		return fmt.Errorf("open whiteboard: %w", err)
	}
	record.Whiteboard = board
	if raw != nil {
		data, err := json.Marshal(board)
		if err != nil {
			return err
		}
		fmt.Fprintf(raw, "\x00topic-folders\x00%d\x00", len(data))
		_, err = raw.Write(data)
		return err
	}
	return nil
}
