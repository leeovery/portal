# Implementation Review: Restore Host Terminal Windows

**Plan**: restore-host-terminal-windows
**QA Verdict**: Approve

## Summary

The feature is fully and faithfully implemented. All **69 tasks across 10 phases** (6 build phases + 4 analysis cycles) were independently verified against their acceptance criteria and the specification, and every one returned **STATUS: Complete with zero blocking issues and zero latent bugs**. The `internal/spawn` service (detect-self identity walk, adapter resolver with config→native→unsupported precedence, env-self-sufficient command composition, `@portal-spawn` token-ack channel, pre-flight gate, leave-what-opened partial-failure and permission burst-stop) and its two callers (the `portal spawn` CLI test seam and the in-process picker burst) match the spec's contracts precisely — including the load-bearing invariants (net-N never N+1, `TMUX`/`TMUX_PANE` stripped, `os.Executable()` for version-latch parity, per-window ack timer, session-identity selection, single-slot notice-band precedence). The four analysis cycles did their job: result classification, message rendering, log emission, the exec boundary, the nanoid alphabet and the net-N split are now single-sourced in `internal/spawn`, a real TOCTOU nil-adapter panic was fixed (Task 10-1), and a zero-value `Result` is no longer silently a success (Task 9-5). Tests are well-balanced — behaviour-focused, byte-identity-pinned where cross-caller parity matters, with the irreducible live-terminal/TCC surface correctly quarantined behind `//go:build manual`. The 62 non-blocking notes below are overwhelmingly test-hardening (pinning already-correct edge branches) and cosmetic (naming, stale doc comments); a small number of `[idea]`s carry genuine decision weight (plan/spec artifact reconciliation, burst-flash lifecycle consistency, remaining single-sourcing follow-ons) and are surfaced for your call.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across every section. Verified in depth:

- **Spawn architecture** — one service, two callers; net-N split centralized behind `spawn.SplitNetN`; env-self-sufficient argv (`/usr/bin/env -u TMUX -u TMUX_PANE PATH=… <abs-exe> attach <session> --spawn-ack <batch>:<token>`) built from the picker's own binary.
- **Detection** — outside-tmux env fast-path + process-tree walk, inside-tmux `list-clients` NULL-filter + local-only activity tiebreak, bundle-id family matching, clean-NULL vs typed transient error folded to the same unsupported path with a WARN; detect-once-cached lifecycle off the first-paint gate; in-flight-at-Enter awaited, never mistaken for unsupported.
- **Burst & partial-failure** — pre-flight `has-session` all-or-nothing gate (runs *before* the unsupported gate on both CLI and picker, Task 8-3); token-ack self-attach gated on all N−1 confirming; leave-what-opened with retry-set preserved; permission-required burst-stop with once-per-batch guidance; markers self-cleaned on every terminal path; async non-blocking `tea.Cmd` with input-lock and live cancellation.
- **Config escape hatch** — `terminals.json` tolerant decode, exactly-one-of `argv`/`script` + `{command}`-presence validation, within-config most-specific precedence, `spawn`-component WARNs, never-`permission-required` for recipes.
- **Observability** — new spec-governed `spawn` component with the closed attr vocabulary and count semantics (`total`=N incl. trigger, `opened`=surfaced), emitted from the chokepoint via single-source renderers with CLI/picker parity.
- **Design references** — the three approved Paper frames (`sessions-multi-select-active`, `sessions-multi-select-preflight-abort`, `sessions-unsupported-terminal`) match their captured fixtures at the visual gate; the `Opening n/N…` residual frame was captured fresh; all NO_COLOR variants render glyph-backed.

Deviations from the *authored task text* are all deliberate, spec-consistent convergences the analysis cycles introduced (three-outcome taxonomy + `OutcomeUnknown` sentinel replacing the authored four-outcome set; shared message/log renderers replacing illustrative literal strings; `session.NanoIDAlphabet` reuse replacing a local const). Two of these leave the *plan/spec documents of record* reading slightly out of date — surfaced under Ideas #14 below, not as compliance failures.

