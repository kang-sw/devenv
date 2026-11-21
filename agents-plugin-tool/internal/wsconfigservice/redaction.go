package wsconfigservice

import (
	"encoding/json"
	"path"
	"strings"
	"unicode"
)

const redacted = "[redacted]"

func safeStructured(key string, raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	if !strings.HasPrefix(key, "agent.profile.") {
		return raw
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return redactProfile(value, false)
}

func redactProfile(value any, masked bool) any {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			name := normalizeCredentialKey(key)
			if !masked && name == "mcpservers" {
				node[key] = redactServers(child)
				continue
			}
			secret := false
			for _, part := range []string{"token", "apikey", "secret", "password", "authorization", "credential"} {
				secret = secret || strings.Contains(name, part)
			}
			node[key] = redactProfile(child, masked || secret)
		}
	case []any:
		for i, child := range node {
			node[i] = redactProfile(child, masked)
		}
	default:
		if masked {
			return redacted
		}
	}
	return value
}

func normalizeCredentialKey(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
}

func redactServers(value any) any {
	servers, ok := value.(map[string]any)
	if !ok {
		return redactProfile(value, true)
	}
	for id, value := range servers {
		server, ok := value.(map[string]any)
		if !ok {
			servers[id] = redactProfile(value, true)
			continue
		}
		for key, field := range server {
			if s, ok := field.(string); ok {
				switch key {
				case "type", "transport":
					if s == "stdio" || s == "http" || s == "sse" || s == "streamable-http" || s == "websocket" {
						continue
					}
				case "command":
					server[key] = executableName(s)
					continue
				}
			}
			server[key] = redactProfile(field, true)
		}
	}
	return servers
}

func executableName(command string) string {
	command = strings.TrimSpace(command)
	if command == "" || strings.Contains(command, "://") {
		return redacted
	}
	if quote := command[0]; quote == '"' || quote == '\'' {
		end := strings.IndexByte(command[1:], quote)
		if end < 0 {
			return redacted
		}
		command = command[1 : end+1]
	} else {
		command = strings.Fields(command)[0]
	}
	name := path.Base(strings.ReplaceAll(command, "\\", "/"))
	if strings.Trim(name, ".") == "" || strings.ContainsAny(name, "=?:@") {
		return redacted
	}
	return name
}
