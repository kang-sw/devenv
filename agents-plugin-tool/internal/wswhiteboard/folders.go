package wswhiteboard

// The command model remains pure. This file alone owns the authoritative
// folder store; callers own session identity, legacy reads, and derived HTML.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// FolderSchemaVersion replaces the unversioned (implicitly v1) session board.
const FolderSchemaVersion = 2

// FolderStore serializes every open and update, including first-read conversion.
// Fault is a test seam called at durable commit boundaries. An error after
// "intent" has an uncertain result: the next open completes that committed
// intent, never retries the caller's mutation.
//
// Dir is a reply-ID session directory, not a session key or a topic name.
// Authoritative folders outlive session keys; this store never removes them.
type FolderStore struct {
	Dir   string
	Fault func(stage string) error
}

type folderTopicRef struct {
	Key      string `json:"key"`
	Dir      string `json:"dir"`
	HasDoc   bool   `json:"has_doc,omitempty"`
	LogBytes int64  `json:"log_bytes"`
}

type folderSession struct {
	SchemaVersion int              `json:"schema_version"`
	Generation    int              `json:"generation"`
	NextTopic     int              `json:"next_topic"`
	State         State            `json:"state"`
	Topics        []folderTopicRef `json:"topics,omitempty"`
	ItemOrder     []int            `json:"item_order,omitempty"`
}

type folderTopic struct {
	SchemaVersion int       `json:"schema_version"`
	Doc           *TopicDoc `json:"doc,omitempty"`
	Items         []Item    `json:"items,omitempty"`
	Label         *string   `json:"label,omitempty"`
	Closed        bool      `json:"closed,omitempty"`
	CloseResult   *string   `json:"close_result,omitempty"`
}

type folderWrite struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

type folderAppend struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
}

// pending.json is the commit decision, atomically installed before any data
// publication. It holds exact bytes and append offsets, so replay tolerates
// both a partial append and a crash after publication without duplicating it.
// meta.json is published last. committed.json is only a receipt: it prevents
// missing authoritative metadata from silently resurrecting a stale legacy
// board, including a committed board with no topics.
type folderIntent struct {
	SchemaVersion  int            `json:"schema_version"`
	BaseGeneration int            `json:"base_generation"`
	Session        folderSession  `json:"session"`
	Panels         []folderWrite  `json:"panels,omitempty"`
	Logs           []folderAppend `json:"logs,omitempty"`
	Topics         []folderWrite  `json:"topics,omitempty"`
}

type folderReceipt struct {
	SchemaVersion int `json:"schema_version"`
	Generation    int `json:"generation"`
}

// Open loads the one authority, recovering and converting under the same lock
// used by writers. An absent board with no legacy snapshot has no side effects.
// Legacy is copied, never modified or written back.
func (f FolderStore) Open(legacy *State) (*State, error) {
	if legacy == nil {
		present := false
		for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
			_, err := os.Stat(filepath.Join(f.Dir, name))
			if err == nil {
				present = true
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		if !present {
			return nil, nil
		}
	}
	var out *State
	err := f.locked(func() error {
		var err error
		out, _, err = f.load(legacy)
		return err
	})
	return out, err
}

// Update runs fn on a fresh authoritative state while holding the session
// flock through durable publication. fn receives a nonnil state, even for a
// new board. A refused callback writes nothing (apart from first-open legacy
// conversion). Render bookkeeping can use this same boundary without events.
func (f FolderStore) Update(legacy *State, fn func(*State) error) error {
	return f.UpdateAndPublish(legacy, fn, nil)
}

// UpdateAndPublish keeps the same lock through derived-output publication, so
// an older writer cannot publish over a later writer's output. A publication
// failure is returned, but never rolls back the committed authoritative state.
func (f FolderStore) UpdateAndPublish(legacy *State, fn func(*State) error, publish func() error) error {
	return f.locked(func() error {
		before, meta, err := f.load(legacy)
		if err != nil {
			return err
		}
		work := &State{}
		if before != nil {
			work = before.clone()
		}
		if err := fn(work); err != nil {
			return err
		}
		if before == nil || !reflect.DeepEqual(before, work) {
			intent, err := f.plan(meta, before, work)
			if err != nil {
				return err
			}
			if err := f.commit(intent); err != nil {
				return err
			}
		}
		if publish != nil {
			return publish()
		}
		return nil
	})
}

func (f FolderStore) locked(fn func() error) error {
	if f.Dir == "" {
		return fmt.Errorf("whiteboard folder: empty directory")
	}
	// Check before even creating a lock file; check again under the lock to
	// cover another writer publishing between this check and acquisition.
	if err := f.guard(); err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(f.Dir, ".lock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock whiteboard folder: %w", err)
	}
	if !ok {
		return fmt.Errorf("timed out locking whiteboard folder")
	}
	defer lock.Unlock() //nolint:errcheck
	if err := f.guard(); err != nil {
		return err
	}
	return fn()
}

