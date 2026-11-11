package wstrackerhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// FileName is the hooks file in the dashboard daemon's state directory.
// Only the machine's own user writes it: no config layer, and nothing a
// store holds, can choose what runs.
const FileName = "tracker-hooks.json"

// Hook scopes.
const (
	// ScopeMine delivers the events that concern the store's local member,
	// leaving out the member's own actions. It is the default.
	ScopeMine = "mine"
	// ScopeAll delivers every event of the store.
	ScopeAll = "all"
)

// DefaultDueSoon is how long before an item's due date its due_soon event
// fires when the hooks file sets no due_soon_hours.
const DefaultDueSoon = 24 * time.Hour

// Hook is one entry of the hooks file.
type Hook struct {
	// Command is the argv the daemon runs, without a shell.
	Command []string `json:"command"`
	// Scope is ScopeMine (also when empty) or ScopeAll.
	Scope string `json:"scope,omitempty"`
}

// Config is the hooks file.
type Config struct {
	Hooks []Hook `json:"hooks"`
	// DueSoonHours is the due_soon threshold in hours; zero means
	// DefaultDueSoon.
	DueSoonHours float64 `json:"due_soon_hours,omitempty"`
}

// key identifies a hook across reloads: an entry with the same command and
// scope keeps its queue.
func (h Hook) key() string { return h.Scope + "\x00" + strings.Join(h.Command, "\x00") }

// dueSoon is the due_soon threshold c sets.
func (c Config) dueSoon() time.Duration {
	if c.DueSoonHours <= 0 {
		return DefaultDueSoon
	}
	return time.Duration(c.DueSoonHours * float64(time.Hour))
}

// parseConfig reads a hooks file. It is strict, so a misspelled field is
// an error instead of a hook that silently does something else; an empty
// scope becomes ScopeMine.
func parseConfig(data []byte) (Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("trailing data after the JSON object")
	}
	if c.DueSoonHours < 0 {
		return Config{}, errors.New("due_soon_hours must not be negative")
	}
	seen := map[string]bool{}
	for i := range c.Hooks {
		h := &c.Hooks[i]
		if len(h.Command) == 0 || h.Command[0] == "" {
			return Config{}, fmt.Errorf("hooks[%d]: command must be a non-empty array whose first element names the program", i)
		}
		switch h.Scope {
		case "":
			h.Scope = ScopeMine
		case ScopeMine, ScopeAll:
		default:
			return Config{}, fmt.Errorf("hooks[%d]: scope %q is not %q or %q", i, h.Scope, ScopeMine, ScopeAll)
		}
		if seen[h.key()] {
			return Config{}, fmt.Errorf("hooks[%d]: the same command and scope appear twice", i)
		}
		seen[h.key()] = true
	}
	return c, nil
}
