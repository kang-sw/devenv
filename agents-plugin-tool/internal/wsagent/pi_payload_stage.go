package wsagent

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wspackage"
)

// Native Pi's Python materializer needs real archive paths. Stage only its
// build-owned inputs, under a content hash; never overwrite a settled build.
// Retain successful paths in memory so another launch does no staging I/O.
var piPayloadStages = struct {
	sync.Mutex
	paths map[string]string
}{paths: make(map[string]string)}

func stagePiPayloadInputs(stateDir, name string) (string, error) {
	owner, err := wspackage.Open(name)
	if err != nil {
		return "", err
	}
	shared, err := wspackage.Open("ws")
	if err != nil {
		return "", err
	}
	files := make(map[string][]byte)
	collect := func(source fs.FS, selectFile func(string) bool) error {
		return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path != "." && path != "bin" && !selectFile(path+"/") {
					return fs.SkipDir
				}
				return nil
			}
			if !selectFile(path) {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("native Pi input is a symlink: %s", path)
			}
			data, err := fs.ReadFile(source, path)
			if err == nil {
				files[path] = data
			}
			return err
		})
	}
	if err := collect(owner, func(path string) bool {
		return strings.HasPrefix(path, "rsrc/") || strings.HasPrefix(path, "skills/") ||
			path == "runtime.json" || path == "dashboard-config-manifest.json" || path == "bin/ws-mcp-launcher.py"
	}); err != nil {
		return "", err
	}
	if err := collect(shared, func(path string) bool {
		return strings.HasPrefix(path, "native-pi/") || path == "bin/ws-pi-payload.py"
	}); err != nil {
		return "", err
	}
	return stagePiFiles(stateDir, files)
}

func stagePiFiles(stateDir string, files map[string][]byte) (string, error) {
	return stagePiFilesWithRename(stateDir, files, os.Rename)
}

func stagePiFilesWithRename(stateDir string, files map[string][]byte, rename func(string, string) error) (string, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
			return "", fmt.Errorf("invalid native Pi input path %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%s\x00%d\x00", name, len(files[name]))
		hash.Write(files[name])
	}
	target := filepath.Join(stateDir, "native-pi-inputs", fmt.Sprintf("%x", hash.Sum(nil)))
	piPayloadStages.Lock()
	defer piPayloadStages.Unlock()
	if root := piPayloadStages.paths[target]; root != "" {
		return root, nil
	}
	parent := filepath.Dir(target)
	reclaimPiInputStages(parent, time.Now())
	// Verify a previous daemon's staged input once. A partial/corrupt directory
	// is refused rather than mutating files a surviving process may still use.
	if err := validatePiInputStage(target, names, files); err == nil {
		piPayloadStages.paths[target] = target
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	for _, name := range names {
		path := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, files[name], 0o600); err != nil {
			return "", err
		}
	}
	if err := rename(tmp, target); err != nil {
		// Another daemon may have published the same hash after our initial
		// check. Accept its directory only under the same checks as warm reuse.
		if verifyErr := validatePiInputStage(target, names, files); verifyErr != nil {
			return "", fmt.Errorf("publish native Pi inputs: %w (target validation: %v)", err, verifyErr)
		}
	}
	piPayloadStages.paths[target] = target
	return target, nil
}

func validatePiInputStage(target string, names []string, files map[string][]byte) error {
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid native Pi input directory: %s", target)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || string(data) != string(files[name]) {
			return fmt.Errorf("staged native Pi input %s is corrupt", name)
		}
	}
	return nil
}

// Crashed preparations are disposable; settled hash directories are retained.
// Cleanup is best effort and runs only before uncached preparation, never on
// the same-process warm path.
func reclaimPiInputStages(parent string, now time.Time) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".stage-") {
			continue
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > time.Hour {
			_ = os.RemoveAll(filepath.Join(parent, entry.Name()))
		}
	}
}
