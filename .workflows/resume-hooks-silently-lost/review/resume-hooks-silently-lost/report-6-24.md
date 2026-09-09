TASK: resume-hooks-silently-lost-6-24 — Document the shape-aware reaper and the token vocabulary in CLAUDE.md (documentation-only; commit 5e270a9f)

ACCEPTANCE CRITERIA:
- The retain-forever rule is stated in CLAUDE.md with its justification.
- `IsTokenShaped` and `NewPaneToken` are both named in the `session` row.
- The helper-package row describes every file in the package(s) it covers.
- No CLAUDE.md statement about these surfaces is false after the edit.

STATUS: complete

SPEC CONTEXT: The specification's §3.2/§9 arrangement put the pane-token mint and the shape predicate in `internal/session`, then moved them to a stdlib-only `internal/nanoid` leaf (corrigendum 2026-08-30) with `internal/session/panetoken.go` forwarding; corrigendum 2026-09-01 (specification.md:560) records that the forwarder was deleted, `cmd/hooks.go` reaches the leaf directly, and the leaf now holds two unexported widths — a general-purpose `width` behind `NewGenerator` and `paneTokenWidth` behind `NewPaneTokenGenerator`, the width `IsTokenShaped` reads. The safety property this task documents is the spec's retain-the-unjudgeable rule: a persisted hook key the staleness rule cannot judge holds a user-authored `on-resume` command with no other copy, so it is retained permanently.

