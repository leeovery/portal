TASK: open-with-forced-filter-7-2 (tick-424d49) — Hold the command-pending picker's mint until the bootstrap's terminal event arrives

ACCEPTANCE CRITERIA:
1. On a command-pending model with a non-nil `progressReceiver` and `bootstrapComplete` false, Enter on a project and `n` in the cwd both reach `sessionCreator` with nothing: no `CreateFromDir` call, no `SessionCreatedMsg`, no quit.
2. The staged directory is minted from the `BootstrapCompleteMsg` arm, forwarding the same `command` an immediate mint would forward, and the mint is issued for exactly the directory the keypress chose.
3. No staged mint is ever issued from the `BootstrapFatalMsg` arm, nor by a `BootstrapCompleteMsg` arriving after a fatal; the fatal still returns `tea.Quit` and the run ends non-zero having minted nothing.
4. A warm command-pending model (nil `progressReceiver`) mints on the keypress exactly as it does today.
5. While a mint is staged, `activeProjectNoticeBand` returns the wait claim in place of the `bandCommand` banner, and returns the banner again once the mint is issued.
6. A live flash still claims the Projects slot ahead of the wait band.
7. `projectBandHeight` matches the rendered slot while the wait band is live, and the list budget is re-measured at both the staging and the issuing.
8. Esc and Ctrl-C still quit while a mint is staged.
9. The loading-gated routes are untouched: the existing `internal/tui` and `cmd` suites pass unchanged, with no new call reaching a real tmux server.

STATUS: complete

