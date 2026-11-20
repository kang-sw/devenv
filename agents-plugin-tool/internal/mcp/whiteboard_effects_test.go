package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func effectsWhiteboardFixture(f liveFixture) *whiteboardFixture {
	return &whiteboardFixture{s: f.server, key: f.keyA, root: f.root, id: 100}
}

func effectsRecords(t *testing.T, f liveFixture, reply string) []wslive.RingRecord {
	t.Helper()
	var out []wslive.RingRecord
	for _, rec := range readLiveRing(t, f.sessionDir(reply)) {
		if rec.Kind == wslive.RingWhiteboardEffects {
			out = append(out, rec)
		}
	}
	return out
}

func TestWhiteboardEffectsCallingSessionWithoutAdapterTerminalAndHosted(t *testing.T) {
	for _, mode := range []string{"terminal-claude", "terminal-pi", "hosted-claude", "hosted-pi"} {
		t.Run(mode, func(t *testing.T) {
			f := newLiveFixture(t, true)
			if strings.HasPrefix(mode, "hosted-") {
				hostedSessionEnv(t, f.keyA)
				f.server = NewServer(f.root, "test")
			}
			f.server.sessionHarness = strings.Split(mode, "-")[1]
			wb := effectsWhiteboardFixture(f)
			wb.update(t, `<wb:floor id="40"/><wb:topic name="design" label="Design"><wb:add class="check"><wb:title>Attributable `+f.keyA+`</wb:title></wb:add><wb:panel recap="Unsafe text">&lt;script&gt;alert(1)&lt;/script&gt;</wb:panel><wb:log>Committed</wb:log></wb:topic>`)
			records := effectsRecords(t, f, f.replyA)
			if len(records) != 1 {
				t.Fatalf("calling session entries: %+v", records)
			}
			effects, ok := wslive.DecodeWhiteboardEffects(records[0].RingEntry)
			if !ok || !strings.Contains(records[0].Text, `"id":41`) || strings.Contains(records[0].Text, f.keyA) {
				t.Fatalf("bad identity/scrubbing: %+v", records[0])
			}
			if len(effects) == 0 || len(effectsRecords(t, f, f.replyB)) != 0 {
				t.Fatal("effects absent or delivered to another session")
			}
			if _, err := os.Stat(wslive.SnapshotPath(f.sessionDir(f.replyA))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("projection invented an adapter snapshot: %v", err)
			}
			board, err := f.server.sessions.readWhiteboard(f.keyA)
			if err != nil || board.Whiteboard == nil || board.Whiteboard.Items[0].ID != 41 || !strings.Contains(board.Whiteboard.Items[0].Title, f.keyA) {
				t.Fatalf("projection mutated board data: %+v %v", board.Whiteboard, err)
			}
		})
	}
}

func TestWhiteboardEffectsNoopRefusalAndAuthorization(t *testing.T) {
	f := newLiveFixture(t, true)
	wb := effectsWhiteboardFixture(f)
	wb.update(t, `<wb:topic name="same"><wb:body>Same</wb:body><wb:add class="check"><wb:title>Same</wb:title></wb:add></wb:topic>`)
	for _, commands := range []string{``, `<wb:topic name="same"><wb:body>Same</wb:body></wb:topic>`, `<wb:patch id="1"><wb:title>Same</wb:title></wb:patch>`} {
		wb.update(t, commands)
	}
	refused := wb.call(t, "whiteboard.update", map[string]any{"commands": `<wb:patch id="900"/>`})
	if !isToolError(refused) || len(effectsRecords(t, f, f.replyA)) != 1 {
		t.Fatalf("refusal/noop appended: %s", refused)
	}
	for _, spec := range []struct {
		role   toolRole
		parent string
	}{{roleDelegate, ""}, {roleLead, f.keyA}} {
		key, err := f.server.sessions.mint(f.root, spec.role, spec.parent)
		if err != nil {
			t.Fatal(err)
		}
		resp := callToolOnce(t, f.server, 900, "whiteboard.update", map[string]any{"session_key": key, "commands": `<wb:topic name="bad"><wb:log>Forbidden</wb:log></wb:topic>`})
		if !isToolError(resp) {
			t.Fatalf("ordinary/child session gained board authority: %s", resp)
		}
		if dir, ok := f.server.liveSessionDir(key); ok {
			t.Fatalf("unauthorized projection directory: %s", dir)
		}
	}
}

func TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation(t *testing.T) {
	f := newLiveFixture(t, true)
	wb := effectsWhiteboardFixture(f)

	// Use the existing durable mutation seam to give two separate updates the
	// same recorded clock without teaching the production handler a test clock.
	applyAt := func(commands string, now time.Time) {
		t.Helper()
		var (
			outcome  wswhiteboard.Outcome
			problems []string
			observed boardMutationObservation
		)
		err := f.server.sessions.mutateRecordPartsObserved(f.keyA, partAll, partWhiteboard, func(rec *sessionRecord) error {
			if rec.Whiteboard == nil {
				rec.Whiteboard = &wswhiteboard.State{}
			}
			var err error
			outcome, problems = wswhiteboard.Apply(rec.Whiteboard, commands, wswhiteboard.Env{Now: now})
			if len(problems) > 0 {
				return errWhiteboardRefused
			}
			return err
		}, &observed)
		if err != nil || !observed.Committed {
			t.Fatalf("controlled whiteboard update failed: %v (%v), observed=%+v", err, problems, observed)
		}
		f.server.recordWhiteboardEffects(f.keyA, outcome.Effects)
	}
	readBoard := func() *wswhiteboard.State {
		t.Helper()
		rec, err := f.server.sessions.readWhiteboard(f.keyA)
		if err != nil || rec.Whiteboard == nil {
			t.Fatalf("read whiteboard: %v", err)
		}
		return rec.Whiteboard
	}
	entriesFor := func(board *wswhiteboard.State, name string) []wswhiteboard.Entry {
		t.Helper()
		for _, topic := range board.Topics {
			if topic.Name == name {
				// A new topic also records its initial topic-state line; this
				// test is about item-creation lines, so leave those out.
				var entries []wswhiteboard.Entry
				for _, entry := range topic.Entries {
					if len(entry.TopicState) == 0 {
						entries = append(entries, entry)
					}
				}
				return entries
			}
		}
		t.Fatalf("missing topic %q in %+v", name, board.Topics)
		return nil
	}

	legacyAt := time.Date(2026, 11, 15, 1, 0, 0, 0, time.UTC)
	applyAt(`<wb:topic name="legacy"><wb:add class="technical"><wb:title>Legacy one</wb:title></wb:add></wb:topic>`, legacyAt)
	applyAt(`<wb:topic name="legacy"><wb:add class="check"><wb:title>Legacy two</wb:title></wb:add></wb:topic>`, legacyAt)
	legacyBefore := append([]wswhiteboard.Entry(nil), entriesFor(readBoard(), "legacy")...)
	if len(legacyBefore) != 2 || legacyBefore[0].ItemID != 1 || legacyBefore[1].ItemID != 2 || legacyBefore[0].Text != "Item {{1}} added [technical]: Legacy one" || legacyBefore[1].Text != "Item {{2}} added [check]: Legacy two" || legacyBefore[0].At != legacyBefore[1].At {
		t.Fatalf("legacy creation entries were not separate equal-time records: %+v", legacyBefore)
	}

	// Open the topic first: a new topic records its initial topic-state line
	// right after its first item, which would split the creation batch.
	wb.update(t, `<wb:topic name="known"><wb:body>Known batch.</wb:body></wb:topic>`)
	wb.update(t, `<wb:topic name="known"><wb:add class="policy"><wb:title>First</wb:title></wb:add><wb:add class="check"><wb:title>Second</wb:title></wb:add></wb:topic>`)
	board := readBoard()
	known := entriesFor(board, "known")
	if len(known) != 3 || known[0].Kind != wswhiteboard.EntryPanel || !known[0].LegacyContent || known[1].Text != "Content updated." || known[2].ItemID != 0 || known[2].Text != "Items added: policy {{3}}, check {{4}}." {
		t.Fatalf("known creation batch was not one stored line: %+v", known)
	}
	if got := entriesFor(board, "legacy"); !reflect.DeepEqual(got, legacyBefore) {
		t.Fatalf("legacy creation bytes/order changed: before=%+v after=%+v", legacyBefore, got)
	}

	records := effectsRecords(t, f, f.replyA)
	if len(records) != 4 {
		t.Fatalf("accepted updates produced %d effects carriers, want 4: %+v", len(records), records)
	}
	effects, ok := wslive.DecodeWhiteboardEffects(records[len(records)-1].RingEntry)
	if !ok {
		t.Fatalf("known creation carrier is not typed: %+v", records[len(records)-1])
	}
	type itemValue struct {
		id    int
		field string
		after string
	}
	var values []itemValue
	for _, effect := range effects {
		if effect.Kind == "item" && (effect.Field == "class" || effect.Field == "title") {
			values = append(values, itemValue{id: effect.ID, field: effect.Field, after: effect.After})
		}
	}
	wantValues := []itemValue{{id: 3, field: "class", after: "policy"}, {id: 3, field: "title", after: "First"}, {id: 4, field: "class", after: "check"}, {id: 4, field: "title", after: "Second"}}
	if !reflect.DeepEqual(values, wantValues) {
		t.Fatalf("typed creation effects lost resolved order/values: got %+v want %+v", values, wantValues)
	}

	beforeRefusal := readBoard()
	beforeRecords := len(effectsRecords(t, f, f.replyA))
	refused := wb.call(t, "whiteboard.update", map[string]any{
		"commands": `<wb:topic name="known"><wb:add class="technical"><wb:title>Rejected</wb:title></wb:add></wb:topic><wb:nope/>`,
	})
	if !isToolError(refused) {
		t.Fatalf("invalid creation batch was accepted: %s", refused)
	}
	afterRefusal := readBoard()
	if afterRefusal.Counter != beforeRefusal.Counter+1 || afterRefusal.EntrySeq != beforeRefusal.EntrySeq+1 || afterRefusal.Seq != beforeRefusal.Seq+1 || len(effectsRecords(t, f, f.replyA)) != beforeRecords+1 || afterRefusal.Items[len(afterRefusal.Items)-1].Title != "Rejected" {
		t.Fatal("mixed creation did not retain exactly its accepted add")
	}

	beforeNoop := readBoard()
	beforeNoopRecords := len(effectsRecords(t, f, f.replyA))
	wb.update(t, `<wb:patch id="3"><wb:title>First</wb:title></wb:patch>`)
	afterNoop := readBoard()
	if afterNoop.Seq != beforeNoop.Seq+1 {
		t.Fatalf("accepted no-op did not advance render sequence: before=%d after=%d", beforeNoop.Seq, afterNoop.Seq)
	}
	beforeComparable := *beforeNoop
	afterComparable := *afterNoop
	beforeComparable.Seq = afterComparable.Seq
	if !reflect.DeepEqual(beforeComparable, afterComparable) || len(effectsRecords(t, f, f.replyA)) != beforeNoopRecords {
		t.Fatalf("true no-op changed board semantics or effects ring: before=%+v after=%+v records=%d", beforeComparable, afterComparable, len(effectsRecords(t, f, f.replyA)))
	}
}

