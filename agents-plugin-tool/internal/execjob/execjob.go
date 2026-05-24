package execjob

import (
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

var keyPattern = regexp.MustCompile(`^exec-[0-9]+-[0-9a-f]{16}$`)
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
	SchemaVersion   int      `json:"schema_version"`
	ExecKey         string   `json:"exec_key"`
	Status          string   `json:"status"`
	Root            string   `json:"root"`
	WorkingDir      string   `json:"working_dir"`
	Argv            []string `json:"argv,omitempty"`
	Command         string   `json:"command,omitempty"`
	Shell           string   `json:"shell,omitempty"`
	PID             int      `json:"pid,omitempty"`
	StartedAt       string   `json:"started_at"`
	UpdatedAt       string   `json:"updated_at"`
	CompletedAt     string   `json:"completed_at,omitempty"`
	ExitCode        int      `json:"exit_code,omitempty"`
	Error           string   `json:"error,omitempty"`
	CancelRequested bool     `json:"cancel_requested,omitempty"`
	StdoutBytes     int64    `json:"stdout_bytes"`
	StderrBytes     int64    `json:"stderr_bytes"`
	CombinedBytes   int64    `json:"combined_bytes"`
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
	key, err := newKey()
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
	rec := Record{SchemaVersion: schemaVersion, ExecKey: key, Status: stateRunning, Root: filepath.Clean(opts.Root), WorkingDir: wd, StartedAt: ts(), UpdatedAt: ts()}
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
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
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
	rec, err := refreshSizes(root, key)
	if err != nil {
		return Response{}, err
	}
	return responseFor(root, rec, false), nil
}
func Result(root, key string) (Response, error) {
	rec, err := refreshSizes(root, key)
	if err != nil {
		return Response{}, err
	}
	if !terminal(rec.Status) {
		r := responseFor(root, rec, false)
		r.Guidance = guidance()
		return r, fmt.Errorf("exec job %q is not terminal", key)
	}
	return responseFor(root, rec, true), nil
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
	txt, err := textreader.Tail(p, lines)
	return RawTailResponse{ExecKey: key, Stream: st, Text: txt}, err
}
func Read(root, key, stream string, offset, limit int64) (RawReadResponse, error) {
	p, st, err := streamPath(root, key, stream)
	if err != nil {
		return RawReadResponse{}, err
	}
	rr, err := textreader.Read(p, offset, limit)
	return RawReadResponse{ExecKey: key, Stream: st, ReadResult: rr}, err
}
func Grep(root, key, stream, pattern string, before, after, max int, regex bool) (RawGrepResponse, error) {
	paths, st, err := streamPaths(root, key, stream)
	if err != nil {
		return RawGrepResponse{}, err
	}
	gr, err := textreader.Grep(paths, pattern, before, after, max, regex)
	for i := range gr.Matches {
		gr.Matches[i].Path = ""
	}
	return RawGrepResponse{ExecKey: key, Stream: st, GrepResult: gr}, err
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

func responseFor(root string, rec Record, include bool) Response {
	r := Response{ExecKey: rec.ExecKey, Status: rec.Status, PID: rec.PID, StartedAt: rec.StartedAt, UpdatedAt: rec.UpdatedAt, CompletedAt: rec.CompletedAt, ExitCode: rec.ExitCode, Error: rec.Error, ResultReady: terminal(rec.Status), StdoutBytes: rec.StdoutBytes, StderrBytes: rec.StderrBytes, CombinedBytes: rec.CombinedBytes}
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
	dir, _ := jobDir(root, rec.ExecKey)
	out, _ := os.ReadFile(filepath.Join(dir, "stdout"))
	er, _ := os.ReadFile(filepath.Join(dir, "stderr"))
	r.Stdout = string(out)
	r.Stderr = string(er)
	return r
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
	return wd, nil
}
func shellCommand(shell, command string) (string, []string) {
	if strings.TrimSpace(shell) != "" {
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
	if strings.TrimSpace(stream) == "" {
		stream = "stdout"
	}
	dir, err := jobDir(root, key)
	if err != nil {
		return nil, "", err
	}
	switch stream {
	case "stdout", "stderr", "combined":
		return []string{filepath.Join(dir, stream)}, stream, nil
	default:
		return nil, "", fmt.Errorf("invalid stream %q", stream)
	}
}
func statePath(root, key string) (string, error) {
	dir, err := jobDir(root, key)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}
func readRecord(root, key string) (Record, error) {
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
		return rec, err
	}
	return rec, nil
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
	p, err := statePath(root, rec.ExecKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "state.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, p); err != nil {
		os.Remove(p)
		return os.Rename(tmpPath, p)
	}
	return nil
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
func newKey() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("exec-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(b[:])), nil
}
func activeKey(root, key string) string { return filepath.Clean(root) + "\x00" + key }
func ts() string                        { return time.Now().UTC().Format(time.RFC3339Nano) }
