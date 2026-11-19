package wsrsrc

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// PreloadCommand is one user-invocable lead skill of a package: a skill
// shim whose preload block names the skill's own playbook, so its whole
// procedure is that playbook's body.
type PreloadCommand struct {
	// Name is the skill and playbook name (lead-run).
	Name string
	// Description is the skill's frontmatter description.
	Description string
}

// preloadOpenTag is a shim's preload block open tag; the Claude mod's
// preload.ts reads the same marker.
var preloadOpenTag = regexp.MustCompile(`<ws-preload playbook="([^"\n]+)">`)

// PreloadCommands lists, by name, the skills of a package's skills tree
// whose SKILL.md carries a preload block naming the skill's own playbook.
// That excludes an inline skill (its procedure is its own text) and an
// alias (its block, if any, reads another skill's playbook). The listing
// reads only the tree it is given.
func PreloadCommands(skills fs.FS) ([]PreloadCommand, error) {
	entries, err := fs.ReadDir(skills, ".")
	if err != nil {
		return nil, err
	}
	var out []PreloadCommand
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := fs.ReadFile(skills, path.Join(entry.Name(), "SKILL.md"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("skill %s: %w", entry.Name(), err)
		}
		meta, body := parseFrontmatter(string(raw))
		m := preloadOpenTag.FindStringSubmatch(body)
		if m == nil || m[1] != entry.Name() {
			continue
		}
		description, _ := meta["description"].(string)
		out = append(out, PreloadCommand{Name: entry.Name(), Description: strings.TrimSpace(description)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
