package wsdashboard

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// RunningExecutableFile is private restart evidence, not a runtime selection
// registry. Only the daemon holding the singleton writes it. Older daemons
// ignore it; settled executables and the last record survive their runs.
const RunningExecutableFile = "last-running-executable.json"

type runningExecutable struct {
	SchemaVersion int    `json:"schema_version"`
	Executable    string `json:"executable"`
	Version       string `json:"version"`
	SHA256        string `json:"binary_sha256"`
	BuildTime     int64  `json:"build_time"`
}

// recordRunningExecutable retains the current build before publishing restart
// evidence. It is called once at startup under the daemon lock, before hosted
// processes can reconnect. No session, heartbeat or tool call writes this file.
func recordRunningExecutable(dir, exe, version string, buildTime int64) error {
	staged, err := DaemonExecutable(exe, dir)
	if err != nil {
		return fmt.Errorf("retain running executable: %w", err)
	}
	staged, err = filepath.Abs(staged)
	if err != nil {
		return err
	}
	digest, err := fileSHA256(staged)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(runningExecutable{1, staged, version, digest, buildTime})
	if err != nil {
		return err
	}
	return writePrivateFile(dir, RunningExecutableFile, append(raw, '\n'))
}
