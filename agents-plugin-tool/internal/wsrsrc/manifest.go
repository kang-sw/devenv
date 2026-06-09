package wsrsrc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadManifest reads and parses root/manifest.json.
// Returns ErrManifestMissing when the file is absent.
// Returns ErrSchemaMismatch when the schema version is not SupportedSchemaVersion.
func ReadManifest(root string) (Manifest, error) {
	path := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrManifestMissing{Root: root}
		}
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if m.SchemaVersion != SupportedSchemaVersion {
		return Manifest{}, ErrSchemaMismatch{Got: m.SchemaVersion, Want: SupportedSchemaVersion}
	}
	return m, nil
}

// GenerateManifest walks the rsrc tree rooted at root and computes per-file
// sha256 hashes. It excludes manifest.json itself. Results are deterministic:
// filepath.Walk yields entries in lexical order, and Go's json.Marshal sorts
// map keys alphabetically.
//
// Call WriteManifest to persist the result to disk.
func GenerateManifest(root string) (Manifest, error) {
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil // exclude manifest itself from hashes
		}
		hash, err := fileHash(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", rel, err)
		}
		files[rel] = hash
		return nil
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("walk rsrc root %s: %w", root, err)
	}
	return Manifest{
		SchemaVersion: SupportedSchemaVersion,
		Files:         files,
	}, nil
}

// WriteManifest serializes m to root/manifest.json as indented JSON with a
// trailing newline.
func WriteManifest(root string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	path := filepath.Join(root, "manifest.json")
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// fileHash computes the hex-encoded sha256 of a file's content after
// normalizing line endings (\r\n → \n) for cross-platform stability.
// Mirrors wsprompt.normalizePromptHashContent + sha256 pattern.
func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashHex(data), nil
}

// hashHex returns the hex-encoded sha256 of data after \r\n → \n normalization.
func hashHex(data []byte) string {
	normalized := normalizeHashContent(data)
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

// normalizeHashContent normalizes \r\n → \n for cross-platform-stable hashing.
// Copied verbatim from internal/wsprompt/prompts.go normalizePromptHashContent.
func normalizeHashContent(data []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), "\r\n", "\n"))
}