// folderDecode reads path through wsstate.ReadFile: the unlocked guard
// reads while another process's locked commit replaces or removes the same
// file, which on Windows fails an open for a moment.
func folderDecode(path string, out any) (bool, error) {
	data, err := wsstate.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false, fmt.Errorf("read whiteboard %s: %w", path, err)
	}
	return true, nil
}

func folderCheckSchema(path string) error {
	var h struct {
		SchemaVersion int `json:"schema_version"`
	}
	exists, err := folderDecode(path, &h)
	if err != nil || !exists {
		return err
	}
	if h.SchemaVersion > FolderSchemaVersion {
		return fmt.Errorf("whiteboard %s: newer schema_version %d (supported %d)", path, h.SchemaVersion, FolderSchemaVersion)
	}
	if h.SchemaVersion != FolderSchemaVersion {
		return fmt.Errorf("whiteboard %s: unsupported schema_version %d", path, h.SchemaVersion)
	}
	return nil
}

func (f FolderStore) guard() error {
	for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
		if err := folderCheckSchema(filepath.Join(f.Dir, name)); err != nil {
			return err
		}
	}
	var current folderSession
	if _, err := folderDecode(filepath.Join(f.Dir, "meta.json"), &current); err != nil {
		return err
	}
	var pending folderIntent
	if exists, err := folderDecode(filepath.Join(f.Dir, "pending.json"), &pending); err != nil {
		return err
	} else if exists && pending.Session.SchemaVersion != FolderSchemaVersion {
		return fmt.Errorf("whiteboard pending session: unsupported schema_version %d", pending.Session.SchemaVersion)
	}
	// Refuse every newer nested schema before replay can publish even its
	// first panel or log byte, not merely when metadata is reached.
	for _, w := range pending.Topics {
		if !folderIntentPath(w.Path, "meta") {
			return fmt.Errorf("whiteboard: invalid pending metadata path")
		}
		var topic folderTopic
		if err := json.Unmarshal(w.Data, &topic); err != nil {
			return err
		}
		if topic.SchemaVersion != FolderSchemaVersion {
			return fmt.Errorf("whiteboard: unsupported pending topic schema_version %d", topic.SchemaVersion)
		}
	}
	for _, w := range pending.Panels {
		if !folderIntentPath(w.Path, "panel") {
			return fmt.Errorf("whiteboard: invalid pending panel path")
		}
	}
	for _, a := range pending.Logs {
		if !folderIntentPath(a.Path, "log") || a.Offset < 0 {
			return fmt.Errorf("whiteboard: invalid pending log path/offset")
		}
	}
	for _, ref := range append(current.Topics, pending.Session.Topics...) {
		if !folderDirID(ref.Dir) {
			return fmt.Errorf("whiteboard: invalid topic directory %q", ref.Dir)
		}
		if err := folderCheckSchema(filepath.Join(f.Dir, ref.Dir, "meta.json")); err != nil {
			return err
		}
	}
	return nil
}

