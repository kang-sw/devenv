package wsdoc

import (
	"os"
	"strings"
)

func frontmatter(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil
	}
	lines := strings.Split(text, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil
	}

	result := map[string]any{}
	current := ""
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			current = strings.TrimSpace(parts[0])
			value := cleanScalar(parts[1])
			if value == "" || value == "[]" || value == "{}" || value == "null" || value == "~" {
				result[current] = map[string]string{}
			} else {
				result[current] = value
			}
			continue
		}
		if current == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			list, _ := result[current].([]string)
			result[current] = append(list, item)
			continue
		}
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			m, _ := result[current].(map[string]string)
			if m == nil {
				m = map[string]string{}
			}
			m[strings.TrimSpace(parts[0])] = cleanScalar(parts[1])
			result[current] = m
		}
	}
	return result
}

func cleanScalar(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
