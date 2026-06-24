TASK: spectrum-tui-design-1-5 — `appearance: auto|light|dark` pref in prefs.json (default auto, tolerant decode), wired into the TUI at construction

ACCEPTANCE CRITERIA:
- prefs.json round-trips appearance beside session_list_mode; saving one field does not blank the other.
- Store LoadAppearance returns AppearanceAuto for missing file, empty file, corrupt/unparseable content, and an unrecognised appearance value; only a non-ErrNotExist read error propagates.
- The appearance value is read once at TUI construction in cmd/open.go and injected via the new option alongside WithInitialMode.
- session_list_mode load/save behaviour is unchanged (no regression).
- internal/prefs imports only stdlib + internal/fileutil (no internal/log / internal/storelog) — prefs stays a leaf.

STATUS: Complete

SPEC CONTEXT:
§2.6 — the `appearance` override: prefs.json carries `appearance: auto|light|dark` (default auto), beside session_list_mode. `auto` detects with a dark fallback; `light`/`dark` pin the mode and skip detection (and the startup detection wait). It is the recourse for terminals (notably tmux passthrough) where OSC 11 misdetects, and is NOT a second render path. §16.1 lists the `appearance` pref as explicitly in v1 scope. This task only plumbs the pref through to the model; honouring it (skip detection + first-paint wait) is task 1-7 — which, per the codebase, has since landed (internal/tui/appearance_gate.go consumes the pinned value).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/prefs/store.go:74-120 — Appearance enum (AppearanceAuto iota default, Light, Dark), canonical on-disk strings, String() (out-of-range → auto), parseAppearance (unrecognised → auto), all mirroring SessionListMode.
  - internal/prefs/store.go:125-128 — prefsFile gains `Appearance string` json:"appearance" beside the intact SessionListMode field.
  - internal/prefs/store.go:147-164 — readFile() is a shared read-decode helper returning (prefsFile, bool, error) with the tolerant policy; both loaders and both savers route through it.
  - internal/prefs/store.go:188-194 — LoadAppearance() (Appearance, error), separate loader leaving Load()'s signature untouched (the lowest-risk option the task called for).
  - internal/prefs/store.go:201-220 — Save and SaveAppearance both read-modify-write via readFile(), so a one-field save preserves the sibling field.
  - cmd/open.go:499-507 — appearance read once at construction from the SAME prefsStore instance, tolerant (discarded error documented).
  - cmd/open.go:346, 397, 547 — appearance carried through tuiConfig → deps.Appearance → WithAppearance.
  - internal/tui/model.go:704-714 — WithAppearance option stores the value on the model.
- Notes: Clean. The design choice of a separate LoadAppearance (vs widening Load) is explicitly noted in the doc comment, matching the task's "note the choice" instruction. The read-modify-write in Save (newly required by this task to avoid blanking appearance) is correctly in place and its tolerant read-error policy is sound.

TESTS:
- Status: Adequate
- Coverage: internal/prefs/appearance_test.go covers all task-listed cases — missing file, missing field in valid file, empty file, corrupt JSON, unrecognised value (all → auto), non-ErrNotExist read-error propagation (EISDIR via a dir at the path), light/dark/auto round-trip, String() incl. out-of-range(99)→auto, and both cross-field no-blanking directions plus an on-disk both-fields-present assertion. internal/prefs/leaf_guard_test.go enforces the leaf constraint via `go list -deps` with both internal/log and internal/storelog in the forbidden set, plus a positive non-vacuous fileutil check. internal/prefs/store_test.go (pre-existing) is fully intact — every session_list_mode Load/Save/String case still present → no regression.
- Notes: Coverage maps one-to-one onto the task's Tests list. Not over-tested — each case targets a distinct branch. The EISDIR trick for the read-error branch is a legitimate, portable way to exercise non-ErrNotExist propagation. Tests correctly use the external `prefs_test` package (black-box).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); table-driven where natural; doc comments on every exported symbol; the leaf-package contract is documented in the package comment AND enforced by a guard test, matching CLAUDE.md's description of prefs as a deliberate leaf.
- SOLID principles: Good. readFile() is a single shared chokepoint (DRY) for the four public methods; Appearance/SessionListMode are parallel, self-contained enums.
- Complexity: Low. Straight switch-based parse/stringify; no branching beyond the tolerant-decode arms.
- Modern idioms: Yes. errors.Is(err, os.ErrNotExist), json.MarshalIndent, fileutil.AtomicWrite reuse.
- Readability: Good. Intent-revealing comments explain the tolerant-decode rationale and the separate-loader / read-modify-write design choices.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/prefs/store.go:147-194 — the two loaders (Load / LoadAppearance) each do a full file read; the TUI construction path (cmd/open.go:497,506) calls both back-to-back, reading + decoding prefs.json twice per launch. A combined LoadAll() (SessionListMode, Appearance, error) would halve the I/O. The task explicitly preferred the separate-loader form as lowest-risk, so this is a deliberate trade-off, not a defect — recorded only as a possible future consolidation. Decide whether the extra read matters before acting.
- [do-now] internal/prefs/store.go:196-200 — Save's doc comment says "It read-modify-writes so a previously-persisted appearance is preserved"; the readFile() doc (147-164) already covers the bool return's read-modify-write purpose, but Save/SaveAppearance ignore that bool (they always re-read fresh). The comment is accurate; consider a one-line note that the bool is intentionally unused here (the fresh re-read is the preservation mechanism) to pre-empt a "why is the bool discarded" reader question.
