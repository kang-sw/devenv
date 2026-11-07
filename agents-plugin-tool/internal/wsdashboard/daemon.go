package wsdashboard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// ErrLocked reports that another daemon holds the single-instance lock.
var ErrLocked = errors.New("another dashboard daemon holds the lock")

// DefaultLockWait bounds how long a starting daemon waits for the lock. It
// covers a predecessor that is shutting down during a replacement handoff; a
// holder still alive after it is left running.
const DefaultLockWait = 5 * time.Second

const (
	lockRetryDelay   = 50 * time.Millisecond
	shutdownDeadline = 3 * time.Second
	// healthService names the daemon in its /health answer, so a client can
	// tell it from an unrelated server that took the recorded port.
	healthService = "ws-dashboard"
)

// DaemonOptions configures StartDaemon.
type DaemonOptions struct {
	// Dir is the state directory (Dir's result); it is created when absent.
	Dir string
	// Port is the preferred port; 0 asks the OS for one. A taken port falls
	// back to the port the last daemon bound (last-port file), then to an
	// OS-assigned one, so a restart keeps its origin while the preferred
	// port stays taken.
	Port int
	// LAN binds every interface instead of loopback only, and admits a
	// request that is not from this machine only with the LAN access token
	// (access.go).
	LAN bool
	// Version is reported by /health and written to the endpoint file.
	Version string
	// BuildTime is reported by /health (ExecutableBuildTime); zero is
	// unknown and is omitted.
	BuildTime int64
	// LockWait bounds the wait for the single-instance lock; zero means
	// DefaultLockWait.
	LockWait time.Duration
	// Log receives diagnostics; nil discards them.
	Log io.Writer
	// Sessions reads registered sessions' stores for the session index; nil
	// runs the daemon without one (no polling, and /nudge answers 404).
	Sessions SessionSource
	// PollInterval is the polling backstop period; zero means
	// DefaultPollInterval.
	PollInterval time.Duration

	// bindHost overrides the address the daemon binds, so a test runs LAN
	// mode on loopback; empty derives it from LAN.
	bindHost string
	// openSystem replaces the system-open route's OS open command, so a
	// test never starts a browser; nil runs systemOpen.
	openSystem func(target string) error
}

// nudgeBodyLimit bounds a /nudge request body.
const nudgeBodyLimit = 4 << 10

// NudgeBody is the /nudge request body. Readers ignore unknown fields and
// treat absent ones as empty.
type NudgeBody struct {
	ReplyID string `json:"reply_id"`
	// Project is the session's root path, as its registration records it.
	Project string `json:"project"`
}

// Daemon is a running dashboard daemon: it holds the lock, listens on its
// port (loopback, or every interface in LAN mode), and has written its
// endpoint file.
type Daemon struct {
	dir      string
	lock     *flock.Flock
	server   *http.Server
	endpoint Endpoint
	log      io.Writer
	index    *Index
	source   SessionSource
	broker   *broker
	// entries caches rendered conversation entries per session.
	entries *entryCache
	// inboxMu serializes the daemon's own inbox writes (sends and cancels).
	inboxMu sync.Mutex
	// lan is LAN mode; tokenMu serializes reading and minting the LAN token.
	lan     bool
	tokenMu sync.Mutex
	// buildTime is the build time /health reports; zero is unknown.
	buildTime int64
	// openSystem opens a system-open route's target (systemOpen).
	openSystem func(target string) error

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// HealthInfo is the /health answer.
type HealthInfo struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	StartedAt string `json:"started_at"`
	// LAN reports LAN mode; an older daemon omits it, which reads as off.
	LAN bool `json:"lan"`
	// BuildTime is the daemon's ws-mcp build time in Unix milliseconds
	// (ExecutableBuildTime); an older daemon omits it, which reads as older
	// than any known build.
	BuildTime int64 `json:"build_time,omitempty"`
}

