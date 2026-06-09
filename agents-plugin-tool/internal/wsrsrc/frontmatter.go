package wsrsrc

import "strings"

// parseFrontmatter parses YAML-subset frontmatter from a text string.
// It normalizes \r\n → \n before parsing (wsdoc/frontmatter.go does not;
// this mirrors wsprompt.stripFrontmatter's normalization practice).
//
// Returns (frontmatter map, body). If no valid frontmatter block is found,
// returns (nil, normalized) so callers always receive LF-only text regardless
// of the input line endings.
//
// Supported YAML-subset:
//   - Scalar:   key: value
//   - List:     key:\n  - item
//   - Sub-map:  key:\n  subkey: value
//
// Adapted from internal/wsdoc/frontmatter.go; copied (not imported) because
// that function is unexported and path-based.
func parseFrontmatter(text string) (map[string]any, string) {
	// Normalize line endings before processing. All return paths below yield
	// the normalized string so callers receive LF-only text in every case.
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---") {
		return nil, normalized
	}
	lines := strings.Split(normalized, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, normalized
	}

	// Body is everything after the closing ---.
	body := strings.Join(lines[end+1:], "\n")

	result := map[string]any{}
	current := ""
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level key: line starts without leading whitespace and contains ':'
		if !strings.HasPrefix(line, " ") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			current = strings.TrimSpace(parts[0])
			value := cleanFrontmatterScalar(parts[1])
			if value == "" || value == "[]" || value == "{}" || value == "null" || value == "~" {
				// Empty value — initialize as nil []string so that a subsequent
				// "- item" line does a well-typed append and the intent is explicit.
				// A following "subkey: val" line replaces it with a map[string]string.
				result[current] = ([]string)(nil)
			} else {
				result[current] = value
			}
			continue
		}
		if current == "" {
			continue
		}
		// List item
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			list, _ := result[current].([]string)
			result[current] = append(list, item)
			continue
		}
		// Sub-map entry
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			m, _ := result[current].(map[string]string)
			if m == nil {
				m = map[string]string{}
			}
			m[strings.TrimSpace(parts[0])] = cleanFrontmatterScalar(parts[1])
			result[current] = m
		}
	}
	return result, body
}

// cleanFrontmatterScalar trims whitespace, strips inline comments (# ...),
// and removes surrounding quotes. Copied from internal/wsdoc/frontmatter.go.
func cleanFrontmatterScalar(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
