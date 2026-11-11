// Package wswake broadcasts in-process wake signals to the blocking waits a
// process hosts, so a wait rechecks its stores when they may have changed
// instead of only on its own poll tick.
//
// Two kinds of signal reach every subscriber: a hard wake, sent after this
// process itself wrote something a wait watches, and a nudge, sent when
// another process said one of its stores changed (without saying which).
// The hub only delivers them; how a subscriber paces its rechecks (a
// nudge's coalescing, for one) is the subscriber's.
package wswake

import "sync"

// Hub delivers wake signals to its current subscribers. The zero value is
// ready to use. Signalling a hub with no subscriber costs one lock.
type Hub struct {
	mu   sync.Mutex
	subs map[*Sub]struct{}
}

// Mailbox is the process-wide hub for mailbox waits: mailbox and
// board-answer inbox writes made in this process wake it, and so does a
// dashboard daemon's nudge.
var Mailbox = &Hub{}

// Sub is one subscriber's pair of signal channels. Each holds at most one
// pending signal: signals sent while one is pending fold into it, which a
// level-triggered recheck covers.
type Sub struct {
	hub   *Hub
	hard  chan struct{}
	nudge chan struct{}
}

// Subscribe adds a subscriber; Close removes it.
func (h *Hub) Subscribe() *Sub {
	s := &Sub{hub: h, hard: make(chan struct{}, 1), nudge: make(chan struct{}, 1)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[*Sub]struct{}{}
	}
	h.subs[s] = struct{}{}
	return s
}

// Hard receives the hard wakes.
func (s *Sub) Hard() <-chan struct{} { return s.hard }

// Nudge receives the nudges.
func (s *Sub) Nudge() <-chan struct{} { return s.nudge }

// Close unsubscribes; it is safe to call more than once.
func (s *Sub) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	delete(s.hub.subs, s)
}

// Wake sends a hard wake to every subscriber.
func (h *Hub) Wake() { h.signal(func(s *Sub) chan struct{} { return s.hard }) }

// Nudge sends a nudge to every subscriber.
func (h *Hub) Nudge() { h.signal(func(s *Sub) chan struct{} { return s.nudge }) }

func (h *Hub) signal(pick func(*Sub) chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case pick(s) <- struct{}{}:
		default:
		}
	}
}
