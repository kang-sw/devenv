// Source-fixture driver. Build this unchanged inside the pre-feature source
// tree (8cd2566ec), not against the implementation under test. It calls that
// version's real folder loader, Apply, archive eviction and metadata writer.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: compat-old STORE COMMANDS (or read)")
	}
	store := wswhiteboard.FolderStore{Dir: os.Args[1]}
	var problems []string
	if os.Args[2] != "read" {
		err := store.Update(nil, func(s *wswhiteboard.State) error {
			_, problems = wswhiteboard.Apply(s, os.Args[2], wswhiteboard.Env{Now: time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC), IsAbs: func(string) bool { return true }, IsDir: func(string) bool { return true }})
			if len(problems) > 0 {
				return errors.New("refused")
			}
			return nil
		})
		if err != nil && len(problems) == 0 {
			panic(err)
		}
	}
	s, err := store.Open(nil)
	if err != nil {
		panic(err)
	}
	query, p := wswhiteboard.Query(s, map[string]any{"topic": "t", "depth": "summary"})
	if len(p) > 0 {
		panic(p)
	}
	page, err := wswhiteboard.RenderPage(wswhiteboard.BuildView(s, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull}))
	if err != nil {
		panic(err)
	}
	embedPage, err := wswhiteboard.RenderPage(wswhiteboard.BuildView(s, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutEmbed}))
	if err != nil {
		panic(err)
	}
	data, err := json.Marshal(struct {
		State                    *wswhiteboard.State
		Problems                 []string
		Display, Page, EmbedPage string
		LastEmbedSeq             int
		EmbedDir                 string
		EmbedCount               int
	}{s, problems, wswhiteboard.FormatQuery(query), page, embedPage, s.LastEmbedSeq, s.EmbedDir, len(s.Embeds)})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
