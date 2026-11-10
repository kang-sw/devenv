package wsdashboard

import (
	"os"
	"path"
	"runtime"
	"strings"
)

// Reading a pasted folder path. The rail's add-workspace form and the
// add-worktree form's custom folder take text a user pasted from a file
// explorer, and the daemon reads it by the rules of the machine it runs on,
// since the browser may be on another one. normalizeWorkspacePath is pure:
// it takes the host as a value, so every host's rules are testable on any.
//
// On every host: surrounding space and quotes are stripped, "~" is the
// daemon user's home, the result is cleaned (no trailing separator, no "."
// or ".." element), and a relative path or a network share (\\host\share)
// is refused.
//
// Under WSL: C:\x, C:/x, /c/x and /mnt/c/x are /mnt/c/x; \\wsl$\<distro>\x
// and \\wsl.localhost\<distro>\x are /x for the daemon's own distribution
// and refused for another.
//
// On native Windows: a drive path stays one (C:\x), and /c/x is C:\x.
//
// On Linux and macOS: only an absolute path and "~".
//
// /c/x is read as a drive only when the host has no top-level folder /c.

// pathHost is the machine whose rules read a pasted path.
type pathHost struct {
	// Windows marks native Windows; else the host is POSIX.
	Windows bool
	// Distro is the WSL distribution's name (WSL_DISTRO_NAME); non-empty on
	// a POSIX host marks WSL.
	Distro string
	// Home is the daemon user's home folder; empty refuses "~".
	Home string
	// IsDir reports an existing folder. It answers whether /<letter> is a
	// top-level folder; nil reads as none.
	IsDir func(path string) bool
}

// daemonPathHost is the host this process runs on.
func daemonPathHost() pathHost {
	h := pathHost{Windows: runtime.GOOS == "windows", IsDir: isDir}
	if runtime.GOOS == "linux" {
		h.Distro = os.Getenv("WSL_DISTRO_NAME")
	}
	if home, err := os.UserHomeDir(); err == nil {
		h.Home = home
	}
	return h
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pathRefusal is why a pasted path was refused; empty when it was not.
type pathRefusal string

const (
	pathEmpty    pathRefusal = "empty"
	pathRelative pathRefusal = "relative" // not an absolute path
	pathShare    pathRefusal = "share"    // a network share
	pathDistro   pathRefusal = "distro"   // another WSL distribution's path
	pathMissing  pathRefusal = "missing"  // no such path
	pathNotDir   pathRefusal = "notdir"   // not a folder
)

// pathInputMax bounds a pasted path.
const pathInputMax = 4 << 10

// normalizeWorkspacePath reads raw as a folder path on host. It does not
// touch the filesystem beyond host.IsDir, and does not check that the
// result exists.
func normalizeWorkspacePath(raw string, host pathHost) (string, pathRefusal) {
	v := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), `"'`))
	if v == "" {
		return "", pathEmpty
	}
	if len(v) > pathInputMax || strings.ContainsAny(v, "\x00\r\n") {
		return "", pathRelative
	}
	wsl := !host.Windows && host.Distro != ""
	winForms := host.Windows || wsl

	// "~" is the daemon user's home; "~\x" too where backslashes separate.
	if v == "~" || strings.HasPrefix(v, "~/") || (winForms && strings.HasPrefix(v, `~\`)) {
		if host.Home == "" {
			return "", pathRelative
		}
		rest := v[1:]
		if winForms {
			rest = strings.ReplaceAll(rest, `\`, "/")
		}
		if host.Windows {
			return normalizeWorkspacePath(host.Home+strings.ReplaceAll(rest, "/", `\`), pathHost{Windows: true, IsDir: host.IsDir})
		}
		return path.Clean(host.Home + rest), ""
	}

	// \\host\share, and //host/share where that is the same thing.
	if strings.HasPrefix(v, `\\`) || (winForms && strings.HasPrefix(v, "//")) {
		parts := strings.Split(strings.TrimLeft(strings.ReplaceAll(v, `\`, "/"), "/"), "/")
		if wsl && (strings.EqualFold(parts[0], "wsl$") || strings.EqualFold(parts[0], "wsl.localhost")) {
			if len(parts) < 2 || !strings.EqualFold(parts[1], host.Distro) {
				return "", pathDistro
			}
			return path.Clean("/" + strings.Join(parts[2:], "/")), ""
		}
		if wsl && strings.HasPrefix(v, "//") {
			return path.Clean(v), "" // a POSIX path with a doubled slash
		}
		return "", pathShare
	}

	// A drive path: C:\x or C:/x (C: alone is the drive's root).
	if len(v) >= 2 && v[1] == ':' && asciiLetter(v[0]) {
		rest := v[2:]
		if rest != "" && rest[0] != '\\' && rest[0] != '/' {
			return "", pathRelative // C:x is relative to the drive's current folder
		}
		if !winForms {
			return "", pathRelative
		}
		return drivePath(v[0], rest, host.Windows), ""
	}

	if strings.HasPrefix(v, "/") {
		// /c/x names drive C unless the host has a top-level folder /c.
		if winForms && len(v) >= 2 && asciiLetter(v[1]) && (len(v) == 2 || v[2] == '/') {
			if host.IsDir == nil || !host.IsDir(v[:2]) {
				return drivePath(v[1], v[2:], host.Windows), ""
			}
		}
		if host.Windows {
			return "", pathRelative // rooted, but on no drive
		}
		clean := path.Clean(v)
		if wsl {
			clean = lowerMntDrive(clean)
		}
		return clean, ""
	}
	return "", pathRelative
}

func asciiLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// drivePath is the path rest (empty, or starting with a separator) on drive
// letter: C:\rest on Windows, /mnt/c/rest under WSL.
func drivePath(letter byte, rest string, windows bool) string {
	clean := path.Clean("/" + strings.ReplaceAll(rest, `\`, "/"))
	if windows {
		return strings.ToUpper(string(letter)) + ":" + strings.ReplaceAll(clean, "/", `\`)
	}
	return path.Join("/mnt", strings.ToLower(string(letter)), clean)
}

// lowerMntDrive lowers the drive letter of a /mnt/<letter> path, as WSL
// mounts drives.
func lowerMntDrive(p string) string {
	const mnt = "/mnt/"
	if strings.HasPrefix(p, mnt) && len(p) > len(mnt) && asciiLetter(p[len(mnt)]) && (len(p) == len(mnt)+1 || p[len(mnt)+1] == '/') {
		return mnt + strings.ToLower(p[len(mnt):len(mnt)+1]) + p[len(mnt)+1:]
	}
	return p
}
