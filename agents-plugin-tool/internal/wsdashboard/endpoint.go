// Package wsdashboard is the machine-wide dashboard daemon's lifecycle: the
// single-instance OS file lock, the endpoint file, the loopback HTTP server
// with its Host guard, and the ws-mcp side that finds, starts, and replaces
// the daemon.
//
// Liveness rules, all chosen so that "no answer" is never read as "gone":
//
//   - The OS file lock is the only proof a daemon is running. The kernel
//     releases it when the holder exits, so a stale endpoint file never blocks
//     a new daemon, and a pid recorded in that file is never used to signal or
//     kill anything.
//   - Only a daemon process takes the lock. ws-mcp never probes it, so a probe
//     can never make a starting daemon believe another one holds it.
//   - A ws-mcp that cannot reach the recorded daemon starts a new one; when the
//     old holder is alive but hung, the new daemon finds the lock held, waits
//     its bounded lock wait, and exits. Nothing is terminated.
package wsdashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsstate"
)

const (
	// DefaultPort is the dashboard.port built-in value: an IANA-unassigned
	// port below both the Linux (32768+) and Windows (49152+) ephemeral ranges,
	// so an outgoing connection rarely holds it.
	DefaultPort = 29417

	dirName          = "dashboard"
	lockFileName     = "dashboard.lock"
	endpointFileName = "endpoint.json"

	endpointSchemaVersion = 1
)

// Endpoint is the endpoint file a running daemon writes: where it listens and
// the token its shutdown request requires. Readers ignore unknown fields and
// treat absent ones as empty, so a newer daemon's file stays readable by an
// older ws-mcp and the reverse.
type Endpoint struct {
	SchemaVersion int    `json:"schema_version"`
	Port          int    `json:"port"`
	PID           int    `json:"pid"`
	Version       string `json:"version"`
	Token         string `json:"token"`
	StartedAt     string `json:"started_at"`
}

// Dir returns the daemon's state directory under the machine cache root. Each
// OS environment (a WSL distro, native Windows, a macOS user) has its own
// cache root, so each gets its own daemon.
func Dir(cacheHome string) (string, error) {
	root, err := wsstate.CacheRoot(wsstate.Options{CacheHome: cacheHome})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, dirName), nil
}

// ReadEndpoint reads dir's endpoint file. Unknown fields are ignored.
func ReadEndpoint(dir string) (Endpoint, error) {
	raw, err := os.ReadFile(filepath.Join(dir, endpointFileName))
	if err != nil {
		return Endpoint{}, err
	}
	var ep Endpoint
	if err := json.Unmarshal(bytes.TrimSpace(raw), &ep); err != nil {
		return Endpoint{}, fmt.Errorf("parse endpoint file: %w", err)
	}
	return ep, nil
}

// writeEndpoint replaces dir's endpoint file atomically, readable by the
// owner only because it carries the shutdown token.
func writeEndpoint(dir string, ep Endpoint) error {
	raw, err := json.MarshalIndent(ep, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(dir, endpointFileName, append(raw, '\n'))
}

// writePrivateFile replaces dir/name atomically (temp file + rename) with an
// owner-only file holding data.
func writePrivateFile(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// removeEndpointIfOwned deletes dir's endpoint file when it still carries
// token, so a daemon never deletes a successor's file.
func removeEndpointIfOwned(dir, token string) {
	if ep, err := ReadEndpoint(dir); err == nil && ep.Token == token {
		_ = os.Remove(filepath.Join(dir, endpointFileName))
	}
}
