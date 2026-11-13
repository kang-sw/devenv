package main

import (
	"fmt"
	"io"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// The daemon runs even with no listed sessions. Discover queues from disk,
// not from its live session index, so mail to an exited recipient is covered.
func startMailboxSweep(logw io.Writer) (stop func()) {
	done, exited := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(exited)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			posted, err := (wsmailbox.Paths{}).SweepUndelivered(time.Now(), 3*time.Minute)
			if posted > 0 {
				wswake.Mailbox.Wake()
			}
			if err != nil {
				fmt.Fprintf(logw, "dashboard: mailbox undelivered sweep: %v\n", err)
			}
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { close(done); <-exited }
}
