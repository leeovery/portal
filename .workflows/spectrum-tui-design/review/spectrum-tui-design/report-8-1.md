TASK: spectrum-tui-design-8-1 — Collapse BootstrapProgressMsg's duplicate friendly-label computation to a single authority (drop producer-side Label/Name so the §10.4 11-step→5-label mapping lives ONLY in loading_progress.go).

ACCEPTANCE CRITERIA:
- BootstrapProgressMsg no longer carries a Label field; Name retained only if a consumer reads it.
- The §10.4 11-step→5-label mapping exists in exactly one location (loading_progress.go); no second authoritative copy produced or transmitted.
- The producer in bootstrap_progress.go no longer computes/assigns the dropped field(s).
- The loading page renders the identical 5-label progression as before for the full 11-step cold-boot sequence (including restore N/M sub-steps).
- Existing loading-progress unit tests still assert the §10.4 progression derived from Index; a grep/compile guard confirms no remaining reference to the removed field(s); BootstrapProgressMsg constructions in tests compile against the reduced field set.

STATUS: Complete

SPEC CONTEXT:
§10.4 (specification.md:418-431) is the canonical 11-real-step→5-friendly-label table: step 1→"Started tmux server"; steps 2-5→"Registered hooks"; step 6 skeleton phase (N/M)→"Restoring sessions"; step 6 geometry/replay + step 7→"Replaying scrollback"; steps 8-11 + on-resume hydrate→"Running resume commands". Only "Restoring sessions" carries an N/M counter; M=0 suppresses it and the label ticks done immediately. The bar advances through every real step (eleven increments, not five). loading_progress.go is documented as the SINGLE SOURCE OF TRUTH for this contract.

IMPLEMENTATION:
- Status: Implemented (clean, behaviour-preserving).
- Location:
  - internal/tui/model.go:180-184 — BootstrapProgressMsg reduced to {Index, RestoreN, RestoreM}; the Label and Name fields removed; the doc comment (model.go:167-179) rewritten from the "task 5-3/5-4 placeholder" wording to state the mapping lives in exactly one place (loading_progress.go) and the wire carries NO friendly label and NO raw StepName.
  - cmd/bootstrap_progress.go:85-96 — the producer's bootstrapProgress.Label field deleted; the Step doc updated to note only Index reaches the wire.
  - cmd/bootstrap_progress.go:249-253 — receiver() construction no longer assigns Name: ev.Step.Name or Label: ev.Label; it sets only Index/RestoreN/RestoreM. Comment at 245-248 documents the single-authority rationale.
  - internal/tui/loading_progress.go:84-114 (stepLabelTable / LabelForStep) — UNCHANGED; remains the sole §10.4 authority, keyed off Index (+ RestoreM discriminator for the dual-mapped step 6). Matches the spec §10.4 table exactly.
  - internal/tui/model.go:2019-2035 — the consumer Update arm folds each msg through LoadingProgress.Apply (reads Index/RestoreN/RestoreM only), confirming nothing reads the dropped fields.
- Notes:
  - Grep confirms NO internal/tui reader of msg.Label or msg.Name against BootstrapProgressMsg (all internal/tui ".Label"/".Name" hits are Session.Name, Project.Name, LoadingLabel/v.Labels, tok.Name, filter-footer Entry.Label — unrelated).
  - Producer-side StepEvent.Name (cmd/bootstrap/progress_emitter.go:36) is correctly LEFT in place: it is the StepEvent type, not the wire BootstrapProgressMsg, and is out of this task's scope. It is still constructed at bootstrap.go:284,378. See NON-BLOCKING NOTES — it now has no production reader (only a test reads ev.Name).
  - go vet ./cmd/... ./internal/tui/... is clean.

TESTS:
- Status: Adequate (with one strong addition).
- Coverage:
  - TestBootstrapProgressMsgCarriesOnlyConsumedFields (loading_progress_test.go:376-400) — a reflection guard pinning the field set to exactly {Index, RestoreN, RestoreM}, asserting both no extra field rides the wire AND none of the consumed fields is missing. This pins the dead-field removal in the TYPE, not just behaviourally — exactly the drift-prevention the task called for.
  - TestStepMapsToFriendlyLabel (loading_progress_test.go:49-72) — the full §10.4 1..11→label progression (incl. the step-6 RestoreM>0 skeleton vs RestoreM==0 completion split) still asserted, now constructed with Index/RestoreN/RestoreM only.
  - TestBarAdvancesEveryStep, the M=0 / N/M counter tests, and the FailedView fatal-label tests continue to assert §10.4 behaviour.
  - All BootstrapProgressMsg constructions in cmd/open_fatal_test.go, internal/capture/capture_test.go + fixtures.go, internal/tui/bootstrap_progress_test.go / inert_during_loading_test.go / loading_fatal*_test.go / loading_view_test.go were reduced to the new field set and compile.
- Notes: Not over-tested — the reflection guard is the one net-new test and earns its place (it is the compile/grep-level guard the task's Tests section asked for). Not under-tested — the label progression and N/M behaviour remain covered.

CODE QUALITY:
- Project conventions: Followed. Small DI seams untouched; no logger constructed; tests do not use t.Parallel(); the wire-message reduction respects the documented internal/tui → no-cmd/bootstrap import direction (the mapping stays consumer-side).
- SOLID principles: Good. Single-responsibility sharpened — the §10.4 mapping now has exactly one owner (loading_progress.go); the producer no longer carries a parallel encoding.
- Complexity: Low. Net deletion of fields + assignments; no new branches.
- Modern idioms: Yes. reflect-based field-set guard is idiomatic Go for pinning a wire contract.
- Readability: Good. Doc comments on both the producer (bootstrap_progress.go) and the wire type (model.go) were updated to explain WHY no label/StepName rides the wire, so the single-authority intent is self-documenting.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/model.go:323-326 — the model's latestProgress BootstrapProgressMsg field is written (model.go:2029) but never read; its doc ("Kept for any consumer that reads the raw last event") openly marks it speculative. This is the same structurally-dead-field smell this task removed from BootstrapProgressMsg, one layer up on the model struct. Pre-existing (not introduced by 8-1) and out of scope, but a candidate for the same deletion treatment — decide whether any future consumer genuinely needs the raw last event before removing, hence idea not quickfix.
- [quickfix] cmd/bootstrap/progress_emitter.go:36 + cmd/bootstrap/bootstrap.go:284,378 — StepEvent.Name now has no production reader (only progress_emitter_restore_test.go:64 asserts ev.Name == stepRestore). It is still set on both emit sites. Correctly left in this task (StepEvent ≠ the wire message), but it is now producer-internal dead-ish state: the only thing keeping it alive is a test assertion. A follow-up could either drop StepEvent.Name (and the test assertion) or document it as a deliberately-retained diagnostic field. Concrete mechanical edit at known locations, no design decision — quickfix.
