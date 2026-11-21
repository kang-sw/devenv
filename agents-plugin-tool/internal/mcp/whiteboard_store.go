package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Board paths use the session's canonical root and readable reply ID.
func whiteboardBoardDir(root, key string) string {
	return filepath.Join(root, filepath.FromSlash(whiteboardDirRel), wsmailbox.ReplyID(key))
}

type legacyBoardReader func() (*wswhiteboard.State, error)

func whiteboardLegacyImport(root, key string, read legacyBoardReader) legacyBoardReader {
	return func() (*wswhiteboard.State, error) {
		legacy, err := read()
		if err != nil || legacy == nil {
			return legacy, err
		}
		dir := whiteboardBoardDir(root, key)
		return legacy, registerWhiteboardExclude(wsgit.ExecRunner{}, root, filepath.Dir(dir))
	}
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

func (s *sessionStore) openWhiteboard(record *sessionRecord, key string, raw io.Writer, legacy legacyBoardReader) error {
	return s.snapshotWhiteboard(record, key, raw, legacy, true)
}

func (s *sessionStore) snapshotWhiteboard(record *sessionRecord, key string, raw io.Writer, legacy legacyBoardReader, activity bool) error {
	if s.boards == nil {
		record.Whiteboard = nil
		record.boardUnavailable = true
		return nil
	}
	err := s.boards.useLegacy(record.Root, key, whiteboardLegacyImport(record.Root, key, legacy), activity, func(slot *whiteboardSlot) error {
		record.Whiteboard = nil
		if slot.board != nil {
			record.Whiteboard, record.boardRevision = slot.board.Snapshot()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("open whiteboard: %w", err)
	}
	s.retireWhiteboardPages(record.Root)
	if raw != nil && record.Whiteboard != nil {
		data, err := json.Marshal(record.Whiteboard)
		if err != nil {
			return err
		}
		fmt.Fprintf(raw, "\x00board\x00%d\x00", len(data))
		_, err = raw.Write(data)
		return err
	}
	return nil
}

func (s *sessionStore) mutateWhiteboard(record *sessionRecord, key string, legacy legacyBoardReader, fn func(*sessionRecord) error) error {
	if s.boards == nil {
		return errWhiteboardOwner
	}
	root := record.Root
	return s.boards.useLegacy(root, key, whiteboardLegacyImport(root, key, legacy), true, func(slot *whiteboardSlot) error {
		s.retireWhiteboardPages(root)
		mutate := func(board *wswhiteboard.State) error {
			record.Whiteboard = board
			if err := fn(record); err != nil {
				return err
			}
			if record.Whiteboard != nil && record.Whiteboard != board {
				*board = *record.Whiteboard
			}
			return nil
		}
		if slot.board != nil {
			return slot.board.Update(mutate)
		}
		initial := new(wswhiteboard.State)
		if err := mutate(initial); err != nil {
			return err
		}
		dir := whiteboardBoardDir(root, key)
		if err := registerWhiteboardExclude(wsgit.ExecRunner{}, root, filepath.Dir(dir)); err != nil {
			return err
		}
		board, err := wswhiteboard.CreateBoardDB(dir, initial, s.boards.options)
		if err == nil {
			slot.board = board
		}
		return err
	})
}
