package wslive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// ModelObservation is response evidence, never launch settings or RPC state.
// ID is the producer's stable message/step identity; At is its original
// observation time, not intake/heartbeat time. Effort is recorded for this
// response; EffortSource distinguishes message metadata from request metadata.
// OutputTps is supplied only by an eligible existing block-span rate tracker.
// Missing optional values do not inherit from another response or agent.
type ModelObservation struct {
	ID           string   `json:"id"`
	Model        string   `json:"model"`
	Source       string   `json:"source"`
	At           int64    `json:"at"`
	Effort       string   `json:"effort,omitempty"`
	EffortSource string   `json:"effortSource,omitempty"`
	OutputTps    *float64 `json:"outputTps,omitempty"`
}

// ModelChange is a concrete model transition at the answering evidence's time.
// The first entry has no Previous: it establishes the initial observed model.
type ModelChange struct {
	Model    string `json:"model"`
	Previous string `json:"previous,omitempty"`
	At       int64  `json:"at"`
	Source   string `json:"source"`
}

// ObservedModel keeps latest evidence independently of the history of changes.
// History is chronological, includes the first observation, and omits repeated
// output from the same model. This is additive to legacy agent telemetry.
type ObservedModel struct {
	Current ModelObservation `json:"current"`
	History []ModelChange    `json:"history"`
}

// ObservedRecord is one session-local immutable metadata record. AgentID is
// empty for the session's own output. No transcript text or credentials enter it.
type ObservedRecord struct {
	AgentID string `json:"agentId,omitempty"`
	ModelObservation
}

// ObservedDirName is intentionally separate from snapshot.json: both older
// typed snapshot writers replace that file and discard unknown fields. They
// neither rewrite nor prune this additive directory (unlike the bounded ring);
// only an append prunes it, deleting repeats no reader can see (observed_prune.go).
const ObservedDirName = "model-evidence"

// Change is the latest transition, absent before a second model was observed.
func (o *ObservedModel) Change() *ModelChange {
	if o == nil || len(o.History) < 2 {
		return nil
	}
	v := o.History[len(o.History)-1]
	return &v
}

func cloneObservation(o *ModelObservation) *ModelObservation {
	if o == nil {
		return nil
	}
	v := *o
	if o.OutputTps != nil {
		rate := *o.OutputTps
		v.OutputTps = &rate
	}
	return &v
}

func (o *ObservedModel) clone() *ObservedModel {
	if o == nil {
		return nil
	}
	return &ObservedModel{Current: *cloneObservation(&o.Current), History: append([]ModelChange(nil), o.History...)}
}

// ObservedResponses returns one producer's immutable response metadata in
// evidence order, including responses coalesced before a parent reads state.
func (st *State) ObservedResponses(agentID string) []ModelObservation {
	var out []ModelObservation
	for _, r := range st.observations {
		if r.AgentID == agentID {
			out = append(out, *cloneObservation(&r.ModelObservation))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		return out[i].Source+"\x00"+out[i].ID < out[j].Source+"\x00"+out[j].ID
	})
	return out
}

func observationKey(r ObservedRecord) string { return r.AgentID + "\x00" + r.Source + "\x00" + r.ID }

func (st *State) cleanObservation(o *ModelObservation) *ModelObservation {
	if o == nil || o.At <= 0 {
		return nil
	}
	switch o.Source {
	case "claude.step", "claude.assistant", "pi.assistant":
	default:
		return nil
	}
	out := *o
	out.ID = cleanLine(st.scrub.scrub(o.ID), 256)
	out.Model = cleanLine(st.scrub.scrub(o.Model), ModelRunes)
	if out.ID == "" || out.Model == "" {
		return nil
	}
	if strings.HasPrefix(out.Source, "claude.") {
		switch out.Model {
		case "opus", "sonnet", "haiku", "fable", "default", "<synthetic>":
			return nil
		}
	}
	out.Effort = cleanLine(st.scrub.scrub(o.Effort), EffortRunes)
	if out.EffortSource != "message" && out.EffortSource != "request" {
		out.Effort, out.EffortSource = "", ""
	}
	if out.Effort == "" {
		out.EffortSource = ""
	}
	out.OutputTps = nil
	if n := o.OutputTps; n != nil && !math.IsNaN(*n) && !math.IsInf(*n, 0) && *n > 0 && *n <= MaxOutputTps {
		v := *n
		out.OutputTps = &v
	}
	return &out
}

