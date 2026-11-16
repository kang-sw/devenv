package wswhiteboard

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// Status-only writes are a sub-lead's path onto its top-level lead's board
// (Env.StatusAuthor): status log lines and new panels inside a topic the lead
// already opened, never an ask, a topic body, a topic's label or ticket
// links, or a panel the sub-lead did not author. Each check reads work as
// the earlier commands of the same call leave it, so a panel name minted
// earlier in the call is already taken.

// statusProblems records why cmd is not a status command a sub-lead may
// write; it records nothing for an admissible wb:log or wb:panel.
func statusProblems(p *problems, work *State, cmd command, topicOf topicResolver) {
	if cmd.IsGroup() {
		// A plain group only names the topic its commands act on.
		if cmd.Scope.HasExtra() {
			p.Add("%s: a sub-lead cannot create, relabel, or relink a topic; write wb:log or wb:panel inside an existing open topic group that carries no label or tickets attribute", cmd.Label())
		}
		return
	}
	if cmd.Tag != "wb:log" && cmd.Tag != "wb:panel" {
		p.Add("%s: a sub-lead writes status only: wb:log or wb:panel inside an existing open topic", cmd.Label())
		return
	}
	name, _ := cmd.Scoped()
	topic, ok := topicOf(cmd)
	if !ok || !(work.topicKnown(topic) || work.stemmed(topic)) || work.topicClosed(topic) {
		p.Add("%s: no open topic %q on the lead's board; a sub-lead writes only inside a topic its lead has open", cmd.Label(), name)
		return
	}
	if cmd.Tag != "wb:panel" {
		return
	}
	if _, has := cmd.Attr("id"); has {
		p.Add("%s: a sub-lead always adds a new panel; omit id", cmd.At("id"))
	}
	if panel, has := cmd.Attr("name"); has && work.panelNamed(topic, strings.TrimSpace(panel)) {
		p.Add("%s: panel %q already exists in this topic; a sub-lead adds a new panel under a new name and never replaces one", cmd.At("name"), strings.TrimSpace(panel))
	}
}

// panelNamed reports whether topic holds a panel named name.
func (s *State) panelNamed(topic, name string) bool {
	if name == "" {
		return false
	}
	if i := s.topicDocIndex(topic); i >= 0 {
		for _, entry := range s.Topics[i].Entries {
			if entry.Kind == EntryPanel && entry.PanelName == name {
				return true
			}
		}
	}
	return false
}

// statusAttribution is the server-written prefix that leads a sub-lead's log
// line and panel recap, naming the playbook that minted the sub-lead key.
func statusAttribution(author string) string {
	return "[" + author + "] "
}

// attributed returns cmd with the attribution prefix on its log line or
// panel recap. A blank body or recap is left blank so the command's own
// required-value check still refuses it. The original command keeps its
// source, so a failed retry payload is the sub-lead's text as written.
func attributed(cmd command, author string) command {
	prefix := statusAttribution(author)
	switch cmd.Tag {
	case "wb:log":
		if line := strings.TrimSpace(cmd.Body); line != "" {
			cmd.Body = prefix + line
		}
	case "wb:panel":
		attrs := make([]wsxmlcmd.Attr, len(cmd.Attrs))
		copy(attrs, cmd.Attrs)
		for i := range attrs {
			if attrs[i].Name == "recap" {
				if recap := strings.TrimSpace(attrs[i].Value); recap != "" {
					attrs[i].Value = prefix + recap
				}
			}
		}
		cmd.Attrs = attrs
	}
	return cmd
}

// statusCommand is cmd as Apply stores it: attributed for a status-only
// write, unchanged otherwise.
func statusCommand(cmd command, env Env) command {
	if env.StatusAuthor == "" {
		return cmd
	}
	return attributed(cmd, env.StatusAuthor)
}
