# Consolidation Tasks: Lazy Resume On Attach (Phase 18)

## Task 1: One Declaration of Which Panes a Scrollback Dump Skips
placement: phase 18
severity: drift

**Problem**: `CaptureCycle` (`internal/state/scrollback.go:256-269`) carries three sets a caller's scrollback dump must skip — `Skeleton`, `Pending` and, since this phase, `Carried` — but the rule that combines them is written only in `cmd`, as the daemon's private `paneSkipsScrollback` (`cmd/state_daemon.go:340-351`, called at `:304`). Every other `Dump` must restate it, and one already fell behind silently: the lazy-panel integration fixture's inline skip (`internal/restore/lazy_resume_panel_integration_test.go:497-502`) checks `Skeleton` and `Pending` only, while its doc comment (`:480-485`) says it captures "the way the daemon takes one". Any carry scenario added to that suite would capture-pane a carried session's address the daemon never touches — writing another pane's bytes into a carried sibling's file, or failing on a gone session's target — and so pass or fail on behaviour the daemon does not have. The next set added to `CaptureCycle` meets the same gap, since nothing points its author at every `Dump` that must learn it.

**Solution**: Declare the rule once, on the type that owns the sets — derived from the project's own convention of one declaration per rule a divergence would break silently:
- `func (c CaptureCycle) SkipsScrollback(paneKey string) bool` in `internal/state/scrollback.go`, beside `CaptureCycle`, carrying the reason comment from `cmd/state_daemon.go:340-343`.
- `paneSkipsScrollback` is deleted and the daemon's dump calls `capture.SkipsScrollback(paneKey)`.
- The fixture's two inline checks call the same method, so its "the way the daemon takes one" claim holds again.
- The `Dump` field doc in `internal/state/commit_cycle.go:39-41` names the method as the skip a dump takes.

**Outcome**: One declaration of the dump skip, in the package that owns the sets; the daemon and the integration fixture both take it, and production behaviour is unchanged.

**Acceptance Criteria**:
- [ ] A daemon tick over a skeleton-marked pane neither capture-panes it nor writes its scrollback file, while the tick's other panes are dumped as before — `TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` (`cmd/state_daemon_run_test.go`) passes unedited
- [ ] A daemon tick over a pane carrying `@portal-resume-pending` neither capture-panes it nor rewrites its scrollback file — `TestCaptureAndCommit_SkipsResumePendingPanes` (`cmd/state_daemon_resume_pending_test.go`) passes unedited
- [ ] A daemon tick in which a waiting pane's session missed the capture commits the carried session and capture-panes none of its panes — `TestDaemonTick_CarriesAFailingWaitingSessionWithoutCapturingIt` (`cmd/state_carry_missed_session_test.go`) passes unedited
- [ ] The lazy-panel fixture's capture round skips exactly the panes the daemon's tick skips, `Carried` included, and every suite driving it (`internal/restore/lazy_resume_panel_integration_test.go`, `lazy_resume_discard_integration_test.go`, `lazy_resume_renumbered_restore_integration_test.go`, `lazy_resume_burst_integration_test.go`) passes with no assertion changed
- [ ] `rg -n 'paneSkipsScrollback' --glob '*.go'` finds nothing, and neither dump over a `CaptureCycle` indexes `Skeleton`, `Pending` or `Carried` itself — both ask `CaptureCycle.SkipsScrollback`
- [ ] `go test ./...`, `go test -tags integration -p 1 ./...` and `golangci-lint run` pass; the only test-file edit is the fixture's skip

**Do**:
- The set, measured on the tree as it stands:
  - `rg -n 'paneSkipsScrollback' --glob '*.go'` — 2 hits: the declaration (`cmd/state_daemon.go:344`) and its sole call (`cmd/state_daemon.go:304`, in `scrollbackDump.run`).
  - `rg -n '\.(Skeleton|Pending|Carried)\[' --glob '*.go'` — 4 hits. `internal/restore/lazy_resume_panel_integration_test.go:497` and `:500` are the fixture's inline skip in `captureRound`, and are converted. The other two are not dump skips and stay: `internal/state/commit_cycle_test.go:311` asserts commit-now's pending set holds a pane, and `internal/tui/session_list_pending_read_test.go:156` reads a message type's own `Pending` field.
  - `rg -n 'Dump:\s' --glob '*.go'` — 11 hits. Two dump scrollback and are converted: the daemon (`cmd/state_daemon.go:262`, `dump.run`) and the fixture (`internal/restore/lazy_resume_panel_integration_test.go:525`). The other nine, in `internal/state/commit_cycle_test.go` and `internal/state/capture_carry_test.go`, are closures that capture nothing.
  - `rg -n 'state\.CaptureAndHashPane\(' --glob '*.go'` — 6 hits, every scrollback capture in the tree. Beside the daemon's `dumpPane` (`cmd/state_daemon.go:320`) and the fixture (`:504`), three are `internal/state`'s own tests of that function (`scrollback_test.go:195`, `:211`, `capture_period_session_realtmux_test.go:65`), and one is outside the set: `runDaemonTick` (`cmd/bootstrap/daemon_tick_test_helpers_test.go:68`) takes `state.CaptureStructure`'s skip set directly, has no `CaptureCycle` to ask, and its skeleton guard is switchable off (`withoutSkipGuard`) for a negative control — it stays as it is.
