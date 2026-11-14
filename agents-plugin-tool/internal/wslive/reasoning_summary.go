package wslive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"unicode/utf8"
)

const (
	ReasoningSummaryCapability = "reasoning_summary_v1"
	SummaryTextBytes = 8 << 10
	SummaryEnvelopeBytes = 24 << 10
	SummaryFrameBytes = 49152 // Complete UTF-8 JSONL record, including LF.
	SummaryAttemptCap = 16
	SummaryPartCap = 64
	SummaryStreaming = "streaming"
	SummaryComplete = "complete"
	SummaryAborted = "aborted"
	SummaryFailed = "failed"
	SummaryIncomplete = "incomplete"
	maxSummaryCounter int64 = 1<<53 - 1
)

// SummaryPair identifies a bound transport attempt, not just a request hook.
type SummaryPair struct {
	Request int64 `json:"request"`
	Attempt int64 `json:"attempt"`
}

// ReasoningSummary is a whole bounded, stage-free current-turn projection.
// Raw provider events, response/item IDs and private thinking never belong here.
type ReasoningSummary struct {
	V int `json:"v"`
	Producer string `json:"producer"`
	Turn int64 `json:"turn"`
	Revision int64 `json:"revision"`
	ActiveRequest *SummaryPair `json:"activeRequest"`
	Truncated bool `json:"truncated"`
	Requests []SummaryAttempt `json:"requests"`
}

type SummaryAttempt struct {
	Request int64 `json:"request"`
	Attempt int64 `json:"attempt"`
	Provider string `json:"provider"`
	API string `json:"api"`
	Model string `json:"model"`
	Status string `json:"status"`
	Truncated bool `json:"truncated"`
	Parts []SummaryPart `json:"parts"`
}

type SummaryPart struct {
	Output int64 `json:"output"`
	Part int64 `json:"part"`
	Text string `json:"text"`
	ObservedBytes int64 `json:"observedBytes"`
	RetainedBytes int64 `json:"retainedBytes"`
	Truncated bool `json:"truncated"`
	Status string `json:"status"`
}

// SummaryHistory is optional metadata on an actual natural RingTurn only.
type SummaryHistory struct {
	V int `json:"v"`
	Commit string `json:"commit"`
	Requests []SummaryAttempt `json:"requests"`
}

var summaryProducerPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func summaryPositive(n int64) bool { return n>0 && n<=maxSummaryCounter }
func summaryIndex(n int64) bool { return n>=0 && n<=maxSummaryCounter }
func summaryStatus(s string) bool {
	switch s { case SummaryStreaming,SummaryComplete,SummaryAborted,SummaryFailed,SummaryIncomplete: return true }; return false
}

// ValidateReasoningSummary rejects, rather than repairs, untrusted projections.
// Keeping all aggregate bounds here also protects hosted widgets and history UI.
func ValidateReasoningSummary(s *ReasoningSummary) error {
	if s==nil || s.V!=1 || !summaryProducerPattern.MatchString(s.Producer) || !summaryPositive(s.Turn) || !summaryPositive(s.Revision) { return fmt.Errorf("invalid reasoning summary identity") }
	if len(s.Requests)>SummaryAttemptCap { return fmt.Errorf("reasoning summary exceeds aggregate attempt bound") }
	parts,textBytes:=0,0
	var previous SummaryPair
	active:=s.ActiveRequest==nil
	for _,a:=range s.Requests {
		pair:=SummaryPair{a.Request,a.Attempt}
		if !summaryPositive(a.Request) || !summaryPositive(a.Attempt) || a.Request<previous.Request || (a.Request==previous.Request && a.Attempt<=previous.Attempt) { return fmt.Errorf("invalid reasoning summary attempt order") }
		previous=pair
		if s.ActiveRequest!=nil && pair==*s.ActiveRequest { active=true }
		if !((a.Provider=="openai" && a.API=="openai-responses") || (a.Provider=="openai-codex" && a.API=="openai-codex-responses")) || a.Model=="" || len(a.Model)>ModelRunes*4 || utf8.RuneCountInString(a.Model)>ModelRunes || !utf8.ValidString(a.Model) || !summaryStatus(a.Status) { return fmt.Errorf("invalid reasoning summary source or status") }
		var output,part int64 = -1,-1
		for _,p:=range a.Parts {
			parts++; textBytes+=len(p.Text)
			if !summaryIndex(p.Output) || !summaryIndex(p.Part) || p.Output<output || (p.Output==output && p.Part<=part) || !utf8.ValidString(p.Text) || !summaryIndex(p.ObservedBytes) || p.RetainedBytes!=int64(len(p.Text)) || p.ObservedBytes<p.RetainedBytes || (p.ObservedBytes!=p.RetainedBytes && !p.Truncated) || !summaryStatus(p.Status) || (p.Truncated && (!a.Truncated || !s.Truncated)) { return fmt.Errorf("invalid reasoning summary part") }
			output,part=p.Output,p.Part
		}
		if a.Truncated && !s.Truncated { return fmt.Errorf("missing aggregate truncation") }
	}
	if !active || parts>SummaryPartCap || textBytes>SummaryTextBytes { return fmt.Errorf("reasoning summary exceeds bounds or has dangling active attempt") }
	raw,err:=json.Marshal(s)
	if err!=nil || len(raw)>SummaryEnvelopeBytes { return fmt.Errorf("reasoning summary exceeds serialized envelope") }
	return nil
}

