package wsdashboard

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

var configNoLead = configError{409, "no_active_lead", "There is no active lead in this worktree."}

type configProposalRequest struct {
	Target     string                      `json:"target"`
	Operations []wsconfigservice.Operation `json:"operations"`
}

type configProposalMessage struct {
	configProposalRequest
	Path    string `json:"path"`
	Request string `json:"request"`
}

func (d *Daemon) handleConfigProposal(w http.ResponseWriter, r *http.Request) {
	if !configRouteMethod(w, r, http.MethodPost) {
		return
	}
	if local, own := TrustedWrite(r); !local || !own {
		writeConfigError(w, configForbidden)
		return
	}
	var request configProposalRequest
	if !decodeConfigRequest(w, r, &request) || len(request.Operations) == 0 {
		writeConfigError(w, badConfigRequest)
		return
	}
	selection := configSelection{Scope: wsconfig.ScopeRepo, Target: request.Target}
	resolved, failure := d.resolveConfig(selection)
	if failure.status != 0 {
		writeConfigError(w, failure)
		return
	}
	seen := make(map[string]bool, len(request.Operations))
	for _, op := range request.Operations {
		mutation, err := d.config.Catalog.Validate(wsconfig.ScopeRepo, op)
		if err != nil || seen[mutation.StoredKey] {
			writeConfigError(w, badConfigRequest)
			return
		}
		seen[mutation.StoredKey] = true
	}
	message := configProposalMessage{configProposalRequest: request, Path: *resolved.path, Request: "Review and, if appropriate, apply these settings to this worktree's repo config."}
	body, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		writeConfigError(w, configUnavailable)
		return
	}
	if failure := d.deliverConfigProposal(resolved.targetRoot, string(body)); failure.status != 0 {
		writeConfigError(w, failure)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"delivered": true})
}

func (d *Daemon) deliverConfigProposal(root, body string) configError {
	if d.index == nil {
		return configNoLead
	}
	var chosen Registration
	// Index.Sessions is already sorted by project and then reply ID.
	for _, session := range d.index.Sessions() {
		if session.Live && configLeadCandidate(session.Registration, root) &&
			(d.config.IsLead == nil || d.config.IsLead(session.Registration)) {
			chosen = session.Registration
			break
		}
	}
	if chosen.ReplyID == "" {
		return configNoLead
	}
	// A lead can end between the index lookup and the mailbox append. Read
	// its current registration under the same lock used by its writers.
	current, err := ReadRegistration(d.dir, chosen.ReplyID)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && (!current.Live(time.Now()) || !configLeadCandidate(current, root))) {
		return configNoLead
	}
	if err != nil {
		return configUnavailable
	}
	unlock, err := LockRegistration(d.dir, chosen.ReplyID)
	if err != nil {
		return configUnavailable
	}
	defer unlock()
	current, err = ReadRegistration(d.dir, chosen.ReplyID)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && (!current.Live(time.Now()) ||
		!configLeadCandidate(current, root) || (d.config.IsLead != nil && !d.config.IsLead(current)))) {
		return configNoLead
	}
	if err != nil {
		return configUnavailable
	}
	store := (wsmailbox.Paths{State: wsstate.Options{CacheHome: filepath.Dir(d.dir)}}).Store()
	if err := store.PostSystem(current.ReplyID, wsmailbox.SystemLabel, body, time.Now()); err != nil {
		return configUnavailable
	}
	if d.mailWake != nil {
		d.mailWake.Wake()
	}
	return configError{}
}

func configLeadCandidate(reg Registration, root string) bool {
	return !reg.Unlisted && !reg.Agent && reg.ParentReplyID == "" && filepath.Clean(reg.Project) == filepath.Clean(root)
}
