package wsmailbox

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

// legacy.go keeps a previous ws-mcp daemon working across a downgrade. That
// version reads and rewrites only the per-scope mailbox.json files, the
// machine reply registry file, and the per-session history entry files.
//
//   - Open reconciles (reconcileNamedPaths, reconcileReply, reconcileHistory):
//     each file is diffed against the baseline, what the store last read or
//     wrote there. Mail in the file but not the baseline is new (an upgrade's
//     whole backlog, or what an old daemon queued during a downgrade) and is
//     imported; mail in the baseline but gone from the file was consumed by
//     an old daemon and is acknowledged; presence and reply entries the file
//     changed are taken. Then every file holding queues is cleared of them,
//     so an old daemon after a later crash never redelivers mail already
//     received.
//   - A graceful shutdown of a persistent instance exports (export): the live
//     presence, queues, reply entries, and history windows are written back,
//     so an old daemon started next serves them.
//
// Envelopes carry no ID, so they are matched by value (UndeliveredNotified
// aside), with multiplicity. A file this store wrote carries ExportMark; an
// old daemon's rewrite drops the unknown field. A file still carrying it is
// exactly what the store wrote, which closes the crash window between a
// file write and the ring record of its baseline.

// errSkipWrite makes WithFileLock and WithReplyFileLock leave the file as is.
var errSkipWrite = errors.New("skip write")

// baseline is what the legacy files held when the store last read or wrote
// them: per named path, the reply registry, and per session the history
// entry names on disk.
type baseline struct {
	Named   map[string]StoreFile `json:"named,omitempty"`
	Reply   *ReplyStore          `json:"reply,omitempty"`
	History map[string][]string  `json:"history,omitempty"`
}

func emptyBaseline() baseline {
	return baseline{Named: map[string]StoreFile{}, History: map[string][]string{}}
}

func (b *baseline) fill() {
	if b.Named == nil {
		b.Named = map[string]StoreFile{}
	}
	if b.History == nil {
		b.History = map[string][]string{}
	}
}

// merge replaces b's parts that d names.
func (b baseline) merge(d baseline) baseline {
	out := baseline{Named: maps.Clone(b.Named), Reply: b.Reply, History: maps.Clone(b.History)}
	out.fill()
	maps.Copy(out.Named, d.Named)
	if d.Reply != nil {
		out.Reply = d.Reply
	}
	maps.Copy(out.History, d.History)
	return out
}

func storeEqual(a, b StoreFile) bool {
	return maps.Equal(a.Presence, b.Presence) && maps.EqualFunc(nonEmpty(a.Queues), nonEmpty(b.Queues), slices.Equal[[]Envelope])
}

func replyEqual(a, b ReplyStore) bool {
	return maps.EqualFunc(a.Entries, b.Entries, entryEqual) && maps.EqualFunc(nonEmpty(a.Queues), nonEmpty(b.Queues), slices.Equal[[]Envelope])
}

func nonEmpty(queues map[string][]Envelope) map[string][]Envelope {
	out := make(map[string][]Envelope, len(queues))
	for name, q := range queues {
		if len(q) > 0 {
			out[name] = q
		}
	}
	return out
}

// legacyStorePaths lists the legacy named store files that exist: the
// machine store and every worktree and clone store under the cache root.
func (in *instance) legacyStorePaths() []string {
	var out []string
	if machine, err := in.paths.Machine(); err == nil {
		out = append(out, filepath.Clean(machine))
	}
	for _, pattern := range []string{
		filepath.Join(in.cacheRoot, "proj", "*", storeFileName),
		filepath.Join(in.cacheRoot, "proj", "*", "shared", storeFileName),
	} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			out = append(out, filepath.Clean(m))
		}
	}
	return out
}

// reconcileAll reconciles every legacy file at open: the named stores the
// state or the baseline knows and the ones on disk, the reply registry, and
// the history directories.
func (in *instance) reconcileAll() error {
	set := map[string]bool{}
	for path := range in.st.Named {
		set[path] = true
	}
	for path := range in.st.Baseline.Named {
		set[path] = true
	}
	for _, path := range in.legacyStorePaths() {
		set[path] = true
	}
	return errors.Join(in.reconcileNamedPaths(slices.Sorted(maps.Keys(set))), in.reconcileReply(), in.reconcileHistory())
}

