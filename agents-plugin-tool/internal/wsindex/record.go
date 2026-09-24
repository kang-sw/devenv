package wsindex

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// SchemaVersion is the index record schema. v1 stores registration and
// ownership only; stage, project settings, and terminal state are reserved
// for a later version, which adds fields rather than reshaping these.
const SchemaVersion = 1

// Lease phases.
const (
	PhaseActive = "active"
	// PhaseClosed marks a lease whose ticket was closed on some branch but has
	// not landed on the review-track yet; it still counts as owned.
	PhaseClosed = "closed"
)

// Owner is the full ownership identity. Every "caller's own" comparison uses
// the whole triple: two collaborators on the same branch name are different
// owners, and two worktrees of one person are different owners.
type Owner struct {
	Email   string `json:"email"`
	CloneID string `json:"clone_id"`
	Track   string `json:"track"`
}

// String renders the triple for reports and audit lines.
func (o Owner) String() string {
	return fmt.Sprintf("%s (track %s, clone %s)", o.Email, o.Track, o.CloneID)
}

// Index is one index version: the whole document stored at indexFile.
type Index struct {
	Meta          Meta                     `json:"meta"`
	Registrations map[string]*Registration `json:"registrations"`
}

// Meta carries the schema version and the GC watermark.
type Meta struct {
	Schema int        `json:"schema"`
	LastGC *time.Time `json:"last_gc,omitempty"`
}

// Registration records that a stem exists, with its optional lease. The map
// key in Index.Registrations is the stem.
type Registration struct {
	RegisteredAt time.Time `json:"registered_at"`
	Lease        *Lease    `json:"lease,omitempty"`
}

// Lease is an ownership claim. Worktree paths are never stored.
type Lease struct {
	Email     string    `json:"email"`
	CloneID   string    `json:"clone_id"`
	Track     string    `json:"track"`
	Phase     string    `json:"phase"`
	TouchedAt time.Time `json:"touched_at"`
	Impl      *Impl     `json:"impl,omitempty"`
}

// Impl records the worker implementation branch under a lease.
type Impl struct {
	Branch string `json:"branch"`
}

// Owner returns the lease's owner triple.
func (l *Lease) Owner() Owner {
	return Owner{Email: l.Email, CloneID: l.CloneID, Track: l.Track}
}

// NewIndex returns an empty v1 index.
func NewIndex() *Index {
	return &Index{Meta: Meta{Schema: SchemaVersion}, Registrations: map[string]*Registration{}}
}

// Clone deep-copies the index so a retried CAS attempt re-applies its
// operations to an untouched copy of the fresh tip.
func (idx *Index) Clone() *Index {
	out := &Index{Meta: idx.Meta, Registrations: make(map[string]*Registration, len(idx.Registrations))}
	if idx.Meta.LastGC != nil {
		t := *idx.Meta.LastGC
		out.Meta.LastGC = &t
	}
	for stem, reg := range idx.Registrations {
		copyReg := *reg
		if reg.Lease != nil {
			lease := *reg.Lease
			if reg.Lease.Impl != nil {
				impl := *reg.Lease.Impl
				lease.Impl = &impl
			}
			copyReg.Lease = &lease
		}
		out.Registrations[stem] = &copyReg
	}
	return out
}

// Register adds a registration for stem when absent and reports whether it
// changed the index. The first registrant wins; re-registering is a no-op.
func (idx *Index) Register(stem string, now time.Time) bool {
	if _, ok := idx.Registrations[stem]; ok {
		return false
	}
	idx.Registrations[stem] = &Registration{RegisteredAt: now.UTC().Truncate(time.Second)}
	return true
}

// Stems returns the registered stems in sorted order.
func (idx *Index) Stems() []string {
	stems := make([]string, 0, len(idx.Registrations))
	for stem := range idx.Registrations {
		stems = append(stems, stem)
	}
	sort.Strings(stems)
	return stems
}

// EncodeIndex renders the stored form: indented JSON with sorted keys so the
// commit chain diffs line by line.
func EncodeIndex(idx *Index) ([]byte, error) {
	if idx.Registrations == nil {
		idx.Registrations = map[string]*Registration{}
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// DecodeIndex parses a stored index. Unknown fields are ignored so a v1
// reader tolerates additive later fields.
func DecodeIndex(data []byte) (*Index, error) {
	idx := NewIndex()
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, fmt.Errorf("decode ticket index: %w", err)
	}
	if idx.Registrations == nil {
		idx.Registrations = map[string]*Registration{}
	}
	for stem, reg := range idx.Registrations {
		if reg == nil {
			delete(idx.Registrations, stem)
		}
	}
	return idx, nil
}

func equalIndex(a, b *Index) bool {
	ea, errA := EncodeIndex(a)
	eb, errB := EncodeIndex(b)
	return errA == nil && errB == nil && string(ea) == string(eb)
}
