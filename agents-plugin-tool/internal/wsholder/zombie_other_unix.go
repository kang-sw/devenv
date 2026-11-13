//go:build !linux && !windows

package wsholder

func processZombie(int) bool { return false }
