package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

const (
	apiJobSchemaVersion = 1

	apiJobWorkerHeartbeatInterval = time.Second
	apiJobWorkerStaleAfter        = 15 * time.Second

	apiJobStateQueued          = "queued"
	apiJobStateRouting         = "routing"
	apiJobStateRunning         = "running"
	apiJobStateSucceeded       = "succeeded"
	apiJobStatePartialFailed   = "partial_failed"
	apiJobStateFailed          = "failed"
	apiJobStateCancelRequested = "cancel_requested"
	apiJobStateCancelled       = "cancelled"

	apiDomainStatePending   = "pending"
	apiDomainStateRunning   = "running"
	apiDomainStateSucceeded = "succeeded"
	apiDomainStateFailed    = "failed"
	apiDomainStateCancelled = "cancelled"
)

var (
	apiJobKeyPattern = regexp.MustCompile(`^api-[0-9]+-[a-f0-9]{16}$`)
	apiJobStateMu    sync.Mutex
	apiJobCancels    sync.Map // map[root\x00api_job_key]apiJobActiveWorker
)

type apiJobStartResponse struct {
	APIJobKey   string `json:"api_job_key"`
	Status      string `json:"status"`
	Prompt      string `json:"prompt"`
	DomainHint  string `json:"domain_hint,omitempty"`
	StartedAt   string `json:"started_at"`
	ResultReady bool   `json:"result_ready"`
}

type apiJobStatusResponse struct {
	APIJobKey       string                 `json:"api_job_key"`
	Status          string                 `json:"status"`
	Prompt          string                 `json:"prompt,omitempty"`
	DomainHint      string                 `json:"domain_hint,omitempty"`
	ResolvedDomains []string               `json:"resolved_domains,omitempty"`
	Domains         []apiDomainJobProgress `json:"domains,omitempty"`
	ResultReady     bool                   `json:"result_ready"`
	CancelRequested bool                   `json:"cancel_requested,omitempty"`
	StartedAt       string                 `json:"started_at,omitempty"`
	UpdatedAt       string                 `json:"updated_at,omitempty"`
	CompletedAt     string                 `json:"completed_at,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

type apiDomainJobProgress struct {
	Domain      string `json:"domain"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type apiJobRecord struct {
	SchemaVersion     int                    `json:"schema_version"`
	APIJobKey         string                 `json:"api_job_key"`
	Root              string                 `json:"root"`
	Prompt            string                 `json:"prompt"`
	DomainHint        string                 `json:"domain_hint,omitempty"`
	Status            string                 `json:"status"`
	WorkerPID         int                    `json:"worker_pid,omitempty"`
	WorkerGeneration  string                 `json:"worker_generation,omitempty"`
	WorkerStartedAt   string                 `json:"worker_started_at,omitempty"`
	WorkerHeartbeatAt string                 `json:"worker_heartbeat_at,omitempty"`
	ResolvedDomains   []string               `json:"resolved_domains,omitempty"`
	Domains           []apiDomainJobProgress `json:"domains,omitempty"`
	ResultText        string                 `json:"result_text,omitempty"`
	Error             string                 `json:"error,omitempty"`
	CancelRequested   bool                   `json:"cancel_requested,omitempty"`
	StartedAt         string                 `json:"started_at"`
	UpdatedAt         string                 `json:"updated_at"`
	CompletedAt       string                 `json:"completed_at,omitempty"`
}

type apiJobActiveWorker struct {
	cancel     context.CancelFunc
	generation string
}

// startAPIJob persists an apiJobRecord before returning and starts a process-local
// worker that reuses the synchronous API-doc routing and per-domain manager path.
func (s *Server) startAPIJob(ctx context.Context, root, prompt, hint string) (apiJobStartResponse, error) {
	_ = ctx
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return apiJobStartResponse{}, errors.New("prompt is required")
	}
	hint = strings.TrimSpace(hint)
	key, err := newAPIJobKey()
	if err != nil {
		return apiJobStartResponse{}, err
	}
	generation, err := newAPIJobGeneration()
	if err != nil {
		return apiJobStartResponse{}, err
	}
	now := apiJobTimestamp()
	record := apiJobRecord{
		SchemaVersion:     apiJobSchemaVersion,
		APIJobKey:         key,
		Root:              filepath.Clean(root),
		Prompt:            prompt,
		DomainHint:        hint,
		Status:            apiJobStateQueued,
		WorkerPID:         os.Getpid(),
		WorkerGeneration:  generation,
		WorkerStartedAt:   now,
		WorkerHeartbeatAt: now,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := createAPIJobRecord(root, record); err != nil {
		return apiJobStartResponse{}, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	activeKey := apiJobActiveKey(root, key)
	apiJobCancels.Store(activeKey, apiJobActiveWorker{cancel: cancel, generation: generation})
	go func() {
		defer apiJobCancels.Delete(activeKey)
		defer cancel()
		s.runAPIJob(runCtx, root, key, generation)
	}()

	return apiJobStartResponse{
		APIJobKey:   key,
		Status:      apiJobStateQueued,
		Prompt:      prompt,
		DomainHint:  hint,
		StartedAt:   now,
		ResultReady: false,
	}, nil
}

func (s *Server) statusAPIJob(ctx context.Context, root, key string) (apiJobStatusResponse, error) {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return apiJobStatusResponse{}, errors.New("api_job_key is required")
	}
	record, err := reconcileAPIJobRecord(root, key)
	if err != nil {
		return apiJobStatusResponse{}, err
	}
	return apiJobStatusFromRecord(record), nil
}

func (s *Server) resultAPIJob(ctx context.Context, root, key string) (string, error) {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("api_job_key is required")
	}
	record, err := reconcileAPIJobRecord(root, key)
	if err != nil {
		return "", err
	}
	switch record.Status {
	case apiJobStateSucceeded, apiJobStatePartialFailed:
		return record.ResultText, nil
	case apiJobStateFailed:
		message := strings.TrimSpace(record.Error)
		if message == "" {
			message = "api job failed"
		}
		return record.ResultText, errors.New(message)
	case apiJobStateCancelRequested, apiJobStateCancelled:
		message := strings.TrimSpace(record.Error)
		if message == "" {
			message = "api job cancelled"
		}
		return record.ResultText, errors.New(message)
	default:
		return "", fmt.Errorf("api job %q result is not ready", key)
	}
}