func TestWhiteboardEffectsGroupReversionStillProjectsCommittedTransitions(t *testing.T) {
	f := newLiveFixture(t, true)
	wb := effectsWhiteboardFixture(f)
	wb.update(t, `<wb:topic name="t" label="Original"><wb:body>Keep</wb:body></wb:topic>`)
	wb.update(t, `<wb:topic name="t" label="Interim"/><wb:topic name="t" label="Original"/>`)
	records := effectsRecords(t, f, f.replyA)
	if len(records) != 2 {
		t.Fatalf("metadata reversion was omitted as a no-op: %+v", records)
	}
	effects, ok := wslive.DecodeWhiteboardEffects(records[1].RingEntry)
	if !ok || len(effects) != 2 || effects[0].Before != "Original" || effects[0].After != "Interim" || effects[1].Before != "Interim" || effects[1].After != "Original" {
		t.Fatalf("group projection lost ordered values: %+v", effects)
	}
}

func TestWhiteboardEffectsDisabledDashboardWritesNoRing(t *testing.T) {
	f := newLiveFixture(t, false)
	effectsWhiteboardFixture(f).update(t, `<wb:topic name="off"><wb:log>Board only</wb:log></wb:topic>`)
	if len(effectsRecords(t, f, f.replyA)) != 0 {
		t.Fatal("disabled dashboard wrote effects")
	}
}

func TestWhiteboardEffectsDurableIntentErrors(t *testing.T) {
	for _, stage := range []string{"intent-staged", "intent", "log-partial", "complete"} {
		t.Run(stage, func(t *testing.T) {
			f := newLiveFixture(t, true)
			wb := effectsWhiteboardFixture(f)
			// Publication uses the key's canonical root, while liveFixture
			// retains t.TempDir's spelling (/var vs /private/var on macOS).
			entry, ok := f.server.sessions.lookup(f.keyA)
			if !ok {
				t.Fatal("fixture session key is missing")
			}
			wb.root = entry.root
			injected := errors.New("injected " + stage)
			f.server.sessions.boardFault = func(at string) error {
				if at == stage {
					return injected
				}
				return nil
			}

			resp := wb.call(t, "whiteboard.update", map[string]any{"commands": `<wb:topic name="decision"><wb:log>Exactly once</wb:log><wb:add class="check"><wb:title>Durable</wb:title></wb:add></wb:topic><wb:answer id="999" value="ok">Rejected</wb:answer>`})
			if !isToolError(resp) || !strings.Contains(toolText(t, resp), injected.Error()) {
				t.Fatalf("expected injected error: %s", resp)
			}
			committed := stage != "intent-staged"
			if got := strings.Contains(toolText(t, resp), "board committed;"); got != committed {
				t.Fatalf("wrong commitment report: %s", resp)
			}
			text := toolText(t, resp)
			if !strings.Contains(text, `<wb:answer id="999" value="ok">Rejected</wb:answer>`) || !strings.Contains(text, "<wb:failed>") || strings.Contains(text, "nothing was applied") {
				t.Fatal("lost failed payload after persistence error", text)
			}
			if committed && !strings.Contains(text, "applied: 2 accepted commands committed") {
				t.Fatal("committed effects omitted", text)
			}
			if !committed && (!strings.Contains(text, "inspect the board before retrying") || strings.Contains(text, "applied:")) {
				t.Fatal("uncertain commitment claimed application", text)
			}
			if got := len(effectsRecords(t, f, f.replyA)); got != map[bool]int{false: 0, true: 1}[committed] {
				t.Fatalf("effects after %s: %d", stage, got)
			}
			f.server.sessions.boardFault = nil
			board, err := f.server.sessions.readWhiteboard(f.keyA)
			if err != nil {
				t.Fatal(err)
			}
			if committed {
				if board.Whiteboard == nil || len(board.Whiteboard.Items) != 1 || board.Whiteboard.EntrySeq != 3 {
					t.Fatalf("committed board not recovered once: %+v", board.Whiteboard)
				}
				wb.update(t, ``) // no-op update must not replay committed effects
				if len(effectsRecords(t, f, f.replyA)) != 1 {
					t.Fatal("recovery duplicated projection")
				}
			} else if board.Whiteboard != nil {
				t.Fatalf("failed intent install committed a board: %+v", board.Whiteboard)
			}
		})
	}
}

