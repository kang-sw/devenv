package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
)

// persistHolderExit records lead-holder exits without creating or taking over a row.
func persistHolderExit(dir string) func(wsholder.Descriptor, wsholder.Exit) error {
	return func(desc wsholder.Descriptor, exit wsholder.Exit) error {
		var session struct {
			Kind    string `json:"kind"`
			ReplyID string `json:"reply_id"`
		}
		if json.Unmarshal(desc.Session, &session) != nil || session.Kind != "lead" || !wsdashboard.ValidReplyID(session.ReplyID) {
			return nil
		}
		unlock, err := wsdashboard.LockRegistration(dir, session.ReplyID)
		if err != nil {
			return err
		}
		defer unlock()
		row, err := wsdashboard.ReadRegistration(dir, session.ReplyID)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !row.Hosted {
			return nil
		}
		// A restarted lead has a newer holder even before that holder has
		// exited. Do not attach an old holder's outcome to that process.
		entries, err := os.ReadDir(filepath.Join(dir, wsholder.DirName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == desc.ID || !wsholder.ValidID(entry.Name()) {
				continue
			}
			other, err := wsholder.ReadDescriptor(filepath.Join(dir, wsholder.DirName, entry.Name()))
			if err != nil || other.StartedAt <= desc.StartedAt {
				continue
			}
			var otherSession struct {
				Kind    string `json:"kind"`
				ReplyID string `json:"reply_id"`
			}
			if json.Unmarshal(other.Session, &otherSession) == nil && otherSession.Kind == "lead" && otherSession.ReplyID == session.ReplyID {
				return nil
			}
		}
		// An older holder must not replace a newer recorded exit.
		oldTime, oldErr := time.Parse(time.RFC3339Nano, row.HostStoppedAt)
		newTime, newErr := time.Parse(time.RFC3339Nano, exit.StoppedAt)
		if oldErr == nil && (newErr != nil || oldTime.After(newTime)) {
			return nil
		}
		row.HostExitCode, row.HostExitSignal, row.HostExitReason, row.HostStoppedAt = exit.ExitCode, exit.Signal, exit.Reason, exit.StoppedAt
		return wsdashboard.WriteRegistration(dir, row)
	}
}
