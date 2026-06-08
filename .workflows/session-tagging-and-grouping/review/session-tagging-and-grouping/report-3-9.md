TASK: session-tagging-and-grouping-3-9 — Wire prefs-backed initial mode + persister into TUI construction (open.go Option)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: first-ever launch opens Flat; persisted by-tag opens By Tag; corrupt prefs opens Flat (no hard error); persister writes on toggle end-to-end; nil persister tolerated; tui never imports prefs for I/O; initial mode applied before first ingestion.

SPEC CONTEXT: spec § Mode Persistence — first launch Flat, remember thereafter, persist each toggle via AtomicWrite, tolerant decode → Flat. 3-9 is the integration seam reading prefs.json in cmd layer and injecting mode + persister.

IMPLEMENTATION: Implemented.
- model.go:499-511 WithInitialMode; :513-520 WithModePersister; :852-868 New applies options then recomputes title; :83-85 ModePersister interface (satisfied by *prefs.Store). open.go:355-356 tuiConfig fields; :394-400 buildTUIModel always WithInitialMode, WithModePersister only when non-nil; :457-475 openTUI loads store once, swallows path error to nil, tolerant Load; :519-524 typed-nil guard (only assign when store loaded — avoids boxed typed-nil footgun). tui imports prefs only for SessionListMode value type (no NewStore/Load/Save).

TESTS: Adequate. initial_mode_option_test.go (WithInitialMode sets field, paints title first frame, defaults Flat, groups first SessionsMsg By Tag); open_initial_mode_test.go (InjectsInitialMode Flat/ByTag/ByProject titles; InjectsPersister s calls fake once + nil tolerated; OpenTUI_InitialModeFromPrefs first-launch/persisted/corrupt/round-trip with real Store + PORTAL_PREFS_FILE). All 5 edge cases. Behaviour-focused.

CODE QUALITY: Conventions followed (functional-options DI, t.Setenv, typed-nil guard defence-in-depth); SOLID good (single-method ModePersister ISP, DIP, prefs I/O confined to cmd); low complexity; idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
