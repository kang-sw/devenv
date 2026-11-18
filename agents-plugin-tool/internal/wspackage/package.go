// Package wspackage supplies the build-owned workflow package filesystems.
package wspackage

import (
	"embed"
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
	switch name {
	case "ws":
		return "agents-plugin", nil
	case "wsflow":
		return "agents-plugin-wsflow", nil
	default:
		return "", fmt.Errorf("unknown workflow package %q (expected ws or wsflow)", name)
	}
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
	if _, err := packageDir(name); err != nil {
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
