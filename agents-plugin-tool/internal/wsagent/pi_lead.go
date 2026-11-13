package wsagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed pi_lead.mjs
var piLeadAdapter string

//go:embed pi_lead_bridge.mjs
var piLeadBridgeSource string

var piBundledPluginRoot = bundledPluginRoot

// preparePiLeadLaunch is replaced by runner tests whose holders never execute
// their fictitious Pi command. Resource discovery itself stays inside Pi: a
// preflight enumeration would execute extensions twice and lose trust hooks.
var preparePiLeadLaunch = piLeadLaunch

func piLeadLaunch(command string, pre []string, spec StartSpec) (string, []string, error) {
	args := append(piLeadArgs(spec), "--no-extensions")
	entries, bundleErr := piBundledEntries()
	if bundleErr != nil {
		if spec.Lead.WSMCP == nil {
			return "", nil, fmt.Errorf("pi bundled extension unavailable (%v); no ws MCP fallback configured", bundleErr)
		}
		mcp, err := spec.Lead.WSMCP()
		if err != nil {
			return "", nil, fmt.Errorf("pi lead MCP fallback: %w", err)
		}
		servers, err := piMCPServers(StartSpec{Root: spec.Root, Cwd: spec.Cwd, WSMCP: &mcp})
		if err != nil {
			return "", nil, err
		}
		bridge, err := writePiLeadBridge(spec, servers)
		if err != nil {
			return "", nil, err
		}
		entries = []string{"builtin:mcp", bridge}
		if spec.Lead.PluginWarning != nil {
			spec.Lead.PluginWarning(fmt.Sprintf("Bundled Pi extension unavailable (%v); using the ws MCP bridge instead.", bundleErr))
		}
	} else if spec.Lead.PluginWarning != nil {
		spec.Lead.PluginWarning("")
	}
	for _, entry := range entries {
		args = append(args, "--extension", entry)
	}
	if spec.Lead.PluginIsolation == "isolated" {
		return command, append(pre, args...), nil
	}
	cli, sdk, err := piSDKPaths(command, pre)
	if err != nil {
		return "", nil, err
	}
	// Keep the original CLI entry point (setup, RPC lifecycle, session and
	// model selection), but install the resolver adapter before it starts.
	adapter := filepath.Join(piDir(spec.Dir), "ws-lead.mjs")
	if err := writeFileAtomic(adapter, []byte(piLeadAdapter), 0o600); err != nil {
		return "", nil, err
	}
	if len(pre) == 0 {
		command, err = lookPath("node")
		if err != nil {
			return "", nil, fmt.Errorf("pi lead resolver needs node: %w", err)
		}
	}
	if len(pre) > 1 {
		args = append(append([]string(nil), pre[1:]...), args...)
	}
	return command, append([]string{adapter, sdk, cli}, args...), nil
}

// writePiLeadBridge adds the hosted identity to the effective prompt without
// --append-system-prompt, which would suppress the user's APPEND_SYSTEM.md.
// Native MCP does not fill session_key or expose the hosted key to the model.
func writePiLeadBridge(spec StartSpec, servers map[string]any) (string, error) {
	key := spec.Lead.Env[EnvHostedSessionKey]
	if key == "" {
		return "", fmt.Errorf("pi lead MCP fallback requires a hosted session key")
	}
	raw, err := json.Marshal(servers)
	if err != nil {
		return "", err
	}
	keyJSON, _ := json.Marshal(key)
	source := "const servers = " + string(raw) + ";\nconst sessionKey = " + string(keyJSON) + ";\n" + piLeadBridgeSource
	path := filepath.Join(piDir(spec.Dir), piBridgeFile)
	return path, writeFileAtomic(path, []byte(source), 0o600)
}

// piBundledEntries deliberately reads the sibling manifest, not the installed
// Pi package or an entry name baked into the runner.
func piBundledEntries() ([]string, error) {
	root, err := piBundledPluginRoot()
	if err != nil {
		return nil, err
	}
	root = filepath.Join(filepath.Dir(root), "agents-plugin-pi")
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Pi struct {
			Extensions []string `json:"extensions"`
		} `json:"pi"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if len(manifest.Pi.Extensions) == 0 {
		return nil, fmt.Errorf("%s declares no Pi extension", root)
	}
	var entries []string
	for _, entry := range manifest.Pi.Extensions {
		path := filepath.Join(root, filepath.FromSlash(entry))
		rel, err := filepath.Rel(root, path)
		if err != nil || entry == "" || filepath.IsAbs(entry) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !fileExists(path) {
			return nil, fmt.Errorf("bundled Pi extension entry %q is missing or invalid", entry)
		}
		// Pi loads a local package in place and never installs its npm
		// dependencies, so a source entry needs them on disk or the Pi process
		// exits at load. Prefer the self-contained bundle while it is fresh; a
		// missing dependency is a bundleErr so the caller takes the MCP bridge.
		var bundleNote error
		if filepath.ToSlash(rel) == piBundleSource {
			if bundleNote = piBundleFresh(root); bundleNote == nil {
				entries = append(entries, filepath.Join(root, filepath.FromSlash(piBundleFile)))
				continue
			}
		}
		if missing := piMissingLoadDeps(path); len(missing) > 0 {
			err := fmt.Errorf("%s cannot load: npm dependencies %s are not installed in any node_modules above it", path, strings.Join(missing, ", "))
			if bundleNote != nil {
				err = fmt.Errorf("%w, and %v", err, bundleNote)
			}
			return nil, err
		}
		entries = append(entries, path)
	}
	return entries, nil
}

// piSDKPaths resolves the SDK belonging to the actual CLI (including npm
// symlinks and Windows' already-unwrapped cmd shim), never a project dependency.
// A standalone executable without its SDK cannot safely enumerate extensions;
// refuse default mode rather than silently drop them or bypass project trust.
func piSDKPaths(command string, pre []string) (cli, sdk string, err error) {
	cli = command
	if len(pre) > 0 {
		cli = pre[0]
	}
	cli, err = filepath.EvalSymlinks(cli)
	if err == nil {
		cli, err = filepath.Abs(cli)
	}
	if err != nil {
		return "", "", fmt.Errorf("pi lead resolver: %w", err)
	}
	for dir := filepath.Dir(cli); ; dir = filepath.Dir(dir) {
		raw, readErr := os.ReadFile(filepath.Join(dir, "package.json"))
		var pkg struct {
			Name string `json:"name"`
		}
		if readErr == nil && json.Unmarshal(raw, &pkg) == nil && (pkg.Name == "@earendil-works/pi-coding-agent" || pkg.Name == "@mariozechner/pi-coding-agent") {
			// npm releases use dist/bundle/cli.js; its resolver classes
			// are distinct from dist/index.js. Patch the SDK adjacent to
			// the actual CLI, never a different build's module graph.
			sdk = filepath.Join(filepath.Dir(cli), "index.js")
			if fileExists(sdk) {
				return cli, sdk, nil
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return "", "", fmt.Errorf("pi default plugin isolation requires the Pi JavaScript SDK beside its CLI; install Pi with npm or select isolated mode")
}
