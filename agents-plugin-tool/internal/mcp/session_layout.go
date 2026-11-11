package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// session_layout.go owns the session key store's on-disk layout. Each key has
// a directory under <cache-root>/sessions/ holding one file per part:
//
//	sessions/<key>/key.json         identity and small fields (sessionKeyRecord)
//	sessions/<key>/agenda.json      agenda blobs
//	sessions/<key>/todos.json       todo list
//	sessions/<key>/whiteboard.json  whiteboard store
//
// key.json is the key's existence claim and its activity mark: touch and
// prune read its mtime. A part file that does not exist is that part empty. A
// mutation rewrites only the parts it changes, so writers of different parts
// no longer replace each other's data, and a read parses only the parts it
// asks for.
//
// The pre-split layout, one <cache-root>/keys/<key>.json holding every field,
// is still read: a key with no key.json falls back to it, served whole. The
// first mutation of such a key moves it into the split layout
// (migrateRecord). Reads never move it: a reader may be a process that
// outlives an update (the dashboard daemon) while the session's older ws-mcp,
// which knows only keys/, still serves the key.
//
// More than one process can serve one key (a worker spawned with its lead's
// key), so every write of a record's content holds the key's record lock,
// sessions/<key>/key.json.lock (lockRecord), across the whole read-modify-write;
// the in-process s.mu alone would let two processes lose each other's update.
// The lock serializes writers only: readers take no lock, and a multi-part
// write (an agenda and todos together) is still one replace per part, so a
// reader in another process can see the first part replaced and not the next.

const (
	sessionsDirName   = "sessions"
	legacyKeysDirName = "keys"

	// recordLockName is the record lock file in a key's directory.
	recordLockName = "key.json.lock"
	// recordLockTimeout bounds the wait for a record lock, matching
	// wsmailbox.LockTimeout.
	recordLockTimeout = 2 * time.Second
)

// recordPart is a set of a session record's part files.
type recordPart uint8

const (
	partKey recordPart = 1 << iota
	partAgenda
	partTodos
	partWhiteboard

	partAll = partKey | partAgenda | partTodos | partWhiteboard
)

// recordParts lists every part in write order. key.json comes last, so a
// migration cut short before it leaves no key.json and the legacy file keeps
// serving the key.
var recordParts = []recordPart{partAgenda, partTodos, partWhiteboard, partKey}

func (p recordPart) fileName() string {
	switch p {
	case partKey:
		return "key.json"
	case partAgenda:
		return "agenda.json"
	case partTodos:
		return "todos.json"
	case partWhiteboard:
		return "whiteboard.json"
	}
	return ""
}

// errSessionNotFound is loadRecord's answer for a key with no readable record.
var errSessionNotFound = errors.New("session key not found")

// errSessionUnreadable marks a record that exists but could not be read this
// time: an I/O error other than absence, such as a Windows reader meeting
// the record's replace in flight. loadRecord returns it wrapped in a
// sessionUnreadableError, which every caller matching errSessionNotFound
// still sees as not found; only a caller that must not act on a transient
// failure (the live session's activity check) asks for this one.
var errSessionUnreadable = errors.New("session record unreadable")

type sessionUnreadableError struct{ err error }

func (e sessionUnreadableError) Error() string {
	return errSessionUnreadable.Error() + ": " + e.err.Error()
}
func (e sessionUnreadableError) Unwrap() error { return e.err }
func (e sessionUnreadableError) Is(target error) bool {
	return target == errSessionNotFound || target == errSessionUnreadable
}

// recordNotFound classifies a record file's read error: a path error other
// than absence is a transient unreadable record; anything else (absent,
// malformed) is not found.
func recordNotFound(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && !errors.Is(err, fs.ErrNotExist) {
		return sessionUnreadableError{err: err}
	}
	return errSessionNotFound
}

// errRecordUnchanged, returned by a mutateRecord fn, ends the mutation with
// nothing written and no error.
var errRecordUnchanged = errors.New("session record unchanged")

// partValue returns a pointer to the record field that part stores.
func (r *sessionRecord) partValue(part recordPart) any {
	switch part {
	case partKey:
		return &r.sessionKeyRecord
	case partAgenda:
		return &r.Agenda
	case partTodos:
		return &r.Todos
	case partWhiteboard:
		return &r.Whiteboard
	}
	return nil
}

