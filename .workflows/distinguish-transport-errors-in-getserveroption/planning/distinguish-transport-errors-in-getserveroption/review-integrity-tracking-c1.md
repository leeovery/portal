---
status: in-progress
created: 2026-05-13
cycle: 1
phase: Plan Integrity Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: Distinguish Transport Errors in GetServerOption - Integrity

## Findings

### 1. Tasks 1-2 and 1-3 are implementation-only — tests deferred to later tasks

**Severity**: Important
**Plan Reference**: Phase 1, Tasks 1-2 and 1-3 (Tests sections)
**Category**: Task Template Compliance / Vertical Slicing
**Change Type**: update-task

**Details**:
Task 1-2 (`Wire RealCommander.Run and RunRaw to wrap errors as *CommandError`) declares `**Tests**: covered behaviourally by Task 1-5 (...). This task's implementation is verified by Task 1-5's tests; no additional tests authored here.`

Task 1-3 (`Add optionAbsentStderrPatterns slice and rewrite GetServerOption to discriminate via errors.As`) declares `**Tests** (covered by Task 1-4 — reshaped TestGetServerOption plus new transport / non-exit / try-wrapper / discriminator-set tests). This task's implementation surface is verified by Task 1-4's tests; no additional tests authored here.`

This violates two integrity criteria:

- **Task Template Compliance**: `task-design.md` requires "At least one test name; include edge cases, not just happy path". Pointing to another task's tests does not satisfy this field.
- **Vertical Slicing**: `task-design.md`'s "One Task = One TDD Cycle. Write test → implement → pass → commit. Each task produces a single, verifiable increment." Tasks 1-2 and 1-3 cannot be independently verified — they are implementation increments whose verification lives in 1-4/1-5. The independence test fails: "Can I write a test for this task that passes without any other task being complete?"

The current split also inverts TDD ordering — under natural execution order (1-1 → 1-2 → 1-3 → 1-4 → 1-5), the implementer writes production wiring (1-2, 1-3) before any tests exist, then writes tests in 1-4 and 1-5 to lock the behaviour after the fact.

The spec's "single PR, single commit" guidance and the (1)+(2)+(3) co-landing constraint do not force this horizontal split — they allow it, but the plan can still authorise per-task tests at the seam each task introduces. Pulling test deliverables back into 1-2 and 1-3 (and shrinking 1-4/1-5 to the additions that genuinely belong there) restores per-task TDD cycles without changing what ultimately ships in the single commit.

**Recommendation**: fold each test-deliverable task back into its implementation task:

- Move `TestRealCommander_RunWrapsExitError` and `TestRealCommander_RunWrapsNonExitError` (currently Task 1-5) into Task 1-2 as its own Tests section.
- Move the reshape of `TestGetServerOption "option does not exist"`, plus `TestGetServerOption_TransportError`, `TestGetServerOption_NonExitErrorPropagates`, `TestTryGetServerOption_PropagatesTransportError`, and `TestGetServerOption_DiscriminatorSet` (currently Task 1-4) into Task 1-3.
- Remove Tasks 1-4 and 1-5 as standalone tasks (their content folds upward).

This would reduce the plan from 7 to 5 tasks, each a complete TDD cycle. Task 1-6 (daemon tests) remains a standalone task because it exercises a different package (`cmd/state_daemon_run_test.go`) and depends on the full chain being live, not just one seam.

**Current** (Task 1-2 Tests section):
```
**Tests**: covered behaviourally by Task 1-5 (`TestRealCommander_RunWrapsExitError`, `TestRealCommander_RunWrapsNonExitError`). This task's implementation is verified by Task 1-5's tests; no additional tests authored here.
```

