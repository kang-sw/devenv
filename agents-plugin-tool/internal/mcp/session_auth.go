package mcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// sessionEntry associates a canonical repository root and a capability scope
// with an ephemeral session key minted by the session-bootstrap tool.
type sessionEntry struct {
	root   string
	scope  toolRole
	parent string
}

// isTopLevelLead reports whether the entry is a top-level lead: lead scope with
// no parent. A worker's key is also lead-scoped but carries its lead's key as
// parent, as does a spawner-launched fork's. Plan allocation, the mailbox
// peer-lead stamp, the note read-only gate, and the notes cleanup warning
// share this test so they cannot drift apart.
func (e sessionEntry) isTopLevelLead() bool {
	return e.scope == roleLead && e.parent == ""
}

// sessionChild is one enumerated descendant of a queried key.
type sessionChild struct {
	key    string
	root   string
	scope  toolRole
	parent string
	depth  int
	live   bool
	note   string
}

// sessionKeyRecord is key.json's shape (session_layout.go): the session's
// identity and its small fields. It is versioned so the format can grow
// (render lineage, permission/capability metadata) without a migration;
// unknown future fields are simply ignored by older readers.
//
// Note: retired typed fields are not migrated. Old records carrying a field
// these structs no longer declare are silently ignored on read (Go's
// json.Unmarshal drops unknown fields).
type sessionKeyRecord struct {
	SchemaVersion int    `json:"schema_version"`
	Root          string `json:"root"`
	Scope         string `json:"scope"`
	Parent        string `json:"parent,omitempty"`
	// Overrides is the session-scope generic config overlay. Keys are item
	// identifiers; values are string-encoded config values. Added as an additive
	// field; existing records without it parse with a nil map.
	Overrides map[string]string `json:"overrides,omitempty"`
	// Note is a free-form one-line annotation on this child's own record,
	// surfaced and filtered (unnoted_only) by session.children. Its writer,
	// session.note, is retired; the field stays so records that already carry a
	// note keep reading back through session.children unchanged.
	Note string `json:"note,omitempty"`
	// ReviewTrackNudgeShown marks that the review-track config nudge (see
	// review_track_alarm.go) has already fired once for this session. Added as
	// an additive field; older records parse with the zero value (false), so
	// the nudge still fires on the first workflow_manual call after an
	// upgrade — the correct "not shown yet" behavior.
	ReviewTrackNudgeShown bool `json:"review_track_nudge_shown,omitempty"`
	// MailboxReplyOpened marks that this session has, at least once, sent
	// mail or performed an env-less mailbox.lookup_peers self-lookup,
	// opening its live-only reply-id return channel (mailbox core Decision
	// 11). Gates the piggyback wrapper's decision to check this session's
	// own reply-id queue at all, keeping a session that has never touched
	// mailbox at zero additional disk I/O (mailbox core Decision 1). Added
	// as an additive field; older records parse with the zero value
	// (false), the correct "never opened" default.
	MailboxReplyOpened bool `json:"mailbox_reply_opened,omitempty"`
	// HarnessSession and HarnessCwd are what the session's harness adapter
	// reported at its last top-lead login (the bootstrap tool's
	// harness_session and harness_cwd, recordHarnessSession): the harness's
	// own name for the conversation (a Claude session id, a Pi session file
	// path) and the working directory the harness runs in, which need not
	// be Root (Root is the canonical git root; the harness may run in a
	// folder below it, and resolves its sessions by that folder). Harness
	// is the harness that reported them. Together they are what resumes
	// the conversation in another process. Additive; a record from a login
	// that reported none has them empty.
	Harness        string `json:"harness,omitempty"`
	HarnessSession string `json:"harness_session,omitempty"`
	HarnessCwd     string `json:"harness_cwd,omitempty"`
}

// sessionRecord is a session's whole record in memory: key.json plus its
// part files (session_layout.go). A reader fills only the parts it asked
// for. The embedded sessionKeyRecord's fields flatten into one JSON object,
// which is exactly the legacy one-file record, so a legacy file still
// decodes into sessionRecord unchanged.
type sessionRecord struct {
	sessionKeyRecord
	// Agenda holds session-level mode-context blobs keyed by name
	// (agenda.json). Values are arbitrary JSON objects (typed enter-payloads
	// or freeform notes). Nil when empty. See session_state.go.
	Agenda map[string]json.RawMessage `json:"agenda,omitempty"`
	// Todos is the ordered step-level checklist for the session (todos.json).
	// Nil when empty. See session_state.go.
	Todos []todoItem `json:"todos,omitempty"`
	// Whiteboard decodes the legacy snapshot; board-aware loaders replace this
	// in-memory value with the topic-folder authority. New writes never update
	// the cache part. Nil means no board and gates implicit panel refresh.
	Whiteboard *wswhiteboard.State `json:"whiteboard,omitempty"`
	// Prepared derived output, published under the folder lock after commit.
	boardOutput *whiteboardRender
}

