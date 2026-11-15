package wsdashboard

import (
	"sync"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// BoardUnits is a source-owned immutable board and its ordered topic units.
// Theme variants are derived once and shared across non-board page renders.
// A source without units leaves this nil and keeps the HTML-parser fallback.
type BoardUnits struct {
	mu      sync.Mutex
	regions *wswhiteboard.TopicRegions
	themes  map[string]boardUnitRender
}

type boardUnitRender struct {
	region renderedRegion
	topics *boardTopics
}

// NewBoardUnits captures a structured render. The source must not mutate its
// topic slice after handing it off, just as it must not mutate cached HTML.
func NewBoardUnits(regions *wswhiteboard.TopicRegions) *BoardUnits {
	if regions == nil {
		return nil
	}
	return &BoardUnits{regions: regions, themes: map[string]boardUnitRender{}}
}

func (u *BoardUnits) rendered(theme string) boardUnitRender {
	u.mu.Lock()
	defer u.mu.Unlock()
	if theme != "light" && theme != "dark" {
		theme = ""
	}
	if r, ok := u.themes[theme]; ok {
		return r
	}
	b := &boardTopics{order: []string{}, topics: map[string]boardTopic{}, shell: string(wswhiteboard.ThemeFrames(u.regions.Shell, theme))}
	for _, unit := range u.regions.Topics {
		t := boardTopic{Key: unit.ID, Hash: unit.Hash, HTML: string(wswhiteboard.ThemeFrames(unit.HTML, theme))}
		b.order = append(b.order, t.Key)
		b.topics[t.Key] = t
	}
	r := boardUnitRender{region: newRegion(string(wswhiteboard.ThemeFrames(u.regions.Board, theme))), topics: b}
	u.themes[theme] = r
	return r
}
