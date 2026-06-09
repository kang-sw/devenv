package wsrsrc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// writeFile is a concise helper: write content to root/relPath, creating parent dirs.
func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// buildMinimalTree builds a minimal valid rsrc tree and writes its manifest.
// Returns (root, name) for the single sample playbook.
func buildMinimalTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, root, "sample/sample.md", "---\nkind: print\ndelegates: false\nvariables:\n  - Name\n---\nHello {{.Name}}\n")
	writeFile(t, root, "sample/sample.codex.md", "---\nkind: print\ndelegates: false\nvariables:\n  - Name\n---\nHello {{.Name}} (codex)\n")
	writeFile(t, root, "greetings.md", "---\nkind: text\n---\nGreetings text.\n")

	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

// buildTreeWithIncludes builds a tree where the playbook uses an include.
func buildTreeWithIncludes(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, root, "pb/pb.md", "---\nkind: print\ndelegates: false\nincludes:\n  - shared\n---\nBody text.\n")
	writeFile(t, root, "shared.md", "---\nkind: text\n---\nShared content.\n")

	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

func TestFrontmatterScalar(t *testing.T) {
	fm, body := parseFrontmatter("---\nkind: print\ndelegates: false\n---\nbody\n")
	if fm["kind"] != "print" {
		t.Errorf("kind = %v, want print", fm["kind"])
	}
	if fm["delegates"] != "false" {
		t.Errorf("delegates = %v, want false", fm["delegates"])
	}
	if !strings.Contains(body, "body") {
		t.Errorf("body = %q, want 'body'", body)
	}
}

func TestFrontmatterList(t *testing.T) {
	text := "---\nvariables:\n  - Foo\n  - Bar\n---\nbody\n"
	fm, _ := parseFrontmatter(text)
	list, ok := fm["variables"].([]string)
	if !ok {
		t.Fatalf("variables type = %T, want []string", fm["variables"])
	}
	if len(list) != 2 || list[0] != "Foo" || list[1] != "Bar" {
		t.Errorf("variables = %v, want [Foo Bar]", list)
	}
}

func TestFrontmatterSubMap(t *testing.T) {
	text := "---\nconfig:\n  timeout: 30\n  retries: 3\n---\nbody\n"
	fm, _ := parseFrontmatter(text)
	m, ok := fm["config"].(map[string]string)
	if !ok {
		t.Fatalf("config type = %T, want map[string]string", fm["config"])
	}
	if m["timeout"] != "30" || m["retries"] != "3" {
		t.Errorf("config = %v, want {timeout:30 retries:3}", m)
	}
}

func TestFrontmatterNoBlock(t *testing.T) {
	text := "no frontmatter here\n"
	fm, body := parseFrontmatter(text)
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	// body is the normalized (LF-only) text — for pure-LF input it equals the original.
	if body != text {
		t.Errorf("body = %q, want normalized text %q", body, text)
	}
}

