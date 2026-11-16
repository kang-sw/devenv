package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

const mailboxSafetyInterval = 5 * time.Minute

type mailboxSweepWork interface {
	Stores() ([]wsmailbox.QueueStore, error)
	Target(string) (wsmailbox.QueueStore, bool)
	Sweep(wsmailbox.QueueStore, time.Time, time.Duration) (int, time.Time, error)
}

// Only deadlines and invalidated paths live here. Durable queue/receipt state
// stays under the existing store locks; a restart starts with full discovery.
type mailboxMaintenance struct {
	work      mailboxSweepWork
	due       map[wsmailbox.QueueStore]time.Time
	scanAt    time.Time
	threshold func() time.Duration
	wake      func()
	log       io.Writer
}

func (m *mailboxMaintenance) step(now time.Time, changed []string) time.Time {
	targets := map[wsmailbox.QueueStore]bool{}
	if m.due == nil {
		m.due = map[wsmailbox.QueueStore]time.Time{}
	}
	if m.scanAt.IsZero() || !now.Before(m.scanAt) {
		stores, err := m.work.Stores()
		if err != nil {
			fmt.Fprintf(m.log, "dashboard: mailbox discovery: %v\n", err)
			m.scanAt = now.Add(wsmailbox.UndeliveredRetryInterval)
		} else {
			current := map[wsmailbox.QueueStore]time.Time{}
			for _, store := range stores {
				current[store] = m.due[store]
				targets[store] = true
			}
			m.due = current
			m.scanAt = now.Add(mailboxSafetyInterval)
		}
	}
	for _, path := range changed {
		if store, ok := m.work.Target(path); ok {
			targets[store] = true
		}
	}
	for store, due := range m.due {
		if !due.IsZero() && !now.Before(due) {
			targets[store] = true
		}
	}
	if len(targets) > 0 {
		threshold := m.threshold()
		ordered := make([]wsmailbox.QueueStore, 0, len(targets))
		for store := range targets {
			ordered = append(ordered, store)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
		for _, store := range ordered {
			posted, next, err := m.work.Sweep(store, now, threshold)
			if posted > 0 {
				m.wake()
			}
			if err != nil {
				fmt.Fprintf(m.log, "dashboard: mailbox undelivered sweep: %v\n", err)
				retry := now.Add(wsmailbox.UndeliveredRetryInterval)
				if next.IsZero() || next.After(retry) {
					next = retry
				}
			}
			if !next.IsZero() && !next.After(now) {
				next = now.Add(wsmailbox.UndeliveredRetryInterval)
			}
			m.due[store] = next
		}
	}
	next := m.scanAt
	for _, due := range m.due {
		if !due.IsZero() && due.Before(next) {
			next = due
		}
	}
	return next
}

// The daemon runs without pages or listed sessions. Subscribe before startup
// discovery, then visit only changed/due queue files until the safety scan.
func startMailboxSweep(logw io.Writer) (stop func()) {
	done, exited := make(chan struct{}), make(chan struct{})
	sub := wswake.Mailbox.SubscribeStores()
	go func() {
		defer close(exited)
		defer sub.Close()
		paths := wsmailbox.Paths{}
		var work *wsmailbox.UndeliveredSweep
		for {
			var err error
			work, err = wsmailbox.NewUndeliveredSweep(paths)
			if err == nil {
				break
			}
			fmt.Fprintf(logw, "dashboard: mailbox discovery: %v\n", err)
			timer := time.NewTimer(wsmailbox.UndeliveredRetryInterval)
			select {
			case <-done:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		maintenance := mailboxMaintenance{work: work, threshold: func() time.Duration { return mailboxUndeliveredThreshold(paths.Config) }, wake: wswake.Mailbox.Wake, log: logw}
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			default:
			}
			next := maintenance.step(time.Now(), sub.TakeStores())
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(max(time.Millisecond, time.Until(next)))
			select {
			case <-done:
				return
			case <-sub.Stores():
			case <-timer.C:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }); <-exited }
}

// Read once per processing batch, without a session/project layer. A changed
// global threshold is rechecked before posting, and at each safety scan.
func mailboxUndeliveredThreshold(opts wsconfig.Options) time.Duration {
	raw, _ := wsconfig.GlobalOverride(opts, mcp.DashboardMailboxUndeliveredKey)
	minutes, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || minutes < 1 || minutes > int64((1<<63-1)/time.Minute) {
		return 3 * time.Minute
	}
	return time.Duration(minutes) * time.Minute
}
