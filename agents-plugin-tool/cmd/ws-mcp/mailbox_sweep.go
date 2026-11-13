package main

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
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
			paths := wsmailbox.Paths{}
			posted, err := paths.SweepUndelivered(time.Now(), mailboxUndeliveredThreshold(paths.Config))
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

// Read each sweep, without a session or project layer: this daemon serves
// every project. Invalid hand-edited values use the declared default.
func mailboxUndeliveredThreshold(opts wsconfig.Options) time.Duration {
	raw, _ := wsconfig.GlobalOverride(opts, mcp.DashboardMailboxUndeliveredKey)
	minutes, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || minutes < 1 || minutes > int64((1<<63-1)/time.Minute) {
		return 3 * time.Minute
	}
	return time.Duration(minutes) * time.Minute
}
