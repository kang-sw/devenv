package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
)

func FormatConfigView(view wsconfig.View) string {
	return formatConfigView(view)
}

func FormatGitStatus(result wsgit.StatusResult) string {
	return formatGitStatus(result)
}

func FormatGitLog(result wsgit.LogResult) string {
	return formatGitLog(result)
}

func FormatGitMergeBase(result wsgit.MergeBaseResult) string {
	return formatMergeBase(result)
}

func FormatGitCommit(result wsgit.CommitResult) string {
	return formatGitCommit(result)
}

func FormatTickets(tickets []wsdoc.TicketInfo) string {
	return formatTickets(tickets)
}

func FormatTicketMutate(verb string, result wsdoc.TicketMutateResult) string {
	return formatTicketMutate(verb, result)
}

func FormatTicketVerify(result wsdoc.VerifyResult) string {
	return formatTicketVerify(result)
}

// VerifyAdapter exposes verifyAdapter for CLI wiring parity with the MCP
// git.commit dispatch case: both entry points inject the same
// wsgit.Client.Verifier adapter so a raw `ws-mcp git commit` never bypasses
// the guardrails `ws-mcp serve`'s git.commit tool enforces (see
// {#260720-wsdoc-commit-boundary}).
func VerifyAdapter(root string, paths []string) ([]string, error) {
	return verifyAdapter(root, paths)
}

func FormatTicketCreate(res wsdoc.TicketCreateResult) string {
	return formatTicketCreate(res)
}
