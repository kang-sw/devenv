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
			// A malformed remnant must not starve later committed answers.
			if err := os.Remove(filepath.Join(journal, file.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Fprintf(os.Stderr, "inbox: discarded invalid answer journal name %q\n", file.Name())
			continue
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
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			fmt.Fprintf(os.Stderr, "inbox: discarded unreadable answer journal %q: %v\n", file.Name(), err)
			continue
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
