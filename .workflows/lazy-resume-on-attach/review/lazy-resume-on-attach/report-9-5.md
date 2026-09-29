TASK: Waiting Panes Preview Their Saved Transcript (lazy-resume-on-attach-9-5, tick-576aff)

ACCEPTANCE CRITERIA:
- A waiting pane that never moved has been re-filed under its token. Space on its session shows the pane's saved transcript, the bytes in its token-named file, not `(no saved content)`.
- A waiting pane that has moved to a different address since it was filed shows the same saved transcript.
- A waiting pane not yet re-filed, with no token-named file on disk, previews its positional file as today.
- A waiting pane carrying no token, and every pane that is not waiting, previews its positional file as today.
- Opening the preview still costs its one enumeration and each focus no tmux read: `TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads` holds, and `internal/tui` makes no `os.Stat` call (`TestConstruction_ReadsNoThemesDirectory`).
- The enumeration still returns every window and pane of the session in today's order, and now also carries each pane's token and whether it waits, from the same single `list-panes` call.

STATUS: complete

SPEC CONTEXT: Section 7.2 (as corrected by the 2026-09-29 corrigendum, specification.md:312 and :468) says the re-file moves every tokened waiting pane's bytes to `scrollback/pane-<token>.bin`, moved or not. The picker preview therefore resolves a waiting pane through its token. Its single enumeration carries each pane's token and whether it waits. For a waiting pane whose token passes the pane-token rule, it reads the token-named file first and falls back to the positional file only when no token-named file exists yet. A token-named file that exists but cannot be read is reported as unreadable, with no fallback, because the positional address can hold another pane's capture. Panes that are not waiting, or that carry no token, preview their positional file as before. No preview artifact is accepted for a waiting pane.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:571-595. `WindowGroup.Panes []WindowPane{Index, Token, Pending}` replaces `PaneIndices []int`. `listWindowsAndPanesFormat` adds `HookKeyFormat` and `#{` + `state.ResumePendingOption` + `}` to the one existing `list-panes -s -t =<session>:` call.
  - internal/tmux/tmux.go:603-643. Grouping and ordering are unchanged: windows sort by index, then panes by `Index`.
  - internal/tmux/tmux.go:646-661. `parseWindowPaneRow` requires exactly 5 fields and reads presence through `state.ResumePendingSet`.
  - internal/tui/preview_seams.go:9-26. `PaneScrollback{PaneKey, PendingToken}`; `ScrollbackReader.Tail(pane PaneScrollback)`.
  - internal/tui/pagepreview.go:260-267. `currentScrollback` hands over the token only when the focused pane is `Pending`.
  - internal/tui/pagepreview.go:285. The per-focus read goes through that value.
  - internal/tui/preview_adapter.go:20-31. The adapter reads the token path from `state.PendingScrollbackPath`, which is the same declaration `refilePendingPane` uses (internal/state/scrollback.go:121-124) and is shape-gated. It returns the token read when that read gives bytes or an error, and otherwise reads `state.ScrollbackFile(stateDir, PaneKey)`.
- Notes:
  - A later task, 9-6 (fe09e1099), moved the shape check and the path composition into `state.PendingScrollbackPath`. The adapter and the re-file now share one declaration, so the preview reads the file the re-file wrote by construction.
  - Divergence from the spec wording, not a finding. The fallback fires on `TailScrollback`'s whole `(nil, nil)` class: a missing file, but also a zero-byte file or one holding only an unterminated line. The spec says "only when no token-named file exists yet"; the task's own condition names the `(nil, nil)` shape. No realistic input reaches the difference. A re-filed file is always a daemon-written `capture-pane -e -p -S -` output (internal/tmux/tmux.go:847) or a hard link to one, written through `AtomicWrite`. Every captured row ends in a newline and a pane has at least one row, so such a file is never empty and never partial-line-only.
  - Production `internal/tui` sources contain no `os.Stat`/`os.Lstat` call.
  - The seam moved as the task measured: `rg '\) Tail\('` finds 15 implementations and `rg '\) ListWindowsAndPanesInSession\('` finds 8. No `PaneIndices` field reference remains; two test names still use the word as a description.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: `TestPreviewWaitingPane_UnmovedRefiledPaneShowsItsTokenFile` (internal/tui/pagepreview_waiting_pane_test.go:68). Only the token file is staged, so a positional-only reader would render the placeholder and fail.
  - AC2: `TestPreviewWaitingPane_MovedPaneShowsItsTokenFileNotTheOccupantOfItsAddress` (:80) stages another pane's capture at the current address. A positional-first reader fails it.
  - AC3: `TestPreviewWaitingPane_NotYetRefiledPaneShowsItsPositionalFile` (:93).
  - AC4: the table at :107 covers four cases: a tokenless waiting pane, a width-short token, a directory-escaping token (with a setup check that the escape really lands at `<stateDir>/<token>.bin`), and a tokened pane that is not waiting. Each case stages a file the pane must not be read from, so it would catch a missing shape check or a missing `Pending` gate.
  - The spec's rule that an unreadable token file is an error, not a fallback, is pinned by `TestPreviewWaitingPane_UnreadableTokenFileReportsTheErrorRatherThanTheAddressOccupant` (:163).
  - At model level, `TestPreviewWaitingPane_ReaderIsHandedTheFocusedPanesTokenOnlyWhileItWaits` (:185) pins that the token reaches the reader only while the pane waits.
  - AC5: `TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads` (internal/tui/pagepreview_hermetic_test.go:37) now includes a waiting pane and still asserts one enumeration and 1+6 reads. `TestConstruction_ReadsNoThemesDirectory` (internal/tui/nomination_test.go:173) scans production sources only (`ParsePackageSources(t, ".", false)`), so the `os.Stat` in the test-only `fileExists` helper is correctly outside it.
  - AC6: `TestListWindowsAndPanesInSession` pins the exact five-field `-F` argv of the single call. It keeps the ordering, non-contiguous and base-index subtests and adds three more: token/pending carriage in unsorted input, any non-empty pending value reading as waiting, and a short-row rejection. `TestListWindowsAndPanesInSession_CarriesTokenAndPendingFromRealServer` (internal/tmux/list_windows_and_panes_realtmux_test.go:12) checks an unstamped pane and a stamped, marked pane against a disposable `ptl-` server. The exact-target suite was updated at internal/tmux/exact_session_target_realtmux_test.go:234.
- Notes: The remaining fakes changed mechanically, from `PaneIndices` to `Panes` and from `Tail(string)` to `Tail(PaneScrollback)`; I found no dropped assertion. The tests are focused, and none duplicates another.

CODE QUALITY:
- Project conventions: Followed. The format is composed from the single declarations `HookKeyFormat` / `state.PortalPaneIDOption` and `state.ResumePendingOption`, and presence is read through `state.ResumePendingSet`. The target stays pinned through `CoordTargetExact`. Seam assertions stay in a non-test file (internal/tui/preview_adapter.go:34-37).
- SOLID principles: Good. The model decides which pane's token applies, and the adapter decides which file to read.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments on `WindowPane`, `PaneScrollback` and the adapter hold true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
