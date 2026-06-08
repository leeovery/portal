TASK: session-tagging-and-grouping-3-5 — Mode-aware title via SessionListTitle()

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: Flat title "Sessions" unchanged; By Project "Sessions — by project"; By Tag "Sessions — by tag"; updates on s mode change; updates on SessionsMsg refresh; inside-tmux current-session decoration preserved alongside mode suffix (reconciled, documented).

SPEC CONTEXT: spec §182-187 three base title strings (em-dash U+2014); spec silent on inside-tmux "(current: %s)" so plan mandated documented reconciliation, not blind overwrite.

IMPLEMENTATION: Implemented.
- model.go:670-684 sessionListTitleForMode(mode, insideTmux, currentSession) — base mapping switch (default Flat), composes (current: %s) only when insideTmux && currentSession!=""; base strings byte-for-byte. :1066 title set inside rebuildSessionList (single chokepoint — s-toggle, SessionsMsg, WithInsideTmux all inherit). :862-866 New recomputes title (initial injected mode paints right). Old per-site rewrite removed; divergence-reconciliation comment :664-669.

TESTS: Adequate. session_list_title_test.go — TestSessionListTitleForMode (pure table, 6 cases incl. inside-tmux compose/empty-current drop); TestSessionListTitleModeAware (6 subtests through real rebuildSessionList/Update: base titles, updates on mode change via keyS, updates on SessionsMsg, decoration+suffix inside tmux). Complementary layers, no redundancy.

CODE QUALITY: Conventions followed (functional-option+chokepoint, em-dash centralised, no t.Parallel); SOLID good (pure function, DRY removed 3-site dup); low complexity; idiomatic switch. Documented rationale. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