func TestFrontmatterNoBlockCRLFNormalized(t *testing.T) {
	// A CRLF input with no frontmatter block must return LF-only body.
	crlfText := "no frontmatter\r\nhere\r\n"
	_, body := parseFrontmatter(crlfText)
	if strings.Contains(body, "\r") {
		t.Errorf("body contains \\r — CRLF not normalized in no-frontmatter early-return path: %q", body)
	}
	wantBody := "no frontmatter\nhere\n"
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestFrontmatterNormalizesLineEndings(t *testing.T) {
	text := "---\r\nkind: render\r\n---\r\nbody\r\n"
	fm, body := parseFrontmatter(text)
	if fm["kind"] != "render" {
		t.Errorf("kind = %v, want render (CRLF not normalized?)", fm["kind"])
	}
	if !strings.Contains(body, "body") {
		t.Errorf("body = %q, want body text", body)
	}
}

func TestFrontmatterInlineComment(t *testing.T) {
	text := "---\nkind: print # a comment\n---\nbody\n"
	fm, _ := parseFrontmatter(text)
	if fm["kind"] != "print" {
		t.Errorf("kind = %v, want print (inline comment not stripped?)", fm["kind"])
	}
}

func TestFrontmatterQuotedValue(t *testing.T) {
	text := "---\nkind: \"print\"\n---\nbody\n"
	fm, _ := parseFrontmatter(text)
	if fm["kind"] != "print" {
		t.Errorf("kind = %v, want print (quotes not stripped?)", fm["kind"])
	}
}

// ---------------------------------------------------------------------------
// isBareStem
// ---------------------------------------------------------------------------

func TestIsBareStem(t *testing.T) {
	good := []string{"foo", "sample-playbook", "a.b", "foo.codex"}
	for _, s := range good {
		if !isBareStem(s) {
			t.Errorf("isBareStem(%q) = false, want true", s)
		}
	}
	bad := []string{"", ".", "..", "../secret", "dir/file", "dir\\file", "a..b"}
	for _, s := range bad {
		if isBareStem(s) {
			t.Errorf("isBareStem(%q) = true, want false", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Manifest read / write / generate
// ---------------------------------------------------------------------------

func TestManifestRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")

	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if m.SchemaVersion != SupportedSchemaVersion {
		t.Errorf("schema_version = %d, want %d", m.SchemaVersion, SupportedSchemaVersion)
	}
	if _, ok := m.Files["pb/pb.md"]; !ok {
		t.Errorf("expected pb/pb.md in manifest files, got %v", m.Files)
	}
	// manifest.json itself must not be in files
	if _, ok := m.Files["manifest.json"]; ok {
		t.Errorf("manifest.json must not be listed in its own files")
	}

	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	m2, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest after write: %v", err)
	}
	if m2.SchemaVersion != m.SchemaVersion {
		t.Errorf("round-trip schema_version = %d, want %d", m2.SchemaVersion, m.SchemaVersion)
	}
	if m2.Files["pb/pb.md"] != m.Files["pb/pb.md"] {
		t.Errorf("round-trip hash mismatch for pb/pb.md")
	}
}

func TestManifestMissing(t *testing.T) {
	root := t.TempDir()
	_, err := ReadManifest(root)
	var target ErrManifestMissing
	if !asError(err, &target) {
		t.Errorf("expected ErrManifestMissing, got %T: %v", err, err)
	}
}

func TestManifestSchemaMismatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "manifest.json", `{"schema_version":999,"files":{}}`)

	_, err := ReadManifest(root)
	var target ErrSchemaMismatch
	if !asError(err, &target) {
		t.Errorf("expected ErrSchemaMismatch, got %T: %v", err, err)
	}
	if target.Got != 999 {
		t.Errorf("ErrSchemaMismatch.Got = %d, want 999", target.Got)
	}
}

func TestManifestFileHashStability(t *testing.T) {
	root := t.TempDir()
	// CRLF and LF versions of the same content should hash identically.
	lfContent := "---\nkind: print\n---\nbody\n"
	crlfContent := strings.ReplaceAll(lfContent, "\n", "\r\n")
	writeFile(t, root, "a.md", lfContent)
	m, _ := GenerateManifest(root)
	hashLF := m.Files["a.md"]

	writeFile(t, root, "a.md", crlfContent)
	m2, _ := GenerateManifest(root)
	hashCRLF := m2.Files["a.md"]

	if hashLF != hashCRLF {
		t.Errorf("hash differs between LF (%s) and CRLF (%s): line-ending normalization broken", hashLF, hashCRLF)
	}
}

// ---------------------------------------------------------------------------
// Loader: base and harness variant
// ---------------------------------------------------------------------------

func TestLoaderBase(t *testing.T) {
	root := buildMinimalTree(t)
	pb, err := Load(root, "sample", "", map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pb.Name != "sample" {
		t.Errorf("Name = %q, want sample", pb.Name)
	}
	if pb.Harness != "" {
		t.Errorf("Harness = %q, want empty (base)", pb.Harness)
	}
	if pb.Meta.Kind != "print" {
		t.Errorf("Kind = %q, want print", pb.Meta.Kind)
	}
	if !strings.Contains(pb.Body, "Hello World") {
		t.Errorf("Body = %q, want 'Hello World'", pb.Body)
	}
}

func TestLoaderHarnessVariant(t *testing.T) {
	root := buildMinimalTree(t)
	pb, err := Load(root, "sample", "codex", map[string]string{"Name": "Agent"})
	if err != nil {
		t.Fatalf("Load codex: %v", err)
	}
	if pb.Harness != "codex" {
		t.Errorf("Harness = %q, want codex", pb.Harness)
	}
	if !strings.Contains(pb.Body, "codex") {
		t.Errorf("Body = %q: expected codex variant content", pb.Body)
	}
}

func TestLoaderHarnessFallsBackToBase(t *testing.T) {
	root := buildMinimalTree(t)
	// "unknown" harness overlay does not exist; should fall back to base.
	pb, err := Load(root, "sample", "unknown", map[string]string{"Name": "X"})
	if err != nil {
		t.Fatalf("Load with unknown harness: %v", err)
	}
	if pb.Harness != "" {
		t.Errorf("Harness = %q, want empty (fell back to base)", pb.Harness)
	}
}

func TestLoaderNilVarsNoSubstitution(t *testing.T) {
	root := buildMinimalTree(t)
	// nil vars → no substitution; placeholder is preserved in body.
	pb, err := Load(root, "sample", "", nil)
	if err != nil {
		t.Fatalf("Load with nil vars: %v", err)
	}
	if !strings.Contains(pb.Body, "{{.Name}}") {
		t.Errorf("Body = %q: expected placeholder preserved when vars=nil", pb.Body)
	}
}

// ---------------------------------------------------------------------------
// Variable substitution
// ---------------------------------------------------------------------------

func TestSubstituteProvided(t *testing.T) {
	result, err := substituteVars("Hello {{.Name}}!", []string{"Name"}, map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("substituteVars: %v", err)
	}
	if result != "Hello World!" {
		t.Errorf("result = %q, want 'Hello World!'", result)
	}
}

func TestSubstituteUndeclaredInContext(t *testing.T) {
	// vars has a key not declared in the playbook → ErrUndeclaredVar
	_, err := substituteVars("body", []string{"Declared"}, map[string]string{"Undeclared": "x"})
	var target ErrUndeclaredVar
	if !asError(err, &target) {
		t.Fatalf("expected ErrUndeclaredVar, got %T: %v", err, err)
	}
	if target.Name != "Undeclared" {
		t.Errorf("ErrUndeclaredVar.Name = %q, want Undeclared", target.Name)
	}
}

func TestSubstituteDeclaredButUnprovidedAndUsed(t *testing.T) {
	// declared variable appears in body but is not in vars → ErrUnprovidedVar
	_, err := substituteVars("Hello {{.Name}}!", []string{"Name"}, map[string]string{})
	var target ErrUnprovidedVar
	if !asError(err, &target) {
		t.Fatalf("expected ErrUnprovidedVar, got %T: %v", err, err)
	}
	if target.Name != "Name" {
		t.Errorf("ErrUnprovidedVar.Name = %q, want Name", target.Name)
	}
}

func TestSubstituteDeclaredButUnprovidedAndUnused(t *testing.T) {
	// declared variable not in body and not in vars → no error
	result, err := substituteVars("no placeholder here", []string{"Name"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "no placeholder here" {
		t.Errorf("result = %q, want unchanged", result)
	}
}

func TestSubstituteUndeclaredInBody(t *testing.T) {
	// body uses {{.Ghost}} but it's not in declared → ErrUndeclaredVar after substituting declared ones
	_, err := substituteVars("{{.Declared}} {{.Ghost}}", []string{"Declared"}, map[string]string{"Declared": "val"})
	var target ErrUndeclaredVar
	if !asError(err, &target) {
		t.Fatalf("expected ErrUndeclaredVar, got %T: %v", err, err)
	}
	if target.Name != "Ghost" {
		t.Errorf("ErrUndeclaredVar.Name = %q, want Ghost", target.Name)
	}
}

// TestSubstituteNoDoubleExpansion verifies that a replacement value containing
// another placeholder literal is not re-expanded (single-pass semantics).
func TestSubstituteNoDoubleExpansion(t *testing.T) {
	// A's value contains "{{.B}}" as a literal. B's placeholder also appears in body.
	// Expected: the {{.B}} from A's value stays as a literal; the {{.B}} originally
	// in the body is replaced with "World".
	result, err := substituteVars(
		"start {{.A}} end {{.B}}",
		[]string{"A", "B"},
		map[string]string{"A": "{{.B}}", "B": "World"},
	)
	if err != nil {
		t.Fatalf("substituteVars: %v", err)
	}
	const want = "start {{.B}} end World"
	if result != want {
		t.Errorf("result = %q, want %q (A's value must not be re-expanded)", result, want)
	}
}

// ---------------------------------------------------------------------------
// Auto-include resolver
// ---------------------------------------------------------------------------

func TestAutoIncludeConcatenated(t *testing.T) {
	root := buildTreeWithIncludes(t)
	pb, err := Load(root, "pb", "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(pb.Body, "Body text") {
		t.Errorf("Body = %q: missing main body text", pb.Body)
	}
	if !strings.Contains(pb.Body, "Shared content") {
		t.Errorf("Body = %q: missing included text", pb.Body)
	}
}

func TestAutoIncludeDangling(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\nincludes:\n  - missing\n---\nbody\n")

	m, _ := GenerateManifest(root)
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	_, err := Load(root, "pb", "", nil)
	if err == nil {
		t.Fatal("expected error for dangling include, got nil")
	}
	// "missing.md" is the filename of the dangling include; asserting this rather
	// than "missing" alone avoids false-positive matches on other error phrases.
	if !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("error = %q, want mention of filename 'missing.md'", err)
	}
}

// TestAutoIncludeCRLFNoFrontmatter verifies that an include file with CRLF line
// endings and no frontmatter block delivers LF-only body text after Load
// (parseFrontmatter's early-return must yield the normalized, not original, text).
func TestAutoIncludeCRLFNoFrontmatter(t *testing.T) {
	root := t.TempDir()
	// Playbook uses a CRLF include that has no frontmatter block.
	writeFile(t, root, "pb/pb.md", "---\nkind: print\nincludes:\n  - crlf-dep\n---\nbody\n")
	// Write the include with CRLF and no frontmatter.
	crlfContent := "line one\r\nline two\r\n"
	writeFile(t, root, "crlf-dep.md", crlfContent)

	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	pb, err := Load(root, "pb", "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(pb.Body, "\r") {
		t.Errorf("Body contains \\r after Load — CRLF normalization not applied in no-frontmatter path:\n%q", pb.Body)
	}
	if !strings.Contains(pb.Body, "line one") || !strings.Contains(pb.Body, "line two") {
		t.Errorf("Body = %q: expected include lines to be present", pb.Body)
	}
}

// Includes are flat (no nesting) — cycle detection is not needed by design.
// If nested includes were supported, a cycle test would live here.

// ---------------------------------------------------------------------------
// Manifest integrity on load
// ---------------------------------------------------------------------------

func TestLoaderHashMismatch(t *testing.T) {
	root := buildMinimalTree(t)
	// Corrupt a file after manifest generation.
	writeFile(t, root, "sample/sample.md", "---\nkind: print\ndelegates: false\nvariables:\n  - Name\n---\nCORRUPTED\n")

	_, err := Load(root, "sample", "", nil)
	var target ErrHashMismatch
	if !asError(err, &target) {
		t.Errorf("expected ErrHashMismatch, got %T: %v", err, err)
	}
}

func TestLoaderManifestMissing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	// No manifest.json written.

	_, err := Load(root, "pb", "", nil)
	var target ErrManifestMissing
	if !asError(err, &target) {
		t.Errorf("expected ErrManifestMissing, got %T: %v", err, err)
	}
}

func TestLoaderSchemaMismatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	writeFile(t, root, "manifest.json", `{"schema_version":999,"files":{"pb/pb.md":"deadbeef"}}`)

	_, err := Load(root, "pb", "", nil)
	var target ErrSchemaMismatch
	if !asError(err, &target) {
		t.Errorf("expected ErrSchemaMismatch, got %T: %v", err, err)
	}
}

func TestLoaderFileMissingFromManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	// Write manifest that doesn't list pb/pb.md.
	writeFile(t, root, "manifest.json", `{"schema_version":1,"files":{}}`)

	_, err := Load(root, "pb", "", nil)
	var target ErrFileMissing
	if !asError(err, &target) {
		t.Errorf("expected ErrFileMissing, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Validate: good tree
// ---------------------------------------------------------------------------

func TestValidateGoodTree(t *testing.T) {
	root := buildMinimalTree(t)
	if err := Validate(root); err != nil {
		t.Errorf("Validate on valid tree: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Validate: failure modes
// ---------------------------------------------------------------------------

func TestValidateMissingManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	// No manifest.

	err := Validate(root)
	var target ErrManifestMissing
	if !asError(err, &target) {
		t.Errorf("expected ErrManifestMissing, got %T: %v", err, err)
	}
}

func TestValidateSchemaMismatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "manifest.json", `{"schema_version":999,"files":{}}`)

	err := Validate(root)
	var target ErrSchemaMismatch
	if !asError(err, &target) {
		t.Errorf("expected ErrSchemaMismatch, got %T: %v", err, err)
	}
}

func TestValidateMissingRequiredVariant(t *testing.T) {
	root := t.TempDir()
	// Subdirectory exists but has no base .md file.
	if err := os.MkdirAll(filepath.Join(root, "pb"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "manifest.json", `{"schema_version":1,"files":{}}`)

	err := Validate(root)
	if err == nil {
		t.Fatal("expected error for missing base variant, got nil")
	}
	if !strings.Contains(err.Error(), "pb") {
		t.Errorf("error = %q, want mention of playbook name 'pb'", err)
	}
}

func TestValidateUndeclaredVariable(t *testing.T) {
	root := t.TempDir()
	// Body uses {{.Secret}} but variables list is empty.
	writeFile(t, root, "pb/pb.md", "---\nkind: print\nvariables:\n---\nUse {{.Secret}} here.\n")
	m, _ := GenerateManifest(root)
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	err := Validate(root)
	if err == nil {
		t.Fatal("expected error for undeclared variable, got nil")
	}
	if !strings.Contains(err.Error(), "Secret") {
		t.Errorf("error = %q, want mention of 'Secret'", err)
	}
}

func TestValidateDanglingInclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\nincludes:\n  - ghost\n---\nbody\n")
	m, _ := GenerateManifest(root)
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	err := Validate(root)
	if err == nil {
		t.Fatal("expected error for dangling include, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error = %q, want mention of 'ghost'", err)
	}
}

func TestValidateManifestHashDrift(t *testing.T) {
	root := buildMinimalTree(t)
	// Modify file content after manifest was written.
	writeFile(t, root, "sample/sample.md", "---\nkind: print\ndelegates: false\nvariables:\n  - Name\n---\nDRIFTED\n")

	err := Validate(root)
	var target ErrHashMismatch
	if !asError(err, &target) {
		t.Errorf("expected ErrHashMismatch, got %T: %v", err, err)
	}
}

func TestValidateManifestListedFileMissing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	m, _ := GenerateManifest(root)
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	// Remove the file after manifest was generated.
	if err := os.Remove(filepath.Join(root, "pb/pb.md")); err != nil {
		t.Fatal(err)
	}

	err := Validate(root)
	var target ErrFileMissing
	if !asError(err, &target) {
		t.Errorf("expected ErrFileMissing, got %T: %v", err, err)
	}
}

func TestValidateUnlistedFileInTree(t *testing.T) {
	root := buildMinimalTree(t)
	// Add a file that was NOT in the manifest.
	writeFile(t, root, "unlisted.md", "# unlisted\n")

	err := Validate(root)
	if err == nil {
		t.Fatal("expected error for unlisted file, got nil")
	}
	if !strings.Contains(err.Error(), "unlisted.md") {
		t.Errorf("error = %q, want mention of 'unlisted.md'", err)
	}
}

// ---------------------------------------------------------------------------
// Validate: real committed agents-plugin/rsrc tree (CI gate)
// ---------------------------------------------------------------------------

// TestValidateRealTree validates the committed agents-plugin/rsrc/ tree.
// This is the CI tree-sync gate: if the tree content drifts from manifest.json
// (or manifest.json is missing), this test fails.
func TestValidateRealTree(t *testing.T) {
	root := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	if err := Validate(root); err != nil {
		t.Fatalf("Validate(agents-plugin/rsrc): %v", err)
	}
}

// TestGenerateRealManifest regenerates agents-plugin/rsrc/manifest.json from
// the current tree. Run with WSRSRC_REGEN=1 to update after editing rsrc files.
//
//	cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v
func TestGenerateRealManifest(t *testing.T) {
	if os.Getenv("WSRSRC_REGEN") == "" {
		t.Skip("set WSRSRC_REGEN=1 to regenerate agents-plugin/rsrc/manifest.json")
	}
	root := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	t.Logf("regenerated %s/manifest.json with %d files", root, len(m.Files))
}

// ResolveRoot requires WS_RSRC_ROOT; exercise the path-returns-env-value branch.
func TestResolveRootEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv(envRsrcRoot, root)
	got, err := ResolveRoot()
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got != root {
		t.Errorf("ResolveRoot = %q, want %q", got, root)
	}
}

func TestResolveRootMissingEnv(t *testing.T) {
	t.Setenv(envRsrcRoot, "")
	_, err := ResolveRoot()
	if err == nil {
		t.Fatal("expected error when WS_RSRC_ROOT is empty, got nil")
	}
}

// ---------------------------------------------------------------------------
// errors.As helper
// ---------------------------------------------------------------------------

// asError reports whether err or any error in its chain matches type T,
// using errors.As so that wrapped typed errors are also matched.
func asError[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}
