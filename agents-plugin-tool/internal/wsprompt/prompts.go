package wsprompt

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed prompts/*.md infra/*.md
var promptFS embed.FS

var embeddedPromptPaths = []string{
	"prompts/code-reviewer.md",
	"prompts/implementer.md",
	"prompts/mental-model-updater.md",
	"prompts/plan-populator-research.md",
	"prompts/plan-populator-survey.md",
	"prompts/project-survey.md",
	"prompts/skeleton-writer.md",
	"prompts/sprint-survey.md",
	"infra/code-review-correctness.md",
	"infra/code-review-fit.md",
	"infra/code-review-test.md",
	"infra/delegate-orientation.md",
	"infra/impl-playbook.md",
}

type Source struct {
	Spec string `json:"spec"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Resolved struct {
	Text    string
	Tier    string
	Model   string
	Sources []Source
}

type BundleInfo struct {
	SourceCommit  string   `json:"source_commit"`
	ContentSHA256 string   `json:"content_sha256"`
	Prompts       []string `json:"prompts"`
}

func Resolve(specs []string, systemPromptText, explicitTier, explicitModel string) (Resolved, error) {
	resolved := Resolved{
		Tier:  strings.TrimSpace(explicitTier),
		Model: strings.TrimSpace(explicitModel),
	}
	var parts []string
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		body, frontmatter, source, err := resolveOne(spec)
		if err != nil {
			return Resolved{}, err
		}
		if resolved.Tier == "" && resolved.Model == "" {
			if value := strings.TrimSpace(frontmatter["model"]); value != "" {
				if tier, ok := modelTier(value); ok {
					resolved.Tier = tier
				} else {
					resolved.Model = value
				}
			}
		}
		if strings.TrimSpace(body) != "" {
			parts = append(parts, strings.TrimSpace(body))
		}
		resolved.Sources = append(resolved.Sources, source)
	}
	if strings.TrimSpace(systemPromptText) != "" {
		parts = append(parts, strings.TrimSpace(systemPromptText))
	}
	resolved.Text = strings.Join(parts, "\n\n---\n\n")
	return resolved, nil
}

func Bundle(sourceCommit string) (BundleInfo, error) {
	prompts := []string{
		"code-reviewer",
		"implementer",
		"mental-model-updater",
		"plan-populator-research",
		"plan-populator-survey",
		"project-survey",
		"skeleton-writer",
		"sprint-survey",
		"code-review-correctness",
		"code-review-fit",
		"code-review-test",
		"delegate-orientation",
		"impl-playbook",
	}
	hash, err := ContentSHA256()
	if err != nil {
		return BundleInfo{}, err
	}
	return BundleInfo{
		SourceCommit:  sourceCommit,
		ContentSHA256: hash,
		Prompts:       prompts,
	}, nil
}

func ContentSHA256() (string, error) {
	paths := append([]string(nil), embeddedPromptPaths...)
	sort.Strings(paths)
	sum := sha256.New()
	for _, path := range paths {
		data, err := promptFS.ReadFile(path)
		if err != nil {
			return "", err
		}
		sum.Write([]byte(path))
		sum.Write([]byte{0})
		sum.Write(data)
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func resolveOne(spec string) (string, map[string]string, Source, error) {
	if filepath.IsAbs(spec) {
		data, err := os.ReadFile(spec)
		if err != nil {
			return "", nil, Source{}, fmt.Errorf("read prompt %s: %w", spec, err)
		}
		body, frontmatter := stripFrontmatter(string(data))
		return body, frontmatter, Source{Spec: spec, Path: spec, Kind: "absolute"}, nil
	}
	if !isBareStem(spec) {
		return "", nil, Source{}, fmt.Errorf("prompt spec %q must be a bare embedded stem or an absolute path", spec)
	}
	stem := strings.TrimSuffix(spec, ".md")
	for _, prefix := range []string{"prompts", "infra"} {
		path := filepath.ToSlash(filepath.Join(prefix, stem+".md"))
		data, err := promptFS.ReadFile(path)
		if err == nil {
			body, frontmatter := stripFrontmatter(string(data))
			return body, frontmatter, Source{Spec: spec, Path: path, Kind: "embedded"}, nil
		}
	}
	return "", nil, Source{}, fmt.Errorf("unknown embedded prompt stem %q", spec)
}

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

func stripFrontmatter(text string) (string, map[string]string) {
	frontmatter := map[string]string{}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return text, frontmatter
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return text, frontmatter
	}
	header := rest[:end]
	body := rest[end+len("\n---\n"):]
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		frontmatter[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return body, frontmatter
}

func modelTier(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "haiku", "light":
		return "light", true
	case "sonnet", "core":
		return "core", true
	case "opus", "deep":
		return "deep", true
	default:
		return "", false
	}
}
