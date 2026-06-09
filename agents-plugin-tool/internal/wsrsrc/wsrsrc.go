// Package wsrsrc implements the call-time rsrc loader substrate for the
// ws playbook surface. It reads plain-text playbook files from a filesystem
// root (NOT go:embed), resolves auto-includes, applies declared-variable
// substitution, and enforces manifest schema-version compatibility.
//
// Phase 1 ships no MCP tool. The loader API is consumed by the MCP layer in
// Phase 2.
package wsrsrc

import "fmt"

// SupportedSchemaVersion is the manifest schema version this binary supports.
// Increment only when the schema SHAPE changes — not on content-only edits.
// Text-only edits must ship without a binary bump.
const SupportedSchemaVersion = 1

// PlaybookMeta holds the parsed frontmatter fields of a playbook file.
type PlaybookMeta struct {
	// Kind is "print" or "render".
	Kind string
	// Delegates indicates whether this playbook delegates to a sub-agent.
	Delegates bool
	// Includes is the ordered list of bare text-dep names to auto-include.
	Includes []string
	// Variables is the ordered list of declared substitution variable names.
	Variables []string
	// Extra holds any additional frontmatter fields not covered above.
	Extra map[string]any
}

// LoadedPlaybook is the result of loading a playbook variant.
type LoadedPlaybook struct {
	// Name is the playbook bare stem.
	Name string
	// Harness is the harness variant that was loaded ("" means base was loaded).
	Harness string
	// Meta holds the parsed frontmatter.
	Meta PlaybookMeta
	// Body is the fully rendered content: post-substitution, with auto-includes appended.
	Body string
}

// Manifest is the on-disk manifest.json structure.
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	Files         map[string]string `json:"files"`
}

// --- typed errors ---

// ErrManifestMissing is returned when manifest.json does not exist at root.
type ErrManifestMissing struct{ Root string }

func (e ErrManifestMissing) Error() string {
	return fmt.Sprintf("rsrc manifest missing at %s/manifest.json", e.Root)
}

// ErrSchemaMismatch is returned when the manifest schema version is not supported.
type ErrSchemaMismatch struct{ Got, Want int }

func (e ErrSchemaMismatch) Error() string {
	return fmt.Sprintf("rsrc manifest schema version %d not supported (want %d)", e.Got, e.Want)
}

// ErrHashMismatch is returned when a loaded file's hash does not match the manifest.
type ErrHashMismatch struct{ RelPath, Got, Want string }

func (e ErrHashMismatch) Error() string {
	return fmt.Sprintf("rsrc file %q hash mismatch: got %s, want %s", e.RelPath, e.Got, e.Want)
}

// ErrFileMissing is returned when a manifest-listed file does not exist on disk.
type ErrFileMissing struct{ RelPath string }

func (e ErrFileMissing) Error() string {
	return fmt.Sprintf("rsrc manifest-listed file missing: %q", e.RelPath)
}
