package wspackage

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// A hosted launcher needs only these package-relative inputs, not a plugin
// installation. Settled stages outlive the daemon and are never overwritten.
var hostedFiles = []string{
	"bin/ws-mcp-launcher.py", "runtime.json", "config-manifest.json",
	"dashboard-config-manifest.json", ".claude-plugin/plugin.json",
}

var hostedStages = struct {
	sync.Mutex
	roots map[string]string
}{roots: make(map[string]string)}

// StageHosted publishes the build-owned launcher and four manifests under the
// daemon state directory. Successful same-process calls do zero filesystem I/O.
// The explicit dev checkout stays live: rehash its five inputs at each launch
// so staged manifests cannot lag behind the live package environment.
func StageHosted(name, stateDir string) (string, error) {
	return stageHostedPackage(name, stateDir, os.Getenv(EnvDevCheckout), Open)
}

func stageHostedPackage(name, stateDir, checkout string, open func(string) (fs.FS, error)) (string, error) {
	if checkout != "" {
		source, err := open(name)
		if err != nil {
			return "", err
		}
		return stageHosted(name, stateDir, source, os.Rename)
	}
	key := stateDir + "\x00" + name
	hostedStages.Lock()
	defer hostedStages.Unlock()
	if root := hostedStages.roots[key]; root != "" {
		return root, nil
	}
	source, err := open(name)
	if err != nil {
		return "", err
	}
	root, err := stageHosted(name, stateDir, source, os.Rename)
	if err == nil {
		hostedStages.roots[key] = root
	}
	return root, err
}

func stageHosted(name, stateDir string, source fs.FS, rename func(string, string) error) (string, error) {
	files := make(map[string][]byte, len(hostedFiles))
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00", name)
	for _, path := range hostedFiles {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return "", fmt.Errorf("hosted package %s input %s: %w", name, path, err)
		}
		files[path] = data
		fmt.Fprintf(hash, "%s\x00%d\x00", path, len(data))
		hash.Write(data)
	}
	target := filepath.Join(stateDir, "hosted-packages", name, fmt.Sprintf("%x", hash.Sum(nil)))
	if err := validateHostedStage(target, files); err == nil {
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	for _, path := range hostedFiles {
		dest := filepath.Join(tmp, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(dest, files[path], 0o600); err != nil {
			return "", err
		}
	}
	if err := rename(tmp, target); err != nil {
		if verifyErr := validateHostedStage(target, files); verifyErr != nil {
			return "", fmt.Errorf("publish hosted package: %w (target validation: %v)", err, verifyErr)
		}
	}
	return target, nil
}

func validateHostedStage(root string, files map[string][]byte) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid hosted package directory: %s", root)
	}
	count := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == "." || rel == "bin" || rel == ".claude-plugin" {
				return nil
			}
			return fmt.Errorf("unexpected hosted package directory: %s", rel)
		}
		want, ok := files[rel]
		if !ok || !entry.Type().IsRegular() {
			return fmt.Errorf("invalid hosted package file: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(want) {
			return fmt.Errorf("corrupt hosted package file: %s", rel)
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(files) {
		return fmt.Errorf("incomplete hosted package: %s", root)
	}
	return nil
}
