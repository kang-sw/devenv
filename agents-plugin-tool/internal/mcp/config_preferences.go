package mcp

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func preferenceKnobEntry(key string) configKeyEntry {
	return configKeyEntry{
		Key:        key,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in global scope.",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Preference text; an empty string hides this slot. Omit when reset is true.",
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	}
}

// preferenceOverride returns the full point replacement only when slots exist.
// The seed stays the legacy default when neither a default slot nor override exists.
func preferenceOverride(resolver *wsconfig.Resolver, sessionKey, harness string) (string, bool, error) {
	slots, err := resolver.ListScalars(sessionKey, wsconfig.PreferencePrefix)
	if err != nil {
		return "", false, err
	}
	if len(slots) == 0 {
		return "", false, nil
	}
	defaultText := "Use the project's terminology and communication style."
	legacy, _, err := resolveHarnessBucketed(resolver, sessionKey, "prompt.UserPreferenceSection", harness)
	if err != nil {
		return "", false, err
	}
	if legacy.Value != "" {
		defaultText = legacy.Value
	}
	for _, slot := range slots {
		if slot.Key == "preference.default" {
			defaultText = slot.Value
		}
	}
	var paragraphs []string
	if defaultText != "" {
		paragraphs = append(paragraphs, defaultText)
	}
	for _, slot := range slots {
		if wsconfig.ValidPreferenceKey(slot.Key) && slot.Key != "preference.default" && slot.Value != "" {
			paragraphs = append(paragraphs, slot.Value)
		}
	}
	return strings.Join(paragraphs, "\n\n"), true, nil
}