// SeedObserved restores durable evidence without creating agents, ring entries,
// or activity. Reset keeps this evidence, just as a transcript keeps history.
func (st *State) SeedObserved(records []ObservedRecord) {
	for _, r := range records {
		if r.AgentID != "" && !agentIDPattern.MatchString(r.AgentID) {
			continue
		}
		if o := st.cleanObservation(&r.ModelObservation); o != nil {
			r.ModelObservation = *o
			if st.observations == nil {
				st.observations = map[string]ObservedRecord{}
			}
			key := observationKey(r)
			if _, exists := st.observations[key]; !exists {
				st.observations[key] = r
			}
		}
	}
	st.observed = summarizeObserved(st.observations)
}

// SeedLastTurn restores only explicitly recorded last-turn evidence, never
// reconstructing a turn boundary from a model or snapshot heartbeat.
func (st *State) SeedLastTurn(prev Snapshot) {
	if prev.ReplyID != st.replyID {
		return
	}
	st.previousTurnMs = prev.Lead.PreviousTurnMs
	st.lastTurnOutput = st.cleanObservation(prev.Lead.LastTurnOutput)
}

// addObserved accepts r unless its identity is already known, and returns the
// records the evidence directory now needs. Per agent, that directory has to
// hold only the records that define the fold: the first, each model change and
// the latest (observed_prune.go). A record that defines none of them, such as a
// replayed response a prune deleted, changes no reader's fold and stays in
// memory alone: writing it would only re-create a file for a later prune.
// Each writer seeds its state from the directory, so while no other writer
// adds to it the records returned here keep its fold equal to this state's.
// Another writer's records can at worst re-date a return transition, the
// limitation observed_prune.go already states.
func (st *State) addObserved(r ObservedRecord) (accepted bool, persist []ObservedRecord) {
	if st.observations == nil {
		st.observations = map[string]ObservedRecord{}
	}
	key := observationKey(r)
	if _, exists := st.observations[key]; exists {
		return false, nil
	}
	// r's neighbours in the agent's fold order, before r joins it.
	var prev, next ObservedRecord
	hasPrev, hasNext := false, false
	for _, o := range st.observations {
		switch {
		case o.AgentID != r.AgentID:
		case observedBefore(o, r):
			if !hasPrev || observedBefore(prev, o) {
				prev, hasPrev = o, true
			}
		case !hasNext || observedBefore(o, next):
			next, hasNext = o, true
		}
	}
	st.observations[key] = r
	st.observed = summarizeObserved(st.observations)
	// r is the first, the latest or a model change.
	if !hasPrev || !hasNext || prev.Model != r.Model {
		persist = append(persist, r)
	}
	// A late record of another model makes its successor a change point back;
	// that successor may have been a repeat no file kept.
	if hasPrev && hasNext && next.Model != r.Model && next.Model == prev.Model {
		persist = append(persist, next)
	}
	return true, persist
}

// observedBefore is the fold order: observation time, then identity.
func observedBefore(a, b ObservedRecord) bool {
	if a.At != b.At {
		return a.At < b.At
	}
	return observationKey(a) < observationKey(b)
}

func summarizeObserved(records map[string]ObservedRecord) map[string]*ObservedModel {
	grouped := map[string][]ObservedRecord{}
	for _, r := range records {
		grouped[r.AgentID] = append(grouped[r.AgentID], r)
	}
	out := map[string]*ObservedModel{}
	for id, list := range grouped {
		sort.Slice(list, func(i, j int) bool { return observedBefore(list[i], list[j]) })
		model := &ObservedModel{}
		for _, r := range list {
			if model.Current.Model != r.Model {
				model.History = append(model.History, ModelChange{Previous: model.Current.Model, Model: r.Model, At: r.At, Source: r.Source})
			}
			model.Current = r.ModelObservation
		}
		out[id] = model
	}
	return out
}

