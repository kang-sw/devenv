package execjob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/textreader"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wsstore"
)

const (
	InlineBudget         = 4096
	ForegroundWindow     = 5 * time.Second
	schemaVersion        = 1
	stateRunning         = "running"
	stateSucceeded       = "succeeded"
	stateFailed          = "failed"
	stateCancelRequested = "cancel_requested"
	stateCancelled       = "cancelled"
)

var keyPattern = regexp.MustCompile(`^exec-(?:[0-9a-f]{8}|[0-9]+-[0-9a-f]{16})$`)
var mu sync.Mutex
var active sync.Map

type LaunchOptions struct {
	Root, WorkingDir string
	Cmd              string
	Args             []string
	Command          string
	Shell            string
	Env              map[string]string
	Stdin            string
	ShellMode        bool
}
type Record struct {
	SchemaVersion   int               `json:"schema_version"`
	ExecKey         string            `json:"exec_key"`
	Status          string            `json:"status"`
	Root            string            `json:"root"`
	WorkingDir      string            `json:"working_dir"`
	Argv            []string          `json:"argv,omitempty"`
	Command         string            `json:"command,omitempty"`
	Shell           string            `json:"shell,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	StdinPresent    bool              `json:"stdin_present,omitempty"`
	StdinBytes      int64             `json:"stdin_bytes,omitempty"`
	PID             int               `json:"pid,omitempty"`
	StartedAt       string            `json:"started_at"`
	UpdatedAt       string            `json:"updated_at"`
	CompletedAt     string            `json:"completed_at,omitempty"`
	ExitCode        int               `json:"exit_code,omitempty"`
	Error           string            `json:"error,omitempty"`
	CancelRequested bool              `json:"cancel_requested,omitempty"`
	StdoutBytes     int64             `json:"stdout_bytes"`
	StderrBytes     int64             `json:"stderr_bytes"`
	CombinedBytes   int64             `json:"combined_bytes"`
}
type Response struct {
	ExecKey       string `json:"exec_key"`
	Status        string `json:"status"`
	PID           int    `json:"pid,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	ExitCode      int    `json:"exit_code,omitempty"`
	Error         string `json:"error,omitempty"`
	ResultReady   bool   `json:"result_ready"`
	StdoutBytes   int64  `json:"stdout_bytes"`
	StderrBytes   int64  `json:"stderr_bytes"`
	CombinedBytes int64  `json:"combined_bytes"`
	Stdout        string `json:"stdout,omitempty"`
	Stderr        string `json:"stderr,omitempty"`
	Guidance      string `json:"guidance,omitempty"`
}
type RawReadResponse struct {
	ExecKey string `json:"exec_key"`
	Stream  string `json:"stream"`
	textreader.ReadResult
}
type RawTailResponse struct {
	ExecKey string `json:"exec_key"`
	Stream  string `json:"stream"`
	Text    string `json:"text"`
}
type RawGrepResponse struct {
	ExecKey string `json:"exec_key"`
	Stream  string `json:"stream"`
	textreader.GrepResult
}

type activeJob struct {
	root, key string
	cmd       *exec.Cmd
}

