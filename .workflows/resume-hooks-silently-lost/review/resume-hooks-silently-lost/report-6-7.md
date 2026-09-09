TASK: resume-hooks-silently-lost-6-7 — Route Every Session-Level -t Target In The Tmux Client Through The Exactness Rule The Package States

ACCEPTANCE CRITERIA:
- No `-t` argument in `internal/tmux/tmux.go` passes a bare session name.
- Each of the seven sites returns the same error class for a genuinely missing session as it does today.
- A prefix-sibling session cannot be reached by any of the seven when the named session is gone.

STATUS: complete

SPEC CONTEXT:
Phase 6 is an implementation-analysis task, so its authority is its own body rather than the
specification; the specification's Corrigenda touch this area only once, recording that
`ResolveStructuralKey`, `ListAllPanes` and `ListPanes` were deleted during the work unit (their
callers were the hook paths the change replaced). That deletion accounts for one of the seven
sites named in the task body no longer existing in the tree. The wider work unit's subject —
a hook lost because a target resolved onto the wrong session after a rename — is the same failure
class this task closes inside the tmux client: tmux prefix-matches an unpinned session name, so
once `foo` is renamed away a bare `-t foo` silently answers as the live `foo-2`.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved past the task's wording by later phases)
- Location:
  - `internal/tmux/tmux.go:263` (ActivePaneCurrentPath / display-message),
    `:325` (SetSessionOption / set-option), `:494` (ListPanesInSession),
    `:548` (ListWindowsAndPanesInSession), `:613` (ShowEnvironment),
    `:791` (SetSessionEnvironment) — all six surviving sites of the seven compose
    `CoordTargetExact(session)`.
  - `internal/tmux/tmux.go:467` `CoordTargetExact` and `:448` `SessionTargetExact` — the two
    documented forms this task introduced (as `exactCoordTarget` / `exactTarget`; later phases
    exported them and gave them the `Target` type).
  - `internal/tmux/tmux.go:488` `windowTargetExact` — added by this task's second fix round after
    review found `SelectLayout` (`:802`) composing its own bare `fmt.Sprintf("%s:%d", …)`, a silent
    wrong-session *write* the task's own criterion 1 covers; `SelectWindow` was folded onto the same
    helper.
  - The seventh site named in the task body (`ListPanes`) was deleted in a later phase with its last
    caller; the task body's `:488 (ListAllPanesWithFormat)` label was a mislabel that in fact pointed
    at `ListWindowsAndPanesInSession`, which is what was routed. `ListAllPanesWithFormat`
    (`internal/tmux/tmux.go:601`) composes `list-panes -a` and no `-t` at all, so it needs no pinning.
- Notes:
  - Criterion 1 holds in the current tree: every `"-t"` in `internal/tmux` (`tmux.go`, `clients.go`,
    `saver_pane_pid.go`) is followed by a `Target`-typed value — `CoordTargetExact`,
    `windowTargetExact`, `PaneTargetExact`, `PaneIDTarget`, or a `Target` parameter. It is now held
    structurally as well as by inspection: `internal/tmux/target_composition_guard_test.go` derives
    its scanned package set from the importers of `internal/tmux` and reports any `-t` whose target
    the vocabulary did not produce, including an untyped constant or an explicit conversion handed
    to a `Target` parameter.
  - Criterion 2 is met at five of the six surviving sites and *deliberately* diverges at one:
    `ActivePaneCurrentPath` no longer produces `ErrNoSuchSession` for a missing session, because
    `display-message -p -t '=name:'` answers an unmatched target with an empty expansion at exit 0
    and no stderr, so there is no stderr to classify. This is a considered, recorded divergence,
    not a loss: the doc comment that used to promise the sentinel was corrected in a later phase
    (`internal/tmux/tmux.go:250-262` now states the `("", nil)` contract and why it exists), the sole
    consumer `internal/session/dirresolve.go:41` already treated an empty path as "unresolvable this
    pass", and the only caller of that (`internal/tui/model.go:1163`) gates on `ok && err == nil`,
    so both shapes reach identical behaviour. The realtmux suite pins the new contract explicitly
    rather than leaving it to omission.
  - The remaining classification is unchanged or strengthened: `ShowEnvironment` (`:613`) and
    `SetSessionEnvironment` (`:791`) route through `wrapSessionTargetErr`, which reaches
    `ErrNoSuchSession` via `wrapNoSuchSession`; `ListPanesInSession`, `ListWindowsAndPanesInSession`
    and `SetSessionOption` returned unclassified errors before this change and still do, and no
    production caller of any of the three discriminates (`cmd/state_signal_hydrate.go:31`,
    `internal/restore/session.go:123`, `internal/tui/pagepreview.go:208` treat any error as failure).
  - Criterion 3 is verified against real tmux rather than asserted: see TESTS.
  - The fakes updated by this task to strip the pin (`cmd/state_daemon_capture_logging_test.go:228`,
    `internal/state/capture_test.go:68`) were correct for `exactTarget` and were correctly widened to
    `TrimSuffix(TrimPrefix(target, "="), ":")` when a later phase moved those routes onto the
    coordinate form — so no fake silently stopped resolving its session and no test quietly stopped
    exercising its subject.

