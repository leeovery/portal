---
status: in-progress
created: 2026-05-13
cycle: 1
phase: Input Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Input Review

## Findings

### 1. Rejected design alternatives not recorded

**Source**: investigation §Fix Direction → "Options Explored" (B, C, D, E)
**Category**: New topic
**Affects**: Design section (could be a new "Alternatives Considered" subsection)

**Details**:
The investigation enumerates four rejected approaches with concrete rejection reasoning:
- **B.** `fmt.Errorf("%w: %s", err, stderr)` wrap — rejected: discriminator becomes substring-match against formatted error string, brittle, harder to test the boundary cleanly.
- **C.** New `Commander.RunWithStderr(args...) (out, stderr string, err error)` method — rejected: parallel API surface forces every mock to stub a second method; doesn't fix similar latent conflations.
- **D.** Inline type-assert against `*exec.ExitError` inside `GetServerOption` — rejected: couples `internal/tmux` to `os/exec` semantics through the public discriminator site; mocks would need synthetic `*exec.ExitError` instances.
- **E.** String-match against `err.Error()` (no wrap) — rejected on examination: `(*exec.ExitError).Error()` returns `"exit status 1"`; stderr is on `.Stderr`, not in the error string. Mechanically broken without first wrapping.

The spec records only the chosen approach. Without these alternatives, future readers can't tell why this shape was chosen over the obvious cheaper options (especially D, which is the smallest diff).

**Proposed Addition**:
(leave blank — discuss whether to include rejected alternatives in spec or treat investigation as the historical record)

**Resolution**: Pending
**Notes**:

---

### 2. Empty option name → "ambiguous option:" probe result

**Source**: investigation §Code Trace → tmux error surface table (row 3)
**Category**: Enhancement to existing topic
**Affects**: Design: Discrimination in `GetServerOption` → "Option-absent pattern family"

**Details**:
The investigation's probing table documents that `tmux show-option -sv ""` (empty option name) returns `ambiguous option: ` (with trailing space, treated as ambiguous match). The spec lists `ambiguous option:` as a pattern but parenthetically says "(including the empty string case observed during investigation)" without specifying it as the exact probe result that produced the entry. This is the empirical origin of the third pattern; without it, a reader might assume `ambiguous option:` is speculative rather than observed.

**Current**:
> - `ambiguous option:` — option name is an ambiguous prefix match (including the empty string case observed during investigation).

**Proposed Addition**:
Tighten to note the empirical probe: empty option name was the trigger that surfaced `ambiguous option:` during investigation on Darwin 25.3.0.

**Resolution**: Pending
**Notes**:

---

### 3. RealCommander uses cmd.Output() to auto-populate exec.ExitError.Stderr

**Source**: investigation §Code Trace → paragraph following the stderr table
**Category**: Enhancement to existing topic
**Affects**: Design: `CommandError` at the Commander Layer → "Wiring at `RealCommander`"

**Details**:
The investigation explicitly notes: *"`RealCommander.Run` (`internal/tmux/tmux.go:39-46`) uses `cmd.Output()`, which populates `(*exec.ExitError).Stderr` automatically when `cmd.Stderr` is nil — so the stderr text is available on the returned error, just not surfaced through the `Commander` interface today."*

The spec says `(*exec.ExitError).Stderr` is "already captured automatically by `cmd.Output()`" — accurate, but does not state the precondition (`cmd.Stderr` must be nil) which is load-bearing. If a future change ever assigns `cmd.Stderr`, the wrapping silently breaks because `(*exec.ExitError).Stderr` will be empty. This precondition belongs in the spec as a constraint on the wrapping implementation.

**Current**:
> - If the error is `*exec.ExitError`, populate `Stderr` from `(*exec.ExitError).Stderr` (already captured automatically by `cmd.Output()`).

**Proposed Addition**:
Add the `cmd.Stderr == nil` precondition that makes `cmd.Output()` populate `(*exec.ExitError).Stderr`, framing it as a constraint on the wiring (assigning `cmd.Stderr` later would silently break wrapping).

