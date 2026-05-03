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
	layout, _, _, err := m.Ensure(repoPath)
	if err != nil {
		return nil, err
	}
	dir, ext, err := generatedPathTarget(layout, kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create generated path dir %s: %w", dir, err)
	}

	runID, err := randomHex(6)
	if err != nil {
		return nil, err
	}
	prefix := m.now().UTC().Format("20060102T150405Z") + "-" + runID

	paths := make([]GeneratedPath, 0, len(stems))
	for index, stem := range stems {
		safeStem := sanitizeGeneratedPathStem(stem)
		path := filepath.Join(dir, fmt.Sprintf("%s-%02d-%s%s", prefix, index+1, safeStem, ext))
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

func generatedPathTarget(layout Layout, kind string) (dir string, ext string, err error) {
	switch kind {
	case "review":
		return layout.ReviewDir, ".md", nil
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

func randomHex(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random path id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
