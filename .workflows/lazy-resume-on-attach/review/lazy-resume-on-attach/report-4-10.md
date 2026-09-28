TASK: One Flag Reset the Next Chain Command Cannot Miss (lazy-resume-on-attach-4-10)

ACCEPTANCE CRITERIA:
- `resetRootCmd` resets every flag of every command in `stateChildCommands` to its `DefValue` with `Changed` cleared, and the hydrate-flag block it replaces is gone
- The three local helpers are gone and no test file declares another
- A chain subcommand's required-flag refusal fails for the right reason whatever order its siblings ran in
- A state child registered later is covered with no edit to `resetRootCmd`
- `go test ./cmd -count=1` and `go test ./cmd -shuffle=on` pass

STATUS: issues_found

SPEC CONTEXT: Test infrastructure with no specification section of its own. It protects the lazy-resume chain commands (`state resume-draw` / `resume-wait` / `resume-recover`, beside `state hydrate`). Each parses a payload from argv and refuses a missing `--command`. Cobra keeps flag values and `Changed` bits on the shared package-level command instances between Execute calls. A reset that misses a chain command therefore lets a refusal subtest pass without checking anything, and hides a stale-payload parse. The plan's phase-4 row (planning.md:157) states the same criteria.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/root_test.go:92-100: one `VisitAll` loop over `stateChildCommands` inside `resetRootCmd`. It sets each flag to `f.DefValue`, clears `Changed`, and carries the required-flag reason once as a comment.
  - The old `fifo`/`file`/`hook-key` hydrate block is gone (commit 6265f7f03). No other part of `resetRootCmd` changed: the help-flag pass, the `openCmd` re-`Init` and the per-command blocks are the same.
  - cmd/state_test.go:217-226: `stateChildCommands` lists all 8 state children.
  - cmd/state_test.go:250-253: asserts `len(stateCmd.Commands()) == len(stateChildCommands)`. A newly registered child that is left off the list fails this test, so the reset reaches it with no edit to `resetRootCmd`.
  - `resetResumeDrawFlags`, `resetResumeWaitFlags` and `resetResumeRecoverFlags` are deleted, together with their five call sites and the three `pflag` imports.
- Notes:
  - Only hydrate, resume-draw, resume-wait and resume-recover declare flags, plus Cobra's lazily added `help`. The types are string, int and bool, so `Set(DefValue)` round-trips for every one of them.
  - A later task (7-2, commit 58dad7d23) added a local reset again inside `executeResumeRecover`. It is redundant and cuts against criterion 2 (see FINDINGS). It is not code from this task.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/root_test.go:707-735, "it clears every flag on every state child". Table-driven over `stateChildCommands`: it sets every flag to a non-default value, calls `resetRootCmd`, and asserts `DefValue` and `!Changed`. It fails if no state child declares a flag. `nonDefaultFlagValue` (root_test.go:761-779) calls `t.Fatalf` on an unhandled flag type, so a future duration or slice flag cannot be skipped silently.
  - cmd/root_test.go:737-758, "it refuses a resume-wait naming no command after a sibling set one". It runs the full chain argv, resets, then runs with `--pane` only and asserts an error.
    - `--command` is the only required flag on resume-wait (cmd/state_resume_wait.go:501), and the RunE seam is stubbed to return nil. The error can therefore only come from the required-flag check.
    - Without the loop, every flag would still be Changed from the first Execute, the second Execute would succeed, and the test would fail. The test catches the regression it exists for.
  - The existing draw, wait and recover command suites now rely on `resetRootCmd()` alone.
- Notes:
  - The new refusal subtest overlaps the existing wait refusal subtest in cmd/state_resume_wait_test.go. The plan asked for it, and it is the version that does not depend on run order, so it is not over-testing.
  - `-shuffle=on` stability cannot be judged by reading; see UNSETTLED.

CODE QUALITY:
- Project conventions: Followed. There is no `t.Parallel()`, the seam is staged through `withFuncSeam`, and the comment gives the reason without citing process artifacts.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: One reintroduced local reset elsewhere in the delivered change-set (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_recover_test.go:355-359 — `executeResumeRecover` re-resets `--pane`/`--pane-key` by hand via `stateResumeRecoverCmd.Flags().Set(name, "")` before calling `resetRootCmd()` at line 370, which already resets every recover flag to its default with `Changed` cleared. Delete the five-line loop. `t` is still used by `t.Helper`, `withFuncSeam` and `t.Logf`, so no import or variable is orphaned. — FAILS: acceptance criterion "no test file declares another" no longer holds in the tree. The recover suite once more carries a per-file reset that duplicates the canonical one. That is the duplication this task removed, and it tells the next person writing a chain suite that `resetRootCmd` does not reach recover's flags, when it does.

UNSETTLED:
- "`go test ./cmd -count=1` and `go test ./cmd -shuffle=on` pass" — run `go test ./cmd -count=1` and `go test ./cmd -shuffle=on`, the latter over several seeds, and confirm both are green. Reading found no test that relies on a state-child flag surviving `resetRootCmd`; the only direct state-child flag write outside root_test.go is the redundant recover loop above, which `resetRootCmd` follows immediately.