IMPLEMENTATION:
- Status: Implemented (delivered end-state differs from the criteria's literal wording where the code moved; see Notes)
- Location:
  - CLAUDE.md:190 — the retain-forever paragraph inside the "Resume hooks" section (heading at CLAUDE.md:182): the rule, its shape gate (token-shaped per `nanoid.IsTokenShaped`, or empty), the pre-token `<session>:<window>.<pane>` shape it protects, the "inert but not cruft" framing, the sanctioned removals, and the explicit "do not add an expiry/migration/tidy pass".
  - CLAUDE.md:72 — the `hooks` row naming the shape-aware staleness rule as the reason the store consults the id vocabulary.
  - CLAUDE.md:79 — the `nanoid` row (added by this task, later amended by the width split) naming `Alphabet`, the two unexported widths, `NewGenerator`, `NewPaneTokenGenerator` and `IsTokenShaped`.
  - CLAUDE.md:64 — the `session` row.
  - CLAUDE.md:89 — the helper-package row (`transienttest` / `hookstest` among others).
- Notes:
  - AC2 asks for `IsTokenShaped` and `NewPaneToken` in the `session` row. In the delivered tree neither symbol lives in `internal/session`: `internal/session/` holds create/naming/dirresolve/prepare/quickstart only, and the vocabulary is `internal/nanoid/nanoid.go:35` (`NewPaneTokenGenerator`), `:58` (`paneTokenWidth`), `:65` (`IsTokenShaped`). This task's commit did name `panetoken.go` / `NewPaneToken` in the `session` row at the time it landed; task 7-30 later deleted the forwarder and removed that sentence, adding nothing false in its place. The criterion's substance — the vocabulary named where an agent reading CLAUDE.md will meet it, and the row no longer mis-describing its package — is met by the `nanoid` row plus the current `session` row, which claims no token ownership. Sound divergence, not a loss.
  - AC1 verified in full against code: `internal/hooks/store.go:248` `StaleKeys` is the only implementation (no unexported `staleKeys` survives; `internal/hooks/cleanstale_staleness_guard_test.go` pins that), `:258` is the `key == "" || nanoid.IsTokenShaped(key)` gate the paragraph describes, and the three readers it names are real and exhaustive: the daemon's throttled sweep (`cmd/state_daemon.go:214` → `hooksweep.Run`), `portal doctor --fix` (`cmd/doctor.go:201`), and doctor's read-only count (`cmd/doctor.go:387`). The `:` and `.` of the legacy key are absent from `nanoid.Alphabet` (`internal/nanoid/nanoid.go:17`), so the "can never be mistaken for a token" claim holds. The sanctioned escape hatch exists: `cmd/hooks.go:310` registers `--pane-key`, documented as taking any key including an old-format one, and `cmd/hooks.go:270-271` passes it through verbatim. The "a fired key is always a token baked from saved state" claim holds — `internal/restore/session.go:70` bakes the hook key from `Pane.PortalPaneID`.
  - AC3 verified: `internal/transienttest/` now holds exactly `commander.go`, `socket.go`, `doc.go`, and the row's "list-panes -a tmux-transient scaffolding and nothing else (`Commander` + `FailureMode` + `PassThrough`/`FailExitNonZero`/`FailEmptyStdout`, `SocketCommander` pass-through)" matches those files symbol for symbol (`internal/transienttest/commander.go:11-17,25`, `socket.go:13`). Its named consumers are the only three importers in the tree (the two `cmd` doctor-fix transient suites and `cmd/bootstrap/transient_listpanes_helpers_integration_test.go`). The `hooks_lock.go` surface the task flagged as undescribed now lives in `internal/hookstest/hooks_lock.go` and is described in the same row (`CreateHooksSidecar`, `HoldHooksSidecar`, `HoldHooksSidecarShared`, `AssertSidecarFree`, `UnlockedRecords`/`AssertDegradedRead`, `AssertLockWarn` — all present at `internal/hookstest/hooks_lock.go:26,39,60,76,98,108,135`), alongside `hooks.go`'s seed vocabulary (`ReapableSeed*`/`LiveSeed*`/`SubjectSeed*`/`UnjudgeableSeed*`/`StaleHookSeed` at `internal/hookstest/hooks.go:167-209`, constructors `tokenShapedHookKey`/`unjudgeableHookKey` unexported as claimed) and `staging.go`'s `Staging`/`StageStore`/`HooksPath` (`internal/hookstest/staging.go:14,55,102`, every declared field present). Every file in both packages is covered.
  - AC4: every claim in the regions this task touched was checked against the tree and none is false. `internal/nanoid/leaf_guard_test.go` backs the row's "a package guard test pins the stdlib-only dependency set"; the three packages the row says must not import each other (`hooks`, `session`, `spawn`) each reach the leaf directly (`internal/hooks/store.go:258`, `internal/session/naming.go:14`, `internal/spawn/ackid.go:15`). No stale reference to `session.NewPaneToken`, `session.IsTokenShaped`, `panetoken.go` or `tokenshape.go` survives in CLAUDE.md or README (only `.workflows/` records, which are history rather than references).
  - Register held: the added paragraph carries no section numbers, no spec cross-references, and reads in CLAUDE.md's dense load-bearing style.

TESTS:
- Status: Adequate (none required)
- Coverage: The task is documentation-only and prescribes verification by grep that each named symbol exists at the claimed path; that verification was performed above for every symbol and path named in the touched regions. No new guard is warranted — CLAUDE.md prose is not guard-tested anywhere in this repo, and the underlying properties it describes already carry their own guards (`internal/hooks/cleanstale_staleness_guard_test.go` for the single-home rule, `internal/nanoid/leaf_guard_test.go` for the leaf dependency set, `cmd/hooks_pane_token_width_guard_test.go` for the mint being the pane-token generator).
- Notes: No over-testing, no under-testing to report.

CODE QUALITY:
- Project conventions: Followed. The amendment sits in the correct sections (package table row for the map, "Resume hooks" for the safety rule), states the consequence rather than the mechanism, and names no spec section.
- SOLID principles: N/A (documentation).
- Complexity: N/A.
- Modern idioms: N/A.
- Readability: Good. The retain-forever paragraph leads with the consequence ("deleting one is data loss"), gives the rule, the shape it protects, why the shape is unjudgeable, and closes with the explicit prohibition — which is the shape that survives an agent skimming for permission to tidy.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