### Plan Completion

- [x] All 10 phases' acceptance criteria met (build phases 1–6 + analysis cycles 7–10)
- [x] All 69 tasks completed and independently verified
- [x] No scope creep — the one behavioural addition beyond authored text (suppressing `n`/new-session in multi-select mode, Task 8-2) is spec-aligned ("and other row actions") and tested; deferred scope (group-select, Spaces placement, headless `portal spawn`, parallel spawn) correctly left unbuilt

### Code Quality

No issues found. The service keeps OS/terminal specifics quarantined behind the typed `Result` taxonomy and the `Adapter` seam; the two adapter runners stay deliberately distinct while sharing only the identical exec plumbing; DI seams are small and idiomatic; the picker burst is a well-contained async state machine reusing the existing connectors. Cross-caller drift — the standing hazard in a one-service/two-callers design — is now structurally prevented by single-source helpers in `internal/spawn` with byte-identity parity tests. Non-blocking cosmetics (a param shadowing `context`, a couple of stale doc comments, a mis-hoisted logger name) are listed below.

### Test Quality

Tests adequately verify requirements and are not over-fitted. Every acceptance criterion has focused coverage; byte-identity is pinned where the spec demands identical output at multiple sites; the manual/integration residue (real `osascript`, real window, TCC modal, real `ps`/`defaults`) is correctly fenced off from the unit lane. The notes below are almost entirely *additive* hardening — locking an already-correct edge branch (a `path.ErrBadPattern` non-match, an `IsDir()` rejection arm, a currentSession-error fold, a precedence ordering) so a future refactor cannot regress it silently — plus a few honesty fixes (a "sorted" helper that doesn't sort, a "no I/O" subtest name that asserts only output).

### Required Changes (if any)

None. Verdict is **Approve**.

## Recommendations

62 non-blocking notes, clustered. Full per-task detail lives in the `report-{phase}-{task}.md` files in this directory.

### Do now

1. Parameter/identifier cleanups (mechanical renames, zero logic impact)
   - `internal/spawn/walk.go:98` — `transient()`'s first param is named `context`, shadowing the stdlib package name; rename to `msg`/`what` (Report 1-2)
   - `internal/tui/footer.go:210` — shared fitter's param `w` actually receives a computed budget; rename `w` → `budget` to match its doc comment (Report 9-4)

2. Stale / imprecise doc comments
   - `internal/spawn/adapter.go:24` — the "three members are the whole closed taxonomy" type comment predates `OutcomeUnknown`; add a clause noting the sentinel is excluded (Report 9-5)
   - `cmd/spawn.go:66-68` — `SpawnDeps.Logger` comment says "package-level spawnLogger" but it now defaults from `buildProductionSpawnSeams` (`log.For("spawn")`); reword (Report 10-2)
   - `internal/tui/model.go:3567` — add a one-line comment on the empty-body `for name = range m.selectedSessions {}` (sole key of the guaranteed one-element map) (Report 5-7)
   - `internal/tui/burst_progress.go:330-332` — comment the defensive `burstPipe == nil` early return in `cancelBurst` as unreachable-in-production (only the capture harness sets pending without a pipe) (Report 6-8)
   - `internal/tui/burst_selfattach_test.go:20-21` & `burst_cancel_test.go:30-31` — file-header "helpers live in sibling files" prose omits the newly-consumed `markedSupportedBurstModel`/`sessionsFromNames` (Report 8-5)

3. Test-diagnostic honesty (naming vs behaviour)
   - `internal/spawn/ack_realtmux_test.go:36` — `sortedSet` helper does not sort (unlike `ack_test.go`'s `sortedKeys`); rename or add `sort.Strings` so failure output is deterministic (Report 3-2)
   - `internal/spawn/ghostty_command_test.go:80` — purity subtest name claims "no I/O" but the body only asserts identical output; reword the name (Report 2-4)

4. Safe test-assertion additions (all pass as-is; lock intent)
   - `internal/spawn/identity_test.go:100` — add a malformed-pattern non-match case to cover the `path.ErrBadPattern` branch at `identity.go:86-88` (the one untested path) (Report 1-1)
   - `cmd/spawn_test.go:56-61` — assert the exact `Ghostty · com.mitchellh.ghostty` U+00B7 separator, not just the two substrings (Report 1-6)
   - `internal/spawn/recipe_test.go:101-104` — assert the WARN `detail` also contains the rejection reason (its spec-stated purpose), not just the entry key (Report 4-2)
   - `internal/tui/help_modal_test.go:203-214` — add the new help-only `m` "Multi-select mode" row to the asserted help-action list (Report 5-1)
   - `internal/tui/unsupported_banner_test.go:43` — add a negative assertion that the banner carries no `▌` notice-bar glyph (Report 6-2)
   - `internal/tui/burst_preflight_abort_test.go:374` — assert the multi-select footer renders after Esc-dismiss, making AC6's "footer unchanged" explicit (Report 6-7)

### Quick-fixes

5. `internal/spawn` detection test coverage
   - `walk_test.go` — add an intermediate-hop `.app` (ppid > 1, further ancestors above) to pin the ".app check precedes ppid check" short-circuit at a non-root hop (Report 1-2)
   - `detect_outside_test.go:96` — lock the precedence: malformed/empty `__CFBundleIdentifier` + a `GHOSTTY_*` var must resolve via the Ghostty fast-path (not fall through to the walk) (Report 1-3)
   - `detect_test.go` — add the `currentSession`-error → WARN-fold branch (an untested `resolve()` arm); strengthen the attr-scope guard from a blacklist to a positive `allowed ⊆ {component,terminal,bundle_id,detail}` bound (Report 1-5)

6. `internal/spawn` config adapter/resolver test coverage
   - `configadapter_argv_test.go:36` — add a case with `{command}` in two distinct template elements (Report 4-4)
   - `configadapter_script_test.go` — add a directory-path input to cover the `info.IsDir()` rejection arm (currently only the no-exec-bit half is hit) (Report 4-5)
   - `configmatch_test.go` — add a friendly-alias + `.app`-name both-match case to exercise the pure key-string tie-break (Report 4-3)
   - `resolver_config_test.go` — add a missing/non-executable `script` recipe falling through to native at the resolver boundary (Report 4-6)

7. `internal/spawn/terminalsconfig.go` cleanups
   - `terminalsconfig_test.go:191-201` — replace hand-rolled `equalStrings` with `slices.Equal` (the repo's `modernize` linter flags exactly this) (Report 4-1)
   - `terminalsconfig.go:68,74` — hoist the two inline WARN literals into named `msg*` constants to match `detect.go`'s convention (Report 4-1)

8. `internal/spawn` spawn-component logger name
   - `recipe.go:83` (and `terminalsconfig.go:68,74`) — the `spawn` logger is still named `detectLogger` (Phase-1 origin) but now serves non-detection WARNs; rename to `spawnLogger` across the package (Report 4-2)

9. `internal/spawn` burst/ack behaviour coverage
   - `ackid_test.go:148` — add the missing `if first == second` assertion so the "no cached id reuse" test actually verifies AC7 (Report 3-1)
   - `burst_test.go` — add an always-erroring ack double asserting the window resolves to `AckTimeout` (not a false confirm), pinning the documented safe-direction Collect-error contract at `burst.go:189-205` (Report 3-5)
   - `burst_rederive_test.go:85-87` — remove the tautological second `spawnedSession(...) == "alpha"` re-check (line 82 already proves it) (Report 7-5)

10. Picker burst/mode UI test coverage
    - `multi_select_banner_test.go:342` — assert the `N selected` banner *still* renders while a flash owns the notice band, pinning the intended two-row co-render in one place (Report 5-3)
    - `multi_select_footer_test.go:200` — add a `FilterApplied`-in-mode resolver test (multi-select footer, not the `esc clear filter` footer) (Report 5-4)
    - `capture_test.go:791-808` — drive the Init/Update load loop to assert the initial cursor actually lands on `fab-flowx-explore` (currently only the seam value is checked) (Report 5-8)
    - `spawn_detect_test.go:188` — add a zero-records assertion on the transient/NULL detection path to guard the "no additional WARN" criterion (Report 6-1)
    - `burst_partial_failure.go:111-113` — add a direct table test for `burstPartialFailureFlash` returning `""` when failed is empty and no permission present (Report 6-6)
    - `burst_preflight_abort_test.go:318` — add the composed AC4 assertion: after a prune, a second Enter dispatches a fresh burst over the survivor set (not a re-abort) (Report 6-7)
    - `burst_unsupported_noop_test.go` — add a focused unsupported-terminal + one-mark N=1 self-attach test (currently N=1 is only covered generally) (Report 6-9)
    - `burst_observability_test.go:350` — add a zero-DEBUG-records assertion on the permission path to pin the picker/CLI permission-arm asymmetry (Report 6-10)

11. Error-context & gate self-documentation
    - `internal/spawn/burst.go:135-137` — `Burster.Run` returns the raw `os.Executable` error unwrapped; add context (Report 2-3)
    - `cmd/spawn.go:156` — restore the plan's explicit `len(external) >= 1 &&` conjunct to the unsupported gate; currently redundant but self-documents the N≥2 precondition against a future refactor (Report 2-7)
    - `ghostty_command_test.go:10-16` — have the test's `realAttachArgv` append the trailing `--spawn-ack <batch>:<token>` pair to match the real `composeAttachArgv` (fidelity only) (Report 2-4)

12. Dedupe the unsupported no-op body
    - `internal/tui/burst_progress.go:483-487` & `:434-436` — the `emitUnsupportedNoop` + `setFlash(unsupportedFlashText)` + `return m, nil` body is now duplicated verbatim in `decideBurst`'s unsupported branch and `dispatchBurst`'s nil-adapter guard; extract a `func (m Model) unsupportedNoOp() (Model, tea.Cmd)` (Report 10-1)

### Ideas

13. Plan/spec artifact reconciliation (documents of record now trail the shipped contract — decide whether to amend converged/frozen artifacts)
    - `.workflows/.../planning/.../phase-2-tasks.md:20,32,46` — task text still describes a four-outcome taxonomy incl. an `Unsupported()` constructor; the shipped design is three outcomes + `OutcomeUnknown`, with "unsupported" owned by the resolver tier. Reconcile or annotate as superseded (Report 2-1)
    - `.workflows/.../specification/.../specification.md:138` — the precedence line still reads as strict single-slot ("highest wins"), while the code deliberately co-renders the spawn-failure/permission flash *with* the `N selected` banner across two rows (the intended reading, documented in-code at `notice_band.go:348-360` per Task 7-6). Add a clause so the frozen spec doesn't read as contradicting the in-code decision (Report 7-6)

14. Burst notice-band precedence & flash lifecycle (family-wide behaviour, near-zero-probability paths)
    - `internal/tui/burst_progress.go:436` (and siblings) — the unsupported/partial-failure outcome flashes schedule no `flashTickCmd`, so unlike the async gone-flash they don't auto-clear on a timer; on the deferred detection path an idle user could keep the flash indefinitely. Decide whether burst-family flashes should auto-clear for consistency (Report 6-9)
    - `internal/tui/notice_band.go:361` — the spec places `Opening n/N…` above the transient flash, but they live on separate physical rows and only co-render in the tiny detection-defer window before `burstPending` latches; consider gating the flash arm on `!m.burstPending` (Report 6-5)

15. CLI/picker log parity residuals (pre-existing, out of the cycles' targeted scope)
    - `internal/tui/burst_partial_failure.go:43-47` — on a pre-spawn `Burster.Run` error (nothing opened, possibly empty batch) the picker still emits `spawn: opened 0/N`, whereas the CLI's `runSpawn` returns the error and emits nothing; decide whether to align (Reports 6-6, 7-1)
    - `cmd/spawn.go:229-254` — the four thin `logSpawn*` wrappers now only forward to `spawn.LogX`; consider inlining and retargeting the CLI parity tests to the central `logemit_test.go` golden (Report 8-1)

16. Remaining spawn-seam single-sourcing follow-ons (Task 10-2 explicitly left these open)
    - `cmd/spawn.go:292-302` & `:337-339` — the `Detector` seam is still constructed at two sites (builder for the picker, `spawnDetector` for the CLI/`--detect`); single-source it via a shared `newDetector(client)` so the last of the seven seams can't drift (Report 10-2)
    - `cmd/spawn_seams_test.go` — no test asserts the picker path sources its shared seams from `buildProductionSpawnSeams`; the picker's Bubble Tea entry makes this awkward, so decide whether a parity assertion is worth adding (Report 10-2)

17. Performance sensitivity (matches an existing tracked follow-up)
    - `internal/tui/burst_progress.go:428` — the unsupported-branch pre-flight runs N sequential `has-session` probes synchronously on the Bubble Tea Update thread (the supported path defers into a goroutine). It's a rare one-shot no-op over the small marked set and matches the CLI, but the repo already tracks N-sequential-tmux-reads-on-the-UI-thread sensitivity (`project_grouped_switch_perf_followup`); batch into one `list-sessions` read if ever a concern (Report 8-3)

18. Config robustness / surface polish
    - `internal/spawn/detect_outside.go:18` — the `GHOSTTY_*` signal set is intentionally two keys; revisit at the build-time residual whether more stable Ghostty keys warrant inclusion (degrades gracefully via the walk) (Report 1-3)
    - `cmd/attach.go:92-95` — the internal `--spawn-ack` flag is described as "internal:" but appears in `portal attach --help`; consider `MarkHidden` (judgment call — recipe authors composing the command may want it visible) (Report 3-3)
    - `internal/spawn/ack.go:141` — `optionNames` re-implements the `@name value` line-split shape in `state.ListSkeletonMarkers`; consider homing a shared splitter in leaf `internal/tmuxout` (deliberately duplicated for import-cycle avoidance; needs a where-to-home decision) (Report 3-2)

19. Structural refactors & test scaffolding (all genuinely optional)
    - `internal/tui/model.go:505-526` — the tick's deferred step 5: group the ~9 cohesive burst fields into a `burstState` struct so the sub-state-machine is one addressable unit (Report 9-2)
    - `internal/tui/burst_dispatch_test.go` etc. — extract a shared `enterAndMarkAll(t, m, names)` helper for the three suites the two custom-adapter constructors can't route through `markedSupportedBurstModel` (residual drift risk near-zero) (Report 8-5)
    - `internal/tui/burst_partial_failure.go:43,73,108` — thread the already-detected `perm`/`ok` into `burstPartialFailureFlash` to avoid a second `FirstPermission` scan (or keep the flash builder pure) (Report 7-1)
    - `internal/tui/burst_selfattach_test.go:210-238` — the attached-elsewhere test is structurally near-identical to the includes-self test and can't model a real remote-client state at the unit layer; consolidate, or comment-point to the integration layer (Report 6-4)
    - `internal/tui/row_style_helpers_test.go` — the test file tests free functions that live in `session_item.go` with no matching `row_style_helpers.go`; align the naming (Report 5-2)
    - `internal/spawn/resolver_config_test.go` — optionally pin the spec-literal "invalid most-specific entry falls through to *native*, not a less-specific valid config entry" (structurally guaranteed today) (Report 4-6)
    - `internal/tui/model.go:3394-3444` — the four `if m.multiSelectMode { return m, nil }` suppression arms could share a guard, but the plan endorsed the per-arm form and each comment differs; flagged for awareness, no change recommended (Report 5-5)
    - `internal/tui/footer.go:189-190,359-360` — both fitters now render sep/ellipsis eagerly (two extra `lipgloss.Render` calls per footer in the common wide case); negligible, mandated by the task's pre-rendered-string contract; revisit only if that contract is relaxed to closures (Report 9-4)