func (s *Server) cancelAPIJob(ctx context.Context, root, key string) (apiJobStatusResponse, error) {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return apiJobStatusResponse{}, errors.New("api_job_key is required")
	}
	record, err := updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		if apiJobResultReady(record.Status) {
			return
		}
		record.CancelRequested = true
		record.Status = apiJobStateCancelRequested
		if record.Error == "" {
			record.Error = "api job cancellation requested"
		}
	})
	if err != nil {
		return apiJobStatusResponse{}, err
	}
	if cancelValue, ok := apiJobCancels.Load(apiJobActiveKey(root, key)); ok {
		if worker, ok := cancelValue.(apiJobActiveWorker); ok {
			worker.cancel()
		}
	}
	record, err = reconcileAPIJobRecord(root, key)
	if err != nil {
		return apiJobStatusResponse{}, err
	}
	return apiJobStatusFromRecord(record), nil
}

func (s *Server) runAPIJob(ctx context.Context, root, key, generation string) {
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go heartbeatAPIJob(heartbeatCtx, root, key, generation)

	if apiJobCancellationRequested(ctx, root, key) {
		completeAPIJobCancelled(root, key)
		return
	}
	_, err := updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		record.Status = apiJobStateRouting
	})
	if err != nil {
		return
	}

	record, err := loadAPIJobRecord(root, key)
	if err != nil {
		return
	}
	domains, err := s.resolveAPIDomains(ctx, root, record.Prompt, record.DomainHint)
	if err != nil {
		if apiJobCancellationRequested(ctx, root, key) || errors.Is(err, context.Canceled) {
			completeAPIJobCancelled(root, key)
			return
		}
		completeAPIJobFailed(root, key, "", err)
		return
	}
	if len(domains) == 0 {
		completeAPIJobFailed(root, key, "", errors.New("api.ask resolved no domains"))
		return
	}

	progress := make([]apiDomainJobProgress, len(domains))
	for i, domain := range domains {
		progress[i] = apiDomainJobProgress{Domain: domain, Status: apiDomainStatePending}
	}
	_, err = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		record.Status = apiJobStateRunning
		record.ResolvedDomains = append([]string(nil), domains...)
		record.Domains = append([]apiDomainJobProgress(nil), progress...)
	})
	if err != nil {
		return
	}
	if apiJobCancellationRequested(ctx, root, key) {
		completeAPIJobCancelled(root, key)
		return
	}

	results := make([]apiDomainResult, len(domains))
	var wg sync.WaitGroup
	for i, domain := range domains {
		i, domain := i, domain
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = s.runAPIJobDomain(ctx, root, key, domain, record.Prompt)
		}()
	}
	wg.Wait()

	if apiJobCancellationRequested(ctx, root, key) {
		completeAPIJobCancelled(root, key)
		return
	}

	text, successes := formatAPIResults(results)
	if successes == 0 {
		completeAPIJobFailed(root, key, text, errors.New("api.ask failed for all resolved domains"))
		return
	}
	status := apiJobStateSucceeded
	if successes < len(results) {
		status = apiJobStatePartialFailed
	}
	completeAPIJobSucceeded(root, key, text, status)
}

