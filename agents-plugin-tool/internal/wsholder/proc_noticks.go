//go:build !linux

package wsholder

// processStartTicks reports no boot-clock start on this OS: zero sends a
// guarded kill to the wall-clock comparison. The creation time the OS keeps
// here is stored at process start, so a later wall-clock step does not move
// it.
func processStartTicks(int) (uint64, error) { return 0, nil }
