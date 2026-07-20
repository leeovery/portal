TASK: cli-verb-surface-redesign-5-1 — Retire `attach`: delete the command and migrate its behaviours to `open --session`

ACCEPTANCE CRITERIA (task edge cases from the plan row + Phase 5 ACs #1/#4/#5):
- `SessionValidator` relocated (still consumed by `kill`), not deleted
- `mockSessionValidator` relocated to a surviving `_test.go`
- abridged latch-satisfied fast-path stays command-agnostic — `abridged_route_test` migrates attach→`open --session` (AC #5 regression)
- inside-tmux switch-client vs outside exec-attach reattach cases migrate to `open --session`
- session-not-found hard-fail preserved
- `--spawn-ack` removed, role now `open --ack`
- `internal/spawn` AckWriter/NewServerOptionAckChannel/ParseSpawnAckFlag retained
- drop the stale attach argv→role case in `internal/log` ResolveProcessRole + test
- no compat alias / no deprecation warning
- version_guard/root/root_integration attach cases migrated or removed + `attachCmd` flag-reset removed
- picker keymap "attach" action label is UI copy, untouched

STATUS: Complete

SPEC CONTEXT:
Spec §"attach — Retired" mandates `portal attach` be deleted outright — not aliased, not deprecated-with-warning. Its two jobs are absorbed by `open`: (1) exact/no-guess attach for scripts → `open --session <name>`; (2) the spawned host-window exec target → `portal open --session <name> --ack <batch>:<token>`. Both `open` and former `attach` already call the same in-process connect() (exec `tmux attach-session` outside / `switch-client` inside). Spec §"Back-Compat" reiterates the no-alias posture. AC #5: the abridged latch-satisfied fast-path is command-agnostic (keys on the `@portal-bootstrapped` version-stamped latch, never on the verb), so `open` takes the same fast-path attach did.

IMPLEMENTATION:
- Status: Implemented (clean, complete)
- Location / evidence (commit e7afb76a):
  - `cmd/attach.go` (95 LOC) and `cmd/attach_test.go` (374 LOC) deleted outright. No `attachCmd`/`AttachDeps`/`attachDeps`/`buildAttachDeps`/`runAttach` references remain anywhere (grep clean). `go build ./...` succeeds.
  - `SessionValidator` interface relocated to `cmd/kill.go:18-20`; still consumed by `buildKillDeps` (`cmd/kill.go:49`). `kill_test.go` uses `mockSessionValidator` (6 sites).
  - `mockSessionValidator` (+ `mockSessionConnector`, `mockAckWriter`) relocated to the new `cmd/session_mocks_test.go`; header documents the relocation and consumers (kill/version_guard/abridged/reattach).
  - `internal/log/process_role.go`: `case "open", "x", "attach":` → `case "open", "x":`; doc-comment table line updated (`open … / x … / attach … / bare` → `open … / x … / bare`).
  - `--spawn-ack` fully gone: no live cobra flag registration remains; `open --ack` registered and `MarkHidden` at `cmd/open.go:1003-1004`. `attachCmd` `spawn-ack` flag-reset removed from `root_test.go:resetRootCmd`.
  - `internal/spawn` AckWriter/NewServerOptionAckChannel/ParseSpawnAckFlag retained and reached via `cmd/open.go` (writeAckMarker/buildAckWriter).
  - No compat alias / no deprecation warning — proven by `cmd/retired_surface_test.go`.
  - Picker keymap "attach" action labels (`internal/tui/keymap.go:93,194`) untouched (UI copy) — correct per AC.
- In-scope routing fix (correctly discovered): the commit also fixed `isTUIPath` (`cmd/root.go`) so a domain-pin `open --session/-p/-z/-a` is NOT classified as the TUI path. Previously `open --session foo` has `len(args)==0`, so it wrongly took the concurrent-bootstrap loading-page route; it now takes the synchronous direct-path bootstrap (restore before resolution) exactly as retired `attach` did. New helper `anyOpenDomainPin`; `-f/--filter` and `-e/--exec` still flip TUI (picker). This is a faithful part of "migrate attach's behaviours to open --session" (attach was always synchronous), not scope creep, and is thoroughly tested.
- Notes: All remaining "attach" strings in production `cmd/*.go` refer to the attach *outcome* (attach-vs-mint), `AttachConnector`, `attach-session`, or the "attached" list status — correct domain vocabulary, not the deleted command.

TESTS:
- Status: Adequate (well-balanced; no over-testing observed)
- Coverage:
  - `cmd/abridged_route_test.go` — `TestPersistentPreRunE_Abridged_OpenSessionTakesAbridgedPath` is the AC #5 regression: `open --session` + satisfied latch → orchestrator calls == 0 AND still connects. Companion cases prove not-satisfied verdicts fold into full bootstrap and warning-drain parity holds.
  - `cmd/concurrent_bootstrap_gate_test.go` (new) — `TestIsTUIPath` + `TestShouldRunConcurrentBootstrap` table-cover each domain pin (session/path/zoxide/alias) routing non-concurrent/synchronous, while `-f`/`-e` remain concurrent TUI paths; `TestShouldRunConcurrentBootstrap_IssuesNoProbe` pins zero tmux round-trips.
  - `cmd/reattach_integration_test.go` — migrated to `open --session`: inside-tmux `SwitchConnector.Connect → switch-client` case, outside-tmux `AttachConnector` exec-attach case, and the session-not-found hard-fail (`open --session nope-not-here` → `No session found: nope-not-here`, connector NOT invoked — proves never-mints/never-picker preserved).
  - `cmd/version_guard_test.go` — attach case migrated to `open --session` with `openSessionFunc` no-op routing.
  - `cmd/root_test.go` / `cmd/root_integration_test.go` — attach rows removed (behaviour still covered by open/kill rows); `attachCmd` flag-reset removed.
  - `internal/log/process_role_test.go` — `attach foo` removed from both the role table and the closed-result-space table; contributor-note comment updated.
  - `cmd/retired_surface_test.go` — asserts no child named attach, no cobra alias resolving to attach, absent from `--help`/generated completion, and absorbed behaviours (`--session`, hidden `--ack`, ≥2 positionals) reachable via `open`.
- Notes: Tests are focused; each file targets a distinct concern (bootstrap routing, reattach connectors, version guard, retired-surface guard, process-role taxonomy). No redundant duplication.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel` in cmd tests (documented headers); deps injected via package-level `*Deps` + `t.Cleanup`; integration reattach test carries `//go:build integration`; `SessionValidator` kept as a 1-method interface.
- SOLID: Good. `anyOpenDomainPin` extracted as a single-responsibility predicate; SessionValidator narrow interface segregation preserved.
- Complexity: Low. `isTUIPath`/`anyOpenDomainPin` are flat boolean checks.
- Modern idioms: Yes.
- Readability: Good. Doc comments on `isTUIPath`/`anyOpenDomainPin` explain the retire-attach rationale and cite the spec.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/reattach_integration_test.go:3 — header comment reads "Phase 5 task 5-10"; the task is 5-1 (no 5-10 exists in Phase 5, and L8 already cites the 5-1 planning row). Fix the stale task id to 5-1.
- [do-now] internal/spawn/recipe_test.go:170,174 — `TestRenderCommandString` uses `"attach"`/`"--spawn-ack"` as the sample argv (a retired verb + removed flag). Replace with `open`/`--session`/`--ack` sample tokens and update the coupled `want` string. Zero logic risk (renderCommandString quotes any argv identically). NB: `internal/spawn` is nominally 5-2's "otherwise untouched" territory, but these literals reference exactly the verb+flag this task retired.