func (s *Server) runAPIJobDomain(ctx context.Context, root, key, domain, prompt string) apiDomainResult {
	result := apiDomainResult{domain: domain}
	if apiJobCancellationRequested(ctx, root, key) {
		result.err = context.Canceled
		markAPIJobDomain(root, key, domain, apiDomainStateCancelled, context.Canceled)
		return result
	}
	markAPIJobDomain(root, key, domain, apiDomainStateRunning, nil)
	if err := os.MkdirAll(filepath.Join(apiDepsDir(root), domain), 0o755); err != nil {
		result.err = err
		markAPIJobDomain(root, key, domain, apiDomainStateFailed, err)
		return result
	}
	lock := apiLockFor(root, domain)
	lock.Lock()
	text, err := s.apiRuntime().AskManager(ctx, root, domain, prompt)
	lock.Unlock()
	result.text = text
	result.err = err
	if apiJobCancellationRequested(ctx, root, key) || errors.Is(err, context.Canceled) {
		result.err = context.Canceled
		markAPIJobDomain(root, key, domain, apiDomainStateCancelled, context.Canceled)
		return result
	}
	if err != nil {
		markAPIJobDomain(root, key, domain, apiDomainStateFailed, err)
		return result
	}
	markAPIJobDomain(root, key, domain, apiDomainStateSucceeded, nil)
	return result
}

func markAPIJobDomain(root, key, domain, status string, err error) {
	_, _ = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		for i := range record.Domains {
			if record.Domains[i].Domain != domain {
				continue
			}
			if record.Domains[i].StartedAt == "" && status == apiDomainStateRunning {
				record.Domains[i].StartedAt = now
			}
			if status != apiDomainStateRunning && record.Domains[i].StartedAt == "" {
				record.Domains[i].StartedAt = now
			}
			record.Domains[i].Status = status
			if status == apiDomainStateSucceeded || status == apiDomainStateFailed || status == apiDomainStateCancelled {
				record.Domains[i].CompletedAt = now
			}
			if err != nil {
				record.Domains[i].Error = err.Error()
			} else if status == apiDomainStateSucceeded {
				record.Domains[i].Error = ""
			}
			return
		}
	})
}

func completeAPIJobSucceeded(root, key, text, status string) {
	_, _ = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		record.Status = status
		record.ResultText = text
		record.Error = ""
		record.CompletedAt = now
	})
}

func completeAPIJobFailed(root, key, text string, err error) {
	_, _ = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		record.Status = apiJobStateFailed
		record.ResultText = text
		if err != nil {
			record.Error = err.Error()
		}
		record.CompletedAt = now
		for i := range record.Domains {
			if record.Domains[i].Status == apiDomainStatePending || record.Domains[i].Status == apiDomainStateRunning || record.Domains[i].Status == "" {
				record.Domains[i].Status = apiDomainStateFailed
				record.Domains[i].CompletedAt = now
				if err != nil && record.Domains[i].Error == "" {
					record.Domains[i].Error = err.Error()
				}
			}
		}
	})
}

func completeAPIJobCancelled(root, key string) {
	_, _ = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
		record.Status = apiJobStateCancelled
		record.CancelRequested = true
		if record.Error == "" || record.Error == "api job cancellation requested" {
			record.Error = "api job cancelled"
		}
		record.CompletedAt = now
		for i := range record.Domains {
			if record.Domains[i].Status == apiDomainStatePending || record.Domains[i].Status == apiDomainStateRunning || record.Domains[i].Status == "" {
				record.Domains[i].Status = apiDomainStateCancelled
				if record.Domains[i].StartedAt == "" {
					record.Domains[i].StartedAt = now
				}
				record.Domains[i].CompletedAt = now
				record.Domains[i].Error = context.Canceled.Error()
			}
		}
	})
}

func apiJobCancellationRequested(ctx context.Context, root, key string) bool {
	if ctx.Err() != nil {
		return true
	}
	record, err := loadAPIJobRecord(root, key)
	if err != nil {
		return false
	}
	return record.CancelRequested || record.Status == apiJobStateCancelRequested || record.Status == apiJobStateCancelled
}

func heartbeatAPIJob(ctx context.Context, root, key, generation string) {
	ticker := time.NewTicker(apiJobWorkerHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = updateAPIJobRecord(root, key, func(record *apiJobRecord, now string) {
				if record.WorkerGeneration != generation || apiJobResultReady(record.Status) {
					return
				}
				record.WorkerHeartbeatAt = now
			})
		}
	}
}

func reconcileAPIJobRecord(root, key string) (apiJobRecord, error) {
	record, err := loadAPIJobRecord(root, key)
	if err != nil {
		return apiJobRecord{}, err
	}
	if apiJobResultReady(record.Status) || apiJobWorkerLikelyLive(root, record) {
		return record, nil
	}
	if record.CancelRequested || record.Status == apiJobStateCancelRequested {
		completeAPIJobCancelled(root, key)
	} else {
		completeAPIJobFailed(root, key, record.ResultText, errors.New("api job worker is no longer active"))
	}
	return loadAPIJobRecord(root, key)
}

