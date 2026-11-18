package wsrsrc

import (
	"io/fs"

	"github.com/kang-sw/devenv/internal/wspackage"
)

// PackageName maps the package's existing namespace seam to its build-owned
// inputs. Unset (and custom display namespaces) retain the flagship defaults.
func PackageName(namespace string) string {
	if namespace == "wsflow" {
		return "wsflow"
	}
	return "ws"
}

// PackageResources resolves a resource FS from the daemon build, or the
// explicit daemon-process dev checkout. Caller-owned roots are not consulted.
func PackageResources(namespace string) (fs.FS, error) {
	root, err := wspackage.Open(PackageName(namespace))
	if err != nil {
		return nil, err
	}
	return fs.Sub(root, "rsrc")
}

// PackageSkills resolves the daemon-owned workflow skills/template FS. Harness
// skill discovery itself remains the installed plugin's responsibility.
func PackageSkills(namespace string) (fs.FS, error) {
	root, err := wspackage.Open(PackageName(namespace))
	if err != nil {
		return nil, err
	}
	return fs.Sub(root, "skills")
}