// partEmpty reports whether part holds nothing, so its file is removed rather
// than written. An empty agenda or todo list is nothing, as the legacy file's
// omitempty fields already treated them; a whiteboard is nothing only when
// nil, since an empty board still marks that the board exists.
func (r *sessionRecord) partEmpty(part recordPart) bool {
	switch part {
	case partAgenda:
		return len(r.Agenda) == 0
	case partTodos:
		return len(r.Todos) == 0
	case partWhiteboard:
		return r.Whiteboard == nil
	}
	return false
}

// sessionDirs is the key store's two roots under the cache root.
type sessionDirs struct {
	sessions string // the split layout: one directory per key
	legacy   string // the pre-split layout: one record file per key
}

// dirs resolves the key store's roots and creates the split layout's. The
// cache root honors WS_CACHE_HOME (and otherwise ~/.cache/ws@...), the same
// seam every other ws cache artifact uses, so all server instances agree on
// the location. The legacy root is never created: only an older ws-mcp
// writes there.
func (s *sessionStore) dirs() (sessionDirs, error) {
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{CacheHome: s.cacheHome})
	if err != nil {
		return sessionDirs{}, err
	}
	d := sessionDirs{
		sessions: filepath.Join(cacheRoot, sessionsDirName),
		legacy:   filepath.Join(cacheRoot, legacyKeysDirName),
	}
	if err := os.MkdirAll(d.sessions, 0o755); err != nil {
		return sessionDirs{}, fmt.Errorf("create session keys dir: %w", err)
	}
	return d, nil
}

func (d sessionDirs) keyDir(key string) string { return filepath.Join(d.sessions, key) }

func (d sessionDirs) partPath(key string, part recordPart) string {
	return filepath.Join(d.sessions, key, part.fileName())
}

func (d sessionDirs) legacyPath(key string) string { return filepath.Join(d.legacy, key+".json") }

// loadRecord reads key's record: key.json and the parts in parts from the
// split layout, or, when key.json does not exist, the whole legacy file (every
// part, whatever parts asks for); legacy reports the second case. A path-unsafe
// key, a missing record, and an unparsable key.json or legacy file are
// errSessionNotFound; an unparsable part file is its own error. raw, when not
// nil, receives every file read, for a change fingerprint.
func (s *sessionStore) loadRecord(d sessionDirs, key string, parts recordPart, raw io.Writer) (record sessionRecord, legacy bool, err error) {
	read := s.readFile
	if read == nil {
		read = readRecordFile
	}
	return loadRecordWith(d, key, parts, raw, read)
}

// recordFileReader reads one record file and decodes it into a value
// newValue makes, returning the file's bytes and the decoded value (a
// pointer of newValue's type). A file that does not exist is an error that
// is fs.ErrNotExist.
type recordFileReader func(path string, newValue func() any) (data []byte, value any, err error)

// readRecordFile is the recordFileReader that reads the file afresh.
func readRecordFile(path string, newValue func() any) ([]byte, any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	v := newValue()
	if err := json.Unmarshal(data, v); err != nil {
		return nil, nil, err
	}
	return data, v, nil
}

// loadRecordWith is loadRecord reading each file through read. A reader may
// hand out values it shares with other callers (the dashboard source's
// cache), so the record's slices, maps, and whiteboard may be shared: a
// caller of such a reader only reads them.
func loadRecordWith(d sessionDirs, key string, parts recordPart, raw io.Writer, read recordFileReader) (record sessionRecord, legacy bool, err error) {
	if !sessionKeyFilenamePattern.MatchString(key) {
		return sessionRecord{}, false, errSessionNotFound
	}
	data, v, err := read(d.partPath(key, partKey), func() any { return new(sessionKeyRecord) })
	if errors.Is(err, fs.ErrNotExist) {
		data, v, err = read(d.legacyPath(key), func() any { return new(sessionRecord) })
		if err != nil {
			return sessionRecord{}, false, recordNotFound(err)
		}
		record = *v.(*sessionRecord)
		if raw != nil {
			raw.Write(data)
		}
		return record, true, nil
	}
	if err != nil {
		return sessionRecord{}, false, recordNotFound(err)
	}
	record.sessionKeyRecord = *v.(*sessionKeyRecord)
	writeRawPart(raw, partKey, data)
	for _, part := range recordParts {
		if part == partKey || parts&part == 0 {
			continue
		}
		data, v, err := read(d.partPath(key, part), func() any { return reflect.New(reflect.TypeOf(record.partValue(part)).Elem()).Interface() })
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return sessionRecord{}, false, fmt.Errorf("read session %s: %w", part.fileName(), err)
		}
		reflect.ValueOf(record.partValue(part)).Elem().Set(reflect.ValueOf(v).Elem())
		writeRawPart(raw, part, data)
	}
	return record, false, nil
}

