package wsagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// claude_hosted_variant.go builds the plugin directory a hosted Claude lead
// loads when its ws-mcp is attached over HTTP (mcp_session.go): the hosted
// bundle with its MCP server entry replaced by the daemon's HTTP route.
// The entry keeps the bundle's server name, so the server stays
// plugin:<package>:<package>, the name the bundle's Claude mod resolves
// and reconnects, and the lead attaches exactly one ws server.
//
// Claude Code resolves a plugin's manifest agents and hook modules through
// their real paths and refuses any outside the plugin directory, so the
// variant holds copies, not links, of the entries it loads. Only bin is a
// link: the mod runs the launcher there, and the launcher resolves its own
// real path, so its runtime and state stay the bundle's. Dot entries are
// left out (the manifest is generated; .mcp.json would define the server
// again; the rest belong to other hosts or the launcher), and so are the
// entries no Claude session reads.

// HostedVariantsDir is the directory, under the daemon's state directory,
// where the HTTP variants of hosted bundles are kept.
const HostedVariantsDir = "hosted-bundles"

var (
	claudeVariantLinked  = []string{"bin"}
	claudeVariantSkipped = []string{"native-pi", "tests"}
)

// claudeHTTPVariant is the HTTP variant of the bundle at root under dir,
// built on first use. A variant is named by its content, so one built from
// an equal bundle is reused and a changed bundle gets a new one; a variant
// a running lead may still read is never removed.
func claudeHTTPVariant(root, dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		return "", err
	}
	manifest, name, err := claudeHTTPManifest(raw)
	if err != nil {
		return "", fmt.Errorf("bundle manifest %s: %w", root, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	fmt.Fprintf(sum, "root %s\nmanifest %s\n", root, manifest)
	var copied, linked []string
	for _, e := range entries {
		switch n := e.Name(); {
		case strings.HasPrefix(n, ".") || slices.Contains(claudeVariantSkipped, n):
		case slices.Contains(claudeVariantLinked, n):
			linked = append(linked, n)
			fmt.Fprintf(sum, "link %s\n", n)
		default:
			copied = append(copied, n)
			if err := stampTree(sum, root, n); err != nil {
				return "", err
			}
		}
	}
	target := filepath.Join(dir, name+"-"+hex.EncodeToString(sum.Sum(nil))[:16])
	if _, err := os.Stat(filepath.Join(target, ".claude-plugin", "plugin.json")); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(dir, ".stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	for _, n := range copied {
		if err := copyTree(filepath.Join(root, n), filepath.Join(stage, n)); err != nil {
			return "", err
		}
	}
	for _, n := range linked {
		if err := os.Symlink(filepath.Join(root, n), filepath.Join(stage, n)); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Join(stage, ".claude-plugin"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, ".claude-plugin", "plugin.json"), manifest, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(stage, target); err != nil {
		// A concurrent start built the same variant first.
		if _, statErr := os.Stat(filepath.Join(target, ".claude-plugin", "plugin.json")); statErr == nil {
			return target, nil
		}
		return "", err
	}
	return target, nil
}

// claudeHTTPManifest is the bundle manifest raw with its MCP servers
// replaced by one HTTP entry, named after the package as the bundle's own
// server is, whose URL and token Claude Code expands from the lead's
// environment. Every other manifest member is kept as it is.
func claudeHTTPManifest(raw []byte) ([]byte, string, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, "", err
	}
	var name string
	if err := json.Unmarshal(members["name"], &name); err != nil || name == "" || strings.ContainsAny(name, `/\.`) {
		return nil, "", errors.New("no usable package name")
	}
	entry := claudeHTTPServer("${" + EnvMCPSessionURL + "}")
	servers, err := json.Marshal(map[string]any{name: entry})
	if err != nil {
		return nil, "", err
	}
	members["mcpServers"] = servers
	out, err := json.MarshalIndent(members, "", "  ")
	return append(out, '\n'), name, err
}

// stampTree writes rel's files under root (path, mode, size, modification
// time) to w, following links as copyTree does.
func stampTree(w io.Writer, root, rel string) error {
	return walkFollow(filepath.Join(root, rel), func(path string, info fs.FileInfo) error {
		sub, _ := filepath.Rel(root, path)
		fmt.Fprintf(w, "file %s %v %d %d\n", filepath.ToSlash(sub), info.Mode(), info.Size(), info.ModTime().UnixNano())
		return nil
	})
}

// copyTree copies src to dst, following links: a copy must hold no path
// that resolves outside it.
func copyTree(src, dst string) error {
	return walkFollow(src, func(path string, info fs.FileInfo) error {
		sub, _ := filepath.Rel(src, path)
		out := filepath.Join(dst, sub)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, in); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
}

// walkFollow calls fn for path and, for a directory, everything under it
// in lexical order, following links; a link cycle ends at maxWalkDepth.
func walkFollow(path string, fn func(string, fs.FileInfo) error) error {
	return walkDepth(path, fn, 0)
}

const maxWalkDepth = 32

func walkDepth(path string, fn func(string, fs.FileInfo) error, depth int) error {
	if depth > maxWalkDepth {
		return fmt.Errorf("%s: nested too deep", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := fn(path, info); err != nil || !info.IsDir() {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := walkDepth(filepath.Join(path, e.Name()), fn, depth+1); err != nil {
			return err
		}
	}
	return nil
}