**Proposed** (Task 1-2 Tests section — absorbing Task 1-5's deliverables):
```
**Tests** (in `internal/tmux/tmux_test.go` or a sibling `internal/tmux/realcommander_test.go`, same package; no `t.Parallel()` per CLAUDE.md):
- `"TestRealCommander_RunWrapsExitError"` — drives `sh -c 'echo "synthetic stderr marker" 1>&2; exit 1'` through the production exec path; asserts the returned error is non-nil, `errors.As(err, &cmdErr)` succeeds, and `strings.Contains(cmdErr.Stderr, "synthetic stderr marker")` is true. Because `RealCommander.Run` is hard-coded to invoke `tmux`, factor out a small unexported `runner` helper that accepts the binary name and have `Run`/`RunRaw` and the test both call it — or expose a test-only constructor targeting a configurable binary. Pick whichever shape is lower-cost. Skip via `t.Skip(...)` when `exec.LookPath("sh")` fails (defensive — Darwin + Linux always have `sh`).
- `"TestRealCommander_RunWrapsExitError/runs_raw_variant"` — same assertion against `RunRaw`, confirming the two methods behave identically on the error path.
- `"TestRealCommander_RunWrapsNonExitError"` — invokes the deterministic non-existent binary `__portal_test_nonexistent_binary__`; asserts `errors.As(err, &cmdErr)` succeeds, `cmdErr.Stderr == ""`, and `var exitErr *exec.ExitError; !errors.As(cmdErr.Err, &exitErr)` — i.e. the underlying error is not an `*exec.ExitError` (it is `*exec.Error` from `exec.LookPath`/`cmd.Start`, but the assertion stays behavioural).
- `"TestRealCommander_RunWrapsNonExitError/runs_raw_variant"` — same against `RunRaw`.

All assertions are behavioural (`errors.As`, `strings.Contains`, type assertion) — never against the exact `.Error()` string. Tests run independently of any tmux server.
```

**Current** (Task 1-3 Tests section):
```
**Tests** (covered by Task 1-4 — reshaped TestGetServerOption plus new transport / non-exit / try-wrapper / discriminator-set tests). This task's implementation surface is verified by Task 1-4's tests; no additional tests authored here.
```

**Proposed** (Task 1-3 Tests section — absorbing Task 1-4's deliverables):
```
**Tests** (in `internal/tmux/tmux_test.go`, same-package so the unexported `optionAbsentStderrPatterns` slice is directly addressable; no `t.Parallel()` per CLAUDE.md):
- `"TestGetServerOption/option_does_not_exist"` (reshaped) — the existing subtest's mock previously returned `errors.New("unknown option: @portal-active-%3")`; replace the bare `errors.New(...)` with `&CommandError{Stderr: "unknown option: @portal-active-%3", Err: errors.New("exit status 1")}`. Assertion remains `errors.Is(err, ErrOptionNotFound)`. Under the old code the test passed by accident (every error became `ErrOptionNotFound`); under the new code it passes because stderr genuinely matches the absent-pattern family.
- `"TestGetServerOption_TransportError/socket_connect_failure"` — mock returns `"", &CommandError{Stderr: "error connecting to /tmp/tmux-501//default (No such file or directory)", Err: errors.New("exit status 1")}`. Assert `!errors.Is(err, ErrOptionNotFound)`, `errors.As(err, &cmdErr)` succeeds, `cmdErr.Stderr` matches verbatim.
- `"TestGetServerOption_TransportError/lost_server"` — same shape with `Stderr: "lost server"`. Same assertions.
- `"TestGetServerOption_NonExitErrorPropagates"` — mock returns `"", &CommandError{Stderr: "", Err: errors.New("exec: \"tmux\": not found")}`. Assert `!errors.Is(err, ErrOptionNotFound)` and `errors.As` recovers a `*CommandError` with empty `Stderr`.
- `"TestTryGetServerOption_PropagatesTransportError"` — mock returns the socket-connect `*CommandError`. Call `c.TryGetServerOption("@some-marker")`; assert `val == ""`, `found == false`, `err != nil`, `errors.As(err, &cmdErr)` succeeds with the expected `Stderr`. Exercises the previously-unreachable `if err != nil { return "", false, err }` branch via the public surface.
- `"TestGetServerOption_DiscriminatorSet/<pat>"` — table-driven, iterating `optionAbsentStderrPatterns` directly (not hardcoded) so a future slice extension is automatically covered. For each `pat`: build `stderr := pat + " @foo"`, mock returns `&CommandError{Stderr: stderr, Err: errors.New("exit status 1")}`, assert `errors.Is(err, ErrOptionNotFound)`.
- `"TestGetServerOption_DiscriminatorSet/unrelated_stderr_does_not_match"` — `stderr = "some unrelated error: connection refused"`; assert `!errors.Is(err, ErrOptionNotFound)` and that the original error propagates.

All tests use the existing `Commander` mock surface — no new mock framework, no real `os/exec`. Mock returns the canonical `*CommandError` literal shape.
```

**Resolution**: Pending
**Notes**: If approved, the orchestrator should also remove Tasks 1-4 and 1-5 entirely (separate `remove-task` operations) and renumber 1-6 → 1-4 and 1-7 → 1-5 in the planning.md table, the phase-1-tasks.md headings, and the Acceptance bullets that name specific test functions. The Acceptance bullet at lines 20-21 of `planning.md` already names every test function — that list survives the merge unchanged. If the user prefers to keep the structure as-is for the explicit "single PR" landing model, this finding can be marked rejected.

---

### 2. Task 1-7 acceptance criterion has an unbalanced quote making the example unreadable

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-7 — final acceptance bullet at `phase-1-tasks.md:422`
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The last acceptance criterion of Task 1-7 reads:

> A `grep` for the previous mis-leading phrasing produces no false positives (e.g., no remaining docstring claims `GetServerOption` "always" returns `ErrOptionNotFound" on error, or similar).

The `` `ErrOptionNotFound" `` substring has a stray closing double-quote with no matching opener: backtick opens code-span, then `"on error` introduces an unmatched quote. The intent is clear but the rendered line is ungrammatical and the implementer must guess the example. Markdown also renders this awkwardly because the code-span runs `ErrOptionNotFound" on error, or similar)` until the next backtick.

**Current**:
```
- [ ] A `grep` for the previous mis-leading phrasing produces no false positives (e.g., no remaining docstring claims `GetServerOption` "always" returns `ErrOptionNotFound" on error, or similar).
```

**Proposed**:
```
- [ ] A `grep` for the previous mis-leading phrasing produces no false positives (e.g., no remaining docstring claims `GetServerOption` "always" returns `ErrOptionNotFound` on any error, or similar).
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 1-6 "Do" section embeds an audit decision the implementer must resolve at runtime

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-6 — `phase-1-tasks.md:346-349`
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-6's "Audit and update `tick()` err-branch coverage" sub-bullet asks the implementer to first determine whether `tick()`'s err-branch is already covered by an existing test, then either update that test's mock or add a new test. The spec's wording is preserved verbatim, but the audit decision lives in the implementer's lap — and the audit's outcome is verifiable today (the plan is being authored now, not at implementation time).

A 30-second grep against `cmd/state_daemon_run_test.go` resolves the ambiguity once and folds the result into the plan, so the implementer is not asked to make a decision that the planner could have already made. This crosses the integrity criterion line on "An implementer could pick up any single task and execute it" without making design / audit decisions.

The criterion explicitly allows the implementer to perform a sweep before changes — but the sweep result here is purely structural (does this test exist?) and discoverable now.

**Current** (Task 1-6 "Do" section, the `tick()` audit bullets at lines 346-349):
```
- **Audit and update `tick()` err-branch coverage** in `cmd/state_daemon_run_test.go`:
  - Pre-implementation sweep step: locate any existing test that injects a `TryGetServerOption` error into `tick()`. The test-code sweep section of the spec notes "the documented-gap comment at lines 557–565 indicates no existing test reaches the err-branch through the public Client surface" — but this refers specifically to `defaultShutdownFlush`. The implementer must confirm whether `tick()`'s err-branch already has its own coverage through the daemon-side seam (`tickDeps` or equivalent).
  - If `tick()`'s err-branch is **already covered** with a bare `errors.New(...)` mock return — update that mock to return `&tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")}`. Otherwise the test exercises a code path that under the new contract would propagate as non-absence (non-`ErrOptionNotFound`), which is still the correct branch but no longer the realistic shape.
  - If `tick()`'s err-branch is **not covered** — add a new test `TestTick_SkipsOnTransportError` with the same fault-injection shape as the flush test: inject the same `*CommandError`, drive `tick`, assert no capture/commit calls are performed.
```

**Proposed** (after pre-resolving the audit by inspecting `cmd/state_daemon_run_test.go` for any existing `tick()` err-branch coverage — the planner should grep for `TryGetServerOption.*err\|read @portal-restoring` first; if no such coverage exists, the bullet collapses to the add-new-test branch only):
```
- **Add `tick()` err-branch coverage** in `cmd/state_daemon_run_test.go`. Per the pre-authoring grep (see Pre-implementation sweep below), `tick()`'s err-branch (`cmd/state_daemon.go:95-99`) has no existing coverage through the daemon test seam — the documented-gap comment at lines 557-565 calls this out for `defaultShutdownFlush` but the daemon's `tick()` test surface contains no equivalent fault-injection of `TryGetServerOption` errors.
  - Add `TestTick_SkipsOnTransportError`: inject a tmux-client mock via the daemon's `tickDeps`-equivalent seam whose `TryGetServerOption("@portal-restoring")` returns `("", false, &tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")})`. Drive `tick`. Assert via the existing capture/commit mock-tracking pattern that no capture / no commit calls are performed and the warn-log fires (log-capture optional, per the flush test).
  - If the pre-implementation grep surfaces existing `tick()` err-branch coverage that the audit missed (test returning a bare `errors.New(...)`), update that test's mock to return the same `*CommandError` shape instead of adding a duplicate.
```

**Resolution**: Pending
**Notes**: The planner should perform the audit grep before finalising this proposal — if `tick()` err-branch is already covered, the proposed bullet inverts (default to "update existing, add only if missing"). The current ambiguity is acceptable as Minor because both branches produce correct end-state coverage; it is flagged as a polish opportunity rather than a blocker.

---

### 4. Task 1-1 acceptance criterion duplicates a tautological check

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-1 — `phase-1-tasks.md:35`
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 1-1's acceptance criterion `"&tmux.CommandError{Stderr: "x", Err: errors.New("y")} compiles from outside the package."` is tautological given the preceding criterion already requires the type to be exported with public `Stderr string` / `Err error` fields. Any exported struct with exported fields compiles via struct literal from outside the package by definition of Go's visibility rules.

A more useful AC would be that the test file in this task (which lives **in-package**, as the Tests section makes clear) confirms construction works, and the broader "external-package usability" is incidentally exercised by Task 1-6's daemon test which lives in `cmd/` and constructs `&tmux.CommandError{...}` against the wrapper directly.

The current criterion is harmless but consumes a checklist line that could either be removed or repurposed.

**Current**:
```
- [ ] `&tmux.CommandError{Stderr: "x", Err: errors.New("y")}` compiles from outside the package.
```

**Proposed** (replace with an external-construction smoke check that genuinely adds verifying value — the planner may also choose to simply delete the bullet):
```
- [ ] An external-package consumer (e.g., the daemon test added in Task 1-6) can construct `&tmux.CommandError{Stderr: "...", Err: errors.New("...")}` as a literal — confirming the fields and type remain exported and constructor-free per the spec's "plain struct literal, no NewCommandError factory" rule.
```

**Resolution**: Pending
**Notes**: This is the lowest-priority finding — feel free to reject and keep the existing wording.

---
