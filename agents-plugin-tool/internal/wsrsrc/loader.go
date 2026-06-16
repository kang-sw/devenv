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
//  2. Plugin-path default: filepath.Join(filepath.Dir(os.Executable()), "..", "rsrc").
//
// The production plugin layout is:
//
//	<plugin-cache>/bin/ws-mcp   (or ws-mcp.exe on Windows)
//	<plugin-cache>/rsrc/        ← resource tree
//
// Codex cache materialization assumption: Codex is assumed to place plugin files
// under a cache directory with the same bin/rsrc layout. This is an open
// verification item (see Phase-2 brief for 260609-feat-ws-playbook-surface-mvp).
// If the derived path does not contain a valid manifest, callers will see
// ErrManifestMissing at Load time rather than a silent fallback.
//
// See internal/wsagent/agent.go SelfWorkerStarter.StartAsyncCall for the
// os.Executable() pattern this derivation mirrors.
func ResolveRoot() (string, error) {
	if env := os.Getenv(envRsrcRoot); env != "" {
		return env, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("WS_RSRC_ROOT not set and cannot determine plugin path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "..", "rsrc"), nil
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
//   - nil → no substitution; body is returned as-is with placeholders intact.
//   - non-nil → substitution is applied via a single-pass strings.NewReplacer so
//     that a value containing another placeholder literal is never re-expanded.
//     Every key in vars must appear in the playbook's variables list, otherwise
//     an ErrUndeclaredVar is returned. Every declared variable whose placeholder
//     {{.Name}} appears in the body must be present in vars, otherwise an
//     ErrUnprovidedVar is returned. Declared variables not appearing in the body
//     are silently ignored when absent from vars.
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
	if err := checkPlaybookExists(root, name, filePath, manifest); err != nil {
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
	includeText, err := resolveIncludes(root, name, harness, meta.Includes, manifest)
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
	// Flat-playbook fallback: when no subdir playbook exists at
	// <root>/<name>/<name>.md, fall back to a flat root-level file at
	// <root>/<name>.md. This lets a var-free flat dep (e.g. code-reviewer,
	// which doubles as a flat include target for the review partitions) be
	// loaded as a playbook in its own right — the wsflow prompt.render
	// "code-reviewer" stem resolves through here. Subdir playbooks always win
	// (checked first) so existing resolution is unchanged; the fallback only
	// fires when the subdir base file is absent.
	if _, err := os.Stat(basePath); err != nil {
		flatPath := filepath.Join(root, name+".md")
		if _, flatErr := os.Stat(flatPath); flatErr == nil {
			return flatPath, false, nil
		}
	}
	return basePath, false, nil
}

func checkPlaybookExists(root, name, filePath string, manifest Manifest) error {
	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return fmt.Errorf("resolve relpath for %q: %w", filePath, err)
	}
	relPath = filepath.ToSlash(relPath)
	if _, ok := manifest.Files[relPath]; ok {
		return nil
	}
	if _, err := os.Stat(filePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %q: %w", relPath, err)
	}
	return ErrPlaybookNotFound{Name: name}
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
		case "role":
			if s, ok := v.(string); ok {
				meta.Role = s
			}
		case "tier":
			if s, ok := v.(string); ok {
				meta.Tier = s
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

// resolveIncludes reads each named include, verifies its manifest hash, strips
// its own frontmatter, and returns the bodies joined by a blank line.
//
// Include resolution is playbook-local first:
//   - <root>/<playbook>/<include>.<harness>.md
//   - <root>/<playbook>/<include>.md
//   - <root>/<include>.md
//
// Includes are NOT recursively resolved. This avoids include cycles by design.
func resolveIncludes(root, playbookName, harness string, names []string, manifest Manifest) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	var parts []string
	for _, name := range names {
		if !isBareStem(name) {
			return "", fmt.Errorf("include name %q must be a bare stem", name)
		}
		includePath, relPath := resolveIncludePath(root, playbookName, name, harness)

		data, err := loadAndVerify(root, includePath, manifest)
		if err != nil {
			return "", fmt.Errorf("include %q (%s): %w", name, relPath, err)
		}

		_, body := parseFrontmatter(string(data))
		parts = append(parts, strings.TrimSpace(body))
	}
	return strings.Join(parts, "\n\n"), nil
}

func resolveIncludePath(root, playbookName, includeName, harness string) (string, string) {
	if harness != "" {
		localOverlayRel := filepath.ToSlash(filepath.Join(playbookName, includeName+"."+harness+".md"))
		localOverlayPath := filepath.Join(root, filepath.FromSlash(localOverlayRel))
		if _, err := os.Stat(localOverlayPath); err == nil {
			return localOverlayPath, localOverlayRel
		}
	}

	localRel := filepath.ToSlash(filepath.Join(playbookName, includeName+".md"))
	localPath := filepath.Join(root, filepath.FromSlash(localRel))
	if _, err := os.Stat(localPath); err == nil {
		return localPath, localRel
	}

	rootRel := includeName + ".md"
	return filepath.Join(root, rootRel), rootRel
}

// substituteVars replaces {{.Name}} placeholders in body.
//
// Rules (vars non-nil path):
//   - A key in vars that is not in declared → ErrUndeclaredVar.
//   - A placeholder {{.Name}} in the body where Name is not in declared →
//     ErrUndeclaredVar (detected after all declared replacements are applied).
//   - A declared variable whose placeholder appears in the body but is absent
//     from vars → ErrUnprovidedVar.
//   - A declared variable not present in the body → silently ignored.
//
// Replacements are applied in a single pass via strings.NewReplacer so that a
// value containing another placeholder literal (e.g. "{{.Other}}") is never
// re-expanded.
func substituteVars(body string, declared []string, vars map[string]string) (string, error) {
	declaredSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declaredSet[d] = true
	}

	// Check for undeclared variables in provided vars.
	for k := range vars {
		if !declaredSet[k] {
			return "", ErrUndeclaredVar{Name: k}
		}
	}

	// Build old→new pairs for all declared placeholders found in body.
	// Using strings.NewReplacer ensures a single left-to-right pass: replacement
	// values are never re-scanned, so "{{.B}}" in a value for "{{.A}}" is
	// preserved as a literal and not expanded.
	var pairs []string
	for _, name := range declared {
		placeholder := "{{." + name + "}}"
		if !strings.Contains(body, placeholder) {
			continue // declared but not used in body — fine
		}
		value, provided := vars[name]
		if !provided {
			return "", ErrUnprovidedVar{Name: name}
		}
		pairs = append(pairs, placeholder, value)
	}

	var result string
	if len(pairs) > 0 {
		result = strings.NewReplacer(pairs...).Replace(body)
	} else {
		result = body
	}

	// Detect any remaining {{.xxx}} placeholders whose name is not in declared.
	// Declared-var placeholders inserted by a replacement value (e.g. A's value
	// is "{{.B}}") are left in the result by design and are not errors — the
	// guard below passes them through.
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
		if !declaredSet[varName] {
			return "", ErrUndeclaredVar{Name: varName}
		}
		remaining = remaining[end+2:] // advance past this (benign) placeholder
	}

	return result, nil
}

// isBareStem reports whether spec is a safe bare stem: non-empty, not "." or
// "..", and free of path separators and "..".
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