func (f FolderStore) load(legacy *State) (*State, *folderSession, error) {
	var pending folderIntent
	if exists, err := folderDecode(filepath.Join(f.Dir, "pending.json"), &pending); err != nil {
		return nil, nil, err
	} else if exists {
		if err := f.replay(&pending); err != nil {
			return nil, nil, err
		}
	}
	var meta folderSession
	exists, err := folderDecode(filepath.Join(f.Dir, "meta.json"), &meta)
	if err != nil {
		return nil, nil, err
	}
	if exists {
		state, err := f.readState(&meta)
		return state, &meta, err
	}
	if _, err := os.Stat(filepath.Join(f.Dir, "committed.json")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("whiteboard committed metadata is missing; refusing legacy fallback")
	}
	if legacy == nil {
		return nil, nil, nil
	}
	work := legacy.clone()
	// The legacy recap is the only provable historical event. Keep its time,
	// even if absent; do not manufacture owner answers or body/item events.
	for _, doc := range work.Topics {
		for _, e := range doc.Entries {
			work.EntrySeq = max(work.EntrySeq, folderNumber(e.ID, "e"))
			work.PanelCounter = max(work.PanelCounter, folderNumber(e.PanelID, "p"))
		}
	}
	for i := range work.Topics {
		d := &work.Topics[i]
		if d.Recap != "" {
			work.EntrySeq++
			d.Entries = append(d.Entries, Entry{ID: fmt.Sprintf("e%d", work.EntrySeq), Kind: "agent", Text: d.Recap, At: d.RecapAt, Imported: true})
		}
		d.Recap, d.RecapAt, d.RecapChanged = "", "", 0
	}
	if err := f.hit("conversion"); err != nil {
		return nil, nil, err
	}
	intent, err := f.plan(nil, nil, work)
	if err != nil {
		return nil, nil, err
	}
	if err := f.commit(intent); err != nil {
		return nil, nil, err
	}
	return work, &intent.Session, nil
}

func folderNumber(id, prefix string) int {
	if !strings.HasPrefix(id, prefix) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil || n < 1 || prefix+strconv.Itoa(n) != id {
		return 0
	}
	return n
}

func folderDirID(id string) bool { return folderNumber(id, "t") > 0 }

func folderJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	} // all store values are JSON-safe model structs
	return append(data, '\n')
}

