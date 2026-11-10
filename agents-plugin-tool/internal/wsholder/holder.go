// Package wsholder is the daemon's side of the session holders: small
// Python processes (holder.py, embedded here) that each own one child
// process, an agent over pipes or a shell over a PTY, and outlive the
// dashboard daemon. The daemon stages the script, starts a holder detached,
// finds running holders through their descriptor files, attaches to each
// over loopback TCP, and stops one through the protocol or, when it cannot
// attach, with a kill guarded by the holder's recorded start time.
//
// holder.py's module docstring is the protocol and descriptor reference.
package wsholder

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed holder.py
var script []byte

const (
	// DirName is the holders directory inside the dashboard state directory:
	// one directory per holder, named by its id, plus the staged scripts.
	DirName = "holders"
	// PythonEnv names the interpreter that runs a holder. The plugin
	// launcher sets it to its own interpreter when it starts the daemon;
	// without it the daemon uses python3 on PATH.
	PythonEnv = "WS_MCP_HOLDER_PYTHON"
	// DefaultRescanInterval is how often a running manager looks for
	// holders again.
	DefaultRescanInterval = 3 * time.Minute

	descriptorName = "descriptor.json"
	logName        = "holder.log"
	// scriptDirName holds the staged scripts; it is never a holder id.
	scriptDirName = "bin"
	// stagedScriptGrace keeps a staged script younger than this from the
	// prune: a holder started from it may not have written its descriptor
	// yet.
	stagedScriptGrace = 2 * time.Minute
	// orphanDirGrace is how long a holder directory may stay without a
	// descriptor before a scan removes it; a live holder rewrites a missing
	// descriptor within a minute.
	orphanDirGrace = 10 * time.Minute
)

// protocolVersion is the attach protocol this daemon speaks natively. It
// also attaches to a holder that reports the version before it. A variable
// so a test can stand in for a later daemon.
var protocolVersion = 1

// supportedProtocol reports whether this daemon can attach to a holder that
// reports protocol v: the current version and the one before it.
func supportedProtocol(v int) bool {
	return v >= 1 && (v == protocolVersion || v == protocolVersion-1)
}

// Descriptor is a holder's registration, the descriptor.json it writes
// (owner-only) in its state directory. Unknown fields are ignored.
type Descriptor struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	PID    int    `json:"pid"`
	Port   int    `json:"port"`
	Token  string `json:"token"`
	// Protocol is the attach protocol version the holder speaks.
	Protocol int `json:"protocol"`
	// StartedAt is the holder's own wall-clock start in epoch seconds; a
	// guarded kill compares it with the OS creation time of PID.
	StartedAt float64 `json:"started_at"`
	// ScriptHash is the sha256 of the script the holder runs.
	ScriptHash string `json:"script_hash"`
	// RestartSafe is false for a holder that could not leave its starter's
	// job object (Windows) and so may end when the daemon's host does.
	RestartSafe bool `json:"restart_safe"`
	// Session is the consumer's own description of what the holder hosts.
	Session json.RawMessage `json:"session,omitempty"`
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidID reports whether id can name a holder directory.
func ValidID(id string) bool { return id != scriptDirName && idPattern.MatchString(id) }

// ReadDescriptor reads the descriptor of the holder directory dir.
func ReadDescriptor(dir string) (Descriptor, error) {
	raw, err := os.ReadFile(filepath.Join(dir, descriptorName))
	if err != nil {
		return Descriptor{}, err
	}
	var d Descriptor
	if err := json.Unmarshal(raw, &d); err != nil {
		return Descriptor{}, fmt.Errorf("holder descriptor: %w", err)
	}
	if d.ID != filepath.Base(dir) || d.PID <= 0 || d.Port <= 0 || d.Port > 65535 || d.Token == "" {
		return Descriptor{}, errors.New("holder descriptor: missing or mismatched fields")
	}
	return d, nil
}

// ScriptHash is the sha256 of the embedded holder script.
func ScriptHash() string {
	sum := sha256.Sum256(script)
	return hex.EncodeToString(sum[:])
}

// stagedScriptName is the file name a script of the given sha256 is staged
// under.
func stagedScriptName(hash string) string {
	if len(hash) > 16 {
		hash = hash[:16]
	}
	return "holder-" + hash + ".py"
}

// StageScript writes the embedded holder script into holdersDir's script
// directory under a name taken from its content, reusing a copy already
// there, and returns its path. A running holder keeps the file it was
// started from, so an update never rewrites a script in use.
func StageScript(holdersDir string) (string, error) {
	binDir := filepath.Join(holdersDir, scriptDirName)
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(binDir, stagedScriptName(ScriptHash()))
	if existing, err := os.ReadFile(target); err == nil && string(existing) == string(script) {
		return target, nil
	}
	tmp, err := os.CreateTemp(binDir, filepath.Base(target)+".*.tmp")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(script); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return target, nil
}

// pruneStagedScripts removes every staged script that no hash in keep
// names, except the embedded script's own copy and files younger than
// stagedScriptGrace.
func pruneStagedScripts(holdersDir string, keep map[string]bool, now time.Time) {
	binDir := filepath.Join(holdersDir, scriptDirName)
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	names := map[string]bool{stagedScriptName(ScriptHash()): true}
	for hash := range keep {
		names[stagedScriptName(hash)] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		if names[name] || !strings.HasPrefix(name, "holder-") || !strings.HasSuffix(name, ".py") {
			continue
		}
		if info, err := entry.Info(); err != nil || now.Sub(info.ModTime()) < stagedScriptGrace {
			continue
		}
		_ = os.Remove(filepath.Join(binDir, name))
	}
}
