package wsinbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReplayAnswerJournal records committed sends before discarding their journal
// copies. The caller must exclude concurrent journaled sends for this inbox.
// A staging file still present means its send never reached the rename commit.
func ReplayAnswerJournal(dir string, record func(id string, entry Entry) error) error {
	journal := filepath.Join(dir, "answer-pending")
	files, err := os.ReadDir(journal)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, file := range files {
		name, temp, ok := strings.Cut(file.Name(), "--")
		if !ok || !entryNamePattern.MatchString(name) || !strings.HasPrefix(temp, ".tmp-") || filepath.Base(temp) != temp {
			return fmt.Errorf("invalid answer journal name %q", file.Name())
		}
		path := filepath.Join(journal, file.Name())
		stage := filepath.Join(dir, temp)
		if _, err := os.Stat(stage); err == nil {
			if err := os.Remove(path); err != nil {
				return err
			}
			// Keep the stage until the journal removal is durable, otherwise a
			// crash could resurrect the journal as an apparently committed send.
			if err := syncInboxDir(journal); err != nil {
				return err
			}
			if err := os.Remove(stage); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		entry, err := readEntry(path)
		if err != nil {
			return err
		}
		if err := record(Record{Name: name}.ID(), entry); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
