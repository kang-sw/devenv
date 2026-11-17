package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Every board consumer uses this boundary, including read-only opens. Legacy
// cache parts remain rollback snapshots; the folder store alone owns new writes.
func whiteboardFolder(root, key string) wswhiteboard.FolderStore {
	return wswhiteboard.FolderStore{Dir: filepath.Join(root, filepath.FromSlash(whiteboardDirRel), wsmailbox.ReplyID(key))}
}

// watchWhiteboardFiles names the board folder's two commit roots, not its
// leaves: every commit (load-time recovery included) replaces meta.json and
// committed.json after its panel, log, and topic writes, and the takeover edit
// replaces meta.json. Their stats therefore cover every authoritative file
// without a folder walk or a stat per topic file.
func watchWhiteboardFiles(dir string, watch func(string)) {
	watch(filepath.Join(dir, "meta.json"))
	watch(filepath.Join(dir, "committed.json"))
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
	return openWhiteboardCached(record, key, raw, &wswhiteboard.SharedFolderCache)
}

func (src *DashboardSessionSource) openWhiteboard(record *sessionRecord, key string, raw io.Writer) error {
	return openWhiteboard(record, key, raw)
}

func openWhiteboardCached(record *sessionRecord, key string, raw io.Writer, cache *wswhiteboard.FolderCache) error {
	folder := whiteboardFolder(record.Root, key)
	if record.Whiteboard != nil {
		if _, err := wsstate.Stat(filepath.Join(folder.Dir, "meta.json")); os.IsNotExist(err) {
			if err := registerWhiteboardExclude(wsgit.ExecRunner{}, record.Root, filepath.Dir(folder.Dir)); err != nil {
				return err
			}
		}
	}
	board, revision, err := cache.OpenRevision(folder, record.Whiteboard)
	if err != nil {
		return fmt.Errorf("open whiteboard: %w", err)
	}
	record.Whiteboard = board
	record.boardRevision = revision
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