func cloneSummary(s *ReasoningSummary) *ReasoningSummary {
	if s==nil { return nil }
	out:=*s
	if s.ActiveRequest!=nil { pair:=*s.ActiveRequest; out.ActiveRequest=&pair }
	out.Requests=append([]SummaryAttempt{},s.Requests...)
	for i:=range out.Requests { out.Requests[i].Parts=append([]SummaryPart{},s.Requests[i].Parts...) }
	return &out
}

type summaryWatermark struct { turn,revision int64; closed bool }
type summaryState struct {
	current *ReasoningSummary
	// Reset never clears these watermarks. Capacity fails closed instead of
	// forgetting a retired producer and permitting a stale source closure.
	seen map[string]summaryWatermark
	producer string
	lastProducer string
	retired map[string]bool
	resync bool
}

func (st *State) checkSummary(ev Event) error {
	s:=ev.ReasoningSummary
	if s==nil { return nil }
	if ev.AgentID!="" || (ev.Kind!=KindAgent && ev.Kind!=KindTurnEnd) { return fmt.Errorf("reasoning summary is lead agent/real turn_end only") }
	if err:=ValidateReasoningSummary(s); err!=nil { return err }
	if st.turnStartedAt==0 { return fmt.Errorf("reasoning summary needs an actually open turn") }
	if st.summary.retired[s.Producer] { return fmt.Errorf("reasoning summary producer retired") }
	if seen,ok:=st.summary.seen[s.Producer]; ok {
		if seen.closed && s.Turn<=seen.turn || s.Turn<seen.turn || s.Revision<seen.revision { return fmt.Errorf("stale reasoning summary") }
		if s.Revision==seen.revision {
			if st.summary.current==nil || !reflect.DeepEqual(s,st.summary.current) { return fmt.Errorf("conflicting reasoning summary revision") }
		}
	} else if len(st.summary.seen)>=256 { return fmt.Errorf("reasoning summary producer capacity exhausted") }
	if st.summary.producer!="" && st.summary.producer!=s.Producer { return fmt.Errorf("reasoning summary producer retired until real turn boundary") }
	if current:=st.summary.current; current!=nil && (s.Producer!=current.Producer || s.Turn!=current.Turn) { return fmt.Errorf("overlapping reasoning summary source turn") }
	return nil
}

func (st *State) acceptSummary(s *ReasoningSummary) {
	if s==nil { return }
	if st.summary.seen==nil { st.summary.seen=map[string]summaryWatermark{} }
	if previous:=st.summary.lastProducer; previous!="" && previous!=s.Producer {
		if st.summary.retired==nil { st.summary.retired=map[string]bool{} }
		st.summary.retired[previous]=true
	}
	st.summary.lastProducer=s.Producer
	st.summary.current=cloneSummary(s)
	st.summary.producer=s.Producer
	st.summary.seen[s.Producer]=summaryWatermark{turn:s.Turn,revision:s.Revision}
}

func (st *State) beginSummaryTurn() {
	if !st.summary.resync && st.summary.current!=nil {
		s:=st.summary.current
		st.summary.seen[s.Producer]=summaryWatermark{turn:s.Turn,revision:s.Revision,closed:true}
		st.summary.current=nil
		st.summary.producer=""
	}
	st.summary.resync=false
}

// SummaryCommit binds the receiver reply ID to real source identity. The
// runner's holder/start sequence is supplied separately, never by a widget.
func SummaryCommit(replyID, source string) string {
	h:=sha256.Sum256([]byte(replyID+"\x00"+source))
	return hex.EncodeToString(h[:])
}

func (st *State) finishSummary(reason string) *SummaryHistory {
	s:=st.summary.current
	if s==nil { return nil }
	st.summary.seen[s.Producer]=summaryWatermark{turn:s.Turn,revision:s.Revision,closed:true}
	st.summary.current=nil
	st.summary.producer=""
	requests:=cloneSummary(s).Requests
	terminal:=""
	switch reason { case TurnAborted: terminal=SummaryAborted; case "error", "failed": terminal=SummaryFailed; case "length", "incomplete": terminal=SummaryIncomplete }
	if terminal!="" {
		for i:=range requests {
			if requests[i].Status!=SummaryStreaming { continue }
			requests[i].Status=terminal
			for j:=range requests[i].Parts { if requests[i].Parts[j].Status==SummaryStreaming { requests[i].Parts[j].Status=terminal } }
		}
	}
	// A successful outer settlement alone never fabricates provider completion.
	// Only actual error/abort/length facts can terminalize an unfinished prefix.
	return &SummaryHistory{V:1,Commit:SummaryCommit(st.replyID,fmt.Sprintf("%s/%d",s.Producer,s.Turn)),Requests:requests}
}
