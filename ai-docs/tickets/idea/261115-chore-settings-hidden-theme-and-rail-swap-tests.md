---
title: Cover hidden-frame theme and open Settings rail replacement
related:
  261115-feat-ws-dashboard-settings-in-usage-strip: Minor independent Lite review follow-up
---

# Cover hidden-frame theme and open Settings rail replacement

## Background

The Settings relocation Lite review identified two remaining coverage gaps, not a demonstrated product defect: changing theme while a cached session frame is hidden, and replacing the shared rail while Settings is open before Escape/focus return. Hidden-frame layout/input changes, denied/full storage and actual send-key behavior are already covered in `agents-plugin-tool/internal/wsdashboard/testdata/settings-root.browser.cjs`.

## Phases

### Phase 1: Extend the isolated Settings browser contract

Add focused assertions for hidden-frame theme propagation and a real rail-region replacement with the Settings dialog open, including trigger re-identification and Escape/focus return. Reuse the isolated fixture and cached browser; do not add screenshot matrices, whole-suite gates or shared-runtime operations. Keep production behavior unchanged unless a separately diagnosed defect requires a fix.