func (f FolderStore) readState(meta *folderSession) (*State, error) {
	out := meta.State.clone()
	out.Topics, out.Items, out.Labels, out.CloseResults = nil, nil, nil, nil
	items := map[int]Item{}
	heldEntries, heldDirs, heldKeys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, ref := range meta.Topics {
		if !folderDirID(ref.Dir) || heldDirs[ref.Dir] || heldKeys[ref.Key] {
			return nil, fmt.Errorf("whiteboard: invalid or duplicate topic mapping")
		}
		heldDirs[ref.Dir], heldKeys[ref.Key] = true, true
		var topic folderTopic
		if exists, err := folderDecode(filepath.Join(f.Dir, ref.Dir, "meta.json"), &topic); err != nil {
			return nil, err
		} else if !exists {
			return nil, fmt.Errorf("whiteboard topic %s: metadata missing", ref.Dir)
		}
		if (topic.Doc != nil) != ref.HasDoc {
			return nil, fmt.Errorf("whiteboard topic %s: content mapping mismatch", ref.Dir)
		}
		if topic.Doc != nil && topic.Doc.Name != ref.Key {
			return nil, fmt.Errorf("whiteboard topic %s: key mismatch", ref.Dir)
		}
		for _, item := range topic.Items {
			if _, ok := items[item.ID]; ok || item.Topic != ref.Key {
				return nil, fmt.Errorf("whiteboard: invalid or duplicate item %d", item.ID)
			}
			items[item.ID] = item
		}
		if topic.Label != nil {
			if out.Labels == nil {
				out.Labels = map[string]string{}
			}
			out.Labels[ref.Key] = *topic.Label
		}
		if topic.CloseResult != nil {
			if out.CloseResults == nil {
				out.CloseResults = map[string]string{}
			}
			out.CloseResults[ref.Key] = *topic.CloseResult
		}
		if topic.Closed != slices.Contains(out.ClosedTopics, ref.Key) {
			return nil, fmt.Errorf("whiteboard topic %s: close state mismatch", ref.Dir)
		}
		log, err := os.ReadFile(filepath.Join(f.Dir, ref.Dir, "log.jsonl"))
		if err != nil {
			return nil, err
		}
		if int64(len(log)) != ref.LogBytes {
			return nil, fmt.Errorf("whiteboard topic %s: committed log length mismatch", ref.Dir)
		}
		var entries []Entry
		last := 0
		for _, line := range bytes.Split(log, []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var e Entry
			if err := json.Unmarshal(line, &e); err != nil {
				return nil, fmt.Errorf("whiteboard topic %s log: %w", ref.Dir, err)
			}
			n := folderNumber(e.ID, "e")
			if n <= last || n > out.EntrySeq || heldEntries[e.ID] {
				return nil, fmt.Errorf("whiteboard: invalid entry sequence %q", e.ID)
			}
			last, heldEntries[e.ID] = n, true
			if e.Kind == "panel" {
				if e.Text != "" {
					return nil, fmt.Errorf("whiteboard: panel payload in log")
				}
				var panel Entry
				if exists, err := folderDecode(filepath.Join(f.Dir, ref.Dir, "panels", e.ID+".json"), &panel); err != nil {
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
			if err := folderValidEntry(e, out.PanelCounter); err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
		if len(entries) != 0 && topic.Doc == nil {
			return nil, fmt.Errorf("whiteboard topic %s: entries without content mapping", ref.Dir)
		}
		if topic.Doc != nil {
			topic.Doc.Entries = entries
			out.Topics = append(out.Topics, *topic.Doc)
		}
	}
	for _, id := range meta.ItemOrder {
		item, ok := items[id]
		if !ok {
			return nil, fmt.Errorf("whiteboard: missing or repeated item %d", id)
		}
		out.Items = append(out.Items, item)
		delete(items, id)
	}
	if len(items) != 0 {
		return nil, fmt.Errorf("whiteboard: incomplete item order")
	}
	return out, nil
}

func folderValidEntry(e Entry, panelCounter int) error {
	if folderNumber(e.ID, "e") == 0 {
		return fmt.Errorf("whiteboard: invalid entry ID %q", e.ID)
	}
	switch e.Kind {
	case "panel":
		if n := folderNumber(e.PanelID, "p"); n == 0 || n > panelCounter {
			return fmt.Errorf("whiteboard: invalid panel ID %q", e.PanelID)
		}
	case "agent", "system":
		if e.PanelID != "" {
			return fmt.Errorf("whiteboard: log entry has panel ID")
		}
	default:
		return fmt.Errorf("whiteboard: invalid entry kind %q", e.Kind)
	}
	return nil
}

func folderKeys(s *State) []string {
	var keys []string
	add := func(k string) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	for _, d := range s.Topics {
		add(d.Name)
	}
	for _, item := range s.Items {
		add(item.Topic)
	}
	for _, k := range s.ClosedTopics {
		add(k)
	}
	// Maps must not introduce nondeterministic topic ordering.
	var rest []string
	for k := range s.Labels {
		rest = append(rest, k)
	}
	for k := range s.CloseResults {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		add(k)
	}
	return keys
}

func folderDoc(s *State, key string) *TopicDoc {
	if s != nil {
		for i := range s.Topics {
			if s.Topics[i].Name == key {
				return &s.Topics[i]
			}
		}
	}
	return nil
}

// Entries and items are stable anchors across a stemless topic's adoption.
// For content-only legacy topics the adopting label is the old logical key.
func folderSameTopic(before, after *State, old, key string) bool {
	if after.Labels[key] == old {
		return true
	}
	a, b := folderDoc(before, old), folderDoc(after, key)
	if a != nil && b != nil {
		for _, x := range a.Entries {
			for _, y := range b.Entries {
				if x.ID == y.ID {
					return true
				}
			}
		}
	}
	for _, x := range before.Items {
		if x.Topic == old {
			for _, y := range after.Items {
				if y.Topic == key && x.ID == y.ID {
					return true
				}
			}
		}
	}
	return false
}

func (f FolderStore) plan(meta *folderSession, before, after *State) (*folderIntent, error) {
	intent := &folderIntent{SchemaVersion: FolderSchemaVersion}
	intent.Session = folderSession{SchemaVersion: FolderSchemaVersion, Generation: 1, State: *after.clone()}
	if meta != nil {
		intent.BaseGeneration = meta.Generation
		intent.Session.Generation = meta.Generation + 1
		intent.Session.NextTopic = meta.NextTopic
		if after.EntrySeq < before.EntrySeq || after.PanelCounter < before.PanelCounter {
			return nil, fmt.Errorf("whiteboard: entry/panel counters cannot decrease")
		}
	}
	intent.Session.State.Items, intent.Session.State.Topics, intent.Session.State.Labels, intent.Session.State.CloseResults = nil, nil, nil, nil
	for _, item := range after.Items {
		intent.Session.ItemOrder = append(intent.Session.ItemOrder, item.ID)
	}
	keys := folderKeys(after)
	used, entries, panels := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, key := range keys {
		ref := folderTopicRef{Key: key}
		oldKey := key
		if meta != nil {
			// Adoption retains the source topic's history directory even when
			// the destination key still has a stale close-only mapping.
			for _, old := range meta.Topics {
				if !used[old.Dir] && !slices.Contains(keys, old.Key) && before.topicKnown(old.Key) && folderSameTopic(before, after, old.Key, key) {
					ref, oldKey = old, old.Key
					ref.Key = key
					break
				}
			}
			if ref.Dir == "" {
				for _, old := range meta.Topics {
					if old.Key == key {
						ref = old
						break
					}
				}
			}
		}
		if ref.Dir == "" {
			intent.Session.NextTopic++
			ref.Dir = fmt.Sprintf("t%d", intent.Session.NextTopic)
		}
		used[ref.Dir] = true
		topic := folderTopic{SchemaVersion: FolderSchemaVersion, Closed: slices.Contains(after.ClosedTopics, key)}
		if label, ok := after.Labels[key]; ok {
			topic.Label = &label
		}
		if result, ok := after.CloseResults[key]; ok {
			topic.CloseResult = &result
		}
		for _, item := range after.Items {
			if item.Topic == key {
				topic.Items = append(topic.Items, item)
			}
		}
		doc, prior := folderDoc(after, key), folderDoc(before, oldKey)
		var oldEntries, newEntries []Entry
		if prior != nil {
			oldEntries = prior.Entries
		}
		if doc != nil {
			copy := *doc
			copy.Entries = nil
			topic.Doc = &copy
			newEntries = doc.Entries
		}
		ref.HasDoc = doc != nil
		if len(newEntries) < len(oldEntries) || !reflect.DeepEqual(newEntries[:len(oldEntries)], oldEntries) && len(oldEntries) != 0 {
			return nil, fmt.Errorf("whiteboard topic %q: entries are append-only", key)
		}
		last := 0
		for i, e := range newEntries {
			if err := folderValidEntry(e, after.PanelCounter); err != nil {
				return nil, err
			}
			n := folderNumber(e.ID, "e")
			if n <= last || n > after.EntrySeq || entries[e.ID] {
				return nil, fmt.Errorf("whiteboard: invalid entry sequence %q", e.ID)
			}
			if i >= len(oldEntries) && before != nil && n <= before.EntrySeq {
				return nil, fmt.Errorf("whiteboard: new entry reuses sequence %q", e.ID)
			}
			last, entries[e.ID] = n, true
			if e.Kind == "panel" {
				if owner, ok := panels[e.PanelID]; ok && owner != key {
					return nil, fmt.Errorf("whiteboard: panel %s belongs to multiple topics", e.PanelID)
				}
				panels[e.PanelID] = key
			}
		}
		var log bytes.Buffer
		for _, e := range newEntries[len(oldEntries):] {
			if e.Kind == "panel" {
				intent.Panels = append(intent.Panels, folderWrite{Path: ref.Dir + "/panels/" + e.ID + ".json", Data: folderJSON(e)})
				e.Text = ""
			}
			log.Write(folderJSON(e))
		}
		if ref.LogBytes == 0 || log.Len() != 0 {
			intent.Logs = append(intent.Logs, folderAppend{Path: ref.Dir + "/log.jsonl", Offset: ref.LogBytes, Data: log.Bytes()})
		}
		ref.LogBytes += int64(log.Len())
		intent.Topics = append(intent.Topics, folderWrite{Path: ref.Dir + "/meta.json", Data: folderJSON(topic)})
		intent.Session.Topics = append(intent.Session.Topics, ref)
	}
	if meta != nil {
		for _, old := range meta.Topics {
			if !used[old.Dir] {
				if d := folderDoc(before, old.Key); d != nil && len(d.Entries) != 0 {
					return nil, fmt.Errorf("whiteboard: cannot remove topic entry history")
				}
			}
		}
	}
	return intent, nil
}

func (f FolderStore) hit(stage string) error {
	if f.Fault != nil {
		return f.Fault(stage)
	}
	return nil
}

func (f FolderStore) commit(intent *folderIntent) error {
	if err := f.atomic("pending.json", folderJSON(intent), "intent-staged"); err != nil {
		return err
	}
	if err := f.hit("intent"); err != nil {
		return err
	}
	return f.replay(intent)
}

func (f FolderStore) replay(intent *folderIntent) error {
	var current folderSession
	exists, err := folderDecode(filepath.Join(f.Dir, "meta.json"), &current)
	if err != nil {
		return err
	}
	if !exists && intent.BaseGeneration != 0 || exists && current.Generation != intent.BaseGeneration && current.Generation != intent.Session.Generation {
		return fmt.Errorf("whiteboard: pending generation does not follow committed metadata")
	}
	if exists && current.Generation == intent.Session.Generation && !bytes.Equal(folderJSON(current), folderJSON(intent.Session)) {
		return fmt.Errorf("whiteboard: pending metadata conflicts with committed generation")
	}
	for _, w := range intent.Panels {
		if !folderIntentPath(w.Path, "panel") {
			return fmt.Errorf("whiteboard: invalid pending panel path")
		}
		path := filepath.Join(f.Dir, filepath.FromSlash(w.Path))
		if data, err := os.ReadFile(path); err == nil {
			if !bytes.Equal(data, w.Data) {
				return fmt.Errorf("whiteboard: immutable panel %s conflicts with intent", w.Path)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else if err := f.atomic(w.Path, w.Data, "panel-staged"); err != nil {
			return err
		}
		if err := f.hit("panel"); err != nil {
			return err
		}
	}
	for _, a := range intent.Logs {
		if !folderIntentPath(a.Path, "log") || a.Offset < 0 {
			return fmt.Errorf("whiteboard: invalid pending log path/offset")
		}
		if err := f.append(a); err != nil {
			return err
		}
		if err := f.hit("log"); err != nil {
			return err
		}
	}
	for _, w := range intent.Topics {
		if !folderIntentPath(w.Path, "meta") {
			return fmt.Errorf("whiteboard: invalid pending metadata path")
		}
		var topic folderTopic
		if err := json.Unmarshal(w.Data, &topic); err != nil {
			return err
		}
		if topic.SchemaVersion != FolderSchemaVersion {
			return fmt.Errorf("whiteboard: unsupported pending topic schema_version %d", topic.SchemaVersion)
		}
		if err := f.atomic(w.Path, w.Data, "topic-meta-staged"); err != nil {
			return err
		}
		if err := f.hit("topic-meta"); err != nil {
			return err
		}
	}
	if err := f.atomic("meta.json", folderJSON(intent.Session), "session-meta-staged"); err != nil {
		return err
	}
	if err := f.hit("session-meta"); err != nil {
		return err
	}
	if err := f.atomic("committed.json", folderJSON(folderReceipt{FolderSchemaVersion, intent.Session.Generation}), "receipt-staged"); err != nil {
		return err
	}
	if err := f.hit("cleanup"); err != nil {
		return err
	}
	// Another process's unlocked guard may be reading pending.json, which
	// on Windows fails a plain removal; wsstate.RemoveFile retries it.
	if err := wsstate.RemoveFile(filepath.Join(f.Dir, "pending.json")); err != nil {
		return err
	}
	if err := folderSyncDir(f.Dir); err != nil {
		return err
	}
	return f.hit("complete")
}

func folderIntentPath(path, kind string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || !folderDirID(parts[0]) {
		return false
	}
	switch kind {
	case "panel":
		return len(parts) == 3 && parts[1] == "panels" && strings.HasSuffix(parts[2], ".json") && folderNumber(strings.TrimSuffix(parts[2], ".json"), "e") > 0
	case "log":
		return len(parts) == 2 && parts[1] == "log.jsonl"
	case "meta":
		return len(parts) == 2 && parts[1] == "meta.json"
	}
	return false
}

func (f FolderStore) append(a folderAppend) error {
	path := filepath.Join(f.Dir, filepath.FromSlash(a.Path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	written := stat.Size() - a.Offset
	if written < 0 || written > int64(len(a.Data)) {
		return fmt.Errorf("whiteboard log %s: append offset mismatch", a.Path)
	}
	prefix := make([]byte, int(written))
	if _, err := file.ReadAt(prefix, a.Offset); err != nil && err != io.EOF {
		return err
	}
	if !bytes.Equal(prefix, a.Data[:int(written)]) {
		return fmt.Errorf("whiteboard log %s: replay prefix mismatch", a.Path)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	rest := a.Data[int(written):]
	// Splitting deliberately exposes the real partial-append recovery path.
	if len(rest) > 1 {
		half := len(rest) / 2
		if _, err := file.Write(rest[:half]); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := f.hit("log-partial"); err != nil {
			return err
		}
		rest = rest[half:]
	}
	if _, err := file.Write(rest); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return folderSyncDir(filepath.Dir(path))
}

func (f FolderStore) atomic(relative string, data []byte, stage string) error {
	path := filepath.Join(f.Dir, filepath.FromSlash(relative))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Sync parent directory entries as well as file data before committing
	// references to newly created topic/panels directories.
	for parent := dir; ; parent = filepath.Dir(parent) {
		if err := folderSyncDir(parent); err != nil {
			return err
		}
		if parent == filepath.Dir(f.Dir) || parent == filepath.Dir(parent) {
			break
		}
	}
	file, err := os.CreateTemp(dir, ".whiteboard-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := f.hit(stage); err != nil {
		return err
	}
	if err := wsstate.AtomicReplaceFile(file.Name(), path); err != nil {
		return err
	}
	return folderSyncDir(dir)
}

func folderSyncDir(path string) error {
	// MoveFileEx on Windows uses WRITE_THROUGH; directory Sync is unsupported.
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
