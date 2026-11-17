package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// The topic memo lets a board reload reread only the topics a commit touched,
// for writers and cached readers alike, in this process. Nothing is written
// through: every hit is revalidated against the file on disk, so a commit by
// another process is seen as it would be by an uncached read.
//
//   - Topic meta.json and log.jsonl bytes are served from memory only while a
//     stat shows the same file metadata (size, mtime, identity) as when they
//     were read. A file modified within folderMemoRacyWindow of its read is not
//     kept: a later same-size rewrite in the same mtime tick would otherwise
//     look unchanged.
//   - A topic's decoded log entries are reused for a byte-identical log
//     prefix. Committed logs only grow by whole lines (plan appends at
//     LogBytes; replay refuses a prefix mismatch in append), and a panel file
//     is immutable once its entry is committed (plan writes panels only for
//     new entry IDs; replay refuses different bytes for an existing panel).
//     So only the appended lines' panels are read.
//
// Edits that bypass the commit protocol must FolderCache.Forget the board,
// which also drops its memo, exactly as they must for the cached read.
var folderMemoRacyWindow = 2 * time.Second

// folderMemoLimit bounds each map; overflow starts over, like FolderCache.
const folderMemoLimit = 16384

type folderFileMemo struct {
	stat os.FileInfo
	data []byte
}

type folderLogMemo struct {
	log     []byte
	entries []Entry
}

var folderMemo struct {
	mu    sync.Mutex
	files map[string]folderFileMemo
	logs  map[string]folderLogMemo
}

// folderReadMemo is folderReadFile for topic files. The returned bytes may be
// shared with the memo and must not be modified.
func folderReadMemo(path string) ([]byte, error) {
	stat, statErr := wsstate.Stat(path)
	if statErr == nil {
		folderMemo.mu.Lock()
		m, ok := folderMemo.files[path]
		folderMemo.mu.Unlock()
		if ok && wsstate.SameFileMetadata(m.stat, stat) {
			return m.data, nil
		}
	}
	// The stat precedes the read, so bytes replaced in between are kept
	// under the older metadata and reread on the next call.
	data, err := folderReadFile(path)
	folderMemo.mu.Lock()
	defer folderMemo.mu.Unlock()
	delete(folderMemo.files, path)
	if err == nil && statErr == nil && time.Since(stat.ModTime()) >= folderMemoRacyWindow {
		if folderMemo.files == nil || len(folderMemo.files) >= folderMemoLimit {
			folderMemo.files = map[string]folderFileMemo{}
		}
		folderMemo.files[path] = folderFileMemo{stat: stat, data: data}
	}
	return data, err
}

// logEntries decodes a topic's committed log, with each panel entry's body
// read from its immutable panel file. Entries of a previously decoded
// byte-identical prefix are reused; the caller still validates every entry.
// The result is the caller's own: memo entries are cloned in and out.
func (f FolderStore) logEntries(dir string, log []byte) ([]Entry, error) {
	path := filepath.Join(f.Dir, dir, "log.jsonl")
	folderMemo.mu.Lock()
	m, ok := folderMemo.logs[path]
	folderMemo.mu.Unlock()
	var entries []Entry
	rest := log
	if ok && bytes.HasPrefix(log, m.log) && (len(m.log) == 0 || m.log[len(m.log)-1] == '\n') {
		entries = make([]Entry, 0, len(m.entries))
		for _, e := range m.entries {
			entries = append(entries, e.clone())
		}
		rest = log[len(m.log):]
	}
	for _, line := range bytes.Split(rest, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("whiteboard topic %s log: %w", dir, err)
		}
		if e.Kind == "panel" {
			if e.Text != "" {
				return nil, fmt.Errorf("whiteboard: panel payload in log")
			}
			var panel Entry
			if exists, err := folderDecode(filepath.Join(f.Dir, dir, "panels", e.ID+".json"), &panel); err != nil {
				return nil, err
			} else if !exists {
				return nil, fmt.Errorf("whiteboard panel %s missing", e.ID)
			}
			body := panel.Text
			panel.Text = ""
			if !reflect.DeepEqual(e, panel) {
				return nil, fmt.Errorf("whiteboard panel %s metadata mismatch", e.ID)
			}
			e.Text = body
		}
		entries = append(entries, e)
	}
	kept := make([]Entry, len(entries))
	for i, e := range entries {
		kept[i] = e.clone()
	}
	folderMemo.mu.Lock()
	if folderMemo.logs == nil || len(folderMemo.logs) >= folderMemoLimit {
		folderMemo.logs = map[string]folderLogMemo{}
	}
	folderMemo.logs[path] = folderLogMemo{log: log, entries: kept}
	folderMemo.mu.Unlock()
	return entries, nil
}

// forgetFolderMemo drops every memo under one board directory.
func forgetFolderMemo(dir string) {
	prefix := filepath.Clean(dir) + string(filepath.Separator)
	folderMemo.mu.Lock()
	defer folderMemo.mu.Unlock()
	for path := range folderMemo.files {
		if strings.HasPrefix(path, prefix) {
			delete(folderMemo.files, path)
		}
	}
	for path := range folderMemo.logs {
		if strings.HasPrefix(path, prefix) {
			delete(folderMemo.logs, path)
		}
	}
}
