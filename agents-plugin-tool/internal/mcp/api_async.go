package mcp

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
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

var errAPIAsyncJobsNotImplemented = errors.New("api async jobs not implemented")

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
	APIJobKey       string                 `json:"api_job_key"`
	Root            string                 `json:"root"`
	Prompt          string                 `json:"prompt"`
	DomainHint      string                 `json:"domain_hint,omitempty"`
	Status          string                 `json:"status"`
	ResolvedDomains []string               `json:"resolved_domains,omitempty"`
	Domains         []apiDomainJobProgress `json:"domains,omitempty"`
	ResultText      string                 `json:"result_text,omitempty"`
	Error           string                 `json:"error,omitempty"`
	CancelRequested bool                   `json:"cancel_requested,omitempty"`
	StartedAt       string                 `json:"started_at"`
	UpdatedAt       string                 `json:"updated_at"`
	CompletedAt     string                 `json:"completed_at,omitempty"`
}

// startAPIJob is the MCP-visible async API-doc start contract. The eventual
// implementation should persist an apiJobRecord before returning and run the
// existing API-doc routing/manager path asynchronously instead of duplicating
// manager registration or cache ownership logic.
func (s *Server) startAPIJob(ctx context.Context, root, prompt, hint string) (apiJobStartResponse, error) {
	_ = ctx
	_ = s
	_ = root
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return apiJobStartResponse{}, errors.New("prompt is required")
	}
	return apiJobStartResponse{
		Status:      apiJobStateQueued,
		Prompt:      prompt,
		DomainHint:  strings.TrimSpace(hint),
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		ResultReady: false,
	}, errAPIAsyncJobsNotImplemented
}

func (s *Server) statusAPIJob(ctx context.Context, root, key string) (apiJobStatusResponse, error) {
	_ = ctx
	_ = s
	_ = root
	key = strings.TrimSpace(key)
	if key == "" {
		return apiJobStatusResponse{}, errors.New("api_job_key is required")
	}
	return apiJobStatusResponse{APIJobKey: key}, errAPIAsyncJobsNotImplemented
}

func (s *Server) resultAPIJob(ctx context.Context, root, key string) (string, error) {
	_ = ctx
	_ = s
	_ = root
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("api_job_key is required")
	}
	return "", errAPIAsyncJobsNotImplemented
}

func (s *Server) cancelAPIJob(ctx context.Context, root, key string) (apiJobStatusResponse, error) {
	_ = ctx
	_ = s
	_ = root
	key = strings.TrimSpace(key)
	if key == "" {
		return apiJobStatusResponse{}, errors.New("api_job_key is required")
	}
	return apiJobStatusResponse{
		APIJobKey:       key,
		Status:          apiJobStateCancelRequested,
		CancelRequested: true,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}, errAPIAsyncJobsNotImplemented
}
