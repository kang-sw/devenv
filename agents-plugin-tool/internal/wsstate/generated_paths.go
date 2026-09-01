package wsstate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeGeneratedPathStemChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type GeneratedPath struct {
	Kind string `json:"kind"`
	Stem string `json:"stem"`
	Path string `json:"path"`
}

func (m Manager) GeneratePaths(repoPath, kind string, stems []string) ([]GeneratedPath, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, fmt.Errorf("path kind is required")
	}
	if len(stems) == 0 {
		return nil, fmt.Errorf("at least one path stem is required")
	}
	layout, _, worktree, err := m.Ensure(repoPath)
	if err != nil {
		return nil, err
	}
	if kind == "plan" {
		return m.generatePlanPaths(worktree.WorktreePath, kind, stems)
	}
	if kind == "clone" {
		return m.generateClonePaths(layout, kind, stems)
	}
	dir, ext, err := generatedPathTarget(layout, kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create generated path dir %s: %w", dir, err)
	}

	runID, err := randomHex(4)
	if err != nil {
		return nil, err
	}

	paths := make([]GeneratedPath, 0, len(stems))
	for index, stem := range stems {
		safeStem := sanitizeGeneratedPathStem(stem)
		path := filepath.Join(dir, fmt.Sprintf("%s-%02d-%s%s", runID, index+1, safeStem, ext))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, fmt.Errorf("reserve generated path %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close generated path %s: %w", path, err)
		}
		paths = append(paths, GeneratedPath{
			Kind: kind,
			Stem: safeStem,
			Path: path,
		})
	}
	return paths, nil
}

func (m Manager) generatePlanPaths(worktreePath, kind string, stems []string) ([]GeneratedPath, error) {
	now := m.now().Local()
	dir := filepath.Join(worktreePath, "ai-docs", ".plans", now.Format("2006-01"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create generated path dir %s: %w", dir, err)
	}

	paths := make([]GeneratedPath, 0, len(stems))
	for _, stem := range stems {
		safeStem := sanitizeGeneratedPathStem(stem)
		base := fmt.Sprintf("%s-%s", now.Format("02-1504"), safeStem)
		path, err := reserveUniqueGeneratedPath(dir, base, ".md")
		if err != nil {
			return nil, err
		}
		paths = append(paths, GeneratedPath{
			Kind: kind,
			Stem: safeStem,
			Path: path,
		})
	}
	return paths, nil
}

// generateClonePaths allocates clone-layer shared docs under
// layout.SharedDir/docs. Unlike the generic review/prompt path above, clone
// filenames are readable (<sanitized-stem>-<6-char-suffix>.md, not an opaque
// runID-NN scheme) and collisions are resolved by retrying with a fresh
// random suffix, mirroring reserveUniqueGeneratedPath's never-truncate
// guarantee so a collision can never clobber a sibling clone doc.
func (m Manager) generateClonePaths(layout Layout, kind string, stems []string) ([]GeneratedPath, error) {
	dir, ext, err := generatedPathTarget(layout, kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create generated path dir %s: %w", dir, err)
	}

	paths := make([]GeneratedPath, 0, len(stems))
	for _, stem := range stems {
		safeStem := sanitizeGeneratedPathStem(stem)
		path, err := reserveUniqueSuffixedGeneratedPath(dir, safeStem, ext)
		if err != nil {
			return nil, err
		}
		paths = append(paths, GeneratedPath{
			Kind: kind,
			Stem: safeStem,
			Path: path,
		})
	}
	return paths, nil
}

func generatedPathTarget(layout Layout, kind string) (dir string, ext string, err error) {
	switch kind {
	case "review":
		return layout.ReviewDir, ".md", nil
	case "prompt":
		return layout.PromptDir, ".md", nil
	case "clone":
		return filepath.Join(layout.SharedDir, "docs"), ".md", nil
	default:
		return "", "", fmt.Errorf("unsupported path kind %q", kind)
	}
}

func sanitizeGeneratedPathStem(stem string) string {
	stem = unsafeGeneratedPathStemChars.ReplaceAllString(stem, "-")
	stem = strings.Trim(stem, "-")
	if stem == "" {
		return "unnamed"
	}
	return stem
}

func reserveUniqueGeneratedPath(dir, base, ext string) (string, error) {
	for attempt := 1; attempt <= 999; attempt++ {
		suffix := ""
		if attempt > 1 {
			suffix = fmt.Sprintf("-%02d", attempt)
		}
		path := filepath.Join(dir, base+suffix+ext)
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if err := file.Close(); err != nil {
				return "", fmt.Errorf("close generated path %s: %w", path, err)
			}
			return path, nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", fmt.Errorf("reserve generated path %s: %w", path, err)
	}
	return "", fmt.Errorf("reserve generated path %s: exhausted collision suffixes", filepath.Join(dir, base+ext))
}

// cloneSuffixGenerator produces the fresh random suffix
// reserveUniqueSuffixedGeneratedPath tries on each reservation attempt. A
// package-level var (mirroring the Options.Now injection seam elsewhere in
// this package) so tests can force a deterministic suffix sequence to
// exercise the collision-retry path without relying on an astronomically
// unlikely crypto/rand collision.
var cloneSuffixGenerator = func() (string, error) { return randomBase36(6) }

// reserveUniqueSuffixedGeneratedPath reserves <dir>/<base>-<suffix><ext> for a
// fresh random base36 suffix generated per attempt (not an incrementing
// numeric suffix, unlike reserveUniqueGeneratedPath), retrying on collision.
// Never opens with O_TRUNC, so a collision can never clobber a sibling file.
func reserveUniqueSuffixedGeneratedPath(dir, base, ext string) (string, error) {
	for attempt := 1; attempt <= 999; attempt++ {
		suffix, err := cloneSuffixGenerator()
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, suffix, ext))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if err := file.Close(); err != nil {
				return "", fmt.Errorf("close generated path %s: %w", path, err)
			}
			return path, nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", fmt.Errorf("reserve generated path %s: %w", path, err)
	}
	return "", fmt.Errorf("reserve generated path for base %s: exhausted collision suffixes", filepath.Join(dir, base+ext))
}

func randomHex(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random path id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// randomBase36 generates a random lowercase base36 ([a-z0-9]) string of
// length n, used for clone-kind generated path filename suffixes. Distinct
// from randomHex (hex alphabet, used for the opaque runID prefix).
func randomBase36(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random path suffix: %w", err)
	}
	out := make([]byte, n)
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