TESTS:
- Status: Adequate
- Coverage:
  - Unit (fake commander): `internal/tmux/exact_session_target_test.go` — a shared route table
    (`perSessionRoutes`) driving `TestSessionTargetsAreComposedExactly`, which asserts each route
    composes exactly one tmux call and that its `-t` argument is verbatim `"=exact:"`. The fake is
    `commandertest.New`, whose default is loud on an unscripted argv, so a route issuing an extra or
    different call fails rather than passing quietly.
  - Real tmux, unit lane, per-test `-S <tmpdir>/ptl-exacttgt-*/s` socket, no daemon and no built
    binary — compliant with the lane and isolation rules: `internal/tmux/exact_session_target_realtmux_test.go`
    stages a two-window, four-pane live `sib-2` with `sib` never created (exactly the state a rename
    leaves) and runs two halves. The gone-session half asserts each route misses AND that the
    sibling was not touched — the write cases re-read the sibling's environment / options / window
    layout, which is the only assertion that can see a wrong-session write; the read cases compare
    against the sibling's own dir, pane id and pane pid. The live-session half asserts pinning did
    not cost the ordinary case, which is what stops a form tmux cannot resolve at all from passing
    as a fix.
  - `TestSessionTargets_EveryPerSessionRouteIsCovered` ties the two guards together, so a route added
    to the shared table without both real-tmux halves fails rather than landing unguarded.
  - The multi-window fixture is load-bearing and was added in fix round 1 for the right reason: a
    single-window sibling would let a form that resolves only the current window pass as if it had
    resolved the whole session, which is precisely the scope question the argv table cannot answer.
  - `ShowEnvironment`'s `ErrNoSuchSession` is pinned against *real* tmux stderr, not a hand-supplied
    mock string — the one assertion that can catch wire-form drift in the chain
    `internal/state/capture.go` depends on to tell natural session churn from anomaly.
- Notes:
  - Not over-tested. The one apparent overlap — `internal/tmux/tmux_test.go` still asserting the full
    argv for each method while the new table asserts the `-t` form — carries different subjects
    (whole-argv shape per method vs. the target-form rule across routes) and neither subsumes the
    other.
  - Not under-tested in any way that names a failure. Four routes assert only `err != nil` on the
    gone-session half rather than an error class, but those four return unclassified errors in
    production and no caller discriminates them, so there is no contract left unobserved.
  - `SelectLayout` sits in the real-tmux halves but not in the shared table; its argv form is pinned
    separately at `internal/tmux/tmux_test.go:1866`, so it is not unguarded.

CODE QUALITY:
- Project conventions: Followed. The realtmux file is correctly unit-lane (`internal/tmux/*_realtmux_test.go`,
  per-test socket, no daemon, no binary build), uses `tmuxtest.New`'s server-killing cleanup, and
  touches no ambient tmux server. The `Target`-typed vocabulary and the source guard over it match
  the architecture row in `CLAUDE.md`.
- SOLID principles: Good. One rule, one home: the two target forms are the single declaration of the
  pinning rule, and the review-driven `windowTargetExact` removed the last ad-hoc `"=" + …`
  concatenation on the window path.
- Complexity: Low. The change is a substitution at each call site plus three small helpers.
- Modern idioms: Yes — `slices.Index`, `slices.Sorted(maps.Keys(...))` for deterministic subtest order,
  table-and-map-driven cases.
- Readability: Good. The doc comments carry the measured reasoning — why the coordinate form and not
  the bare one, why the trailing `:` is load-bearing, and the explicit instruction to measure per
  command rather than infer from the target's shape — which is what makes the next call site's choice
  a decision rather than a guess.
- Issues: None meeting the reporting bar. The comment corrections the review rounds forced
  (`ListPanes`'s no-`-s` scope, the display-message-vs-list-panes failure-mode split) landed with the
  change rather than being deferred, and the one comment this task left overstated —
  `ActivePaneCurrentPath`'s `ErrNoSuchSession` promise, false before this change and still false
  after it — was found and corrected in a later phase (7-32) and is true in the tree today.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
