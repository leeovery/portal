TASK: resume-hooks-silently-lost-6-25 — Clear The Repo-Wide Lint And Format Debt (tick-5c5992)

ACCEPTANCE CRITERIA:
- `gofmt -l .` prints nothing.
- `golangci-lint run ./...` reports zero issues.
- Both lanes green (`go test ./...` and `go test -tags integration -p 1 ./...`).
- No behavioural change in the diff.

STATUS: complete

SPEC CONTEXT:
This is a phase-6 implementation-analysis task, not specified bugfix work — per the shared verifier
context, its authority is its own body rather than the specification. Its stated purpose is hygiene:
lint is not wired into CI (CLAUDE.md, "Build & Test"), so pre-existing `modernize` noise and one
unformatted file would otherwise hide a genuine new finding from a local run. Nothing in the
resume-hooks specification bears on it.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `ca46c467` — 41 files, +83/−157. Production changes are confined to five files:
  `cmd/bootstrap_progress.go:132`, `cmd/root.go:198`, `internal/spawn/exec_boundary.go:19`,
  `internal/tmux/command_error.go:45` and `:72`, `internal/tmux/tmux.go:102` and `:356`, `main.go:63`
  and `:73`. Everything else is `_test.go`.
- Notes:
  Exactly four kinds of edit, all semantics-preserving, and I checked each rewrite site individually:
  1. `var e *T; if errors.As(err, &e)` → `if e, ok := errors.AsType[*T](err); ok` (the bulk). Every
     site's extracted variable was used only inside the `if` body or not at all, so the tightened
     scope is safe; sites where the variable is read *after* the `if` were correctly left on
     `errors.As` (e.g. `cmd/root_test.go:565`, `cmd/bootstrap/errors_test.go:60`,
     `internal/tmux/tmux_test.go:497` — all read the extracted value below the block).
     `errors.AsType[E](err)` is defined as `var e E; ok := errors.As(err, &e)`, so the truth value
     and the extracted value are identical; the module is `go 1.26.0` (go.mod:3), where the API exists.
  2. `strings.SplitN(s, "\n", 2)[0]` → `before, _, _ := strings.Cut(s, "\n")` at
     `internal/tui/projects_header_test.go:52` and `internal/tui/section_header_test.go:206`.
     `SplitN(s, sep, 2)[0]` and `Cut`'s first result agree on both the separator-present and
     separator-absent cases, so this is exact.
  3. `fields.Field(i).Interface().(Token)` → `reflect.TypeAssert[Token](fields.Field(i))` at
     `internal/theme/validate_test.go:220`. Same `(value, ok)` contract and the same panic conditions
     (zero Value / unexported field); the `Theme` fields are exported and the old form already worked.
  4. One gofmt realignment in `internal/tui/help_modal_test.go:294`.
  No control flow, no error classification, no logging, no argv and no persisted format is touched.
  Nothing crosses the tmux/daemon/state boundaries CLAUDE.md's isolation invariants govern.

  Acceptance verified against the delivered tree (HEAD `41596acd`) by read-only inspection:
  - `gofmt -l .` — no output.
  - `golangci-lint run ./...` — "0 issues", under the repo's own `.golangci.yml` (standard set +
    `modernize`, `build-tags: integration`, both issue caps lifted). Both criteria hold.
  Note the config's `build-tags`/uncapping arrived later, in task 7-10 (`11c155bb`); the executor
  nonetheless swept the uncapped set here (commit message: 77 findings, vs the 30 the capped plain
  command reported), so the later, stricter invocation found nothing left to report. No file this
  commit touched carries a build constraint, and the only two `//go:build !integration` files in the
  repo (`internal/state/pgrep_sandbox_prod.go`, `internal/sourceguardtest/buildconstraint_test.go`)
  are untouched — so the lint run's type-check covered the whole change-set in both lanes' shapes.

TESTS:
- Status: Adequate (no new tests warranted)
- Coverage: The task's own micro acceptance is "both lanes green; the diff reviewed for
  behaviour-neutrality" — correct for a mechanical sweep. Every rewritten assertion keeps the exact
  predicate it had, so the existing suites remain the guard: e.g.
  `internal/fileutil/atomic_classify_test.go:47` and `:69` still pin that `AtomicWrite` preserves the
  concrete `*os.PathError`/`*os.LinkError` through its wrap, and
  `internal/tmux/tmux_test.go:464`, `:468` and `:487` still pin `HasSessionProbe`'s exit-error
  discrimination. A new test here would be testing the compiler.
- Notes: I did not execute either lane (no test execution in this role). The basis for the green
  claim is (a) every edit is a type-level rewrite with an identical truth value, (b) `golangci-lint`
  type-checks every package under the `integration` tag and reports clean, which proves the whole
  change-set compiles in both lane configurations, and (c) four further plan phases (7–10) landed on
  top of this commit and ran both lanes.

CODE QUALITY:
- Project conventions: Followed. The sweep is precisely what CLAUDE.md asks of the `modernize` linter
  ("keep modernization from drifting"), and it left the legitimate `errors.As` sites alone rather
  than forcing a rewrite that would have required restructuring the surrounding assertions.
- SOLID principles: N/A — no structure changed.
- Complexity: Low. `errors.AsType` removes a declaration per site (−74 net lines across the commit).
- Modern idioms: Yes — that is the whole change.
- Readability: Good. The `if v, ok := …; ok` form scopes the extracted value to the branch that uses
  it, which is stricter than the `var` form it replaced.
- Issues: One consistency slip in the failure-message text, below. Nothing structural.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/log/exec_context_test.go:32 — twelve assertions converted to
  `errors.AsType` in this commit kept a failure message naming `errors.As`, while two others in the
  same commit had theirs updated (`internal/fileutil/atomic_classify_test.go:47` and `:69` now read
  `errors.AsType[*os.PathError](err) = false` / `errors.AsType[*os.LinkError](err) = false`, and
  `internal/tmux/tmux_test.go:465`, `:469`, `:488` likewise). Rewrite the twelve messages to name
  `errors.AsType`, matching the ones the commit already fixed. The full set, each verified against the
  current file (the `errors.AsType` assertion is on the immediately preceding line in every case):
  `internal/log/exec_context_test.go:32`, `:69`, `:82`;
  `internal/resolver/realcommandrunner_test.go:44`, `:57`;
  `internal/state/daemon_identity_ps_test.go:34`; `internal/state/pgrep_test.go:49`;
  `cmd/hooks_rm_exit_test.go:175`; `cmd/hooks_test.go:448`;
  `internal/tmux/list_all_pane_hookkeys_realtmux_test.go:51`;
  `internal/tmux/resolve_hookkey_realtmux_test.go:40`; `internal/tmux/resolve_hookkey_test.go:87`.
  — FAILS: when one of these tests fails, the diagnostic tells the reader "errors.As did not recover
  *exec.ExitError" / "(errors.As failed)" for an assertion that calls no `errors.As`; grepping the
  named function in the file returns nothing on the failing line, so the message points away from the
  code that produced it. Remedy is message text only, so this is not a blocking issue.