// reconcileNamedPaths reconciles the named stores at paths: one import
// record for the state changes, with the files as read for baseline, then
// each file holding queues cleared and one record of the cleared baseline.
func (in *instance) reconcileNamedPaths(paths []string) error {
	importRec := &record{Kind: recordTx, Op: "import", At: storeNow().Format(time.RFC3339Nano), Baseline: &baseline{Named: map[string]StoreFile{}}}
	read := map[string]StoreFile{}
	var errs []error
	for _, path := range paths {
		in.loaded[path] = true
		if _, err := os.Stat(path); err != nil {
			// No file: nothing to learn. A file someone deleted is not mail
			// an old daemon consumed.
			continue
		}
		file, err := LoadFile(path)
		if err != nil {
			// Unreadable: left as is, and not read again before the next
			// open.
			errs = append(errs, err)
			continue
		}
		// A file still carrying the export mark is what the store wrote:
		// it is its own baseline.
		base := in.st.Baseline.Named[path]
		if file.ExportMark != "" {
			base = file
		}
		file.ExportMark = ""
		cur, ok := in.st.Named[path]
		if !ok {
			cur = emptyStore()
		}
		next := reconcileStore(cur, base, file)
		if d := diffNamed(cur, next); d != nil {
			if importRec.Named == nil {
				importRec.Named = map[string]*namedDelta{}
			}
			importRec.Named[path] = d
		}
		if !storeEqual(file, in.st.Baseline.Named[path]) {
			importRec.Baseline.Named[path] = file
		}
		read[path] = file
	}
	if len(importRec.Baseline.Named) == 0 {
		importRec.Baseline = nil
	}
	if !importRec.empty() {
		if err := in.ring.append(importRec); err != nil {
			return errors.Join(append(errs, err)...)
		}
		in.st.apply(importRec)
	}
	clearRec := &record{Kind: recordTx, Op: "import-clear", At: storeNow().Format(time.RFC3339Nano), Baseline: &baseline{Named: map[string]StoreFile{}}}
	for path, file := range read {
		if len(nonEmpty(file.Queues)) == 0 {
			continue
		}
		cleared := StoreFile{SchemaVersion: schemaVersion, Presence: file.Presence, Queues: map[string][]Envelope{}}
		err := WithFileLock(path, func(current *StoreFile) error {
			current.ExportMark = ""
			if !storeEqual(*current, file) {
				// Changed since read: the next open reconciles it.
				return errSkipWrite
			}
			*current = cleared
			current.ExportMark = exportMark()
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		clearRec.Baseline.Named[path] = cleared
	}
	if len(clearRec.Baseline.Named) > 0 {
		if err := in.ring.append(clearRec); err != nil {
			errs = append(errs, err)
		} else {
			in.st.apply(clearRec)
		}
	}
	in.maybeRotate()
	return errors.Join(errs...)
}

// reconcileReply reconciles the reply registry file like reconcileNamedPaths.
func (in *instance) reconcileReply() error {
	if _, err := os.Stat(in.replyPath); err != nil {
		return nil
	}
	file, err := LoadReplyFile(in.replyPath)
	if err != nil {
		return err
	}
	recorded := emptyReplyStore()
	if in.st.Baseline.Reply != nil {
		recorded = *in.st.Baseline.Reply
	}
	base := recorded
	if file.ExportMark != "" {
		base = file
		base.ExportMark = ""
	}
	file.ExportMark = ""
	next := reconcileReplyStore(in.st.Reply, base, file)
	rec := &record{Kind: recordTx, Op: "import", At: storeNow().Format(time.RFC3339Nano), Reply: diffReply(in.st.Reply, next)}
	if !replyEqual(file, recorded) {
		rec.Baseline = &baseline{Reply: &file}
	}
	if !rec.empty() {
		if err := in.ring.append(rec); err != nil {
			return err
		}
		in.st.apply(rec)
	}
	if len(nonEmpty(file.Queues)) == 0 {
		return nil
	}
	cleared := ReplyStore{SchemaVersion: schemaVersion, Entries: file.Entries, Queues: map[string][]Envelope{}}
	if err := WithReplyFileLock(in.replyPath, func(current *ReplyStore) error {
		current.ExportMark = ""
		if !replyEqual(*current, file) {
			return errSkipWrite
		}
		*current = cleared
		current.ExportMark = exportMark()
		return nil
	}); err != nil {
		return err
	}
	rec = &record{Kind: recordTx, Op: "import-clear", At: storeNow().Format(time.RFC3339Nano), Baseline: &baseline{Reply: &cleared}}
	if err := in.ring.append(rec); err != nil {
		return err
	}
	in.st.apply(rec)
	return nil
}

// reconcileHistory imports history entries the legacy directories hold and
// the state does not, skipping directories idle past HistoryRetention. The
// files stay: they hold no mail to deliver.
func (in *instance) reconcileHistory() error {
	root := filepath.Join(in.cacheRoot, HistoryDirName)
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	now := storeNow()
	rec := &record{Kind: recordTx, Op: "import", At: now.Format(time.RFC3339Nano), History: map[string][]HistoryEntry{}, Baseline: &baseline{History: map[string][]string{}}}
	for _, de := range dirs {
		if !de.IsDir() {
			continue
		}
		key := de.Name()
		dir := filepath.Join(root, key)
		info, err := os.Stat(dir)
		if err != nil || now.Sub(info.ModTime()) > HistoryRetention {
			continue
		}
		names, err := legacyHistoryNames(dir)
		if err != nil {
			continue
		}
		known := in.st.Baseline.History[key]
		if slices.Equal(names, known) {
			continue
		}
		skip := map[string]bool{}
		for _, name := range known {
			skip[name] = true
		}
		for _, e := range in.st.History[key] {
			skip[e.Name] = true
		}
		var add []HistoryEntry
		for _, name := range names[:min(len(names), HistoryCap)] {
			if skip[name] {
				continue
			}
			if e, ok := readLegacyHistoryEntry(dir, name); ok {
				add = append(add, e)
			}
		}
		if len(add) > 0 {
			sort.Slice(add, func(i, j int) bool { return add[i].Name < add[j].Name })
			rec.History[key] = add
		}
		rec.Baseline.History[key] = names
	}
	if len(rec.History) == 0 {
		rec.History = nil
	}
	if len(rec.Baseline.History) == 0 {
		rec.Baseline = nil
	}
	if rec.empty() {
		return nil
	}
	if err := in.ring.append(rec); err != nil {
		return err
	}
	in.st.apply(rec)
	return nil
}

// reconcileStore applies file's changes since base onto cur.
func reconcileStore(cur, base, file StoreFile) StoreFile {
	next := cloneStore(cur, false)
	for name, p := range file.Presence {
		if b, ok := base.Presence[name]; !ok || b != p {
			next.Presence[name] = p
		}
	}
	next.Queues = reconcileQueues(next.Queues, base.Queues, file.Queues)
	return next
}

// reconcileReplyStore applies file's changes since base onto cur. An entry
// the file changed takes the file's fields (an old daemon ran after the
// state's last write), keeping the newer LastSeen and merging contacts by
// their newest exchange.
func reconcileReplyStore(cur, base, file ReplyStore) ReplyStore {
	next := cloneReply(cur)
	for id, e := range file.Entries {
		if b, ok := base.Entries[id]; ok && entryEqual(b, e) {
			continue
		}
		if mem, ok := next.Entries[id]; ok {
			e = mergeEntry(mem, e)
		}
		next.Entries[id] = e
	}
	for id, b := range base.Entries {
		if _, ok := file.Entries[id]; ok {
			continue
		}
		if mem, ok := next.Entries[id]; ok && entryEqual(mem, b) {
			delete(next.Entries, id)
		}
	}
	next.Queues = reconcileQueues(next.Queues, base.Queues, file.Queues)
	return next
}

func mergeEntry(mem, file ReplyEntry) ReplyEntry {
	out := file
	if mem.LastSeen > out.LastSeen {
		out.LastSeen = mem.LastSeen
	}
	contacts := maps.Clone(file.Contacts)
	if contacts == nil {
		contacts = map[string]Contact{}
	}
	for addr, c := range mem.Contacts {
		if f, ok := contacts[addr]; !ok || c.LastExchange > f.LastExchange {
			contacts[addr] = c
		}
	}
	if len(contacts) > MaxContacts {
		for _, rec := range SortedContacts(contacts)[MaxContacts:] {
			delete(contacts, rec.Address)
		}
	}
	if len(contacts) == 0 {
		contacts = nil
	}
	out.Contacts = contacts
	return out
}

func reconcileQueues(cur, base, file map[string][]Envelope) map[string][]Envelope {
	out := copyQueues(cur)
	names := map[string]bool{}
	for name := range base {
		names[name] = true
	}
	for name := range file {
		names[name] = true
	}
	for name := range names {
		q := reconcileQueue(out[name], base[name], file[name])
		if len(q) == 0 {
			delete(out, name)
		} else {
			out[name] = q
		}
	}
	return out
}

// envelopeKey is an envelope's identity for matching: its value without
// the notified flag an old daemon's sweep may set.
func envelopeKey(e Envelope) Envelope {
	e.UndeliveredNotified = false
	return e
}

// reconcileQueue applies a legacy queue's change since base onto cur: an
// envelope base held and the file lost was consumed (dropped from cur), an
// envelope the file holds beyond base is new (appended), and a flag the
// file set is set on cur's matching envelope.
func reconcileQueue(cur, base, file []Envelope) []Envelope {
	remaining := map[Envelope]int{}
	for _, e := range base {
		remaining[envelopeKey(e)]++
	}
	out := slices.Clone(cur)
	var added []Envelope
	for _, e := range file {
		k := envelopeKey(e)
		if remaining[k] > 0 {
			remaining[k]--
			if e.UndeliveredNotified {
				for i := range out {
					if !out[i].UndeliveredNotified && envelopeKey(out[i]) == k {
						out[i].UndeliveredNotified = true
						break
					}
				}
			}
			continue
		}
		added = append(added, e)
	}
	for k, n := range remaining {
		for ; n > 0; n-- {
			i := slices.IndexFunc(out, func(e Envelope) bool { return envelopeKey(e) == k })
			if i < 0 {
				break
			}
			out = slices.Delete(out, i, i+1)
		}
	}
	out = append(out, added...)
	if len(out) > MaxQueueLen {
		out = out[len(out)-MaxQueueLen:]
	}
	return out
}

// exportMark stamps a file the store writes (see ExportMark).
func exportMark() string { return storeNow().Format(time.RFC3339Nano) }

// export writes the live state back to the legacy files where it differs
// from the baseline, then records the new baseline. A file is written
// before its baseline record: a crash between the two leaves a file still
// carrying ExportMark, which the next open takes as its own baseline.
func (in *instance) export() error {
	rec := &record{Kind: recordTx, Op: "export", At: storeNow().Format(time.RFC3339Nano), Baseline: &baseline{Named: map[string]StoreFile{}, History: map[string][]string{}}}
	var errs []error
	for _, path := range slices.Sorted(maps.Keys(in.st.Named)) {
		s := in.st.Named[path]
		want := StoreFile{SchemaVersion: schemaVersion, Presence: s.Presence, Queues: nonEmpty(s.Queues)}
		recorded, known := in.st.Baseline.Named[path]
		if known && storeEqual(want, recorded) {
			continue
		}
		if !known && len(want.Presence) == 0 && len(want.Queues) == 0 {
			continue
		}
		if err := WithFileLock(path, func(current *StoreFile) error {
			*current = want
			current.ExportMark = exportMark()
			return nil
		}); err != nil {
			errs = append(errs, err)
			continue
		}
		rec.Baseline.Named[path] = want
	}
	want := ReplyStore{SchemaVersion: schemaVersion, Entries: in.st.Reply.Entries, Queues: nonEmpty(in.st.Reply.Queues)}
	recorded := emptyReplyStore()
	if in.st.Baseline.Reply != nil {
		recorded = *in.st.Baseline.Reply
	}
	if !replyEqual(want, recorded) {
		if err := WithReplyFileLock(in.replyPath, func(current *ReplyStore) error {
			*current = want
			current.ExportMark = exportMark()
			return nil
		}); err != nil {
			errs = append(errs, err)
		} else {
			rec.Baseline.Reply = &want
		}
	}
	root := filepath.Join(in.cacheRoot, HistoryDirName)
	for _, key := range slices.Sorted(maps.Keys(in.st.History)) {
		window := in.st.History[key]
		dir := filepath.Join(root, key)
		// Trust the baseline only for names still on disk: a prune may
		// have removed a directory the store last wrote long ago.
		onDisk := map[string]bool{}
		if names, err := legacyHistoryNames(dir); err == nil {
			for _, name := range names {
				onDisk[name] = true
			}
		}
		known := map[string]bool{}
		for _, name := range in.st.Baseline.History[key] {
			if onDisk[name] {
				known[name] = true
			}
		}
		var missing []HistoryEntry
		for _, e := range window {
			if !known[e.Name] {
				missing = append(missing, e)
			}
		}
		if len(missing) == 0 {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range missing {
			if err := writeLegacyHistoryEntry(dir, e); err != nil {
				errs = append(errs, err)
			}
		}
		names, err := legacyHistoryNames(dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, name := range names[min(len(names), HistoryCap):] {
			_ = os.Remove(filepath.Join(dir, name))
		}
		rec.Baseline.History[key] = names[:min(len(names), HistoryCap)]
	}
	if len(rec.Baseline.Named) == 0 && rec.Baseline.Reply == nil && len(rec.Baseline.History) == 0 {
		return errors.Join(errs...)
	}
	if err := in.ring.append(rec); err != nil {
		errs = append(errs, err)
	} else {
		in.st.apply(rec)
	}
	return errors.Join(errs...)
}
