// Tests for board db lifecycle.
package wswhiteboard

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBoardDBCloseCheckpointsAndNoSharedMemory(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	for pragma, want := range map[string]string{
		"locking_mode":       "exclusive",
		"journal_mode":       "wal",
		"synchronous":        "1",
		"wal_autocheckpoint": "0",
		"cache_size":         "-256",
	} {
		var got string
		if err := board.db.QueryRow(`PRAGMA ` + pragma).Scan(&got); err != nil || got != want {
			t.Fatalf("PRAGMA %s = %q, %v; want %q", pragma, got, err, want)
		}
	}
	if err := board.Update(func(state *State) error { state.Seq++; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, boardDBName+"-shm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exclusive mode created shm: %v", err)
	}
	if err := board.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	openTestBoardDB(t, dir, nil, BoardDBOptions{})
}

func TestBoardDBProcessKillRecovery(t *testing.T) {
	if stage := os.Getenv("WS_BOARD_DB_KILL_STAGE"); stage != "" {
		board, err := OpenBoardDB(os.Getenv("WS_BOARD_DB_KILL_DIR"), nil, BoardDBOptions{Observe: func(got string) {
			if got == stage {
				fmt.Println("board-kill-ready")
				select {} // Parent kills this process without transaction/handle cleanup.
			}
		}})
		if err != nil {
			t.Fatal(err)
		}
		defer board.Close()
		if err := board.Update(func(state *State) error {
			state.Seq += 1000
			state.Topics[0].BodyTimeProof.Digest = "uncommitted-proof"
			state.EntrySeq++
			state.PanelCounter++
			state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{
				ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryPanel,
				PanelID: fmt.Sprintf("p%d", state.PanelCounter), Text: strings.Repeat("uncommitted", 100000),
			})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		t.Fatal("child did not reach kill stage")
	}
	for _, stage := range []string{"import-committed", "transaction-before-commit"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			want := makeLegacyBoard(t, dir, boardDBFixture())
			if stage == "transaction-before-commit" {
				board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
				if err := board.Update(folderAdd); err != nil {
					t.Fatal(err)
				}
				want = board.State()
				if err := board.Close(); err != nil {
					t.Fatal(err)
				}
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBoardDBProcessKillRecovery$")
			cmd.Env = append(os.Environ(), "WS_BOARD_DB_KILL_STAGE="+stage, "WS_BOARD_DB_KILL_DIR="+dir)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := false
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if scanner.Text() == "board-kill-ready" {
					ready = true
					break
				}
			}
			killErr := cmd.Process.Kill()
			waitErr := cmd.Wait()
			if !ready || killErr != nil || waitErr == nil {
				t.Fatalf("child ready=%v kill=%v wait=%v stderr=%s", ready, killErr, waitErr, stderr.String())
			}
			if stage == "import-committed" {
				if has, err := boardHasSentinel(dir); err != nil || has {
					t.Fatalf("pre-recovery sentinel = %v, %v", has, err)
				}
			}
			reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
			if got := reopened.State(); !reflect.DeepEqual(got, want) {
				t.Fatalf("killed transaction escaped\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			if has, err := boardHasSentinel(dir); err != nil || !has {
				t.Fatalf("post-recovery sentinel = %v, %v", has, err)
			}
		})
	}
}

func TestBoardDBBackgroundCheckpointWaitsForIdleAndWALThreshold(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if board.checkpointTimer == nil {
		t.Fatal("import did not schedule an idle checkpoint")
	}
	before := folderRead(t, board.path)
	board.mu.Lock()
	board.checkpointDue = time.Now().Add(-time.Second)
	board.mu.Unlock()
	board.backgroundCheckpoint()
	if after := folderRead(t, board.path); !bytes.Equal(before, after) {
		t.Fatal("background checkpoint ran below WAL threshold")
	}
	if err := board.Update(func(state *State) error {
		state.EntrySeq++
		state.PanelCounter++
		state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{
			ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryPanel,
			PanelID: fmt.Sprintf("p%d", state.PanelCounter), Text: strings.Repeat("x", BoardDBCheckpointWALBytes+1),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(board.path + "-wal")
	if err != nil || info.Size() < BoardDBCheckpointWALBytes {
		t.Fatalf("large WAL fixture: %v, %v", info, err)
	}
	before = folderRead(t, board.path)
	board.backgroundCheckpoint() // A stale timer callback after the new write.
	if after := folderRead(t, board.path); !bytes.Equal(before, after) {
		t.Fatal("background checkpoint ignored the latest write's idle deadline")
	}
	board.mu.Lock()
	board.checkpointDue = time.Now().Add(-time.Second)
	board.mu.Unlock()
	board.backgroundCheckpoint()
	if after := folderRead(t, board.path); bytes.Equal(before, after) {
		t.Fatal("idle large WAL was not checkpointed")
	}
	want := board.State()
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if reopened.checkpointTimer == nil || !reflect.DeepEqual(reopened.State(), want) {
		t.Fatal("reopen lost state or did not schedule idle checkpoint")
	}
}
