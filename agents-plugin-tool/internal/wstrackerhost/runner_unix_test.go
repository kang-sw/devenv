//go:build !windows

package wstrackerhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunnerTimeoutKillsTransportChildren: a git whose transport child
// keeps the output pipes open (a hung ssh or https helper) still returns
// when the context ends, so a check's timeout bounds how long it holds the
// store. The fake git backgrounds a sleeper that inherits the pipes.
func TestRunnerTimeoutKillsTransportChildren(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nsleep 30 &\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, args := range [][]string{{"rev-list", "--count", "HEAD"}, {"-c", "credential.interactive=false", "fetch", "origin"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		_, err := batchRunner{}.RunGit(ctx, t.TempDir(), args...)
		cancel()
		if err == nil {
			t.Fatalf("%v: a killed git must fail", args)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("%v: returned after %s; the timeout must bound a lingering transport child", args, elapsed)
		}
	}
}
