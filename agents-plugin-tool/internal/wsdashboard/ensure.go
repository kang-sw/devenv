package wsdashboard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Outcome is what Ensure found or did.
type Outcome string

const (
	// OutcomeRunning: a healthy daemon was already running and was left
	// alone: newer than this ws-mcp, or as new and in the LAN mode it asks
	// for.
	OutcomeRunning Outcome = "running"
	// OutcomeStarted: no reachable daemon was running; a new one is up.
	OutcomeStarted Outcome = "started"
	// OutcomeReplaced: an older daemon, an older build of the same version,
	// or one as new in the other LAN mode, was asked to stop and a new one is up.
	OutcomeReplaced Outcome = "replaced"
	// OutcomeUnavailable: no healthy daemon came up in time, for example
	// because a hung daemon still holds the lock. Nothing was terminated.
	OutcomeUnavailable Outcome = "unavailable"
)

// DefaultStartWait bounds how long Ensure waits for a started daemon's
// endpoint file: the daemon's own lock wait plus start-up.
const DefaultStartWait = DefaultLockWait + 5*time.Second

// maxSpawns bounds the daemons one Ensure starts: the first, and one more to
// replace an older daemon that a concurrent older ws-mcp started and that won
// the lock race.
const maxSpawns = 2

// EnsureOptions configures Ensure.
type EnsureOptions struct {
	// Dir is the state directory (Dir's result).
	Dir string
	// Version is this ws-mcp's version; a running daemon older than it is
	// replaced.
	Version string
	// BuildTime is this ws-mcp's build time (ExecutableBuildTime); zero is
	// unknown. It breaks a version tie: a daemon of the same version with an
	// older (or no reported) build time is replaced.
	BuildTime int64
	// Port is the configured dashboard.port, passed to a started daemon as
	// its --port flag.
	Port int
	// LAN is the LAN mode this ws-mcp asks for, passed to a started daemon as
	// its --lan flag; a running daemon as new in the other mode is replaced.
	LAN bool
	// Spawn starts a detached daemon process with the given ws-mcp
	// arguments ("dashboard", "--port", N). It returns once the process is
	// started; it does not wait for the daemon to serve.
	Spawn func(args []string) error
	// HTTP is the client for /health and shutdown; nil uses a client with
	// DefaultHealthTimeout and no proxy.
	HTTP *http.Client
	// StartWait bounds the wait for a started daemon; zero means
	// DefaultStartWait.
	StartWait time.Duration
	// PollInterval is the endpoint-file poll period; zero means 50ms.
	PollInterval time.Duration
	// Log receives diagnostics; nil discards them.
	Log io.Writer
}

// DaemonArgs is the ws-mcp argument list that starts a daemon on port, in
// LAN mode when lan is set, reporting buildTime when it is known (non-zero).
func DaemonArgs(port int, lan bool, buildTime int64) []string {
	args := []string{"dashboard", "--port", strconv.Itoa(port)}
	if lan {
		args = append(args, "--lan")
	}
	if buildTime > 0 {
		args = append(args, "--build-time", strconv.FormatInt(buildTime, 10))
	}
	return args
}

// Ensure makes sure a daemon at least as new as opts.Version runs:
//
//   - A healthy newer daemon is left alone whatever its LAN mode, so an older
//     ws-mcp never replaces a newer daemon; one as new is left alone when its
//     LAN mode is opts.LAN and, at the same version, its build time is not
//     older than opts.BuildTime.
//   - A healthy older daemon, one of the same version from an older build,
//     or one as new in the other LAN mode, gets the
//     token-authenticated shutdown request; then a new daemon is started,
//     which waits for the old one to release the lock. So a changed LAN value
//     applies at the next ws-mcp start, without waiting for an upgrade.
//   - An unreachable daemon (no endpoint file, a refused or unanswered
//     /health, an answer from another process) is never treated as dead:
//     a new daemon is started and the lock decides. When the recorded daemon
//     is alive but hung, the new daemon cannot take the lock and exits, and
//     Ensure reports OutcomeUnavailable.
//
// When a concurrently started older daemon wins the lock, Ensure replaces it
// once more; it starts at most maxSpawns daemons.
func Ensure(ctx context.Context, opts EnsureOptions) (Outcome, error) {
	if opts.Spawn == nil {
		return OutcomeUnavailable, errors.New("dashboard: no spawner")
	}
	client := opts.HTTP
	if client == nil {
		client = &http.Client{Timeout: DefaultHealthTimeout, Transport: &http.Transport{Proxy: nil}}
	}
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	replaced := false
	spawns := 0
	for {
		previous, readErr := ReadEndpoint(opts.Dir)
		if readErr == nil {
			info, err := Health(ctx, client, previous)
			switch {
			case err == nil && (keepRunning(info, opts) || spawns >= maxSpawns):
				switch {
				case spawns == 0:
					return OutcomeRunning, nil
				case replaced:
					return OutcomeReplaced, nil
				}
				return OutcomeStarted, nil
			case err == nil:
				if err := RequestShutdown(ctx, client, previous); err != nil {
					// The older daemon still serves; leave it rather than race it.
					fmt.Fprintf(logw, "dashboard: older daemon %s refused shutdown: %v\n", info.Version, err)
					return OutcomeRunning, nil
				}
				fmt.Fprintf(logw, "dashboard: replacing daemon %s (lan %v, build %d) with %s (lan %v, build %d)\n", info.Version, info.LAN, info.BuildTime, opts.Version, opts.LAN, opts.BuildTime)
				replaced = true
			default:
				fmt.Fprintf(logw, "dashboard: recorded daemon unreachable (%v); starting one\n", err)
			}
		}
		if spawns >= maxSpawns {
			return OutcomeUnavailable, nil
		}
		spawns++
		if err := opts.Spawn(DaemonArgs(opts.Port, opts.LAN, opts.BuildTime)); err != nil {
			return OutcomeUnavailable, fmt.Errorf("dashboard: start daemon: %w", err)
		}
		if !waitForSuccessor(ctx, client, opts, previous.Token) {
			return OutcomeUnavailable, nil
		}
	}
}

// keepRunning reports whether the healthy daemon info describes may stay:
// it is newer than this ws-mcp, or as new and in the LAN mode opts asks for.
// At an equal version the newer build wins: a daemon whose build time is
// older than opts.BuildTime (a missing one counts as older) is replaced, and
// an unknown opts.BuildTime never replaces.
func keepRunning(info HealthInfo, opts EnsureOptions) bool {
	switch cmp := CompareVersions(info.Version, opts.Version); {
	case cmp > 0:
		return true
	case cmp == 0:
		return info.LAN == opts.LAN && (opts.BuildTime <= 0 || info.BuildTime >= opts.BuildTime)
	}
	return false
}

// waitForSuccessor polls the endpoint file until it names a healthy daemon
// other than the one carrying previousToken, or the start wait ends.
func waitForSuccessor(ctx context.Context, client *http.Client, opts EnsureOptions, previousToken string) bool {
	wait := opts.StartWait
	if wait <= 0 {
		wait = DefaultStartWait
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = 50 * time.Millisecond
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if ep, err := ReadEndpoint(opts.Dir); err == nil && ep.Token != "" && ep.Token != previousToken {
			if _, err := Health(ctx, client, ep); err == nil {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}
