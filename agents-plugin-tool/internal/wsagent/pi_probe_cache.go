package wsagent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

const piProbeCacheSchema = 1

var piProbeNow = time.Now

// These are derived successes only. Keep them apart from support.json and
// the dashboard's session and staged-binary directories: older writers own
// those paths, and must not erase a success on a daemon restart.
type piProbeRecord struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	Identity      piProbeIdentity `json:"identity"`
	ObservedAt    time.Time       `json:"observed_at"`
	Value         json.RawMessage `json:"value"`
}

type piProbeIdentity struct {
	Kind     string         `json:"kind"`
	Files    []piProbeStamp `json:"files"`
	AgentDir string         `json:"agent_dir,omitempty"`
	Config   []piProbeStamp `json:"config,omitempty"`
}

// Absence is a stamp too. In particular, creating auth.json or models.json
// must invalidate a listing without ever reading either file's contents.
type piProbeStamp struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
	MtimeNS int64  `json:"mtime_ns"`
}

type piProbeCache struct {
	path     string
	identity piProbeIdentity
}

func piProbeCanonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	} else if !os.IsNotExist(err) {
		return "", err
	} else if parent := filepath.Dir(abs); parent != abs {
		// Resolve existing parents even when the leaf is absent.
		parent, err = piProbeCanonical(parent)
		if err != nil {
			return "", err
		}
		abs = filepath.Join(parent, filepath.Base(abs))
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

func piProbeStat(path string) (piProbeStamp, error) {
	path, err := piProbeCanonical(path)
	if err != nil {
		return piProbeStamp{}, err
	}
	s := piProbeStamp{Path: path}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !info.Mode().IsRegular() {
		return s, fmt.Errorf("not a regular probe input")
	}
	s.Exists, s.Size, s.MtimeNS = true, info.Size(), info.ModTime().UnixNano()
	return s, nil
}

// command/pre are the actual probe's argv, not a replacement for it. Only
// identity construction unwraps the listing's Windows shim. Version already
// uses harnessCommand; retain its original PATH-selected shim as well.
func newPiProbeCache(kind, command string, pre, env []string) piProbeCache {
	id := piProbeIdentity{Kind: kind}
	add := func(path string, required bool) bool {
		s, err := piProbeStat(path)
		if err != nil || (required && !s.Exists) {
			return false
		}
		id.Files = append(id.Files, s)
		return true
	}
	if !add(command, true) {
		return piProbeCache{}
	}
	entry := id.Files[0].Path
	if kind == "version" && len(pre) != 0 {
		shim, err := lookPath("pi")
		if err != nil || !add(shim, true) || !add(pre[0], true) {
			return piProbeCache{}
		}
		entry = id.Files[len(id.Files)-1].Path
	} else if kind == "models" && hostWindows && (strings.EqualFold(filepath.Ext(command), ".cmd") || strings.EqualFold(filepath.Ext(command), ".bat")) {
		node, args, err := harnessCommand(command, "pi")
		if err != nil || len(args) != 1 || !add(node, true) || !add(args[0], true) {
			return piProbeCache{}
		}
		entry = id.Files[len(id.Files)-1].Path
	} else {
		// POSIX npm installs normally symlink pi to a CLI with a node
		// shebang. Resolve that interpreter without executing anything.
		f, err := os.Open(entry)
		if err != nil {
			return piProbeCache{}
		}
		buf := make([]byte, 256)
		n, _ := f.Read(buf)
		_ = f.Close()
		line, _, _ := strings.Cut(string(buf[:n]), "\n")
		if strings.HasPrefix(line, "#!") {
			fields := strings.Fields(strings.TrimPrefix(line, "#!"))
			if len(fields) == 2 && filepath.Base(fields[0]) == "env" && fields[1] == "node" {
				node, err := lookPath("node")
				if err != nil || !add(node, true) {
					return piProbeCache{}
				}
			} else if len(fields) == 1 && (filepath.Base(fields[0]) == "node" || filepath.Base(fields[0]) == "nodejs") {
				if !add(fields[0], true) {
					return piProbeCache{}
				}
			} else {
				// An opaque wrapper still probes normally, but cannot safely
				// promise a persistent identity for what it launches.
				return piProbeCache{}
			}
		}
	}
	// Include absent nearer manifests as well: a newly installed manifest
	// can change package identity even if the CLI itself is untouched.
	for dir := filepath.Dir(entry); ; dir = filepath.Dir(dir) {
		if !add(filepath.Join(dir, "package.json"), false) {
			return piProbeCache{}
		}
		if id.Files[len(id.Files)-1].Exists || filepath.Dir(dir) == dir {
			break
		}
	}
	if kind == "models" {
		get := func(key string) string {
			var value string
			for _, kv := range env {
				if k, v, ok := strings.Cut(kv, "="); ok && (k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key))) {
					value = v
				}
			}
			return value
		}
		home := get("HOME")
		if runtime.GOOS == "windows" {
			home = get("USERPROFILE")
		}
		dir := get("PI_CODING_AGENT_DIR")
		// Pi also accepts file URLs and Windows shell drive spellings.
		// Without normalizing those, a stat would name the wrong inputs;
		// keep the existing probe rather than persist a guessed identity.
		if strings.HasPrefix(dir, "file://") || (runtime.GOOS == "windows" && strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, "//")) {
			return piProbeCache{}
		}
		if dir == "" {
			if home == "" {
				return piProbeCache{}
			}
			dir = filepath.Join(home, ".pi", "agent")
		} else if dir == "~" || strings.HasPrefix(dir, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(dir, `~\`)) {
			if home == "" {
				return piProbeCache{}
			}
			dir = filepath.Join(home, strings.TrimLeft(dir[1:], `/\`))
		}
		var err error
		id.AgentDir, err = piProbeCanonical(dir)
		if err != nil {
			return piProbeCache{}
		}
		for _, name := range []string{"auth.json", "models.json"} {
			s, err := piProbeStat(filepath.Join(id.AgentDir, name))
			if err != nil {
				return piProbeCache{}
			}
			id.Config = append(id.Config, s)
		}
	}
	root, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return piProbeCache{}
	}
	raw, _ := json.Marshal(id)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(raw))
	return piProbeCache{path: filepath.Join(root, "dashboard", "pi-probes-v1", fingerprint+"-"+kind+".json"), identity: id}
}

func (c piProbeCache) load(now time.Time) (piProbeRecord, bool) {
	var r piProbeRecord
	if c.path == "" {
		return r, false
	}
	raw, err := os.ReadFile(c.path)
	if err != nil || json.Unmarshal(raw, &r) != nil || r.SchemaVersion != piProbeCacheSchema || r.Kind != c.identity.Kind || !reflect.DeepEqual(r.Identity, c.identity) || r.ObservedAt.IsZero() || r.ObservedAt.After(now) {
		return r, false
	}
	if r.Kind == "models" && now.Sub(r.ObservedAt) >= choicesTTL {
		return r, false
	}
	return r, true
}

func (c piProbeCache) save(at time.Time, value any) {
	if c.path == "" {
		return
	}
	// Recheck at publication too: a newer writer may have appeared while
	// this probe ran. Never replace an unrecognized future schema.
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if raw, err := os.ReadFile(c.path); err == nil && json.Unmarshal(raw, &header) == nil && header.SchemaVersion > piProbeCacheSchema {
		return
	}
	parsed, err := json.Marshal(value)
	if err != nil {
		return
	}
	raw, err := json.Marshal(piProbeRecord{SchemaVersion: piProbeCacheSchema, Kind: c.identity.Kind, Identity: c.identity, ObservedAt: at, Value: parsed})
	if err != nil {
		return
	}
	dir := filepath.Dir(c.path)
	if os.MkdirAll(dir, 0o700) != nil || os.Chmod(dir, 0o700) != nil {
		return
	}
	// Cache errors are never probe errors; atomic replacement also leaves
	// no partial success for another daemon to consume.
	_ = writeFileAtomic(c.path, append(raw, '\n'), 0o600)
}

func (r piProbeRecord) models() ([]ModelChoice, bool) {
	var models []ModelChoice
	if json.Unmarshal(r.Value, &models) != nil || models == nil {
		return nil, false
	}
	var table bytes.Buffer
	table.WriteString("provider model thinking\n")
	for _, m := range models {
		provider, model, ok := strings.Cut(m.ID, "/")
		if !ok {
			return nil, false
		}
		fmt.Fprintf(&table, "%s %s", provider, model)
		if m.Thinking != nil {
			if *m.Thinking {
				table.WriteString(" yes")
			} else {
				table.WriteString(" no")
			}
		}
		table.WriteByte('\n')
	}
	parsed, err := parsePiModels(table.Bytes())
	return models, err == nil && reflect.DeepEqual(parsed, models)
}

func (r piProbeRecord) version() (string, bool) {
	var version string
	if json.Unmarshal(r.Value, &version) != nil {
		return "", false
	}
	parsed, err := parsePiVersion([]byte(version))
	return version, err == nil && parsed == version
}
