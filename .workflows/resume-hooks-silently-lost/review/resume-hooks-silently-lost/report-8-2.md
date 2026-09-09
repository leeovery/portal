TASK: resume-hooks-silently-lost-8-2 — Only ShowEnvironment Classifies An Unaddressable Session Name; Every Other Per-Session Op Keeps The Blind Spot

ACCEPTANCE CRITERIA:
- No production site calls `wrapNoSuchSession` directly; every per-session error routes through `wrapSessionTargetErr`.
- Each converted method fails a live colon-named session with `ErrUnaddressableSessionName` and never with `ErrNoSuchSession`.
- A genuinely absent session still classifies as `ErrNoSuchSession` on every converted method.
- `TestSessionTargetsAreComposedExactly` tables `SaverPaneID`, and the real-tmux route enumeration is derived rather than hand-listed.

STATUS: issues_found (one non-blocking over-test finding; all acceptance criteria met)

SPEC CONTEXT: The specification (`.workflows/resume-hooks-silently-lost/specification/.../specification.md`, sections 1–9) is about durable pane identity and the hook key; it mentions neither session-target composition nor unaddressable session names (grep for `colon`/`unaddressable`/`ValidateSessionName` returns nothing). This is a phase-8 implementation-analysis task, so per the shared verifier context its authority is its own body. The class it addresses arose from the earlier exact-target work: `wrapSessionTargetErr` exists because tmux answers an unaddressable name with the same "no such session" stderr a vanished one produces, and the capture loop discriminates on that sentinel (`internal/state/capture.go:71`) to tell natural churn from anomaly.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:101` (`HasSessionProbe` — classified once, used by both the ExitError and OS-fault branches), `:274` (`KillSession`), `:290` (`RenameSession`, on `oldName`), `:298` (`SwitchClient`), `:616` (`ShowEnvironment`, pre-existing), `:794` (`SetSessionEnvironment`).
  - `internal/tmux/saver_pane_pid.go:15` (`saverPanePID`, was the bare `wrapNoSuchSession`) and `:38` (`SaverPaneID`, was unclassified).
  - `internal/tmux/clients.go:18-23` — `ListClients` left swallowing its error as the zero-clients signal, per Do item 3, with the reason stated in-source.
  - `internal/tmux/exact_session_target_test.go:79-84` — the `SaverPaneID` row (`list-panes`, `coordTargetForm`); `:85-90` adds `SaverPanePIDOrAbsent` too.
  - `internal/tmux/exact_session_target_realtmux_test.go:81`/`:197` — both prefix-sibling halves re-keyed by route name; `:331-345` is the coverage guard that walks `perSessionRoutes` and fails a half missing a case.
- Notes:
  - AC1 holds and is verifiable: `grep -rn "wrapNoSuchSession" --include="*.go"` returns only the definition (`internal/tmux/errors.go:41`), the single internal call from `wrapSessionTargetErr` (`:125`), doc-comment mentions, and one test (`internal/tmux/realcommander_test.go:71`). No production site calls it directly.
  - The stronger property this delivers: `wrapSessionTargetErr` is now the *only* route to `ErrNoSuchSession`, and it checks the name first — so no per-session op can mint the absence sentinel for a live unaddressable name. Eight minting sites exist (the six in `tmux.go` plus the two in `saver_pane_pid.go`) and every one classifies.
  - Per-session ops that still return an unclassified error (`SetSessionOption` `:326`, `ListPanesInSession` `:495`, `ListWindowsAndPanesInSession` `:552`, `SelectLayout`/`SelectWindow`/`SelectPane`/`ResizePaneZoom` `:804`-`:840`) are outside the task's Do list, mint no absence sentinel, and have no caller that discriminates on one — so the Outcome's operative half ("never as absent") holds across the client. `ActivePaneCurrentPath` `:263` is deliberately excluded and its doc (`:250-258`) states why: tmux answers an unmatched `display-message` target at exit 0, so there is no stderr to classify.
  - The task body cites `internal/session/dirresolve.go:39` as a fourth production discriminator; that file no longer discriminates on `ErrNoSuchSession` (task 7-32 rewrote it around the empty-path signal — `dirresolve.go:34-43`). The task's derivation is stale on that one point, not the delivered code.
  - No caller behaviour regresses: the two `ErrNoSuchSession` consumers of converted methods (`cmd/uninstall.go:95` on `KillSession`, `cmd/state_daemon.go:325`) both pass the colon-free constant `tmux.PortalSaverName`, where `wrapSessionTargetErr` falls straight through to `wrapNoSuchSession`. `internal/tui/preview_attach.go:51-59` (the one free-form-name caller of `HasSessionProbe`) reads `present` first and only logs `err`, so the richer sentinel changes nothing it decides.

TESTS:
- Status: Adequate, with one redundancy (see FINDINGS)
- Coverage:
  - `internal/tmux/session_name_test.go:127-135` declares `perSessionOps` over all seven exported minting ops; `:137-156` asserts the colon-named case wraps `ErrUnaddressableSessionName` and NOT `ErrNoSuchSession` (AC2), `:158-176` asserts a vanished session still wraps `ErrNoSuchSession` and not the unaddressable sentinel (AC3), `:178-201` covers both sides of `SaverPanePIDOrAbsent`'s absence collapse — which is also the only coverage the unexported `saverPanePID` can have.
  - The fake (`:74-81`) returns the identical "no such session" stderr for both names, which is the point: it proves the discrimination comes from the name check rather than from tmux's words.
  - `internal/tmux/exact_session_target_test.go:102-120` tables `SaverPaneID` (AC4, first half).
  - `internal/tmux/exact_session_target_realtmux_test.go:153-177` adds real-tmux gone-session cases for both saver reads against a live prefix sibling, and `:274-298` the live-session counterparts.
  - AC4's second half is met in substance: the cases remain hand-written functions (real-tmux semantics per route cannot be generated from a table), but `TestSessionTargets_EveryPerSessionRouteIsCovered` (`:331-345`) derives the *requirement* from the shared `perSessionRoutes` set — a route added to the mock table fails this guard until both halves carry a case, which is the "covered by construction" the Do list asked for.
  - Every test would fail if the classification were reverted: dropping `wrapSessionTargetErr` from any op fails that op's subtest in both halves of the table.
- Notes: the tests correctly avoid asserting on error *strings*, discriminating with `errors.Is` against the `tmuxerr` sentinels, so tmux's wording stays uncontracted.

CODE QUALITY:
- Project conventions: Followed. The real-tmux file stays in the unit lane with per-test `-L` sockets (CLAUDE.md's carve-out for `internal/tmux/*_realtmux_test.go`); no daemon, no binary build, no `t.Parallel()`; targets are composed through the typed `CoordTargetExact` vocabulary throughout.
- SOLID principles: Good. The classifier stays one function with one reason to change; call sites do nothing but route through it inside their own message wrap.
- Complexity: Low. Each change is a single call substitution; `HasSessionProbe` hoists the classification to one variable rather than duplicating the call across its two branches.
- Modern idioms: Yes — `maps.Keys` + `slices.Sorted` for the deterministic map-driven subtest ordering (`:316-321`), `strings.Cut` in the pane-field helper.
- Readability: Good. `perSessionOps`' comment ("the operations that mint an absence sentinel") names the membership rule rather than restating the list, and the route maps are keyed so the coverage guard can match them.
- Comment accuracy: Holds. The added `errors.go:115-117` sentence ("An operation classifies inside its own message wrap, so the sentinel and tmux's own words both stay reachable") is true of all eight sites; `saver_pane_pid.go:48-51`'s collapse doc still describes the post-change behaviour, since only `ErrNoSuchSession`/`ErrEmptyPaneList` collapse and an unaddressable name now escapes as an error. No process-artifact references.
- Security / Performance: N/A — no new I/O, no new tmux call on any path.
- Issues: none beyond the finding below.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/tmux/session_name_test.go:83 — `TestShowEnvironmentClassifiesUnaddressableName` (`:83-115`) is now fully subsumed by the table this task added: its two subtests assert exactly the properties `TestPerSessionOpsClassifyUnaddressableName` (`:137-176`) asserts for the `ShowEnvironment` row (`:133`), against the same `noSuchSessionCommander("="+colonSession)` / `noSuchSessionCommander("=gone")` fakes and the same `errors.Is` pairs. Delete `TestShowEnvironmentClassifiesUnaddressableName`; `noSuchSessionCommander` (`:74`) must stay — the new table is its remaining caller. — FAILS: one behaviour now has two test homes in one file, so a future change to the classification contract has to be edited in both, and the surviving single-op test reads as if `ShowEnvironment` were special-cased when it is one row of a general rule.