// StartDaemon takes the single-instance lock, binds the port, writes the
// endpoint file, and starts serving. It returns ErrLocked when another
// daemon still holds the lock after the lock wait.
func StartDaemon(opts DaemonOptions) (*Daemon, error) {
	if opts.Dir == "" {
		return nil, errors.New("dashboard: empty state directory")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, err
	}
	wait := opts.LockWait
	if wait <= 0 {
		wait = DefaultLockWait
	}
	lock := flock.New(filepath.Join(opts.Dir, lockFileName))
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	locked, err := lock.TryLockContext(ctx, lockRetryDelay)
	cancel()
	if !locked {
		_ = lock.Close()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("dashboard lock: %w", err)
		}
		return nil, ErrLocked
	}

	listener, err := listen(bindHostFor(opts), portCandidates(opts.Dir, opts.Port))
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	// Recorded under the lock; losing it only costs the next restart its
	// sticky port.
	if err := writeLastPort(opts.Dir, listener.Addr().(*net.TCPAddr).Port); err != nil {
		fmt.Fprintf(logw, "dashboard: record last port: %v\n", err)
	}
	token, err := newToken()
	if err != nil {
		_ = listener.Close()
		_ = lock.Unlock()
		return nil, err
	}
	d := &Daemon{
		dir:        opts.Dir,
		lock:       lock,
		log:        logw,
		source:     opts.Sessions,
		broker:     newBroker(),
		entries:    newEntryCache(),
		lan:        opts.LAN,
		buildTime:  opts.BuildTime,
		openSystem: opts.openSystem,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		endpoint: Endpoint{
			SchemaVersion: endpointSchemaVersion,
			Port:          listener.Addr().(*net.TCPAddr).Port,
			PID:           os.Getpid(),
			Version:       opts.Version,
			Token:         token,
			StartedAt:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if d.openSystem == nil {
		d.openSystem = systemOpen
	}
	if d.lan {
		// Mint the token now, so the localhost index shows it before any
		// LAN request; a failure leaves every LAN request refused.
		if _, err := d.lanToken(); err != nil {
			fmt.Fprintf(logw, "dashboard: LAN access token unavailable: %v\n", err)
		}
	}
	if opts.Sessions != nil {
		d.index = NewIndex(IndexOptions{Dir: opts.Dir, Source: opts.Sessions, Log: logw, OnChange: d.sessionChanged})
	}
	d.server = &http.Server{Handler: d.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := writeEndpoint(opts.Dir, d.endpoint); err != nil {
		_ = listener.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("write endpoint file: %w", err)
	}
	go func() {
		if err := d.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(d.log, "dashboard: serve: %v\n", err)
			d.Close()
		}
	}()
	if d.index != nil {
		go d.poll(opts.PollInterval)
	}
	fmt.Fprintf(d.log, "dashboard: version %s listening on %s (lan %v, pid %d)\n", d.endpoint.Version, listener.Addr(), d.lan, d.endpoint.PID)
	return d, nil
}

// bindHostFor is the address a daemon binds: loopback, or every interface
// (the empty host) in LAN mode.
func bindHostFor(opts DaemonOptions) string {
	switch {
	case opts.bindHost != "":
		return opts.bindHost
	case opts.LAN:
		return ""
	}
	return "127.0.0.1"
}

// portCandidates is the order a daemon tries ports: the preferred port, then
// the recorded last port when it differs, each only when valid. An
// OS-assigned port (0) is always the final fallback and is left to listen. A
// request for an OS-assigned port (preferred 0) skips the record, so it stays
// deterministic.
func portCandidates(dir string, preferred int) []int {
	if preferred <= 0 {
		return nil
	}
	ports := []int{preferred}
	if last, ok := readLastPort(dir); ok && last != preferred {
		ports = append(ports, last)
	}
	return ports
}

// listen binds the first of ports that is free on host, falling back to an
// OS-assigned port when none is.
func listen(host string, ports []int) (net.Listener, error) {
	for _, port := range ports {
		if l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port))); err == nil {
			return l, nil
		}
	}
	return net.Listen("tcp", net.JoinHostPort(host, "0"))
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// sessionChanged is the index's change hook: it wakes every stream, and
// drops a session's cached conversation entries once it left the index.
func (d *Daemon) sessionChanged(replyID string) {
	if d.index != nil {
		if _, ok := d.index.Session(replyID); !ok {
			d.entries.drop(replyID)
		}
	}
	d.broker.wake(replyID)
}

