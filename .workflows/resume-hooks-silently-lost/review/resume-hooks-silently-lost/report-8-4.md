TASK: resume-hooks-silently-lost-8-4 — "Bare Pane Targets Still Reach The tmux Client From Two Production Sites, And The Guard Cannot See Either" (tick-31db9b, done)

ACCEPTANCE CRITERIA:
- The daemon's capture read addresses `=session:window.pane`.
- `cmd/open.go` composes no exact-target prefix by hand.
- The guard's package set is derived, includes `cmd`, and the derivation fails rather than silently empties if the import scan resolves nothing.
- The guard flags a bare target composed in `cmd` (proven by a staged probe).

STATUS: complete

SPEC CONTEXT:
The specification for this work unit governs the resume-hook key change and its sweep/lock machinery; it says nothing about tmux target exactness (a grep for `ExactSessionTarget`, `prefix-match`, `PaneTarget`, `exact-match` in `.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md` returns nothing, and no corrigendum touches the subject). This is a phase-8 implementation-analysis task drawn from the opportunity bank, so its authority is its own body, as the verifier context states. The surrounding project standard is CLAUDE.md's `internal/tmux` row: every per-session `-t` Portal composes is pinned, because tmux prefix-matches a bare session name and would resolve `-t foo` onto a live `foo-2` once `foo` is gone — on this path, writing a stranger's scrollback under the gone pane's key.

IMPLEMENTATION:
- Status: Implemented (with a documented post-task rename; see Notes)
- Location:
  - `cmd/state_daemon.go:278` — `target := tmux.PaneTargetExact(sess.Name, win.Index, pane.Index)`, spent at `:279` through `state.CaptureAndHashPane(deps.Client, target)`. `internal/tmux/tmux.go:417-419` shows `PaneTargetExact` renders `=%s:%d.%d`, so the read addresses `=session:window.pane`. The scrollback key is still `state.SanitizePaneKey(...)` at `cmd/state_daemon.go:272`, as the task's Do list required.
  - `cmd/open.go:95` — `argv := []string{"tmux", "attach-session", "-t", string(tmux.CoordTargetExact(name))}`. No `"=" + name` remains anywhere in production: the repo-wide scan of non-test `.go` files for a literal `"-t"` shows every remaining composition site drawing its target from the vocabulary (`internal/tmux/tmux.go`, `internal/session/quickstart.go:66-69`, `internal/tmuxtest/socket.go:78,128`, `internal/tmuxtest/stamp.go:15,28`, `internal/restoretest/live_pane_coords.go:34`).
  - `internal/tmux/target_composition_guard_test.go:29-94` — the derived package set: `modulePackageListing` (`:61`) runs one `go list -tags integration -f '{{.ImportPath}}\t{{.Dir}}\t{{join .Imports " "}}' github.com/leeovery/portal/...` per test binary behind `sync.OnceValues`; `importersOfTmux` (`:78`) keeps every package whose imports contain `internal/tmux`, plus `internal/tmux` itself, and returns an error (`:90-92`) when the set is empty; `targetComposingPackageDirs` (`:39`) turns either failure into `t.Fatal`.
- Notes:
  - The task body names `tmux.ExactSessionTarget`; the delivered code calls `tmux.CoordTargetExact`. The 8-4 commit (7b96ebdc) did write `ExactSessionTarget`; task 9-17 (3ccac0d7) later renamed the vocabulary to the `<kind>TargetExact` shape and moved the attach argv onto the coordinate-pinning form (`=name:` rather than `=name`), which is the form CLAUDE.md and `internal/tmux/tmux.go:448-467` document as correct for a `-t` tmux parses as a window or pane target. The criterion — "composes no exact-target prefix by hand" — is met, and the divergence is a strict improvement, not a loss.
  - The capture-failure Warn attr changed after this task too: 8-4 emitted the plain coordinate under `pane_key`, while HEAD (`cmd/state_daemon.go:287`) emits the sanitised `paneKey`, matching the sanitised keys asserted at `cmd/state_daemon_capture_logging_test.go:198-202`. Internally consistent; superseded by later work, not broken by it.
  - The pin at `cmd/state_daemon.go:278` is additionally held by the compiler, not only by the guard: `state.CaptureAndHashPane[T ~string]` (`internal/state/scrollback.go:66`) infers `T` from `deps.Client`, whose `CapturePane` takes `tmux.Target` (`internal/tmux/tmux.go:731`), so the `string`-returning `PaneTarget` no longer type-checks at that call.

