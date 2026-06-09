package wsrsrc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const envRsrcRoot = "WS_RSRC_ROOT"

// ResolveRoot returns the rsrc tree root directory.
//
// Resolution order:
//  1. WS_RSRC_ROOT environment variable, when set and non-empty.
//  2. Plugin-path default (Phase-2 stub — not implemented yet).
//
// Phase-2 stub: the plugin-path default will be derived as:
//
//	filepath.Join(filepath.Dir(os.Executable()), "..", "rsrc")
//
// For now, an empty WS_RSRC_ROOT returns an error directing callers to set it.
// See internal/wsagent/agent.go SelfWorkerStarter.StartAsyncCall for the
// os.Executable() pattern this derivation will mirror.
func ResolveRoot() (string, error) {
	if env := os.Getenv(envRsrcRoot); env != "" {
		return env, nil
	}
	// Phase-2 stub: derive from os.Executable() → filepath.Dir(exe) → ../rsrc
	return "", fmt.Errorf("WS_RSRC_ROOT is not set; plugin-path default requires Phase-2 implementation")
}

// Load loads a playbook by name from root, optionally selecting a harness
// variant. If harness is non-empty and the overlay file exists
// (<root>/<name>/<name>.<harness>.md), it is loaded; otherwise the base file
// (<root>/<name>/<name>.md) is loaded.
//
// Manifest schema-version and per-file hash integrity are verified for every
// file loaded (playbook + includes). Any mismatch returns a typed error with
// no fallback.
//
// vars controls variable substitution:
//   - nil → no substitution; body is returned as-is.
//   - non-nil → substitution is applied. Every key in vars must be declared in
//     the playbook's variables list (ErrUndeclaredVar). Every declared variable
//     that appears as {{.Name}} in the body must be present in vars
//     (ErrUnprovidedVar). Declared variables not appearing in the body are
//     ignored if absent from vars.
func Load(root, name, harness string, vars map[string]string) (LoadedPlaybook, error) {
	if !isBareStem(name) {
		return LoadedPlaybook{}, fmt.Errorf("invalid playbook name %q: must be a bare stem", name)
	}

	// Read and validate manifest (schema-version checked inside ReadManifest).
	manifest, err := ReadManifest(root)
	if err != nil {
		return LoadedPlaybook{}, err
	}

	// Resolve the file path.
	filePath, isOverlay, err := resolvePlaybookPath(root, name, harness)
	if err != nil {
		return LoadedPlaybook{}, err
	}

	// Load and verify file integrity.
	data, err := loadAndVerify(root, filePath, manifest)
	if err != nil {
		return LoadedPlaybook{}, err
	}

	// Parse frontmatter.
	fm, body := parseFrontmatter(string(data))
	meta := metaFromFrontmatter(fm)

	// Resolve includes.
	includeText, err := resolveIncludes(root, meta.Includes, manifest)
	if err != nil {
		return LoadedPlaybook{}, err
	}

	fullBody := strings.TrimSpace(body)
	if includeText != "" {
		fullBody = fullBody + "\n\n" + includeText
	}

	// Variable substitution.
	if vars != nil {
		fullBody, err = substituteVars(fullBody, meta.Variables, vars)
		if err != nil {
			return LoadedPlaybook{}, err
		}
	}

	loadedHarness := ""
	if isOverlay {
		loadedHarness = harness
	}
	return LoadedPlaybook{
		Name:    name,
		Harness: loadedHarness,
		Meta:    meta,
		Body:    fullBody,
	}, nil
}

// resolvePlaybookPath returns (absoluteFilePath, isOverlay, error).
// If harness is set and the overlay file exists, returns the overlay path.
// Otherwise returns the base file path.
func resolvePlaybookPath(root, name, harness string) (string, bool, error) {
	dir := filepath.Join(root, name)
	if harness != "" {
		if !isBareStem(harness) {
			return "", false, fmt.Errorf("invalid harness %q: must be a bare stem", harness)
		}
		overlayPath := filepath.Join(dir, name+"."+harness+".md")
		if _, err := os.Stat(overlayPath); err == nil {
			return overlayPath, true, nil
		}
	}
	basePath := filepath.Join(dir, name+".md")
	return basePath, false, nil
}