// poll fills the session index at once and then re-reads every registered
// session each interval until the daemon stops.
func (d *Daemon) poll(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	d.index.Refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.index.Refresh()
		}
	}
}

// Index returns the daemon's session index; nil when it runs without a
// session source.
func (d *Daemon) Index() *Index { return d.index }

// Endpoint returns what the daemon wrote to its endpoint file.
func (d *Daemon) Endpoint() Endpoint { return d.endpoint }

// Done is closed once the daemon has stopped serving, removed its endpoint
// file, and released the lock.
func (d *Daemon) Done() <-chan struct{} { return d.done }

// Close stops the daemon. The endpoint file is removed before the lock is
// released, so a successor that takes the lock never has its own file
// deleted. Close is idempotent.
func (d *Daemon) Close() {
	d.closeOnce.Do(func() {
		close(d.stop)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
		_ = d.server.Shutdown(ctx)
		cancel()
		removeEndpointIfOwned(d.dir, d.endpoint.Token)
		_ = d.lock.Unlock()
		fmt.Fprintf(d.log, "dashboard: stopped (pid %d)\n", d.endpoint.PID)
		close(d.done)
	})
}

// Handler is the daemon's HTTP surface. Every route first passes the access
// guard (the loopback Host guard, or in LAN mode the guard that also admits
// the LAN token; access.go); the daemon sends no CORS headers. The pages (/,
// /s/<reply-id>, their SSE streams, and a session's image, file view, and
// history routes) are read-only; a session's message, cancel, and steer
// routes (inbox_routes.go) are the only browser writes, and its system-open
// route (sysopen_route.go) the only route that runs a host command.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/shutdown", d.handleShutdown)
	mux.HandleFunc("/nudge", d.handleNudge)
	mux.HandleFunc("/events", d.handleIndexEvents)
	mux.HandleFunc("/s/", d.handleSessionRoutes)
	mux.HandleFunc("/", d.handleIndex)
	if d.lan {
		return d.lanGuard(mux)
	}
	return hostGuard(mux)
}

// hostGuard refuses a request whose Host header is not a loopback name. A
// DNS-rebinding page reaches 127.0.0.1 under its own host name, which this
// refuses.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost reports whether a Host header names the loopback interface:
// localhost, 127.0.0.1, or ::1, with or without a port.
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, HealthInfo{
		Service:   healthService,
		Version:   d.endpoint.Version,
		PID:       d.endpoint.PID,
		Port:      d.endpoint.Port,
		StartedAt: d.endpoint.StartedAt,
		LAN:       d.lan,
		BuildTime: d.buildTime,
	})
}

// handleShutdown stops the daemon when the request carries the endpoint
// file's token as a bearer credential. Only a process that can read the
// endpoint file (the same OS user) holds that token.
func (d *Daemon) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	got, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(d.endpoint.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"shutting_down": true})
	fmt.Fprintf(d.log, "dashboard: shutdown requested\n")
	go d.Close()
}

// handleNudge re-reads the one session the body names. It needs no
// credential: all it can cause is a re-read of disk. A malformed body is 400,
// an unknown session 404, and a re-read 204; the sender ignores the answer.
func (d *Daemon) handleNudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body NudgeBody
	if err := json.NewDecoder(io.LimitReader(r.Body, nudgeBodyLimit)).Decode(&body); err != nil {
		http.Error(w, "malformed nudge", http.StatusBadRequest)
		return
	}
	if d.index == nil || !d.index.Nudge(body.ReplyID, body.Project) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