// writeRawPart frames one part file's bytes into raw, so that moving bytes
// between two files never reads as the same content.
func writeRawPart(raw io.Writer, part recordPart, data []byte) {
	if raw == nil {
		return
	}
	fmt.Fprintf(raw, "%s\x00%d\x00", part.fileName(), len(data))
	raw.Write(data)
}

// readRecord is loadRecord for a read-only caller: ok is false for any
// unreadable record.
func (s *sessionStore) readRecord(d sessionDirs, key string, parts recordPart) (sessionRecord, bool) {
	record, _, err := s.loadRecord(d, key, parts, nil)
	return record, err == nil
}

// writeParts writes the parts in parts of record into key's directory, each
// atomically (temp file and replace in the same directory) and in recordParts
// order, and removes the file of an empty part. The caller holds s.mu and the
// key's record lock.
func (s *sessionStore) writeParts(d sessionDirs, key string, record *sessionRecord, parts recordPart) error {
	for _, part := range recordParts {
		if parts&part == 0 {
			continue
		}
		path := d.partPath(key, part)
		if record.partEmpty(part) {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		payload, err := json.Marshal(record.partValue(part))
		if err != nil {
			return err
		}
		if err := s.writeFileAtomic(path, payload); err != nil {
			return err
		}
	}
	return nil
}

// migrateRecord moves a legacy record into the split layout: the part files,
// then key.json, then the legacy file's removal. A crash between steps leaves
// both layouts: before key.json lands the legacy file still serves, after it
// the split record wins on read, and the next write removes the legacy file.
// The caller holds s.mu and the key's record lock.
func (s *sessionStore) migrateRecord(d sessionDirs, key string, record *sessionRecord) error {
	if err := os.MkdirAll(d.keyDir(key), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	record.SchemaVersion = sessionRecordSchemaVersion
	if err := s.writeParts(d, key, record, partAll); err != nil {
		return err
	}
	removeLegacyRecord(d, key)
	return nil
}

// removeLegacyRecord deletes key's legacy record file once its split record is
// in place. Best effort: a leftover file is shadowed by key.json and the next
// write retries.
func removeLegacyRecord(d sessionDirs, key string) {
	_ = os.Remove(d.legacyPath(key))
}

// claimKey creates key's record in the split layout from the key.json payload,
// or reports false when the key is taken: a legacy record file of that name
// exists, or its directory does (a split record, or another minter's claim).
// Creating the directory is the claim: os.Mkdir fails with EEXIST for every
// concurrent minter of the same candidate but one. The winner writes key.json
// under the record lock, taken only after the claim: a lock file created first
// would make the candidate look taken.
func (s *sessionStore) claimKey(d sessionDirs, key string, payload []byte) (bool, error) {
	if _, err := os.Lstat(d.legacyPath(key)); !errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	dir := d.keyDir(key)
	if err := os.Mkdir(dir, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("create session key dir: %w", err)
	}
	unlock, err := s.lockRecord(d, key)
	if err == nil {
		err = writeClaimFile(d.partPath(key, partKey), payload)
		unlock() // before RemoveAll: Windows cannot delete an open lock file
	}
	if err != nil {
		os.RemoveAll(dir) // do not leave a half-written claim behind
		return false, err
	}
	return true, nil
}

// writeClaimFile creates a newly claimed key's key.json exclusively.
func writeClaimFile(path string, payload []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create session key file: %w", err)
	}
	if _, err := f.Write(payload); err != nil {
		f.Close()
		return fmt.Errorf("write session key file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close session key file: %w", err)
	}
	return nil
}

// lockRecord takes key's record lock (recordLockName in its directory, which
// must exist), waiting up to the store's lock timeout, and returns its
// release. A wait past the timeout is an error, so a caller never writes
// without the lock.
func (s *sessionStore) lockRecord(d sessionDirs, key string) (func(), error) {
	timeout := s.lockTimeout
	if timeout <= 0 {
		timeout = recordLockTimeout
	}
	lockPath := filepath.Join(d.keyDir(key), recordLockName)
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && !locked) {
		return nil, fmt.Errorf("timed out after %s waiting for session record lock %s", timeout, lockPath)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire session record lock: %w", err)
	}
	return func() { _ = fl.Unlock() }, nil
}