// sessionRecordSchemaVersion is key.json's schema version. Version 1 was the
// legacy one-file record; version 2 is the split layout's key.json.
const sessionRecordSchemaVersion = 2

// touchGuardWindow throttles touch() so a burst of keyed calls against the same
// session only issues one Chtimes per window, not one per call.
//
// pruneScanCadence bounds how often maybePrune() actually walks the key
// store: a scan is skipped if the prune marker was refreshed less than this
// long ago.
//
// keyRetentionAge is the mtime age past which a key record is pruned.
// Touch keeps active sessions' mtimes fresh, so retention only bites records
// that have gone genuinely idle for this long.
const (
	touchGuardWindow = time.Hour
	pruneScanCadence = 24 * time.Hour
	keyRetentionAge  = 30 * 24 * time.Hour
)

// pruneMarkerName is a bookkeeping file (not a session key) placed directly in
// sessions/ to record when the last prune scan ran. It is a file, not a
// directory, and its leading dot is outside sessionKeyFilenamePattern, so
// listKeys and mint's key namespace both ignore it automatically.
const pruneMarkerName = ".prune-marker"

// sessionKeyFilenamePattern bounds which key strings may become a file path. It
// is deliberately a path-safety guard (no separators, no dots, lowercase alnum +
// hyphen only), not an exact word-chain format check, so the store tolerates
// future key-format evolution while still rejecting traversal attempts like
// "../../etc/passwd" handed to lookup by an untrusted caller.
var sessionKeyFilenamePattern = regexp.MustCompile(`^[a-z0-9-]{1,128}$`)

// sessionStore maps ephemeral session keys to session entries using one
// directory per key in the ws cache root (layout: session_layout.go). The
// files are the source of truth, not the process: a fresh MCP server instance
// (or a lead that restarted mid-delegation) resolves a key by reading its
// key.json, so session continuity no longer depends on a shared in-memory
// registry. There is no logout, but records do age out: every successful
// keyed resolution touches a record's activity mark (see touch, guarded to at
// most once per touchGuardWindow), and maybePrune deletes records whose mark
// is older than keyRetentionAge, scanned at most once per pruneScanCadence.
// Deleting the record remains the only immediate/manual removal path.
//
// The flat per-key layout is required by the access pattern: a caller
// presents only the opaque key, never its root, so the record path must be
// derivable from the key plus the globally-deterministic cache root.
type sessionStore struct {
	// mu serializes same-process read-modify-write and the mint claim loop.
	// Cross-process safety rests on filesystem primitives: an exclusive
	// directory create for the unique mint claim, the key directory's record
	// lock (lockRecord) around every content write, and an atomic temp+replace
	// per part file.
	mu sync.Mutex

	// lockTimeout bounds the wait for a key's record lock; zero means
	// recordLockTimeout. replace swaps a part file's temp file into place;
	// nil means wsstate.AtomicReplaceFile. Both are test seams.
	lockTimeout time.Duration
	replace     func(tmp, path string) error
	// cacheHome is the caller process's WS_CACHE_HOME; empty means this
	// process's.
	cacheHome string

	// intake is the dashboard intake path's in-memory copy of each key's
	// identity (intakeEntry). now is its clock and readFile reads each
	// record file; nil means time.Now and readRecordFile. Both are test
	// seams.
	intake   intakeCache
	now      func() time.Time
	readFile recordFileReader
}

func newSessionStore() *sessionStore {
	return &sessionStore{}
}