func Launch(opts LaunchOptions) (Response, error) {
	if strings.TrimSpace(opts.Root) == "" {
		return Response{}, errors.New("root is required")
	}
	wd, err := resolveWorkingDir(opts.Root, opts.WorkingDir)
	if err != nil {
		return Response{}, err
	}
	key, err := newKey(opts.Root)
	if err != nil {
		return Response{}, err
	}
	dir, err := jobDir(opts.Root, key)
	if err != nil {
		return Response{}, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Response{}, err
	}
	stdoutPath, stderrPath, combinedPath := filepath.Join(dir, "stdout"), filepath.Join(dir, "stderr"), filepath.Join(dir, "combined")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return Response{}, err
	}
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = stdout.Close()
		return Response{}, err
	}
	combined, err := os.OpenFile(combinedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return Response{}, err
	}
	var cmd *exec.Cmd
	now := ts()
	rec := Record{SchemaVersion: schemaVersion, ExecKey: key, Status: stateRunning, Root: filepath.Clean(opts.Root), WorkingDir: wd, StartedAt: now, UpdatedAt: now}
	if opts.ShellMode {
		shell, argv := shellCommand(opts.Shell, opts.Command)
		if opts.Command == "" {
			_ = stdout.Close()
			_ = stderr.Close()
			_ = combined.Close()
			return Response{}, errors.New("command is required")
		}
		cmd = exec.Command(shell, argv...)
		rec.Command = opts.Command
		rec.Shell = shell
	} else {
		if strings.TrimSpace(opts.Cmd) == "" {
			_ = stdout.Close()
			_ = stderr.Close()
			_ = combined.Close()
			return Response{}, errors.New("cmd is required")
		}
		argv := append([]string{opts.Cmd}, opts.Args...)
		cmd = exec.Command(opts.Cmd, opts.Args...)
		rec.Argv = argv
	}
	cmd.Dir = wd
	cmd.Env = overlayEnv(opts.Env)
	rec.Env = opts.Env
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
		rec.StdinPresent = true
		rec.StdinBytes = int64(len(opts.Stdin))
	}
	lockedCombined := &lockedWriter{w: combined}
	cmd.Stdout = io.MultiWriter(stdout, lockedCombined)
	cmd.Stderr = io.MultiWriter(stderr, lockedCombined)
	configureCommand(cmd)
	if err := writeRecord(opts.Root, rec); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = combined.Close()
		return Response{}, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = combined.Close()
		rec.Status = stateFailed
		rec.Error = err.Error()
		rec.CompletedAt = ts()
		rec.UpdatedAt = rec.CompletedAt
		_ = writeRecord(opts.Root, rec)
		return responseFor(opts.Root, rec, false), nil
	}
	rec.PID = cmd.Process.Pid
	rec.UpdatedAt = ts()
	_ = writeRecord(opts.Root, rec)
	active.Store(activeKey(opts.Root, key), &activeJob{root: opts.Root, key: key, cmd: cmd})
	done := make(chan struct{})
	go func() { finalize(opts.Root, key, cmd, stdout, stderr, combined); close(done) }()
	select {
	case <-done:
		rec, _ = Load(opts.Root, key)
		return responseFor(opts.Root, rec, true), nil
	case <-time.After(ForegroundWindow):
		rec, _ = refreshSizes(opts.Root, key)
		r := responseFor(opts.Root, rec, false)
		r.Guidance = guidance()
		return r, nil
	}
}

func Status(root, key string) (Response, error) {
	rec, err := reconcile(root, key)
	if err != nil {
		return Response{}, err
	}
	return responseFor(root, rec, false), nil
}
func Result(root, key string) (Response, error) {
	return ResultWithTimeout(root, key, 0)
}

func ResultWithTimeout(root, key string, timeout time.Duration) (Response, error) {
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		rec, err := reconcile(root, key)
		if err != nil {
			return Response{}, err
		}
		if terminal(rec.Status) {
			return responseFor(root, rec, true), nil
		}
		r := responseFor(root, rec, false)
		r.Guidance = guidance()
		if timeout <= 0 || time.Now().After(deadline) {
			return r, nil
		}
		sleep := 100 * time.Millisecond
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			return r, nil
		}
		time.Sleep(sleep)
	}
}
func Abort(root, key string) (Response, error) {
	rec, err := Load(root, key)
	if err != nil {
		return Response{}, err
	}
	if terminal(rec.Status) {
		return responseFor(root, rec, false), nil
	}
	rec.CancelRequested = true
	rec.Status = stateCancelRequested
	rec.UpdatedAt = ts()
	_ = writeRecord(root, rec)
	if v, ok := active.Load(activeKey(root, key)); ok {
		_ = cancelProcess(v.(*activeJob).cmd.Process.Pid)
	} else if rec.PID > 0 {
		_ = cancelProcess(rec.PID)
	}
	time.Sleep(100 * time.Millisecond)
	rec, _ = refreshSizes(root, key)
	return responseFor(root, rec, false), nil
}