// OverlayObserved adds retained evidence after a legacy snapshot rewrite. It
// does not revive activity, create missing children, or borrow parent evidence.
// The snapshot's own evidence wins when it is newer (e.g. an unflushed live slot).
func OverlayObserved(snap *Snapshot, records []ObservedRecord) {
	st := NewState("")
	st.SeedObserved(records)
	overlayObserved(snap, st.observed)
}

func overlayObserved(snap *Snapshot, models map[string]*ObservedModel) {
	overlay := func(id string, dst **ObservedModel) {
		if v := models[id]; v != nil && (*dst == nil || v.Current.At >= (*dst).Current.At) {
			*dst = v
		}
	}
	overlay("", &snap.Lead.Observed)
	histories := make(map[string]*ObservedModel, len(snap.ObservedAgents))
	for id, o := range snap.ObservedAgents {
		histories[id] = o
	}
	for id, o := range models {
		if id != "" && (histories[id] == nil || o.Current.At >= histories[id].Current.At) {
			histories[id] = o
		}
	}
	if len(histories) > 0 {
		snap.ObservedAgents = histories
	}
	// Clone before modifying: dashboard cache snapshots can share their slices.
	snap.Agents = append([]Agent(nil), snap.Agents...)
	for i := range snap.Agents {
		overlay(snap.Agents[i].ID, &snap.Agents[i].Observed)
	}
}

// AppendObserved installs complete immutable files, with first-write-wins
// identity. A repeated/replayed observation cannot move its original time.
// A same-directory hard link publishes a synced temp file without replacing
// another writer's record; a crash leaves at worst an ignored .tmp file.
func AppendObserved(dir string, records []ObservedRecord) error {
	if len(records) == 0 {
		return nil
	}
	dir = filepath.Join(dir, ObservedDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	written := false
	// Retention follows a write, never a read: only a new record grows the
	// directory (observed_prune.go).
	defer func() {
		if written {
			pruneObserved(dir)
		}
	}()
	for _, r := range records {
		sum := sha256.Sum256([]byte(observationKey(r)))
		path := filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
		if _, err := wsstate.Stat(path); err == nil {
			continue
		}
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		tmp, err := wsstate.CreateTemp(dir, ".observation-*.tmp")
		if err != nil {
			return err
		}
		name := tmp.Name()
		err = tmp.Chmod(0600)
		if err == nil {
			_, err = tmp.Write(data)
		}
		if err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Link(name, path)
			if errors.Is(err, os.ErrExist) {
				err = nil
			}
		}
		_ = os.Remove(name)
		if err != nil {
			return fmt.Errorf("model evidence: %w", err)
		}
		written = true
	}
	return nil
}

// ReadObserved reads only bounded metadata records, tolerating missing files,
// unknown JSON members, incomplete writes, and unsupported future sources.
func ReadObserved(dir string) ([]ObservedRecord, error) {
	entries, err := wsstate.ReadDir(filepath.Join(dir, ObservedDirName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	st := NewState("")
	var records []ObservedRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		r, valid, err := readObservedRecord(filepath.Join(dir, ObservedDirName, entry.Name()), wsstate.Open)
		if err == nil && valid {
			records = append(records, r)
		}
	}
	st.SeedObserved(records)
	out := make([]ObservedRecord, 0, len(st.observations))
	for _, r := range st.observations {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return observationKey(out[i]) < observationKey(out[j]) })
	return out, nil
}

func readObservedRecord(path string, open func(string) (*os.File, error)) (ObservedRecord, bool, error) {
	f, err := open(path)
	if err != nil {
		return ObservedRecord{}, false, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	_ = f.Close()
	var r ObservedRecord
	valid := err == nil && len(raw) <= 4096 && json.Unmarshal(raw, &r) == nil
	return r, valid, err
}
