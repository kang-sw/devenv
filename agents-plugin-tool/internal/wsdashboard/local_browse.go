package wsdashboard

import (
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
)

// LocalBrowseAnchor is the daemon-owned, non-session address for local files
// and terminals. ReplyID's lowercase-word generator can never mint its digit.
const LocalBrowseAnchor = "host0"

// localBrowseViewer admits reads from this machine's own page, including
// browser GETs without Origin, but never a cross-site fetch or foreign Origin.
func localBrowseViewer(r *http.Request) bool {
	return localViewer(r) && !crossSiteFetch(r) && (r.Header.Get("Origin") == "" || sameOrigin(r))
}

type browseRoots struct {
	Host    string   `json:"host"`
	Home    string   `json:"home"`
	Volumes []string `json:"volumes"`
}

// discoverBrowseRoots offers the filesystem root and conventional user mount
// locations, not mount tables (which include pseudo filesystems). readDir is
// the OS seam: enumeration and readability checks run on any test host.
func discoverBrowseRoots(goos, host, home string, readDir func(string) ([]os.DirEntry, error)) browseRoots {
	out := browseRoots{Host: host, Home: home, Volumes: []string{}}
	seen := map[string]bool{}
	add := func(dir string) {
		if _, err := readDir(dir); err == nil && !seen[dir] {
			seen[dir] = true
			out.Volumes = append(out.Volumes, dir)
		}
	}
	children := func(dir string, visit func(string)) {
		entries, err := readDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				visit(path.Join(dir, e.Name()))
			}
		}
	}
	if goos != "windows" {
		add("/")
	}
	switch goos {
	case "darwin":
		children("/Volumes", add)
	case "linux":
		// /mnt includes WSL's /mnt/<letter> drives as well as Linux mounts.
		children("/mnt", add)
		for _, base := range []string{"/media", "/run/media"} {
			children(base, func(user string) { children(user, add) })
		}
	case "windows":
		for drive := 'A'; drive <= 'Z'; drive++ {
			add(string(drive) + `:\`)
		}
	}
	sort.Strings(out.Volumes)
	return out
}

// GET /s/host0/fs/roots is deliberately independent of the session index.
func (d *Daemon) handleBrowseRoots(w http.ResponseWriter, r *http.Request) {
	if !localBrowseViewer(r) {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	discover := d.browseRoots
	if discover == nil {
		discover = func() browseRoots {
			host, _ := os.Hostname()
			return discoverBrowseRoots(runtime.GOOS, host, daemonPathHost().Home, os.ReadDir)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, discover())
}