// mint generates a unique session key and writes its key.json.
//
// Uniqueness is claimed at the filesystem level (claimKey): creating the key's
// directory is atomic, so two processes (or goroutines) generating the same
// candidate cannot both succeed — the loser retries with a fresh candidate. A
// candidate a legacy record file already holds is taken too. Only the split
// layout is ever minted into.
func (s *sessionStore) mint(root string, scope toolRole, parent string) (string, error) {
	d, err := s.dirs()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(sessionKeyRecord{
		SchemaVersion: sessionRecordSchemaVersion,
		Root:          root,
		Scope:         string(scope),
		Parent:        parent,
	})
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	const maxAttempts = 64
	for i := 0; i < maxAttempts; i++ {
		candidate, err := wskey.Generate()
		if err != nil {
			return "", err
		}
		claimed, err := s.claimKey(d, candidate, payload)
		if err != nil {
			return "", err
		}
		if claimed {
			return candidate, nil
		}
		// rare collision; generate a fresh candidate and retry
	}
	return "", errors.New("mcp: could not mint a unique session key after many attempts")
}

// lookup returns the session entry for the given key and whether it was found.
// A malformed key (path-unsafe, or simply unknown) returns (zero, false), which
// callers translate into the unknown_session re-login contract.
//
// It reads key.json alone (a legacy record file whole), never a part file.
func (s *sessionStore) lookup(key string) (sessionEntry, bool) {
	d, err := s.dirs()
	if err != nil {
		return sessionEntry{}, false
	}
	record, ok := s.readRecord(d, key, partKey)
	if !ok {
		return sessionEntry{}, false
	}
	s.touch(d, key)
	return sessionEntry{
		root:   record.Root,
		scope:  toolRole(record.Scope),
		parent: record.Parent,
	}, true
}

// inspect is lookup without the activity touch, for a reader that is not a
// call (the live session's timer). transient reports a record that exists
// but could not be read this time (errSessionUnreadable), which is neither
// found nor gone.
func (s *sessionStore) inspect(key string) (entry sessionEntry, found, transient bool) {
	d, err := s.dirs()
	if err != nil {
		return sessionEntry{}, false, true
	}
	record, _, err := s.loadRecord(d, key, partKey, nil)
	if errors.Is(err, errSessionUnreadable) {
		return sessionEntry{}, false, true
	}
	if err != nil {
		return sessionEntry{}, false, false
	}
	return sessionEntry{root: record.Root, scope: toolRole(record.Scope), parent: record.Parent}, true, false
}

// intakeRecheck is how long the intake cache trusts that a key still
// exists before it stats the key's record again: a removed key stops intake
// and its live session within this window.
const intakeRecheck = 10 * time.Second

// intakeCache holds, per key, the identity fields the dashboard intake path
// reads: root, scope, and parent, which only mint writes. The adapter's
// intake calls (about ten a second while a reply streams) and the live
// writer's activity check are served from it, so they read no key.json; a
// key's existence is re-checked by a stat at most once per intakeRecheck.
// Only those two callers use it: every other keyed call's lookup gates a
// capability profile and keeps reading key.json, so revocation is never
// delayed there. Its zero value is ready.
type intakeCache struct {
	mu      sync.Mutex
	entries map[string]*intakeCacheEntry
}

type intakeCacheEntry struct {
	entry sessionEntry
	// checkedAt is when the key was last seen to exist (the store's clock).
	checkedAt time.Time
	// markAt is the activity mark's mtime as last known: from the last
	// recheck's stat, or when this cache last touched it. Zero is unknown,
	// which the next touching read treats as due.
	markAt time.Time
}