// loadAndVerify reads filePath, computes its hash, and verifies it matches the
// manifest entry. Returns ErrHashMismatch on mismatch, ErrFileMissing if the
// file is not listed in the manifest.
func loadAndVerify(root, filePath string, manifest Manifest) ([]byte, error) {
	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve relpath for %q: %w", filePath, err)
	}
	relPath = filepath.ToSlash(relPath)

	expectedHash, ok := manifest.Files[relPath]
	if !ok {
		return nil, ErrFileMissing{RelPath: relPath}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileMissing{RelPath: relPath}
		}
		return nil, fmt.Errorf("read %q: %w", relPath, err)
	}

	if got := hashHex(data); got != expectedHash {
		return nil, ErrHashMismatch{RelPath: relPath, Got: got, Want: expectedHash}
	}
	return data, nil
}

// metaFromFrontmatter converts a raw frontmatter map to a PlaybookMeta.
func metaFromFrontmatter(fm map[string]any) PlaybookMeta {
	if fm == nil {
		return PlaybookMeta{Extra: map[string]any{}}
	}
	meta := PlaybookMeta{Extra: map[string]any{}}
	for k, v := range fm {
		switch k {
		case "kind":
			if s, ok := v.(string); ok {
				meta.Kind = s
			}
		case "delegates":
			if s, ok := v.(string); ok {
				meta.Delegates = s == "true"
			}
		case "includes":
			if list, ok := v.([]string); ok {
				meta.Includes = list
			}
		case "variables":
			if list, ok := v.([]string); ok {
				meta.Variables = list
			}
		default:
			meta.Extra[k] = v
		}
	}
	return meta
}

// resolveIncludes reads each named include from <root>/<name>.md, verifies its
// manifest hash, strips its own frontmatter, and returns the bodies joined by a
// blank line.
//
// Includes are flat: they live at the rsrc root as <name>.md and are NOT
// recursively resolved (no nested includes). This avoids include cycles by
// design.
func resolveIncludes(root string, names []string, manifest Manifest) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	var parts []string
	for _, name := range names {
		if !isBareStem(name) {
			return "", fmt.Errorf("include name %q must be a bare stem", name)
		}
		relPath := name + ".md"
		includePath := filepath.Join(root, relPath)

		data, err := loadAndVerify(root, includePath, manifest)
		if err != nil {
			return "", fmt.Errorf("include %q: %w", name, err)
		}

		_, body := parseFrontmatter(string(data))
		parts = append(parts, strings.TrimSpace(body))
	}
	return strings.Join(parts, "\n\n"), nil
}

// substituteVars replaces {{.Name}} placeholders in body.
//
// Rules (vars non-nil path):
//   - A key in vars that is not in declared → error.
//   - A placeholder {{.Name}} in body where Name is not in declared → error.
//   - A declared variable whose placeholder appears in body but is absent from
//     vars → error (caller must supply all used declared variables).
//   - A declared variable not present in body → no-op regardless of vars.
func substituteVars(body string, declared []string, vars map[string]string) (string, error) {
	declaredSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declaredSet[d] = true
	}

	// Check for undeclared variables in provided vars.
	for k := range vars {
		if !declaredSet[k] {
			return "", fmt.Errorf("variable %q is not declared in playbook (declared: %v)", k, declared)
		}
	}

	result := body

	// Replace all declared-variable placeholders.
	for _, name := range declared {
		placeholder := "{{." + name + "}}"
		if !strings.Contains(result, placeholder) {
			continue // declared but not used in body — fine
		}
		value, provided := vars[name]
		if !provided {
			return "", fmt.Errorf("declared variable %q appears in body but was not provided", name)
		}
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Detect any remaining {{.xxx}} placeholders that were never declared.
	remaining := result
	for {
		idx := strings.Index(remaining, "{{.")
		if idx < 0 {
			break
		}
		end := strings.Index(remaining[idx:], "}}")
		if end < 0 {
			break
		}
		end += idx
		varName := remaining[idx+3 : end]
		return "", fmt.Errorf("variable %q used in body but not declared in playbook", varName)
	}

	return result, nil
}

// isBareStem reports whether spec is a safe bare stem: non-empty, not "." or
// "..", and free of path separators and "..". Copied verbatim from
// internal/wsprompt/prompts.go.
func isBareStem(spec string) bool {
	if spec == "" || spec == "." || spec == ".." {
		return false
	}
	if strings.Contains(spec, "/") || strings.Contains(spec, "\\") || strings.Contains(spec, string(filepath.Separator)) {
		return false
	}
	if strings.Contains(spec, "..") {
		return false
	}
	return true
}
