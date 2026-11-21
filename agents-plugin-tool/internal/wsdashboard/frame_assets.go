package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// buildFrameFiles keeps the frame's immutable resources beside the existing
// main script. These URLs deliberately retain the session page's access gate;
// unlike sandboxed topic documents, the session frame has the host's cookie.
// Shell, index and non-frame renderers continue to use the original fields.
func (a *dashboardAssets) buildFrameFiles() {
	a.files = map[string]dashboardFile{}
	add := func(body, ext string) string {
		hash := sha256.Sum256([]byte(body))
		name := "dashboard-" + hex.EncodeToString(hash[:12]) + "." + ext
		ctype := "text/javascript; charset=utf-8"
		if ext == "css" {
			ctype = "text/css; charset=utf-8"
		}
		a.files[name] = dashboardFile{body: body, ctype: ctype}
		return name
	}
	d := &a.data
	d.ScriptFile = add(string(d.Script), "js")
	d.FrameWhiteboardFile = add(string(d.WhiteboardCSS), "css")
	d.FrameCSSFile = add(string(d.CSS)+"\n"+string(d.TrackerCSS), "css")
	d.FrameXtermFile = add(string(d.XtermCSS), "css")
	mermaid, _ := json.Marshal(d.MermaidFile)
	// Preserve the former classic scripts' source order and existing wrappers.
	// The explicit separator also protects a trailing line comment/ASI site.
	d.FrameScriptFile = add(strings.Join([]string{
		string(d.ConfirmScript),
		string(d.AnswersScript),
		"window.wbForm = function () {\n" + string(d.FormScript) + "};\nwindow.wbMermaid = function () {\n" + string(d.MermaidInit) + "};\nwindow.wbMermaidFile = " + string(mermaid) + ";",
		string(d.ViewerScript), string(d.ToastScript), string(d.TrackerScript),
		string(d.MentionScript), string(d.RailScript), string(d.MemoScript),
		string(d.UsageScript), string(d.ShellScript), string(d.CardsScript),
		string(d.OverviewScript), string(d.PreviewScript), string(d.FilesScript),
	}, "\n;\n"), "js")
	d.FrameTermFile = add(string(d.TerminalScript), "js")
	d.FrameWebFile = add(string(d.WebScript), "js")
	d.FrameSelectFile = add(string(d.SelectAllScript), "js")
	d.SettingsCSSFile = add(string(d.SettingsCSS), "css")
	d.SettingsScriptFile = add(string(d.ShellScript)+"\n;\nwindow.WsSettingsCatalog = "+string(d.SettingsCatalog)+";\n"+string(d.SettingsScript), "js")
	d.FrameI18nFiles = map[string]string{}
	d.SettingsI18nFiles = map[string]string{}
	for _, lang := range wsi18n.Languages() {
		d.FrameI18nFiles[lang.Code] = add(string(wsi18n.ScriptExcept(lang.Code, "js.settings.")), "js")
		d.SettingsI18nFiles[lang.Code] = add(string(wsi18n.SubsetScript(lang.Code, "js.settings.")), "js")
	}
}