- Declare `func (c CaptureCycle) SkipsScrollback(paneKey string) bool` in `internal/state/scrollback.go` beside `CaptureCycle` (`:256-269`), answering true for a key in any of `Skeleton`, `Pending` or `Carried`, with the reason comment now at `cmd/state_daemon.go:340-343` moved onto it.
- Delete `paneSkipsScrollback` (`cmd/state_daemon.go:340-351`); `scrollbackDump.run` calls `capture.SkipsScrollback(paneKey)` at `:304`.
- Replace the fixture's two inline checks (`internal/restore/lazy_resume_panel_integration_test.go:497-502`) with one `capture.SkipsScrollback(key)` check; the doc comment at `:480-485` then stands as written.
- The `Dump` field doc in `internal/state/commit_cycle.go:39-41` names `CaptureCycle.SkipsScrollback` as the skip a dump takes.

## Task 2: Corrections
placement: phase 18
severity: corrections

**Problem**: CLAUDE.md describes the capture cycle and a waiting pane's writes as they were before this phase, in two places, and contributors and agents write new committers and hydrate changes from it. A new `Dump` written from the `state` row would skip only `Skeleton` and `Pending` and could capture the panel over a waiting pane's transcript; a reader of the lazy paragraph could take the helper's token write for a violation of the never-read-the-live-token rule and remove it, leaving the sweep to reap a waiting pane's registration whenever restore's re-stamp failed. The same two passages leave a reader chasing a `tick failed` WARN raised by a carry landing on a reached session's name with nothing that explains it, and leave an eager pane whose `set resume pending marker failed` WARN names a token-write error matching none of the listed downgrade causes.

**Solution**: Two edits, derived from the landed code (`internal/state/scrollback.go:256-269`, `internal/state/capture.go`, `cmd/state_daemon.go`, `cmd/state_hydrate.go:384-408`) and the specification's §7.2, §7.3 and §8.2 as corrected this pass:
- CLAUDE.md:61, the `state` row — state that `captureAndRefile` returns `CaptureCycle{Index, Pending, Skeleton, Carried}`, the last being the pane keys of a session carried forward from the previous index, and that the dump skips what `CaptureCycle.SkipsScrollback` answers (Task 1) rather than listing the sets; add one sentence on the carry: a live waiting pane whose session missed the capture has its previous session carried whole under its previous name for that cycle, with no token put on a second record, and a carry onto a reached session's name fails the capture so the callers' existing failure routes retry; change "discards both sets" to "discards the sets".
- CLAUDE.md:190, the Resume hooks lazy paragraph — put the token write first in the ordered writes (the baked key written as the pane's `@portal-pane-id`, with no read first, then the pin, then the marker), and add a refused token write to the causes that downgrade the pane to eager under the same WARN.

**Outcome**: CLAUDE.md's `state` row and Resume hooks lazy paragraph describe the tree as it stands: the capture cycle returns three skip sets whose combination a dump reads through `CaptureCycle.SkipsScrollback`, the missed-session carry and its refused commit are stated, and the lazy pane's writes and downgrade causes open with the token write.

**Acceptance Criteria**:
- [ ] The `state` row states that `captureAndRefile` returns `CaptureCycle{Index, Pending, Skeleton, Carried}` and that `Carried` holds the pane keys of a session carried forward from the previous index
- [ ] A contributor writing a new `Dump` from the `state` row is pointed at `CaptureCycle.SkipsScrollback` as the skip it takes; the row no longer states the saver's skip as mid-restore **or** pending, as `CaptureCycle.Skeleton` or `CaptureCycle.Pending`, or as "two sets"
- [ ] The `state` row carries one sentence on the carry: a live waiting pane whose session missed the capture has its previous session carried whole under its previous name for that cycle, with no token put on a second record, and a carry onto a reached session's name fails the capture so the callers' existing failure routes retry
- [ ] The `state` row's commit-now sentence says it discards the sets, not both sets
- [ ] The lazy paragraph orders the lazy pane's writes as `markResumePending` does: the baked key written as the pane's `@portal-pane-id` with no read first, then the alternate-screen pin, then `@portal-resume-pending`, all before the mid-restore marker is cleared
- [ ] The lazy paragraph's causes that downgrade a pane to eager include a refused token write, under the same `set resume pending marker failed` WARN, so the list names every refusal `markResumePending` can return
- [ ] Every identifier the edited text names exists in the tree, and nothing in CLAUDE.md outside the edited sentences changes

**Do**:
- CLAUDE.md:61, the `state` row. The sentences the first edit replaces, as they stand: "The saver's per-pane scrollback skip is mid-restore **or** pending — `CaptureCycle.Skeleton` (the positional skeleton markers, as before) or `CaptureCycle.Pending` (that second return), …"; "returning a `CaptureCycle{Index, Pending, Skeleton}` whose two sets the caller's own scrollback dump must skip"; and "`state commit-now` reads `sessions.json` under the lock, discards both sets and dumps nothing". The carry sentence is taken from `internal/state/capture.go:44-49` (the carry) and `:195-199` (the refusal, `errCarryNameTaken`); the failure routes it names are the daemon's `tick failed` WARN and `save.requested` re-touch (`cmd/state_daemon.go:201-202`) and commit-now's `failCommitNow` (`cmd/state_commit_now.go:141-147`). The method it names is the one Task 1 declares, so this edit lands after Task 1.
- CLAUDE.md:190, the lazy paragraph. The phrases the second edit replaces, as they stand: "has the pane first pinned to a pane-level `alternate-screen on` … and then marked `@portal-resume-pending`, both **before** the mid-restore marker is cleared", and "an absent `$TMUX_PANE`, an unresolvable executable, a refused alternate-screen pin or a failed marker write each downgrade the decision to eager". Both are taken from `markResumePending` (`cmd/state_hydrate.go:393-419`), which resolves the pane and the executable, then writes the token (`:406`), the pin (`:409`) and the marker (`:412`), and from the WARN at `cmd/state_hydrate.go:372`.
