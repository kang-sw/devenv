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

func FormatSpecs(specs []wsdoc.SpecInfo) string {
	return formatSpecs(specs)
}

func FormatSpecStatus(status *wsdoc.SpecAnchorStatus) string {
	return formatSpecStatus(status)
}

func FormatTickets(tickets []wsdoc.TicketInfo) string {
	return formatTickets(tickets)
}

func FormatMentalModels(models []wsdoc.MentalModelInfo) string {
	return formatMentalModels(models)
}

func FormatReferenceTrace(trace *wsdoc.ReferenceTrace) string {
	return formatReferenceTrace(trace)
}
