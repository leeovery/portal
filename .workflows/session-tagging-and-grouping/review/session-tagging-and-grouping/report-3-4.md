TASK: session-tagging-and-grouping-3-4 — `s` cycle key handler (Flat → By Project → By Tag → Flat)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: cycle wraps By Tag → Flat; unconditional on zero sessions; unconditional on zero tags (lands ByTag with signpost, not skipped); s literal while filter focused; persist once per press; persist failure non-fatal.

SPEC CONTEXT: spec § Toggle key — unconditional cycle on tag/session count; s free on sessions page; s literal filter char while filter focused; persist each press via AtomicWrite, tolerant of failure; signposted state persists as by-tag.

IMPLEMENTATION: Implemented.
- model.go:958-969 nextSessionListMode (pure cycle, default→Flat); :2001-2002 s rune case; :2028-2040 handleSwitchViewKey (advance, rebuild via (&m).rebuildSessionList(), persist once through nil-tolerant modePersister seam, swallowed error); :1955-1957 SettingFilter() guard above rune switch. s case below filter guard with load-bearing comment. No session/tag-count gating. byTagSignpost recomputed in rebuildSessionList (decoupled).

TESTS: Adequate. switch_view_key_test.go — full cycle + out-of-range→Flat; zero sessions; zero tags advances ByTag; persist-once (count 1→2); no-persist on SessionsMsg; s literal while filtering; persist failure non-fatal; nil persister tolerated; re-renders into new mode. Fake persister minimal. Behaviour-focused.

CODE QUALITY: Conventions followed (small ModePersister seam, value-receiver+pointer-method call, nil tolerance, no t.Parallel); SOLID good (pure nextSessionListMode unit-tested, orchestration separated, DI seam); low complexity; explicit _= discard with justification. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