// lockExistingRecord takes the record lock of a key that has a record. A
// legacy record has no key directory yet, so it is created for the lock, but
// only once the legacy file is confirmed: a key with no record in either
// layout is errSessionNotFound and leaves nothing behind.
func (s *sessionStore) lockExistingRecord(d sessionDirs, key string) (func(), error) {
	if !sessionKeyFilenamePattern.MatchString(key) {
		return nil, errSessionNotFound
	}
	if _, err := os.Stat(d.partPath(key, partKey)); errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(d.legacyPath(key)); err != nil {
			return nil, errSessionNotFound
		}
		if err := os.MkdirAll(d.keyDir(key), 0o755); err != nil {
			return nil, fmt.Errorf("create session dir: %w", err)
		}
	}
	return s.lockRecord(d, key)
}

// listKeys returns every key with a directory in the split layout or a record
// file in the legacy one, once each and sorted. A name that is not a path-safe
// key is skipped. A key's directory may not hold key.json yet (a claim in
// progress, or a migration a crash cut short), so a caller reads the record
// before trusting the key. A missing legacy root is empty; an unreadable root
// is an error.
func (s *sessionStore) listKeys(d sessionDirs) ([]string, error) {
	entries, err := os.ReadDir(d.sessions)
	if err != nil {
		return nil, fmt.Errorf("read session keys dir: %w", err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() && sessionKeyFilenamePattern.MatchString(entry.Name()) {
			seen[entry.Name()] = true
		}
	}
	legacy, err := os.ReadDir(d.legacy)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read legacy session keys dir: %w", err)
	}
	for _, entry := range legacy {
		key, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if isJSON && !entry.IsDir() && sessionKeyFilenamePattern.MatchString(key) {
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// activityMark returns the path and stat of key's activity mark: key.json, or
// the legacy record file of a key not yet migrated.
func activityMark(d sessionDirs, key string) (string, os.FileInfo, bool) {
	path := d.partPath(key, partKey)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		path = d.legacyPath(key)
		info, err = os.Stat(path)
	}
	if err != nil {
		return "", nil, false
	}
	return path, info, true
}

// touch refreshes the mtime of key's activity mark to mark it as recently
// active, unless it was already touched within touchGuardWindow (throttling a
// burst of keyed calls to at most one Chtimes per window). Best-effort: stat
// and Chtimes failures are ignored, matching the non-fatal style of the other
// read-only session lookups. Touching a legacy record file does not migrate
// it. Callers are expected to have already validated key via a prior
// readRecord success (loadRecord applies sessionKeyFilenamePattern), so touch
// does not re-derive that guard.
func (s *sessionStore) touch(d sessionDirs, key string) {
	path, info, ok := activityMark(d, key)
	if !ok || time.Since(info.ModTime()) < touchGuardWindow {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// writeFileAtomic replaces path via a temp file in its directory and the
// store's replace (wsstate.AtomicReplaceFile, which retries a Windows sharing
// violation against a reader in another process), so a concurrent reader
// never observes a partial file. A failed replace leaves path's previous
// content and removes the temp file.
func (s *sessionStore) writeFileAtomic(path string, payload []byte) error {
	replace := s.replace
	if replace == nil {
		replace = wsstate.AtomicReplaceFile
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := replace(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
