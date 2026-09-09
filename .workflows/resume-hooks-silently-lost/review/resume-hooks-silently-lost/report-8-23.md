TASK: resume-hooks-silently-lost-8-23 (tick-5f1ded) — "The logtest Accessor Family Is One Member Short In Three Directions, And Two Capture-Handler Twins Survive"

ACCEPTANCE CRITERIA:
- No suite re-authors a string, duration, absence, error or exactly-one accessor `logtest` now offers.
- `Sink` is the only capture handler in the tree; both twins are gone.
- The lossy `gotErr, _ :=` read is gone, and its site fails on a wrong-kind value.
- `log.Discard()` has no open-coded copy, and no `barrierLog` shadow remains.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 consolidation task, not specified bugfix work. Per the shared verifier context, phases 6–9 are
implementation-analysis cycles whose authority is the task body rather than the specification, so the task was
judged against its own Do list, acceptance criteria and Tests section. The relevant standing convention is
CLAUDE.md's `logtest` architecture row (the closed query/accessor surface, the "one route per question" rule,
and the named exceptions to "Sink is the only capture handler"), which the commit amends in the same breath.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/logtest/capture.go:44` (`AttrOrEmpty`), `:82` (`DurationAttr`), `:99` (`RequireDuration` now routed
    through `DurationAttr`), `:153` (`Records.Only`, level-preserving, taking the described-set string).
  - Adoption (commit `cabafa79`, 61 files): `internal/tmux/portal_saver_test.go:1143`–`:1156` — the package-local
    `attrOrEmpty` is deleted and `barrierLog.warns` now calls `rec.AttrOrEmpty("component")`; its cross-file
    consumer is re-homed at `internal/tmux/hooks_register_test.go:733` (`migrationLine`).
  - Raw `rec.Attrs[...]` reads replaced in `cmd/state_daemon_cycle_summary_test.go`, `cmd/bootstrap/latch_test.go`
    and `cmd/bootstrap/eager_signal_hydrate_test.go`; the `took` reads in `cmd/state_hydrate_replayed_log_test.go:143`
    and `cmd/state_hydrate_timeout_log_test.go:45` now go through `DurationAttr`, and
    `cmd/state_daemon_cycle_summary_test.go:95` through `RequireDuration`.
  - Absence checks: `internal/storelog/clean_stale_test.go:33`, `internal/hooks/store_test.go:1064` and
    `internal/state/fifo_sweep_summary_test.go:133` now use `HasAttr`.
  - Handler twins: `RecordingLogger` (`cmd/bootstrap`) and `errorAttrRecorder`
    (`cmd/state_daemon_capture_logging_test.go`) are both deleted. All five files that referenced `RecordingLogger`
    — including the two integration-tagged ones (`composition_e2e_convergence_integration_test.go`,
    `orphan_sweep_integration_test.go`) — were migrated in the same commit, so the tagged lane does not break.
  - Tidies: `cmd/logging_capture_test.go:30` now calls `log.Discard().Handler()`; the nine
    `barrierLog := &barrierLog{}` shadows in `internal/tmux/portal_saver_lifecycle_events_test.go` are renamed to
    `barrier` (verified: 9 `barrier := &barrierLog{}`, 0 remaining shadows).
- Notes:
  - AC2 verified structurally: the only remaining `slog.Handler` implementations in the tree are
    `cmd/open_test.go:2876` (`warnBypassHandler` — models the production level gate and forwards into a `Sink`),
    `cmd/bootstrap/latch_test.go:42` (`orchestrationSeqHandler` — appends one ordering marker, captures nothing),
    and `internal/log`'s own two (`log_test.go:118`, `rotate_test.go:33`, which cannot import `logtest` without an
    import cycle). All four are the documented non-twins.
  - AC3 verified: no `Any().(error)` type assertion survives in any test file. Both former lossy sites
    (`internal/tmux/hooks_register_warn_test.go:27` and `:99`) now read through `ErrorAttr`, which fatals on a
    wrong-kind value.
  - Three raw `.Attrs[...]` reads remain tree-wide, none of them a re-authored member of the family:
    `cmd/bootstrap/eager_signal_hydrate_test.go:116` and `internal/state/signal_hydrate_test.go:150` assert a
    value's *kind* (`slog.KindAny`), for which no accessor exists and each of which is paired with an adjacent
    `ErrorAttr` that fatals on absence; `internal/tmux/hooks_register_test.go:740` is a non-fatal int read with a
    -1 sentinel, which the family (whose `IntAttr` is fatal) does not offer.
  - The new `time` import in `capture.go` is stdlib, which `sourceguardtest.AssertDepsWithin` exempts, so
    `internal/logtest/leaf_guard_test.go` still holds.
  - The commit's CLAUDE.md amendment (the `logtest` row) was itself superseded by later phase-9 work; the current
    row matches the current surface.

TESTS:
- Status: Adequate
- Coverage: All three named micro-acceptance tests exist verbatim —
  `internal/logtest/capture_test.go:160` ("it returns the empty string for an absent attr"), `:179` ("it fatals when
  the attr is not a duration"), `:214` ("it fatals unless exactly one record matched"). Each accessor also carries
  its happy path and, for `Only`, a described-set-in-the-message case (`:227`) that is the `description` parameter's
  only reason to exist. The fatal paths are driven through `harnesstest.Recorder` via the local `expectFail` /
  `captureFailure` helpers, which is the established route for a helper whose own failure is the subject.
- Notes:
  - The adoptions are semantics-preserving or strictly stronger. Several are strengthenings worth naming:
    `internal/spawn/logemit_test.go:87` previously indexed `okInfo[0]` with no length check at all (a latent panic);
    `cmd/bootstrap/orphan_sweep_test.go:122`/`:150`/`:214`/`:240` replaced substring scans over rendered lines with
    exact message + attr assertions, and the four messages they now pin (`sweep: pgrep failed`,
    `sweep: list-panes _portal-saver failed, legitimate set empty`, `sweep: identity-check failed, skipping`,
    `sweep: kill failed`) match `cmd/bootstrap/orphan_sweep.go:60`, `:68`, `:87`, `:95` verbatim.
  - The forbidden-string scans in the two migrated integration files moved from `RecordingLogger.AllEntries()` to
    `Sink.Lines()`/`Body()`. The rendering differs (component inline, capture order rather than level-grouped), but
    both scans are substring tests over the whole set, so neither loses reach.
  - No over-testing: the new coverage is one test per accessor plus the failure paths, and `RequireDuration`'s
    retained test pins the value-less delegation that is its whole surface.
- Not run: per this reviewer's remit, test adequacy was judged by reading; no suite was executed.

CODE QUALITY:
- Project conventions: Followed. `Only` is a terminal on `Records` rather than a new `Sink` method, which keeps
  CLAUDE.md's "no combination of the filters is itself a method" property intact; the fatal accessors take
  `harnesstest.TestingT` like their siblings; the leaf-guard dependency shape is unchanged.
- SOLID principles: Good. `RequireDuration` is now a one-line delegation to `DurationAttr`, so the kind rule has a
  single home; `AttrOrEmpty` is the stated non-fatal counterpart of `AttrString` rather than a second spelling of it.
- Complexity: Low. Every new accessor is a map read plus one guard.
- Modern idioms: Yes.
- Readability: Good. Each new accessor's doc comment states what distinguishes it from its neighbour
  (`AttrOrEmpty` vs `AttrString`; `DurationAttr` "checks the kind, not the rendering"; `Only` "the level survives
  into the returned record").
- Issues: None rising above preference. `_ = <chain>.Only(...)` appears at four call sites where a bare call
  statement would compile; harmless.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