TESTS:
- Status: Adequate
- Coverage:
  - `"it captures a pane through the pinned pane target"` — `cmd/state_daemon_capture_logging_test.go:245-270`. Drives the real `captureAndCommit` over `daemonFakeCommander`, asserts exactly one `capture-pane` call and that its last argv element is `=work:0.0`. Would fail if the target reverted to the unpinned form.
  - `"it attaches through CoordTargetExact"` — `cmd/open_test.go:2598-2617`. Records the exec'd argv through `recordingExecer` and pins it to `{"tmux","attach-session","-t","=foo:"}`.
  - `"it derives the scanned package set from the packages importing internal/tmux"` — `internal/tmux/target_composition_guard_test.go:133-141`. Asserts the derived map holds `cmd`, `internal/tmux` and `internal/restore` by import path, so the widening is proven rather than assumed.
  - `"it flags a bare target composed in cmd"` — `:143-166`. Copies `cmd`'s production sources into a temp dir (`stagePackageWithProbe`, `:175`), adds one authored offender (`probeRun("has-session", "-t", name)`), substitutes that dir for `cmd` in the derived set, and requires exactly one finding naming the probe file. This is the criterion's "staged probe", and it doubles as a zero-findings assertion over the whole real derived set.
  - `TestBareTargetGuard_ErrorsWhenTheImportScanResolvesNothing` (`:488-494`) covers "fails rather than silently empties" by feeding `importersOfTmux` a listing with no tmux importer; `TestBareTargetGuard_FatalsWhenItEnumeratesNoFiles` (`:498`) and `TestBareTargetGuard_FatalsWhenItFindsNoTargetTakingFuncs` (`:514`) keep the two other stopped-looking paths loud.
  - The fake was corrected in step with the production change: `daemonFakeCommander.dispatch` (`cmd/state_daemon_run_test.go:123-135`) now resolves `capture-pane`'s target through `sessionFromExactTarget`, so it keys on the plain coordinate the way real tmux resolves an exact-match target, and the existing `captureErrByTarget`/`captureByTarget` fixtures (`"work:0.0"`) keep working rather than being silently bypassed. Without that the pinned target would have missed every seeded error and the failure-path suites would have gone green for the wrong reason.
- Notes: No under- or over-testing found for this task. Each of the four criteria has exactly one assertion that would fail if it regressed; nothing here re-asserts a property another test already owns.

CODE QUALITY:
- Project conventions: Followed. The guard stays in the unit lane (no build tag, builds no portal binary, spawns no daemon or tmux server — it only shells `go list`), consistent with CLAUDE.md's lane rule and with the other ~20 source guards. It routes its enumeration and parsing through `internal/sourceguardtest` (`PackageGoFiles`, `ParsePackageSources`, `ForEachFuncCall`, `CalleeName`) rather than re-authoring the scan, and reports through `harnesstest.TestingT` so its own fatal paths are testable.
- SOLID principles: Good. `modulePackageListing` (I/O) / `importersOfTmux` (pure classification) / `targetComposingPackageDirs` (fatal-on-failure adapter) is a clean split — which is exactly what lets the empty-scan rule be unit-tested with a literal listing instead of a fabricated module.
- Complexity: Low. The derivation is a single pass over tab-separated lines; the production changes are one-line substitutions.
- Modern idioms: Yes — `sync.OnceValues`, `strings.SplitSeq`, `slices.Sorted`/`maps.Values`, `slices.Contains`.
- Readability: Good. Both production sites and the derivation carry comments stating the conclusion (why the `=` and the trailing `:`, why an empty importer set is an error rather than a clean repo) rather than restating the code, and every claim I checked holds: `PaneTargetExact` does render `=session:window.pane`, `importersOfTmux` does error on an empty set, and `stagePackageWithProbe` does copy rather than write into the repository.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