func TestWhiteboardEffectsAppendFailureDoesNotRollbackOrReplay(t *testing.T) {
	f := newLiveFixture(t, true)
	wb := effectsWhiteboardFixture(f)
	attempts := 0
	previous := setLiveSeams(func(seam *liveSeams) {
		seam.appendRing = func(string, []wslive.RingEntry) error { attempts++; return errors.New("append denied") }
	})
	t.Cleanup(func() { setLiveSeams(func(seam *liveSeams) { *seam = previous }) })
	wb.update(t, `<wb:topic name="gap"><wb:add class="check"><wb:title>Still committed</wb:title></wb:add></wb:topic>`)
	if attempts != 1 || len(effectsRecords(t, f, f.replyA)) != 0 {
		t.Fatal("unexpected append attempts/history")
	}
	setLiveSeams(func(seam *liveSeams) { *seam = previous })
	board, err := f.server.sessions.readWhiteboard(f.keyA)
	if err != nil || board.Whiteboard == nil || board.Whiteboard.Counter != 1 {
		t.Fatalf("append failure rolled back board: %+v %v", board.Whiteboard, err)
	}
	wb.update(t, ``)
	if len(effectsRecords(t, f, f.replyA)) != 0 {
		t.Fatal("render-only repair promised projection replay")
	}
}

func TestWhiteboardEffectsAcceptedCommitToRingCrashGap(t *testing.T) {
	f := newLiveFixture(t, true)
	var observed boardMutationObservation
	// Model a process stopping after the synced board decision and before
	// handleWhiteboardUpdate calls recordWhiteboardEffects. No outbox exists.
	err := f.server.sessions.mutateRecordPartsObserved(f.keyA, partAll, partWhiteboard, func(rec *sessionRecord) error {
		_, problems := wswhiteboard.Apply(rec.Whiteboard, `<wb:topic name="gap"><wb:log>Committed without projection</wb:log></wb:topic>`, wswhiteboard.Env{})
		if len(problems) != 0 {
			t.Fatalf("apply: %v", problems)
		}
		return nil
	}, &observed)
	if err != nil || !observed.Committed {
		t.Fatalf("boundaries: %+v %v", observed, err)
	}
	// A new server and render-only call do not reconstruct historical effects.
	f.server = NewServer(f.root, "test")
	effectsWhiteboardFixture(f).update(t, ``)
	if len(effectsRecords(t, f, f.replyA)) != 0 {
		t.Fatal("accepted crash gap was silently closed by replay")
	}
	if _, err := os.Stat(filepath.Join(whiteboardFolder(f.root, f.keyA).Dir, "outbox.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unexpected projection outbox")
	}
}

func TestWhiteboardEffectsConcurrentLegacyRingWriters(t *testing.T) {
	f := newLiveFixture(t, true)
	var wg sync.WaitGroup
	const n = 12
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			f.server.recordWhiteboardEffects(f.keyA, []wswhiteboard.Effect{{Kind: "log", Topic: "one", EntryID: "e1", Field: "text", After: "effect"}})
		}()
		go func() {
			defer wg.Done()
			if err := wslive.AppendRing(wslive.RingDir(f.sessionDir(f.replyA)), []wslive.RingEntry{{Kind: wslive.RingUser, Text: "legacy"}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if all := readLiveRing(t, f.sessionDir(f.replyA)); len(all) != n*2 || len(effectsRecords(t, f, f.replyA)) != n {
		t.Fatalf("concurrent ring entries lost: %d", len(all))
	}
}