func Tail(root, key, stream string, lines int) (RawTailResponse, error) {
	p, st, err := streamPath(root, key, stream)
	if err != nil {
		return RawTailResponse{}, err
	}
	if err := requirePayloadPresent(st, p); err != nil {
		return RawTailResponse{ExecKey: key, Stream: st}, err
	}
	txt, err := textreader.Tail(p, lines)
	return RawTailResponse{ExecKey: key, Stream: st, Text: txt}, err
}
func Read(root, key, stream string, offset, limit int64) (RawReadResponse, error) {
	p, st, err := streamPath(root, key, stream)
	if err != nil {
		return RawReadResponse{}, err
	}
	if err := requirePayloadPresent(st, p); err != nil {
		return RawReadResponse{ExecKey: key, Stream: st}, err
	}
	rr, err := textreader.Read(p, offset, limit)
	return RawReadResponse{ExecKey: key, Stream: st, ReadResult: rr}, err
}
func Grep(root, key, stream, pattern string, before, after, max int, regex bool) (RawGrepResponse, error) {
	paths, st, err := streamPaths(root, key, stream)
	if err != nil {
		return RawGrepResponse{}, err
	}
	for _, p := range paths {
		if err := requirePayloadPresent(st, p); err != nil {
			return RawGrepResponse{ExecKey: key, Stream: st}, err
		}
	}
	gr, err := textreader.Grep(paths, pattern, before, after, max, regex)
	for i := range gr.Matches {
		gr.Matches[i].Path = ""
	}
	return RawGrepResponse{ExecKey: key, Stream: st, GrepResult: gr}, err
}

func requirePayloadPresent(stream, path string) error {
	if wsstore.ClassifyFileBackedPayload(path) == wsstore.PayloadConsistencyMissingPayload {
		return fmt.Errorf("%s file-backed payload missing (recoverable consistency state): %s", stream, path)
	}
	return nil
}

func finalize(root, key string, cmd *exec.Cmd, closers ...io.Closer) {
	err := cmd.Wait()
	for _, closer := range closers {
		_ = closer.Close()
	}
	active.Delete(activeKey(root, key))
	mu.Lock()
	defer mu.Unlock()
	rec, loadErr := readRecord(root, key)
	if loadErr != nil {
		return
	}
	rec.CompletedAt = ts()
	rec.UpdatedAt = rec.CompletedAt
	if rec.CancelRequested {
		rec.Status = stateCancelled
	} else if err != nil {
		rec.Status = stateFailed
		rec.Error = err.Error()
	} else {
		rec.Status = stateSucceeded
	}
	if cmd.ProcessState != nil {
		rec.ExitCode = cmd.ProcessState.ExitCode()
	}
	updateSizes(&rec, root, key)
	_ = writeRecordLocked(root, rec)
}
func Load(root, key string) (Record, error) {
	mu.Lock()
	defer mu.Unlock()
	return readRecord(root, key)
}
func refreshSizes(root, key string) (Record, error) {
	mu.Lock()
	defer mu.Unlock()
	rec, err := readRecord(root, key)
	if err != nil {
		return rec, err
	}
	updateSizes(&rec, root, key)
	rec.UpdatedAt = ts()
	_ = writeRecordLocked(root, rec)
	return rec, nil
}

