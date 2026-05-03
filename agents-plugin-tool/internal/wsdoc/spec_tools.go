package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var specAnchorRE = regexp.MustCompile(`\{#([0-9]{6}-[a-z0-9-]+)\}`)

func GenerateSpecStem(root, slug string, now time.Time) (string, error) {
	slug = cleanSlug(slug)
	if slug == "" {
		return "", fmt.Errorf("slug is required")
	}
	prefix := now.Format("060102")
	existing, err := SpecAnchors(root)
	if err != nil {
		return "", err
	}
	candidate := prefix + "-" + slug
	if !existing[candidate] {
		return candidate, nil
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%s-%d", prefix, slug, i)
		if !existing[next] {
			return next, nil
		}
	}
}

func VerifySpecIndex(root string) (string, error) {
	anchors, err := specAnchorLocations(root)
	if err != nil {
		return "", err
	}
	duplicates := []string{}
	for anchor, locations := range anchors {
		if len(locations) > 1 {
			duplicates = append(duplicates, fmt.Sprintf("%s: %s", anchor, strings.Join(locations, ", ")))
		}
	}
	sort.Strings(duplicates)
	if len(duplicates) == 0 {
		return "Spec index: ok\n", nil
	}
	return "Spec index: duplicate anchors\n- " + strings.Join(duplicates, "\n- ") + "\n", nil
}

func SpecAnchors(root string) (map[string]bool, error) {
	locations, err := specAnchorLocations(root)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for anchor := range locations {
		result[anchor] = true
	}
	return result, nil
}

func specAnchorLocations(root string) (map[string][]string, error) {
	specRoot := filepath.Join(root, "ai-docs", "spec")
	info, err := os.Stat(specRoot)
	if err != nil {
		return nil, fmt.Errorf("spec directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("spec path is not a directory: %s", specRoot)
	}
	result := map[string][]string{}
	err = filepath.WalkDir(specRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, match := range specAnchorRE.FindAllStringSubmatch(string(data), -1) {
			result[match[1]] = append(result[match[1]], rel)
		}
		return nil
	})
	return result, err
}

func cleanSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var b strings.Builder
	lastHyphen := false
	for _, r := range slug {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