func (s *sessionStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// intakeLookup is lookup served from the intake cache, for dashboard.intake
// alone: its handler and its pass through the keyed gate. It keeps the
// record's activity mark fresh as lookup does, at most once per
// touchGuardWindow, deciding from the cached mtime so a warm call stats
// nothing: an active session fed only by intake must not age out of the
// prune.
func (s *sessionStore) intakeLookup(key string) (sessionEntry, bool) {
	entry, found, _ := s.intakeEntry(key, true)
	return entry, found
}

// intakeInspect is inspect served from the intake cache, for the live
// writer's activity check: no touch (a timer's read is not activity), and
// transient as inspect reports it.
func (s *sessionStore) intakeInspect(key string) (entry sessionEntry, found, transient bool) {
	return s.intakeEntry(key, false)
}

// intakeEntry resolves key through the intake cache. A key not cached is
// read from its record (inspect's answer) and cached when found; a cached
// key is served as is until intakeRecheck passed since it was last seen,
// and then its activity mark is stat-ed: gone drops it (not found), present
// renews it, and a failed stat (a transient read) keeps serving it and
// retries on the next call. touch marks the call as activity.
func (s *sessionStore) intakeEntry(key string, touch bool) (entry sessionEntry, found, transient bool) {
	now := s.clock()
	c := &s.intake
	c.mu.Lock()
	defer c.mu.Unlock()
	cached := c.entries[key]
	switch {
	case cached == nil:
		read, ok, unreadable := s.inspect(key)
		if !ok {
			return sessionEntry{}, false, unreadable
		}
		cached = &intakeCacheEntry{entry: read, checkedAt: now}
		if c.entries == nil {
			c.entries = map[string]*intakeCacheEntry{}
		}
		c.entries[key] = cached
	case now.Sub(cached.checkedAt) >= intakeRecheck:
		d, err := s.dirs()
		if err != nil {
			return cached.entry, true, false
		}
		mtime, present, ok := statActivityMark(d, key)
		if !ok {
			return cached.entry, true, false
		}
		if !present {
			delete(c.entries, key)
			return sessionEntry{}, false, false
		}
		cached.checkedAt, cached.markAt = now, mtime
	}
	if touch && now.Sub(cached.markAt) >= touchGuardWindow {
		if d, err := s.dirs(); err == nil {
			s.touch(d, key)
			cached.markAt = now
		}
	}
	return cached.entry, true, false
}

// statActivityMark stats key's activity mark (key.json, or a legacy record
// file): its mtime when one exists, present false when neither does, and ok
// false when a stat failed otherwise, which is neither present nor gone.
func statActivityMark(d sessionDirs, key string) (mtime time.Time, present, ok bool) {
	info, err := os.Stat(d.partPath(key, partKey))
	if errors.Is(err, fs.ErrNotExist) {
		info, err = os.Stat(d.legacyPath(key))
	}
	switch {
	case err == nil:
		return info.ModTime(), true, true
	case errors.Is(err, fs.ErrNotExist):
		return time.Time{}, false, true
	}
	return time.Time{}, false, false
}

// recordMtime returns the mtime of key's activity mark (key.json, or a legacy
// record file) and whether the record exists and parses. Unlike lookup it
// never touches the record: an inspection read (such as worktree.list
// reporting a lease holder) must not fabricate activity on the keys it
// reports.
func (s *sessionStore) recordMtime(key string) (time.Time, bool) {
	d, err := s.dirs()
	if err != nil {
		return time.Time{}, false
	}
	if _, ok := s.readRecord(d, key, partKey); !ok {
		return time.Time{}, false
	}
	_, info, ok := activityMark(d, key)
	if !ok {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// children returns the descendants of parentKey from the key store (both
// layouts; key.json alone per split record), ordered deterministically
// (depth, then key). maxDepth bounds the walk: maxDepth >= 1 returns that
// many levels; maxDepth <= 0 returns the full subtree. The queried key itself
// is not included. A cycle guard prevents infinite loops on malformed parent
// edges.
func (s *sessionStore) children(parentKey string, maxDepth int) ([]sessionChild, error) {
	d, err := s.dirs()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	keys, err := s.listKeys(d)
	if err != nil {
		return nil, err
	}

	records := map[string]sessionRecord{}
	adjacency := map[string][]string{}
	for _, key := range keys {
		record, ok := s.readRecord(d, key, partKey)
		if !ok {
			continue
		}
		records[key] = record
		adjacency[record.Parent] = append(adjacency[record.Parent], key)
	}
	for parent := range adjacency {
		sort.Strings(adjacency[parent])
	}
	if _, ok := records[parentKey]; !ok {
		return []sessionChild{}, nil
	}
	// Touch only parentKey's own record: this is a read of parentKey's activity,
	// not of every other session visited while walking the directory to build
	// the adjacency map.
	s.touch(d, parentKey)

	type queued struct {
		key   string
		depth int
	}
	queue := []queued{{key: parentKey, depth: 0}}
	visited := map[string]bool{parentKey: true}
	children := []sessionChild{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if maxDepth >= 1 && current.depth == maxDepth {
			continue
		}
		for _, childKey := range adjacency[current.key] {
			if visited[childKey] {
				continue
			}
			visited[childKey] = true
			record := records[childKey]
			childDepth := current.depth + 1
			child := sessionChild{
				key:    childKey,
				root:   record.Root,
				scope:  toolRole(record.Scope),
				parent: record.Parent,
				depth:  childDepth,
				live:   false,
				note:   record.Note,
			}
			if _, err := os.Stat(record.Root); err == nil {
				child.live = true
			}
			children = append(children, child)
			queue = append(queue, queued{key: childKey, depth: childDepth})
		}
	}

	sort.Slice(children, func(i, j int) bool {
		if children[i].depth != children[j].depth {
			return children[i].depth < children[j].depth
		}
		return children[i].key < children[j].key
	})
	return children, nil
}

// setReviewTrackNudgeShown marks the review-track config nudge as already
// fired for targetKey's own session record, via the same atomic
// read-modify-write primitive (mutateRecord) on key.json. Called by the
// workflow_manual wiring sites (review_track_alarm.go) immediately after a
// non-empty nudge is injected, so a later same-session call does not repeat
// it.
func (s *sessionStore) setReviewTrackNudgeShown(targetKey string) error {
	return s.mutateRecord(targetKey, partKey, func(r *sessionRecord) error {
		r.ReviewTrackNudgeShown = true
		return nil
	})
}

// getOverride returns the Overrides entry for the given item key in the session
// record identified by sessionKey. When that record holds no entry for the item,
// the lookup walks the record's Parent chain and returns the nearest ancestor's
// entry, so a value a lead tunes for its session reaches the workers and
// delegates it spawns; a key's own entry always wins. Returns ("", false) when
// the session is not found, the key is path-unsafe, or no record on the chain
// holds the item.
//
// Only sessionKey's own record is touched: ancestors are read without
// refreshing their mtime, because touching every ancestor on each child read
// would keep an idle lead record looking live to the pruner. The walk stops at
// the first unreadable link (pruned, malformed, or path-unsafe parent) and at a
// revisited key, so a corrupt parent cycle terminates.
//
// s.mu is held for the duration of the read to match the mutex discipline of
// setOverride, preventing a data race when another goroutine is concurrently
// writing to the same session record.
func (s *sessionStore) getOverride(sessionKey, itemKey string) (string, bool) {
	d, err := s.dirs()
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(d, sessionKey, partKey)
	if !ok {
		return "", false
	}
	s.touch(d, sessionKey)
	var value string
	found := false
	s.walkLineage(d, sessionKey, record, func(r sessionRecord) bool {
		value, found = r.Overrides[itemKey]
		return found
	})
	return value, found
}

// walkLineage visits record (the already-read record of key) and then each
// readable ancestor along its Parent chain, nearest first, until visit returns
// true, a link is unreadable, or a key repeats. It never touches a record. The
// caller holds s.mu.
func (s *sessionStore) walkLineage(d sessionDirs, key string, record sessionRecord, visit func(sessionRecord) bool) {
	seen := map[string]struct{}{key: {}}
	for {
		if visit(record) {
			return
		}
		parent := record.Parent
		if parent == "" {
			return
		}
		if _, dup := seen[parent]; dup {
			return
		}
		seen[parent] = struct{}{}
		next, ok := s.readRecord(d, parent, partKey)
		if !ok {
			return
		}
		record = next
	}
}

// listOverrideKeys returns all item keys present in the Overrides map of the
// session record identified by sessionKey or of any readable ancestor on its
// Parent chain (the same lineage getOverride resolves through). Returns nil
// when the session is not found or no record on the chain holds an override.
func (s *sessionStore) listOverrideKeys(sessionKey string) []string {
	d, err := s.dirs()
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(d, sessionKey, partKey)
	if !ok {
		return nil
	}
	s.touch(d, sessionKey)
	set := map[string]struct{}{}
	s.walkLineage(d, sessionKey, record, func(r sessionRecord) bool {
		for k := range r.Overrides {
			set[k] = struct{}{}
		}
		return false
	})
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	return keys
}

// setOverride writes an Overrides entry for the given item key/value into the
// session record identified by sessionKey via atomic read-modify-write of
// key.json. Returns an error if the session key is not found or the write
// fails.
func (s *sessionStore) setOverride(sessionKey, itemKey, value string) error {
	return s.mutateRecord(sessionKey, partKey, func(r *sessionRecord) error {
		if r.Overrides == nil {
			r.Overrides = map[string]string{}
		}
		r.Overrides[itemKey] = value
		return nil
	})
}

// deleteOverride removes an Overrides entry for the given item key from the
// session record identified by sessionKey via atomic read-modify-write of
// key.json. Returns an error if the session key is not found. Removing an
// absent item key (or a session with no Overrides map) is a no-op, not an
// error, and writes nothing.
func (s *sessionStore) deleteOverride(sessionKey, itemKey string) error {
	return s.mutateRecord(sessionKey, partKey, func(r *sessionRecord) error {
		if _, present := r.Overrides[itemKey]; !present {
			return errRecordUnchanged
		}
		delete(r.Overrides, itemKey)
		return nil
	})
}