func reconcile(root, key string) (Record, error) {
	mu.Lock()
	defer mu.Unlock()
	rec, err := readRecord(root, key)
	if err != nil {
		return rec, err
	}
	updateSizes(&rec, root, key)
	if rec.Status == stateRunning || rec.Status == stateCancelRequested {
		if _, ok := active.Load(activeKey(root, key)); !ok && !processAlive(rec.PID) {
			if rec.CancelRequested || rec.Status == stateCancelRequested {
				rec.Status = stateCancelled
				if rec.Error == "" {
					rec.Error = "exec job cancelled"
				}
			} else {
				rec.Status = stateFailed
				rec.Error = "exec job worker is no longer active"
			}
			rec.CompletedAt = ts()
		}
	}
	rec.UpdatedAt = ts()
	_ = writeRecordLocked(root, rec)
	return rec, nil
}

func responseFor(root string, rec Record, include bool) Response {
	r := Response{ExecKey: rec.ExecKey, Status: rec.Status, PID: rec.PID, StartedAt: rec.StartedAt, UpdatedAt: rec.UpdatedAt, CompletedAt: rec.CompletedAt, ExitCode: rec.ExitCode, Error: rec.Error, ResultReady: terminal(rec.Status), StdoutBytes: rec.StdoutBytes, StderrBytes: rec.StderrBytes, CombinedBytes: rec.CombinedBytes}
	if warning := payloadConsistencyWarning(root, rec.ExecKey); warning != "" {
		r.Error = appendError(r.Error, warning)
	}
	if !include {
		if !terminal(rec.Status) {
			r.Guidance = guidance()
		}
		return r
	}
	if rec.CombinedBytes > InlineBudget {
		r.Guidance = guidance()
		return r
	}
	outPath, _, _ := streamPathFromStore(root, rec.ExecKey, "stdout")
	errPath, _, _ := streamPathFromStore(root, rec.ExecKey, "stderr")
	out, outErr := os.ReadFile(outPath)
	er, erErr := os.ReadFile(errPath)
	if outErr != nil {
		r.Error = appendError(r.Error, fmt.Sprintf("stdout file-backed payload unavailable: %v", outErr))
	}
	if erErr != nil {
		r.Error = appendError(r.Error, fmt.Sprintf("stderr file-backed payload unavailable: %v", erErr))
	}
	r.Stdout = string(out)
	r.Stderr = string(er)
	return r
}

