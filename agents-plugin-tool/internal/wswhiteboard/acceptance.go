package wswhiteboard

import (
	"html"
	"maps"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func cloneOrigins(origins bodyOrigins) bodyOrigins { return maps.Clone(origins) }

func (u *topicUpdates) clone() *topicUpdates {
	out := &topicUpdates{byKey: map[string]*topicUpdate{}}
	copies := map[*topicUpdate]*topicUpdate{}
	for key, t := range u.byKey {
		copy := *t
		copy.itemKinds = map[int]map[string]string{}
		for id, kinds := range t.itemKinds {
			copy.itemKinds[id] = maps.Clone(kinds)
		}
		out.byKey[key], copies[t] = &copy, &copy
	}
	for _, t := range u.order {
		out.order = append(out.order, copies[t])
	}
	return out
}

// Resolve all known targets before validation, including malformed multi-ID
// attributes. A missing ID cannot poison an unrelated topic's later close.
func commandTopics(s *State, cmd command, topicOf topicResolver) []string {
	var out []string
	add := func(topic string) {
		if topic == "" {
			return
		}
		for _, old := range out {
			if old == topic {
				return
			}
		}
		out = append(out, topic)
	}
	if topic, ok := topicOf(cmd); ok {
		add(topic)
	}
	if cmd.IsGroup() {
		if label, ok := cmd.Scope.Extra("label"); ok && s.topicKnown(label) {
			add(label)
		}
	}
	if cmd.Tag == "wb:answer" || cmd.Tag == "wb:patch" {
		for _, name := range []string{"id", "ids"} {
			raw, _ := cmd.Attr(name)
			for _, token := range strings.Fields(raw) {
				id, _ := strconv.Atoi(token)
				if i := s.index(id); i >= 0 {
					add(s.Items[i].Topic)
				}
			}
		}
	}
	return out
}

// The result wrapper is deliberately not an input command. Bodies and fields
// are copied from source, never decoded/re-escaped or reduced to locators.
func failedCommands(failed []command, keys map[*wsxmlcmd.Scope]string) string {
	if len(failed) == 0 {
		return ""
	}
	groups := map[*wsxmlcmd.Scope]command{}
	for _, cmd := range failed {
		if cmd.IsGroup() {
			groups[cmd.Scope] = cmd
		}
	}
	var b strings.Builder
	b.WriteString("<wb:failed>\n")
	var scope *wsxmlcmd.Scope
	closeScope := func() {
		if scope != nil {
			b.WriteString("</wb:topic>\n")
			scope = nil
		}
	}
	for _, cmd := range failed {
		if cmd.Scope != scope {
			closeScope()
		}
		if cmd.IsGroup() && cmd.Scope.Commands == 0 {
			b.WriteString(cmd.Source + "\n")
			continue
		}
		if cmd.Scope != nil && scope == nil {
			scope = cmd.Scope
			if _, bad := groups[scope]; bad {
				b.WriteString(strings.TrimSuffix(scope.Source, "/>"))
				if strings.HasSuffix(scope.Source, "/>") {
					b.WriteString(">")
				}
			} else {
				key := keys[scope]
				if key == "" {
					key = scope.Value
				}
				b.WriteString(`<wb:topic name="` + html.EscapeString(key) + `">`)
			}
			b.WriteByte('\n')
		}
		if !cmd.IsGroup() {
			b.WriteString(cmd.Source + "\n")
		}
	}
	closeScope()
	b.WriteString("</wb:failed>")
	return b.String()
}
