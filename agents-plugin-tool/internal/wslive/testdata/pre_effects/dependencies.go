package wslive

import current "github.com/kang-sw/devenv/internal/wslive"

// ring.go is byte-for-byte source from 4ddb1910537678e2e8125ee448a1d26d6f4010ec (before effects). Only
// unchanged dependencies are supplied here; its outer struct and actual
// disk reader/writer execute unmodified, not a test-authored imitation.
const RingCap = current.RingCap
const RingHistoryCap = current.RingHistoryCap

type ToolSummary = current.ToolSummary
type UICommand = current.UICommand
type Notice = current.Notice