func payloadConsistencyWarning(root, key string) string {
	var missing []string
	for _, stream := range []string{"stdout", "stderr", "combined"} {
		p, _, err := streamPathFromStore(root, key, stream)
		if err != nil || strings.TrimSpace(p) == "" {
			continue
		}
		if wsstore.ClassifyFileBackedPayload(p) == wsstore.PayloadConsistencyMissingPayload {
			missing = append(missing, stream)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "file-backed exec payload missing (recoverable consistency state): " + strings.Join(missing, ",")
}

func appendError(base, extra string) string {
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return base + "; " + extra
}
func guidance() string {
	return "Large or running output is available to future exec.ask first; use exec.raw.* fallback readers for raw stdout/stderr text."
}
func terminal(s string) bool { return s == stateSucceeded || s == stateFailed || s == stateCancelled }

func resolveWorkingDir(root, wd string) (string, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(wd) == "" {
		return root, nil
	}
	if !filepath.IsAbs(wd) {
		wd = filepath.Join(root, wd)
	}
	wd = filepath.Clean(wd)
	rel, err := filepath.Rel(root, wd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("working_dir %q must resolve inside ws worktree root %q", wd, root)
	}
	return wd, nil
}
func shellCommand(shell, command string) (string, []string) {
	if strings.TrimSpace(shell) != "" {
		if goruntime.GOOS == "windows" {
			base := strings.ToLower(filepath.Base(shell))
			switch base {
			case "cmd", "cmd.exe":
				return shell, []string{"/C", command}
			case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
				return shell, []string{"-Command", command}
			}
		}
		return shell, []string{"-c", command}
	}
	if goruntime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "/bin/sh", []string{"-c", command}
}
func overlayEnv(env map[string]string) []string {
	out := os.Environ()
	for k, v := range env {
		if strings.TrimSpace(k) != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func jobDir(root, key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid exec_key %q", key)
	}
	layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.WorktreeDir, "exec-jobs", key), nil
}
func streamPath(root, key, stream string) (string, string, error) {
	paths, st, err := streamPaths(root, key, stream)
	if err != nil {
		return "", st, err
	}
	return paths[0], st, nil
}
func streamPaths(root, key, stream string) ([]string, string, error) {
	p, st, err := streamPathFromStore(root, key, stream)
	if err != nil {
		return nil, st, err
	}
	return []string{p}, st, nil
}
func statePath(root, key string) (string, error) {
	dir, err := jobDir(root, key)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func readRecord(root, key string) (Record, error) {
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err == nil {
		defer store.Close()
		job, ok, storeErr := store.ExecJob(context.Background(), key)
		if storeErr != nil {
			return Record{}, storeErr
		}
		if ok {
			return recordFromStore(job), nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Record{}, err
	}
	rec, err := readLegacyRecord(root, key)
	if err != nil {
		return rec, err
	}
	if err := writeRecordLocked(root, rec); err != nil {
		return rec, fmt.Errorf("import legacy exec metadata: %w", err)
	}
	return rec, nil
}

func readLegacyRecord(root, key string) (Record, error) {
	p, err := statePath(root, key)
	if err != nil {
		return Record{}, err
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Record{}, fmt.Errorf("exec job %q not found", key)
	}
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return legacyRecoveryRecord(root, key, fmt.Sprintf("legacy exec metadata cannot be migrated: %v", err)), nil
	}
	if rec.ExecKey == "" {
		rec.ExecKey = key
	}
	if rec.Status == "" || rec.Root == "" || rec.WorkingDir == "" {
		return legacyRecoveryRecord(root, key, "legacy exec metadata cannot be migrated: missing required metadata"), nil
	}
	return rec, nil
}

func legacyRecoveryRecord(root, key, msg string) Record {
	now := ts()
	return Record{SchemaVersion: schemaVersion, ExecKey: key, Status: stateFailed, Root: filepath.Clean(root), WorkingDir: filepath.Clean(root), StartedAt: now, UpdatedAt: now, CompletedAt: now, Error: msg}
}

func writeRecord(root string, rec Record) error {
	mu.Lock()
	defer mu.Unlock()
	return writeRecordLocked(root, rec)
}
func writeRecordLocked(root string, rec Record) error {
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = schemaVersion
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		return err
	}
	defer store.Close()
	job, err := storeFromRecord(root, rec)
	if err != nil {
		return err
	}
	return store.UpsertExecJob(context.Background(), job)
}

func storeFromRecord(root string, rec Record) (wsstore.ExecJob, error) {
	dir, err := jobDir(root, rec.ExecKey)
	if err != nil {
		return wsstore.ExecJob{}, err
	}
	envJSON := "{}"
	if rec.Env != nil {
		raw, err := json.Marshal(rec.Env)
		if err != nil {
			return wsstore.ExecJob{}, fmt.Errorf("marshal exec env metadata: %w", err)
		}
		envJSON = string(raw)
	}
	cleanupState := "active"
	if terminal(rec.Status) {
		cleanupState = "completed"
	} else if rec.Status == stateCancelRequested {
		cleanupState = wsstore.ArtifactStateCancelRequested
	} else if rec.Status == stateRunning {
		cleanupState = wsstore.ArtifactStateRunning
	}
	return wsstore.ExecJob{
		ExecKey: rec.ExecKey, Status: rec.Status, SchemaVersion: rec.SchemaVersion,
		Root: rec.Root, WorkingDir: rec.WorkingDir, Argv: rec.Argv, Command: rec.Command, Shell: rec.Shell,
		EnvJSON: envJSON, StdinPresent: rec.StdinPresent, StdinBytes: rec.StdinBytes, PID: rec.PID, StartedAt: rec.StartedAt, UpdatedAt: rec.UpdatedAt, CompletedAt: rec.CompletedAt,
		ExitCode: rec.ExitCode, Error: rec.Error, CancelRequested: rec.CancelRequested,
		StdoutPath: filepath.Join(dir, "stdout"), StderrPath: filepath.Join(dir, "stderr"), CombinedPath: filepath.Join(dir, "combined"),
		StdoutBytes: rec.StdoutBytes, StderrBytes: rec.StderrBytes, CombinedBytes: rec.CombinedBytes,
		LostWorker: strings.Contains(rec.Error, "worker is no longer active"), CleanupState: cleanupState,
	}, nil
}

func recordFromStore(job wsstore.ExecJob) Record {
	rec := Record{SchemaVersion: job.SchemaVersion, ExecKey: job.ExecKey, Status: job.Status, Root: job.Root, WorkingDir: job.WorkingDir, Argv: job.Argv, Command: job.Command, Shell: job.Shell, StdinPresent: job.StdinPresent, StdinBytes: job.StdinBytes, PID: job.PID, StartedAt: job.StartedAt, UpdatedAt: job.UpdatedAt, CompletedAt: job.CompletedAt, ExitCode: job.ExitCode, Error: job.Error, CancelRequested: job.CancelRequested, StdoutBytes: job.StdoutBytes, StderrBytes: job.StderrBytes, CombinedBytes: job.CombinedBytes}
	if strings.TrimSpace(job.EnvJSON) != "" {
		_ = json.Unmarshal([]byte(job.EnvJSON), &rec.Env)
	}
	return rec
}

func streamPathFromStore(root, key, stream string) (string, string, error) {
	if strings.TrimSpace(stream) == "" {
		stream = "stdout"
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err == nil {
		defer store.Close()
		if job, ok, storeErr := store.ExecJob(context.Background(), key); storeErr != nil {
			return "", stream, storeErr
		} else if ok {
			switch stream {
			case "stdout":
				return job.StdoutPath, stream, nil
			case "stderr":
				return job.StderrPath, stream, nil
			case "combined":
				return job.CombinedPath, stream, nil
			default:
				return "", "", fmt.Errorf("invalid stream %q", stream)
			}
		}
	}
	dir, err := jobDir(root, key)
	if err != nil {
		return "", "", err
	}
	switch stream {
	case "stdout", "stderr", "combined":
		return filepath.Join(dir, stream), stream, nil
	default:
		return "", "", fmt.Errorf("invalid stream %q", stream)
	}
}

func updateSizes(rec *Record, root, key string) {
	dir, err := jobDir(root, key)
	if err != nil {
		return
	}
	if st, err := os.Stat(filepath.Join(dir, "stdout")); err == nil {
		rec.StdoutBytes = st.Size()
	}
	if st, err := os.Stat(filepath.Join(dir, "stderr")); err == nil {
		rec.StderrBytes = st.Size()
	}
	if st, err := os.Stat(filepath.Join(dir, "combined")); err == nil {
		rec.CombinedBytes = st.Size()
	}
}
func newKey(root string) (string, error) {
	for i := 0; i < 32; i++ {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		key := "exec-" + hex.EncodeToString(b[:])
		exists, err := keyExists(root, key)
		if err != nil {
			return "", err
		}
		if !exists {
			return key, nil
		}
	}
	return "", errors.New("could not allocate unique exec key")
}

func keyExists(root, key string) (bool, error) {
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err == nil {
		defer store.Close()
		_, ok, storeErr := store.ExecJob(context.Background(), key)
		if storeErr != nil {
			return false, storeErr
		}
		if ok {
			return true, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	dir, err := jobDir(root, key)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(dir); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}
func activeKey(root, key string) string { return filepath.Clean(root) + "\x00" + key }
func ts() string                        { return time.Now().UTC().Format(time.RFC3339Nano) }
