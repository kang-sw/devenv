package wsdashboard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// HiringMetadata is dashboard-owned metadata, separate from registrations and
// runner records: older typed writers of those records must not erase it.
// RequestID correlates a creation retry, not an authority or grouping link.
type HiringMetadata struct {
	Enabled   bool   `json:"enabled"`
	Hired     bool   `json:"hired,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

const hiringFileName = "hiring.json"

func ReadHiringMetadata(dir, replyID string) (HiringMetadata, error) {
	var m HiringMetadata
	raw, err := os.ReadFile(filepath.Join(SessionDir(dir, replyID), hiringFileName))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(raw, &m)
	return m, err
}

func writeHiringMetadata(dir, replyID string, m HiringMetadata) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writePrivateFile(SessionDir(dir, replyID), hiringFileName, append(raw, '\n'))
}

// MarkHired runs before the runner publishes a new lead record. No model-facing
// tool writes this marker; it survives detaching, handoff, and old row writers.
func MarkHired(dir, replyID, requestID, label string) error {
	if err := writeSessionLabel(SessionDir(dir, replyID), label); err != nil {
		return err
	}
	return writeHiringMetadata(dir, replyID, HiringMetadata{Hired: true, RequestID: requestID})
}
