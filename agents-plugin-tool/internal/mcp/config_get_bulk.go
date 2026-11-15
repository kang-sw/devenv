package mcp

import (
	"fmt"
	"strings"
)

// Validate the whole request before resolving anything (including activity touches).
func configGetKeys(args map[string]any) ([]string, bool, error) {
	_, hasKey := args["key"]
	raw, bulk := args["keys"]
	if hasKey && bulk {
		return nil, bulk, fmt.Errorf("config.get: supply exactly one of key or keys")
	}
	if !bulk {
		key, _ := args["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, false, fmt.Errorf("config.get: key is required")
		}
		return []string{key}, false, nil
	}
	entries, ok := raw.([]any)
	if !ok || len(entries) == 0 {
		return nil, true, fmt.Errorf("config.get: keys must be a nonempty string array")
	}
	keys := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		key, ok := entry.(string)
		key = strings.TrimSpace(key)
		if !ok || key == "" || seen[key] {
			return nil, true, fmt.Errorf("config.get: keys entries must be unique nonblank strings")
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys, true, nil
}

type configGetBulkResult struct {
	Mode    string           `json:"mode"`
	Results []map[string]any `json:"results"`
}

func (s *Server) resolveConfigGetBulk(sessionKey string, keys []string) (configGetBulkResult, string) {
	bulk := configGetBulkResult{Mode: "resolved-v1", Results: make([]map[string]any, 0, len(keys))}
	var text strings.Builder
	for _, key := range keys {
		result, err := s.resolveConfigGet(sessionKey, key)
		if err != nil {
			message := fmt.Sprintf("config.get: %v", err)
			bulk.Results = append(bulk.Results, map[string]any{"key": key, "ok": false, "error": map[string]string{"message": message}})
			fmt.Fprintf(&text, "%s: %s\n", key, strings.Join(strings.Fields(message), " "))
			continue
		}
		row := map[string]any{"key": result.Key, "ok": true, "value": result.Value, "scope": result.Scope}
		if len(result.Warnings) > 0 {
			row["warnings"] = result.Warnings
		}
		bulk.Results = append(bulk.Results, row)
		// Bulk text is one line per row, including that row's warnings.
		text.WriteString(strings.ReplaceAll(strings.TrimSuffix(formatConfigGet(result), "\n"), "\n", " "))
		text.WriteByte('\n')
	}
	return bulk, text.String()
}
