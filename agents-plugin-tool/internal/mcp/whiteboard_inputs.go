package mcp

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Input stamps are display provenance on immutable entries, not a cursor or
// another store. The folder transaction already serializes this read/append.
type boardInputStamp struct {
	InboxID string   `json:"inbox_id"`
	SentAt  int64    `json:"sent_at"`
	Excerpt string   `json:"excerpt"`
	ChipIDs []string `json:"chip_ids,omitempty"`
}

func (s *Server) stampBoardInputs(key string, st *wswhiteboard.State, before int) {
	var fresh []*wswhiteboard.Entry
	used := map[string]bool{}
	active, floor := false, ""
	for ti := range st.Topics {
		for ei := range st.Topics[ti].Entries {
			e := &st.Topics[ti].Entries[ei]
			n, _ := strconv.Atoi(strings.TrimPrefix(e.ID, "e"))
			if n > before {
				e.Seq = st.Seq
				fresh = append(fresh, e)
				continue
			}
			if e.Inputs != nil {
				active = true
				var inputs []boardInputStamp
				if json.Unmarshal(e.Inputs, &inputs) == nil {
					for _, input := range inputs {
						used[input.InboxID] = true
					}
				}
			}
			if e.InputsFloor > floor {
				floor = e.InputsFloor
			}
		}
	}
	if len(fresh) == 0 {
		return // Eventless batches leave delivery for the next visible response.
	}
	sort.Slice(fresh, func(i, j int) bool {
		a, _ := strconv.Atoi(fresh[i].ID[1:])
		b, _ := strconv.Atoi(fresh[j].ID[1:])
		return a < b
	})
	dir, err := s.sessionInboxDir(key)
	if err != nil {
		return // No guessed provenance when the separately retained inbox is unreadable.
	}
	records, err := wsinbox.Drained(dir)
	if err != nil {
		return
	}
	if !active {
		names, err := wsinbox.DrainedNames(dir)
		if err != nil {
			return
		}
		fresh[0].Inputs = json.RawMessage(`[]`)
		for _, name := range names {
			if id := (wsinbox.Record{Name: name}).ID(); id > floor {
				floor = id
			}
		}
		fresh[0].InputsFloor = floor
		return
	}
	var inputs []boardInputStamp
	answerTimes := map[int]int64{}
	for _, r := range records {
		if r.ID() <= floor || used[r.ID()] || wsdashboard.IsAnswerCheckNudge(r.Entry) || r.Entry.Command != nil || r.Entry.Interrupt {
			continue
		}
		input := boardInputStamp{InboxID: r.ID(), SentAt: r.Entry.SentAt, Excerpt: r.Entry.Excerpt()}
		for _, chip := range r.Entry.Chips {
			input.ChipIDs = append(input.ChipIDs, chip.ID)
			id, _ := strconv.Atoi(chip.ID)
			if chip.Answer != "" && chip.SentAt > answerTimes[id] {
				answerTimes[id] = chip.SentAt
			}
		}
		inputs = append(inputs, input)
	}
	if len(inputs) == 0 {
		return
	}
	fresh[0].Inputs, _ = json.Marshal(inputs)
	for _, e := range fresh {
		if e.Kind == wswhiteboard.EntrySystem && e.ItemID > 0 && strings.HasPrefix(e.Text, "Item {{"+strconv.Itoa(e.ItemID)+"}} answered [") {
			e.AnswerSentAt = answerTimes[e.ItemID]
		}
	}
}
