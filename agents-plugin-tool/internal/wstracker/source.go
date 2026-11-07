package wstracker

import (
	"context"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Source is the part of the protocol the domain uses. *wsledger.Ledger
// satisfies it; tests feed an in-memory one.
type Source interface {
	// Events returns the events new since after plus every event whose
	// verdict changed since it.
	Events(after wsledger.Cursor) ([]wsledger.Event, wsledger.Cursor)
	// Concurrent reports whether neither event is in the other's causal
	// past.
	Concurrent(a, b wsledger.EventID) bool
	// Identity returns the protocol's identity state; the domain reads only
	// its Genesis passthrough.
	Identity() wsledger.IdentityState
	// Append writes events to the clone's journal.
	Append(ctx context.Context, events []wsledger.NewEvent) ([]wsledger.EventID, error)
}

var _ Source = (*wsledger.Ledger)(nil)
