TASK: session-tagging-and-grouping-3-6 — Footer `s switch view` hint on sessions page

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: absent on projects page; footer column layout unbroken; present at all session counts.

SPEC CONTEXT: spec §182-188 — mode string lives in title; footer gains only `s switch view` key hint; sessions-page only (projects page s already means "go to sessions").

IMPLEMENTATION: Implemented (no drift).
- model.go:638 s→"switch view" binding added to sessionHelpKeys(); :788-790 sessionFooterBindings composes nav/filter prefix + sessionHelpKeys (sessions footer only); :709-719 projectHelpKeys unchanged; :2329-2330 viewSessionList always renders footer regardless of count. No footer-side mode-string. Column math tight but correct (15 bindings = 3×5, switch-view 14th lands col 3 within keymapFooterColumnSize). Placement comment :631-637.

TESTS: Adequate. sessions_footer_switch_view_test.go — ShowsSwitchViewHint (helper + full View path); AtZeroSessions; ProjectsFooter_Unchanged (absent + s/x sessions survives); CommandPendingFooter_NoHint; ThreeColumnLayoutIntact (3 columns, no overflow, binding survives within window). Behaviour-focused.

CODE QUALITY: Conventions followed (no t.Parallel, key.NewBinding pattern); SOLID good (sessionHelpKeys vs projectHelpKeys scope clean, DRY shared prefix); low complexity; load-bearing placement comment. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
