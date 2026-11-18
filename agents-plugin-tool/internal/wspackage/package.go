// Package wspackage supplies the build-owned workflow package filesystems.
package wspackage

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// EnvDevCheckout explicitly selects a source checkout for daemon development.
// It is read only from the daemon process, never from a connection environment.
const EnvDevCheckout = "WS_RSRC_DEV_CHECKOUT"

//go:embed all:generated
var assets embed.FS

var logged sync.Map

func packageDir(name string) (string, error) {
	return packageDirIn(assets, name)
}

func packageDirIn(source fs.FS, name string) (string, error) {
	// The generator owns this small index as well as the full asset set. Its
	// retained placeholder lets a bare build resolve the explicit dev override.
	data, err := fs.ReadFile(source, "generated/PLACEHOLDER")
	if err != nil {
		return "", fmt.Errorf("embedded workflow package index unavailable; regenerate package assets and rebuild: %w", err)
	}
	var directories map[string]string
	if err := json.Unmarshal(data, &directories); err != nil {
		return "", fmt.Errorf("invalid embedded workflow package index: %w", err)
	}
	dir, ok := directories[name]
	if !ok {
		return "", fmt.Errorf("unknown workflow package %q", name)
	}
	if name == "." || !fs.ValidPath(name) || dir == "." || !fs.ValidPath(dir) {
		return "", fmt.Errorf("invalid workflow package index entry %q", name)
	}
	return dir, nil
}

// Open returns a package-root FS, using only the explicit process dev override
// when set. No executable-relative or caller resource-root fallback is used.
func Open(name string) (fs.FS, error) {
	dir, err := packageDir(name)
	if err != nil {
		return nil, err
	}
	if checkout := os.Getenv(EnvDevCheckout); checkout != "" {
		root := filepath.Join(checkout, dir)
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("%s dev override %q: %w", EnvDevCheckout, root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s dev override is not a directory: %s", EnvDevCheckout, root)
		}
		if _, loaded := logged.LoadOrStore(root, true); !loaded {
			log.Printf("workflow package %s: explicit %s dev override: %s", name, EnvDevCheckout, root)
		}
		return os.DirFS(root), nil
	}
	return Embedded(name)
}

// Embedded returns build-owned files, bypassing the development override.
func Embedded(name string) (fs.FS, error) {
	return embedded(assets, name)
}

func embedded(source fs.FS, name string) (fs.FS, error) {
	if _, err := packageDirIn(source, name); err != nil {
		return nil, err
	}
	if _, err := fs.Stat(source, "generated/inventory.json"); err != nil {
		return nil, fmt.Errorf("embedded workflow packages missing: regenerate package assets before compiling, or set %s to a source checkout for development: %w", EnvDevCheckout, err)
	}
	root, err := fs.Sub(source, "generated/"+name)
	if err == nil {
		_, err = fs.Stat(root, "runtime.json")
	}
	if err != nil {
		return nil, fmt.Errorf("embedded workflow package %s incomplete; regenerate package assets and rebuild: %w", name, err)
	}
	return root, nil
}