func apiJobWorkerLikelyLive(root string, record apiJobRecord) bool {
	if cancelValue, ok := apiJobCancels.Load(apiJobActiveKey(root, record.APIJobKey)); ok {
		if worker, ok := cancelValue.(apiJobActiveWorker); ok && worker.generation == record.WorkerGeneration {
			return true
		}
	}
	heartbeat := strings.TrimSpace(record.WorkerHeartbeatAt)
	if heartbeat == "" {
		return false
	}
	ts, err := time.Parse(time.RFC3339Nano, heartbeat)
	if err != nil {
		if fallback, fallbackErr := time.Parse(time.RFC3339, heartbeat); fallbackErr == nil {
			ts = fallback
		} else {
			return false
		}
	}
	return time.Since(ts) <= apiJobWorkerStaleAfter
}

func apiJobStatusFromRecord(record apiJobRecord) apiJobStatusResponse {
	return apiJobStatusResponse{
		APIJobKey:       record.APIJobKey,
		Status:          record.Status,
		Prompt:          record.Prompt,
		DomainHint:      record.DomainHint,
		ResolvedDomains: append([]string(nil), record.ResolvedDomains...),
		Domains:         append([]apiDomainJobProgress(nil), record.Domains...),
		ResultReady:     apiJobResultReady(record.Status),
		CancelRequested: record.CancelRequested,
		StartedAt:       record.StartedAt,
		UpdatedAt:       record.UpdatedAt,
		CompletedAt:     record.CompletedAt,
		Error:           record.Error,
	}
}

func apiJobResultReady(status string) bool {
	switch status {
	case apiJobStateSucceeded, apiJobStatePartialFailed, apiJobStateFailed, apiJobStateCancelled:
		return true
	default:
		return false
	}
}

func createAPIJobRecord(root string, record apiJobRecord) error {
	apiJobStateMu.Lock()
	defer apiJobStateMu.Unlock()
	return writeAPIJobRecordLocked(root, record)
}

func loadAPIJobRecord(root, key string) (apiJobRecord, error) {
	apiJobStateMu.Lock()
	defer apiJobStateMu.Unlock()
	return readAPIJobRecordLocked(root, key)
}

func updateAPIJobRecord(root, key string, mutate func(*apiJobRecord, string)) (apiJobRecord, error) {
	apiJobStateMu.Lock()
	defer apiJobStateMu.Unlock()
	record, err := readAPIJobRecordLocked(root, key)
	if err != nil {
		return apiJobRecord{}, err
	}
	now := apiJobTimestamp()
	mutate(&record, now)
	record.UpdatedAt = now
	if err := writeAPIJobRecordLocked(root, record); err != nil {
		return apiJobRecord{}, err
	}
	return record, nil
}

func readAPIJobRecordLocked(root, key string) (apiJobRecord, error) {
	path, err := apiJobStatePath(root, key)
	if err != nil {
		return apiJobRecord{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return apiJobRecord{}, fmt.Errorf("api job %q not found", key)
	}
	if err != nil {
		return apiJobRecord{}, fmt.Errorf("read api job %q: %w", key, err)
	}
	var record apiJobRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return apiJobRecord{}, fmt.Errorf("parse api job %q: %w", key, err)
	}
	if record.APIJobKey == "" {
		record.APIJobKey = key
	}
	if record.Status == "" {
		record.Status = apiJobStateQueued
	}
	return record, nil
}

func writeAPIJobRecordLocked(root string, record apiJobRecord) error {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = apiJobSchemaVersion
	}
	if record.APIJobKey == "" {
		return errors.New("api_job_key is required")
	}
	path, err := apiJobStatePath(root, record.APIJobKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create api job dir: %w", err)
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal api job %q: %w", record.APIJobKey, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create api job temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write api job temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close api job temp: %w", err)
	}
	if err := apiJobReplaceFile(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace api job state: %w", err)
	}
	return nil
}

func apiJobStatePath(root, key string) (string, error) {
	key = strings.TrimSpace(key)
	if !apiJobKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid api_job_key %q", key)
	}
	layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.WorktreeDir, "api-jobs", key, "state.json"), nil
}

func apiJobReplaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	} else if _, statErr := os.Stat(path); statErr != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, path)
}

func newAPIJobKey() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate api job key: %w", err)
	}
	return fmt.Sprintf("api-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(random[:])), nil
}

func newAPIJobGeneration() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate api job worker generation: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func apiJobActiveKey(root, key string) string {
	return filepath.Clean(root) + "\x00" + key
}

func apiJobTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
