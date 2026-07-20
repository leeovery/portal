TASK: cli-verb-surface-redesign-4-7 — Delete `clean` + `state status`, relocate housed helpers, drop `clean` from exempt set

ACCEPTANCE CRITERIA:
- Delete `cmd/clean.go` and `cmd/state status` command, no dangling refs to `cleanCmd`/`stateStatusCmd`.
- Relocate (not delete) `loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` (used by open/TUI/doctor) and `AllPaneLister` (consumed by `runHookStaleCleanup` + daemon).
- `internal/state.CollectStatus`/`StatusReport` survive (doctor reuses them) — NOTE: later task 8-1 narrowed doctor off these and may have removed them; verify coherence instead.
- Remove clean-only `cleanStaleHooks`/`cleanRotatedLogs`/`CleanDeps`/`buildCleanPaneLister` and status-only `ErrStatusUnhealthy`/render helpers.
- Daemon's throttled hook cleanup (`runHookStaleCleanup`'s remaining caller) still compiles + green.
- Update `run_hook_stale_cleanup.go` doc comment naming `clean.go`.
- Drop `clean` from `skipTmuxCheck`.

STATUS: Complete

SPEC CONTEXT: Spec §295-333 — `portal clean` is deleted (its `--logs` flag removed; logs auto-rotate/retention-sweep) and `state status` is subsumed by the new `portal doctor`. Spec §333 states "Nothing internal calls `clean` or `state cleanup`" (both were purely manual backstops to already-automated work). Removed-public-commands table (§423-432) lists `portal clean [--logs]` → `doctor --fix`, `portal state status` → `doctor`. §367: "`clean` leaves the exempt set (deleted); `state` stays; doctor/uninstall join `skipTmuxCheck`."

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/clean.go and cmd/state_status.go — deleted (absent from filesystem AND `git ls-files`; deletion in commit ff7d255d).
  - No dangling refs: `grep cleanCmd|stateStatusCmd` → zero matches across all .go files.
  - Relocated config helpers in cmd/config.go:131-162 (`loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath`) — sit alongside sibling `loadAliasStore`/`loadHookStore`, the natural config-path home; still consumed by open/TUI/doctor.
  - `AllPaneLister` relocated to cmd/run_hook_stale_cleanup.go:57-70 (interface def) where its sole interface consumer lives; consumed by daemon (cmd/state_daemon.go:456 `maybeRunHookCleanup`) and doctor (cmd/doctor.go:88,413 + `--fix` prune at :290). Compile-time assertion `var _ AllPaneLister = (*tmux.Client)(nil)` present (cmd/bootstrap_production_test.go:127).
  - Removed clean-only helpers `cleanStaleHooks`/`cleanRotatedLogs`/`CleanDeps`/`buildCleanPaneLister` and status-only `ErrStatusUnhealthy`/render helpers — zero live references anywhere.
  - `skipTmuxCheck` (cmd/root.go:58-68): `clean` absent; `doctor` and `uninstall` present. Doc comment (root.go:39-48) documents doctor/uninstall exemption rationale; no stale `clean` mention.
  - run_hook_stale_cleanup.go doc comment (lines 3-49, 72-89) no longer names `clean.go` — names the two live callers (daemon `maybeRunHookCleanup` + doctor `--fix` `pruneDoctorStaleHooks`).
- Notes: Per the task NOTE, `internal/state.CollectStatus`/`StatusReport` were subsequently removed by analysis task 8-1 (zero definitions and zero references remain). This is coherent — no dangling refs — which the NOTE explicitly permits over insisting the symbols persist.

TESTS:
- Status: Adequate
- Coverage: This is a deletion/relocation task; correctness is proven by (a) absence of any dangling reference to the deleted commands/helpers (structural — a leftover ref would fail compilation), and (b) the surviving callers staying green. The relocated `runHookStaleCleanup` is thoroughly exercised by cmd/run_hook_stale_cleanup_test.go (10 subtests: both-empty, empty-persisted, list-panes-error Warn-and-continue, mass-deletion hazard guard, legitimate stale removal, onRemoved callback, nil-logger tolerance, id-keyed no-mass-orphan). The daemon caller is covered by cmd/state_daemon_hook_cleanup_test.go. The `--fix` reuse is covered by cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go. Relocated config helpers are exercised transitively by open/TUI/doctor suites.
- Notes: No new tests are warranted for pure deletions — appropriately scoped, neither under- nor over-tested. The dropped `persisted_empty_early_exit` subtest was intentionally removed (its `cleanStaleHooks` short-circuit no longer exists) with an in-file rationale (cleanstale_transient_listpanes_doctorfix_integration_test.go:39-44).

CODE QUALITY:
- Project conventions: Followed. Helpers landed in the idiomatic homes (config-path helpers in config.go beside their siblings; the `AllPaneLister` seam beside its consumer). Interface stays 1-method (DI convention).
- SOLID principles: Good. `runHookStaleCleanup` remains the single source of truth for the prune algorithm; relocating the interface next to it tightens cohesion (ISP/SRP).
- Complexity: Low. Pure move + delete; no logic change to surviving code.
- Modern idioms: Yes (unchanged).
- Readability: Good. Doc comments were updated to reflect the new caller set rather than left stale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The comment at cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go:40 naming "the deleted cleanStaleHooks" is deliberate, accurate documentary context for a dropped subtest — it proposes no action and is load-bearing; not a finding.)

VERIFICATION METHOD NOTE: "everything compiles" was assessed by static inspection (zero dangling references to any deleted/relocated symbol across all .go files, compile-time interface-satisfaction assertion present) rather than executing `go build`, per the no-command-execution rule.