SPEC CONTEXT:
§7.6 ("The concurrent path itself is unchanged") now carries the delivered behaviour as its requirement: the command-pending picker is the one member of the concurrent-bootstrap set with no loading gate, and "what protects it is not a gate in front of the picker but a hold behind it: while the bootstrap is in flight, choosing a project stages the chosen directory rather than minting at it". The section names four obligations — the staged directory is replayed as a mint from the bootstrap's own terminal event, the notice band announces the wait in place of the pick-a-project banner, the first stage wins, and a bootstrap fatal ends the run rather than minting. The Corrigendum 2026-09-15 closes the defect the 2026-09-14 corrigendum had opened against §7.6/§7.2, and names this task's code as the closure.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:228-233` — `stagedMint` / `stagedMintDir`, beside the bootstrap fields, with the comment stating why the bool is separate from the string (a staged empty cwd stays distinguishable from nothing staged).
  - `internal/tui/model.go:1836-1842` — `bootstrapInFlight()`, the single named predicate (`progressReceiver != nil && !bootstrapComplete`); its comment correctly records that a fatal leaves `bootstrapComplete` false, so a caller that must exclude one checks `fatalActive` itself.
  - `internal/tui/model.go:1844-1864` — `createSession`: `fatalActive` refusal unchanged (1848-1850), in-flight branch stages + `resyncPageLayouts` + returns no command (1851-1863), first-stage-wins early return at 1854-1856, otherwise `mintSession` (1864).
  - `internal/tui/model.go:1869-1877` — `mintSession`, the un-gated mint reached from the keypress and from the terminal event.
  - `internal/tui/model.go:1681-1692` — the `BootstrapCompleteMsg` replay, sequenced after the `pendingBootstrapWarnings` append (1679) and under the arm's `fatalActive` early return (1660-1662).
  - `internal/tui/model.go:1698-1711` — `BootstrapFatalMsg`: sets `fatalActive`, issues no mint, keeps `return m, tea.Quit` for a command-pending model (1708-1710).
  - `internal/tui/model.go:1963` / `:2876` / `:2880` — the two call sites, each assigning both returns to locals.
  - `internal/tui/notice_band.go:35-37` — the copy constant, verbatim as prescribed; `:205-209` — the `bandInfo` arm, below `flashSlotClaim` (202) and above the `commandPending` arm (210).
- Notes:
  - The plan asked for a pointer-receiver `createSession`; the delivered code instead keeps a value receiver and returns `(Model, tea.Cmd)`. That is a sound divergence, not a loss: it removes the evaluation-order hazard the plan's own "assign to a local before returning" caution existed to work around, and it matches the shape `dismissLoadingGate` (`model.go:1509`) already uses in this file. Both call sites were updated to the two-value form and no other caller of `createSession`/`createSessionInCWD` exists in the tree.
  - The hold has no bypass on this route. A command-pending model cannot reach the Sessions page and attach instead: `x` is refused while `commandPending` (`model.go:1921-1924`) and `evaluateDefaultPage` pins `PageProjects` for a command-pending model (`model.go:1301-1302`), so Enter/`n` are the only acting keys and both route through `createSession`.
  - The gate is live in production, not dead code: `portal open -- <cmd>` carries no pre-dash positionals, so `isTUIPath` (`cmd/root.go:173-177`) admits it to `shouldRunConcurrentBootstrap`, and `openTUI` wires the pipe's receiver into the picker's deps (`cmd/open.go:722-723` → `:585`).
  - Criterion 7 holds structurally as well as by test: `projectBandHeight` (`model.go:1132-1138`) measures `renderProjectBandSlot`, which reads the same arbiter, so the wait band's rows are reserved by construction rather than by a parallel count.
  - Criterion 2's warnings ordering holds end to end: the staged replay sits below the append, and `finishTUI` writes `WarningsOwedAtTeardown()` before `processTUIResult` connects (`cmd/open.go:621-628`).
  - The staged replay returns before the arm's `minElapsed && activePage == PageLoading` dismissal (`model.go:1693-1696`). That ordering is unreachable for a loading-gated model — such a model cannot reach `createSession` before `bootstrapComplete`, since the Projects page is only reached through `dismissLoadingGate` — so no behaviour rides on it.

TESTS:
- Status: Adequate
- Coverage: All ten named tests are present and assert the named behaviour.
  - `internal/tui/command_pending_staged_mint_test.go` (black-box, `package tui_test`) covers criteria 1, 2, 3, 4 and 8: staging on Enter (`:50`), staging on `n` (`:96`), the replay with its command and directory and the warnings still owed (`:67`), first-stage-wins (`:117`), the warm immediate mint (`:137`), the fatal and the complete-after-fatal (`:151`), and Esc while staged (`:173`).
  - `internal/tui/staged_mint_band_test.go` (in-package, for the unexported arbiter) covers criteria 5, 6 and 7, including the pre-stage banner baseline (`:28`) and the post-issue return to it (`:42`).
  - Fixtures are honest: `commandPendingDeps` wires a `mockSessionLister`/`mockSessionCreator` and a synthetic receiver closure; `stagedMintBandModel` builds from `noticeBandModel` (termWidth/termHeight set, so `resyncPageLayouts` does not early-return). No tmux client is constructed on either path, and no `t.Parallel()` is used.
  - The width-34 budget fixture guards itself against going vacuous (`staged_mint_band_test.go:72-74` fails if the band no longer wraps past the banner), which is what makes the "re-measured" assertion meaningful.
- Notes: Not over-tested — each subtest pins one distinct behaviour and there is no duplicated assertion across the two files. Ctrl-C is asserted only for Esc's sibling path, which is sound: `keyIsCtrlC` is handled unconditionally at `model.go:1901-1903`, above every staging-aware branch, so staging cannot reach it.

CODE QUALITY:
- Project conventions: Followed. The band arm is token-based (`bandInfo`), so the `colour_literal_guard_test.go` rule is untouched; no new log component or attr; the `(&m)`/value-receiver mix matches the file's established shape.
- SOLID principles: Good. The gate is one predicate (`bootstrapInFlight`) read at one chokepoint, and the mint was extracted to `mintSession` so the keypress route and the event route share one implementation rather than two.
- Complexity: Low. Two new branches in `createSession`, one in the complete arm, one in the arbiter.
- Modern idioms: Yes.
- Readability: Good.
- Comment accuracy: Every comment added by this task holds against the code. Checked specifically: the `bootstrapInFlight` claim that a fatal leaves `bootstrapComplete` false (true — `BootstrapFatalMsg` sets only `fatalActive`/`fatalStep`/`fatalMessage`/`fatalErr`), the replay's claim that it is sequenced after the warnings append (true), and the arbiter's claim that it sits above the `commandPending` arm (true).
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The loading-gated routes are untouched: the existing `internal/tui` and `cmd` suites pass unchanged, with no new call reaching a real tmux server." — the second half is settled by reading (the new tests construct only mocks and in-package fixtures; nothing in either file reaches `tmux.DefaultClient` or a socket). The first half is not: it needs `go test ./internal/tui ./cmd` — and, because the `createSession` signature changed, a compile of the whole `internal/tui` test binary — to confirm no existing suite was left behind by the two-value form.
