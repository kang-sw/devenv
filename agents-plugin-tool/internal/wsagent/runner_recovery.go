package wsagent

import (
	"fmt"
	"slices"

	"github.com/kang-sw/devenv/internal/wslive"
)

// A lead's failure states and MCP recovery. A broken MCP connection is
// first reconnected in-stream (the harness's mcp_reconnect control), which
// keeps the process and its background work. A server that did not come
// back makes the process restart, at the first idle point with no
// background task left: a restart ends the session's background work with
// the process. After such a restart, a server that fails again without
// having connected in the new process restarts nothing more (a restart
// loop): the session shows it broken until it connects again, and the
// user may restart by hand (Runner.Restart).

// syncHost writes the lead's host state into its live snapshot (unless
// quiet, a replayed line: the replay's end writes the snapshot).
func (c *conv) syncHost(quiet bool) {
	if !c.rec.Lead {
		return
	}
	h := &wslive.Host{BackgroundTasks: c.bgTasks, Failure: c.failure, MCPBroken: c.mcpBroken, Retry: c.retry}
	if c.live.setHost(h, quiet) && !quiet {
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
}

// alert takes a typed failure the harness reported: a conversation line,
// and the session's failure state until a turn succeeds.
func (c *conv) alert(n wslive.Notice, quiet bool) {
	c.alerted = true
	c.setStatus(func() { c.failure = &n })
	if !quiet && c.live.notice(n) {
		c.ringDirty = true
	}
	c.syncHost(quiet)
	c.changed(quiet)
}

// turnSettled clears what a turn's end settles: a retry still shown, and
// the failure state once a turn ended with no failure of any kind.
func (c *conv) turnSettled(turnErr string, quiet bool) {
	alerted := c.alerted
	c.alerted, c.retry = false, nil
	if turnErr == "" && !alerted && c.failure != nil {
		c.setStatus(func() { c.failure = nil })
		c.changed(quiet)
	}
	c.syncHost(quiet)
}

// mcpReported takes the MCP server list the harness reported with a turn:
// a connected server is seen (and no longer broken), and a failed one is
// reconnected.
func (c *conv) mcpReported(servers []MCPServer, quiet bool) {
	var failed []string
	for _, s := range servers {
		switch s.Status {
		case MCPConnected:
			c.mcpConnected(s.Name)
		case MCPFailed:
			failed = append(failed, s.Name)
		}
	}
	c.syncHost(quiet)
	if len(failed) > 0 && !quiet {
		c.checkMCP(failed)
	}
}

// mcpConnected marks server connected in this process.
func (c *conv) mcpConnected(server string) {
	if c.mcpSeen == nil {
		c.mcpSeen = map[string]bool{}
	}
	c.mcpSeen[server] = true
	delete(c.mcpRestarted, server)
	if i := slices.Index(c.mcpBroken, server); i >= 0 {
		c.setStatus(func() { c.mcpBroken = slices.Delete(slices.Clone(c.mcpBroken), i, i+1) })
	}
}

// checkMCP reconnects the process's failed MCP servers in-stream, off the
// loop, since a reconnect answers only once the server has started again:
// failed when named, else those the harness's status report calls failed.
// A check already running takes the names into one more after it.
func (c *conv) checkMCP(failed []string) {
	mc, ok := c.proc.(mcpController)
	if !ok || c.closed {
		return
	}
	if c.mcpChecking {
		c.mcpAgain = true
		for _, name := range failed {
			if !slices.Contains(c.mcpAgainNames, name) {
				c.mcpAgainNames = append(c.mcpAgainNames, name)
			}
		}
		return
	}
	c.mcpChecking = true
	proc := c.proc
	go func() {
		var status []MCPServer
		names := failed
		if len(names) == 0 {
			st, err := mc.MCPStatus()
			if err != nil {
				c.do(func() { c.mcpChecked(proc, nil, nil, err) })
				return
			}
			status = st
			for _, s := range st {
				if s.Status == MCPFailed {
					names = append(names, s.Name)
				}
			}
		}
		recovered := map[string]bool{}
		for _, name := range names {
			err := mc.ReconnectMCP(name)
			if err == nil {
				// The reconnect answered; the status report says whether
				// the server is up.
				var st []MCPServer
				if st, err = mc.MCPStatus(); err == nil {
					status = st
				}
			}
			recovered[name] = err == nil && mcpStatusOf(status, name) == MCPConnected
		}
		c.do(func() { c.mcpChecked(proc, status, recovered, nil) })
	}()
}

// mcpChecked takes a check's result in the loop: the servers' status, and
// for each server reconnected whether it came back.
func (c *conv) mcpChecked(proc Process, status []MCPServer, recovered map[string]bool, err error) {
	if proc != c.proc {
		// The process the check asked ended meanwhile.
		return
	}
	c.mcpChecking = false
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: mcp status: %v\n", c.rec.ReplyID, err)
	}
	for _, s := range status {
		if s.Status == MCPConnected {
			c.mcpConnected(s.Name)
		}
	}
	for name, ok := range recovered {
		if ok {
			continue
		}
		fmt.Fprintf(c.r.opts.Log, "agent %s: mcp server %s did not reconnect\n", c.rec.ReplyID, name)
		if !slices.Contains(c.mcpBroken, name) {
			c.setStatus(func() {
				c.mcpBroken = append(slices.Clone(c.mcpBroken), name)
				slices.Sort(c.mcpBroken)
			})
		}
	}
	c.syncHost(false)
	c.changed(false)
	if c.mcpAgain {
		names := c.mcpAgainNames
		c.mcpAgain, c.mcpAgainNames = false, nil
		c.checkMCP(names)
	}
	if !c.turn {
		c.restartIfDue()
	}
}

// mcpStatusOf is server's status in a status report, or "".
func mcpStatusOf(status []MCPServer, server string) string {
	for _, s := range status {
		if s.Name == server {
			return s.Status
		}
	}
	return ""
}

// mcpRestartDue reports whether a broken server asks for a restart: one
// this process had connected, or one no automatic restart was for yet. It
// is read from the broken list each time, so a server that came back
// (mcpConnected) asks for none.
func (c *conv) mcpRestartDue() bool {
	for _, name := range c.mcpBroken {
		if c.mcpSeen[name] || !c.mcpRestarted[name] {
			return true
		}
	}
	return false
}

// restartIfDue restarts the process when a broken MCP connection asked for
// it and the session stands at an idle point with no background task.
func (c *conv) restartIfDue() {
	if !c.mcpRestartDue() || c.proc == nil || !c.idleNow() {
		return
	}
	fmt.Fprintf(c.r.opts.Log, "agent %s: restart for MCP servers %v\n", c.rec.ReplyID, c.mcpBroken)
	if c.mcpRestarted == nil {
		c.mcpRestarted = map[string]bool{}
	}
	for _, name := range c.mcpBroken {
		c.mcpRestarted[name] = true
	}
	if err := c.restartProcess(); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: restart: %v\n", c.rec.ReplyID, err)
	}
}

// restartProcess ends the process, if any, and starts a new one on the
// same backend session: it takes what is held as its first turn, or waits
// for input like after a turn.
func (c *conv) restartProcess() error {
	c.stopProcess("")
	if err := c.startProcess(); err != nil {
		c.setStatus(func() { c.lastErr = err.Error() })
		c.live.writeSnapshot()
		c.r.notify(c.rec.ReplyID, c.rec.Root)
		return err
	}
	c.kick()
	if !c.turn {
		c.armIdle()
	}
	c.r.notify(c.rec.ReplyID, c.rec.Root)
	return nil
}