**Resolution**: Pending
**Notes**:

---

### 4. Contributing factor: Commander interface signature erases stderr distinction

**Source**: investigation §Analysis → "Contributing Factors" (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal (Problem section) — or could fold into Design rationale

**Details**:
The investigation explicitly states the root architectural contributor: *"The `Commander` interface signature `(string, error)` discards the stderr distinction. Callers cannot route on stderr content without type-asserting on `*exec.ExitError`, which couples them to `os/exec` and breaks the mock surface."*

This is the structural justification for why the fix lives at the `Commander` layer rather than inside `GetServerOption` (it's also the implicit reason option D was rejected). The spec leans on this implicitly but never states it. A reader who hasn't read the investigation might wonder why the wrapping doesn't just happen at the `GetServerOption` boundary.

**Proposed Addition**:
(leave blank — discuss whether to anchor the design choice in Problem section, or add a short "Design rationale" preamble to the CommandError design section)

**Resolution**: Pending
**Notes**:

---

### 5. Why-not-caught: existing test uses synthetic string never inspected

**Source**: investigation §Analysis → "Why It Wasn't Caught" (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Testing → "Reshape existing TestGetServerOption" entry

**Details**:
The investigation explains *why* the existing test passes despite the bug: *"Existing tests at `internal/tmux/tmux_test.go:924-934` use `errors.New("unknown option: @portal-active-%3")` — a synthetic error string that never gets inspected. Under the current behaviour, any error becomes `ErrOptionNotFound`, so the test passes without exercising stderr inspection."*

The spec's Testing section references reshaping this test but doesn't capture *why* the existing form is insufficient (the string is decorative, not inspected). Without this context, a planner might preserve the existing shape and add a new test alongside, rather than reshaping the existing test to actually exercise stderr-pattern matching.

**Current**:
> - **Reshape existing `TestGetServerOption` "option does not exist" case** (currently uses `errors.New("unknown option: @portal-active-%3")`): the mock must now return a `*CommandError` whose `Stderr` matches the option-absent pattern family. The test asserts `errors.Is(err, ErrOptionNotFound)` continues to hold.

**Proposed Addition**:
Add a short note that the current synthetic error string is decorative (never inspected because every error becomes `ErrOptionNotFound` under today's code), motivating the reshape rather than an additive new test.

**Resolution**: Pending
**Notes**:

---

### 6. Compatibility-floor pin for pattern family

**Source**: investigation §Fix Direction → "Discussion" (second deferred item)
**Category**: Gap/Ambiguity
**Affects**: Design: Discrimination in `GetServerOption` → "Option-absent pattern family"

**Details**:
The investigation explicitly flags this as an item the spec should pin: *"Pattern-family literal: the working set is `invalid option:`, `unknown option:`, `ambiguous option:`. Spec phase confirms against tmux's source (or a documented compatibility floor) whether to widen or narrow."*

The spec asserts *"Tmux's source uses these literals consistently across versions in the project's compatibility window"* — but the project's "compatibility window" / "compatibility floor" is not defined in the spec or, as far as the investigation surfaces, anywhere else. Without a stated tmux-version floor (e.g., "tmux ≥ 3.0"), the claim is unverifiable and future tmux changes can't be tested against a known baseline.

**Proposed Addition**:
(leave blank — needs discussion to pin a tmux compatibility floor or accept the claim as best-effort with no explicit floor)

**Resolution**: Pending
**Notes**:

---

### 7. Original motivation context (hook-executor two-condition check)

**Source**: investigation §Notes (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal (Problem section) — historical context

**Details**:
The investigation's Notes capture a load-bearing historical observation: *"The original motivation (a hook-executor 'two-condition check') no longer exists — hook firing migrated into the hydrate helper's exec chain. The original symptom site is gone; the architectural concern moved to marker-state reads in the daemon's restoration-window logic."*

This is the reason the bug is latent and why the spec frames it in terms of daemon restoration-window reads rather than the original inbox framing. Without it, a reader looking at archived discussions or the inbox entry (`.workflows/.inbox/.archived/bugs/2026-03-28--distinguish-transport-errors-in-getserveroption.md`) may be confused about why the original symptom site isn't being fixed.

**Proposed Addition**:
(leave blank — discuss whether to capture as a brief historical note in Problem section or treat as investigation-only context)

**Resolution**: Pending
**Notes**:

---

### 8. Reproduction harness shape (fault-injection mock pattern)

**Source**: investigation §Symptoms → "Reproduction Steps"
**Category**: Enhancement to existing topic
**Affects**: Testing section

**Details**:
The investigation provides a concrete reproduction example: *"Construct a `Commander` mock that returns a non-tmux-unknown-option error from `show-option -sv @some-marker` (e.g., simulate `exit 1` with stderr `lost server`)."*

The Testing section uses different example stderr strings (`"error connecting to /tmp/tmux-501//default (No such file or directory)"`, `"exec: \"tmux\": not found"`). The `lost server` case from the reproduction steps is a distinct failure shape (mid-session server crash) not represented in the test catalog. The investigation also names the harness shape (fault-injection via `Commander` mock returning a synthetic exit-1 + stderr), which the spec captures only implicitly.

**Proposed Addition**:
(leave blank — discuss whether to add `lost server` as an additional transport-error test case or treat the existing two examples as sufficient coverage of the "non-absent stderr" branch)

**Resolution**: Pending
**Notes**:

---

### 9. Wide-scope audit results (clean adjacent patterns) not explicit

**Source**: investigation §Analysis → "Adjacent Patterns Audited (Wide Scope)" and §Notes (second bullet)
**Category**: Enhancement to existing topic
**Affects**: Scope → "Out of scope"

**Details**:
The investigation records that the user explicitly confirmed wide investigation scope, and that the audit produced positive results: `ShowAllServerOptions` propagates `fmt.Errorf("failed to show server options: %w", err)` correctly; `ListSkeletonMarkers` returns `(nil, err)` cleanly; `SetServerOption` / `UnsetServerOption` propagate normally. *"The bug is isolated to `GetServerOption` and its single wrapper `TryGetServerOption`."*

The spec's Out-of-scope bullet says "Their error-propagation paths are already correct per the wide-scope audit" — which references the audit but doesn't summarize its findings or list which surfaces were checked. A planner auditing the fix's blast radius cannot tell from the spec which sites were verified clean vs. simply not mentioned.

**Current** (in Scope → Out of scope):
> - Changes to any other `internal/tmux` method (`ShowAllServerOptions`, `SetServerOption`, etc.). Their error-propagation paths are already correct per the wide-scope audit.

**Proposed Addition**:
Enumerate the audited-clean surfaces explicitly: `ShowAllServerOptions` (returns wrapped error), `ListSkeletonMarkers` (returns `(nil, err)`), `SetServerOption` / `UnsetServerOption` (write-side, propagate normally). This documents the audit findings in the spec rather than only in the investigation.

**Resolution**: Pending
**Notes**:

---

### 10. Environment scope: Darwin + Linux, no platform branching

**Source**: investigation §Symptoms → "Environment"
**Category**: Enhancement to existing topic
**Affects**: Scope or Risk & Rollout

**Details**:
The investigation states: *"Affected environments: All (Darwin / Linux). The conflation is in pure Go logic with no platform branching. Trigger conditions: Any `cmd.Run` failure from `show-option -sv <name>`."*

The spec does not explicitly state platform applicability. For a bug whose discriminator is stderr substring-matching (which is platform-sensitive in principle — tmux's emit format could differ across builds), the "no platform branching" assertion is load-bearing for justifying a single pattern set covering Darwin + Linux.

**Proposed Addition**:
(leave blank — discuss whether to add an environment line to Scope, or fold into the pattern-family discussion as "applies to Darwin + Linux, no platform branching needed")

**Resolution**: Pending
**Notes**:

---
