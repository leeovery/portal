TASK: One Declaration of Where a Waiting Pane's Transcript Lives (lazy-resume-on-attach-9-6, tick-eaf350)

ACCEPTANCE CRITERIA:
- A waiting pane whose token the pane-token mint produced, with its bytes still at its positional file, is re-filed on the next capture exactly as today: its record names `scrollback/pane-<token>.bin`, the bytes sit in that file, and the `TestRefilePendingScrollback` and `internal/state/capture_refile_test.go` suites pass unchanged
- A waiting pane whose token is absent, too short, too long or outside the id alphabet keeps its positional record, its positional file and its dedup entry through a re-file, with nothing logged, as today
- Pressing Space on a session holding a waiting pane previews what it previews today in every case the `TestPreviewWaitingPane_*` suite pins
- `state.PendingScrollbackPath(dir, token)` for a minted token answers ok, with a path naming the file under `dir` that the saver's re-file moves that pane's bytes into
- `state.PendingScrollbackPath` answers not-ok for an empty token, a token one character short, and a token that climbs out of the scrollback directory (`/../../<token>`)
- Rule form: the token-shape gate and the join of the token-named file onto the state directory are declared once, in `PendingScrollbackPath` — `internal/tui/preview_adapter.go` calls neither `nanoid.IsTokenShaped` nor `state.PendingScrollbackFile`, and `refilePendingPane` calls no `nanoid.IsTokenShaped` of its own

STATUS: complete

SPEC CONTEXT: The spec's section on frozen panes has a waiting pane's bytes re-filed under `scrollback/pane-<PortalPaneID>.bin` for as long as it waits. The picker preview resolves a waiting pane through its token: "for a waiting pane whose token is token-shaped — the same rule that decides which panes the re-file moves". It reads the token-named file first and falls back to the positional file only when no token-named file exists yet. An unreadable token-named file is reported rather than falling back. This task makes "the same rule" hold structurally: one function declares both the gate and the path.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/paths.go:105-113 — `PendingScrollbackPath(dir, token string) (path string, ok bool)` sits beside `PendingScrollbackFile` (paths.go:101). It returns not-ok when `nanoid.IsTokenShaped` refuses the token. Otherwise it returns `joinStored(dir, PendingScrollbackFile(token))`, the same join the re-file's placement uses (internal/state/scrollback.go:175-177, via `placeStoredScrollback` at scrollback.go:147).
  - internal/state/scrollback.go:120-135 — `refilePendingPane` takes its gate from `PendingScrollbackPath`'s `ok` (line 121) and keeps `PendingScrollbackFile` for the relative form it compares against and stores (lines 124, 126, 133). scrollback.go no longer imports `nanoid`.
  - internal/tui/preview_adapter.go:23-31 — `Tail` makes one `state.PendingScrollbackPath(a.stateDir, pane.PendingToken)` call. The order is unchanged: the token-named file first, the positional file only on a `(nil, nil)` read, and a read error returned as-is. The `nanoid` and `path/filepath` imports are gone.
- Notes: This is a behaviour-preserving refactor, confirmed by reading.
  - The gate is still the same `IsTokenShaped` predicate.
  - The preview's path is byte-identical: its old `filepath.Join(stateDir, filepath.FromSlash(PendingScrollbackFile(token)))` is exactly what `joinStored` computes.
  - The re-file still renames to `joinStored(dir, PendingScrollbackFile(token))`.
  - `linkMovedPane` (scrollback.go:233-247) also gates through `PendingScrollbackPath`. It came from a later task and is consistent with this one.
  - The measured set was converted whole. `FromSlash(state.PendingScrollbackFile` now has one hit, in the refused-token staging helper (internal/tui/pagepreview_waiting_pane_test.go:43), as the Do section prescribes. `IsTokenShaped(` has no hit in internal/tui, and in internal/state only in paths.go:109.
  - internal/restore/lazy_resume_panel_integration_test.go:187 stays on `PendingScrollbackFile`, as prescribed.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/scrollback_test.go:833-875 `TestPendingScrollbackPath`:
    - (a) For a minted token, the returned path holds the bytes `RefilePendingScrollback` moved. So the gate/path function and the re-file are pinned to one file, and a future divergence of either half fails here.
    - (b) An empty token, one a character short, and `/../../<token>` are each refused (AC5).
  - The existing re-file suite is untouched by the commit, including its literal `"scrollback/pane-" + token + ".bin"` expectations (e.g. scrollback_test.go:784, :803). It covers AC1, and AC2's four refused-token shapes (scrollback_test.go:703-734: nothing logged, dedup entry kept).
  - internal/tui/pagepreview_waiting_pane_test.go:
    - Minted-token staging and the `DenyRead` target go through `tokenBinPath` → `state.PendingScrollbackPath` (lines 25-37, 167).
    - The two refused-token cases keep staging where they staged before (`writeRefusedTokenBinFile`, lines 41-44, 127, 134), so they still plant a file the reader must not reach.
    - Assertions are unchanged across all `TestPreviewWaitingPane_*` cases (AC3).
- Notes: No redundancy. The new state test adds the one property no existing test pinned: the preview's path is the re-file's destination.

CODE QUALITY:
- Project conventions: Followed (`t.Run` "it ..." naming, no `t.Parallel`, helpers call `t.Helper()`, `nanoid` stays a leaf reached from `state`)
- SOLID principles: Good — the rule now has a single owner in internal/state; the tui adapter only consumes it
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comment on `PendingScrollbackPath` (paths.go:105-107) holds against the code: `IsTokenShaped` admits only `Alphabet` bytes, so no accepted token can carry `/` or `.`. The trimmed adapter comment (preview_adapter.go:20-22) matches `Tail`.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "the `TestRefilePendingScrollback` and `internal/state/capture_refile_test.go` suites pass unchanged" / "every case the `TestPreviewWaitingPane_*` suite pins" — reading shows the assertions are unchanged and the behaviour is equivalent. The pass itself needs `go test ./internal/state ./internal/tui` run on the current tree.
