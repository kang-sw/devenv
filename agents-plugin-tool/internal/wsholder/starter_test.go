package wsholder

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The starter is this test binary run again as a stand-in for a daemon
// process: it starts a holder with a ticking child, prints the holder's id,
// and then hangs until the test kills it, as a crashed daemon goes.
const (
	starterDirEnv    = "WSHOLDER_TEST_STARTER_DIR"
	starterPythonEnv = "WSHOLDER_TEST_STARTER_PYTHON"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(starterDirEnv); dir != "" {
		runStarter(dir, os.Getenv(starterPythonEnv))
		return
	}
	os.Exit(m.Run())
}

func runStarter(dir, python string) {
	m := New(Options{Dir: dir, Python: python, holderArgs: []string{"--idle-exit-seconds", "60", "--stop-grace-seconds", "1"}})
	id, err := m.Launch(nil)
	if err == nil {
		_, err = m.Client(id).Spawn(SpawnSpec{Argv: []string{python, "-c", tickChild}, Mode: "pipe"})
	}
	if err != nil {
		fmt.Println("error: " + err.Error())
		os.Exit(1)
	}
	fmt.Println("holder: " + id)
	select {}
}

// TestHolderOutlivesTheProcessThatStartedIt pins the survival the holder
// exists for at the process level: the process that started a holder is
// killed outright, and the holder and its child keep running, the child's
// output keeps landing in the ring, and another process attaches and reads
// all of it.
func TestHolderOutlivesTheProcessThatStartedIt(t *testing.T) {
	python := testPython(t)
	dir := testHoldersDir(t)
	starter := exec.Command(os.Args[0], "-test.run=^$")
	starter.Env = append(os.Environ(), starterDirEnv+"="+dir, starterPythonEnv+"="+python)
	stdout, err := starter.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := starter.Start(); err != nil {
		t.Fatal(err)
	}
	killed := false
	defer func() {
		if !killed {
			_ = starter.Process.Kill()
			_ = starter.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	id, ok := strings.CutPrefix(strings.TrimSpace(line), "holder: ")
	if err != nil || !ok {
		t.Fatalf("the starter reported %q (%v)", strings.TrimSpace(line), err)
	}
	desc, err := ReadDescriptor(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}

	// The starter dies without a word to the holder.
	if err := starter.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = starter.Wait()
	killed = true
	time.Sleep(500 * time.Millisecond)
	if !processAlive(desc.PID) {
		t.Fatalf("the holder (pid %d) ended with the process that started it", desc.PID)
	}

	sink := newCollector()
	m := testManager(t, dir, sink)
	m.Scan()
	holders := m.List().Holders
	if len(holders) != 1 || !holders[0].Attached || holders[0].Child == nil || holders[0].Child.State != "running" {
		t.Fatalf("holders after the starter died = %+v, want %s attached with its child running", holders, id)
	}
	if !processAlive(holders[0].Child.PID) {
		t.Fatalf("the child (pid %d) is not alive", holders[0].Child.PID)
	}
	// At 50 ms a tick, ten or more were written after the kill, with no
	// process attached.
	waitFor(t, "the output written while nothing was attached", func() bool {
		return strings.Contains(sink.output(id), "tick 12")
	})
	if s := sink.seqs(id); !contiguousFrom(s, 1) {
		t.Fatalf("sequences = %v, want the whole ring from 1 without holes", s)
	}
	childPID := holders[0].Child.PID
	if err := m.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitFor(t, "the holder and its child to end", func() bool { return !processAlive(desc.PID) && !processAlive(childPID) })
}
