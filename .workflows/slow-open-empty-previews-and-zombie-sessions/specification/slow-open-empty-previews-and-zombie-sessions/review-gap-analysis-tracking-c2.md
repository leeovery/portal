---
status: complete
created: 2026-05-22
cycle: 2
phase: Gap Analysis
topic: slow-open-empty-previews-and-zombie-sessions
---

Resolution summary: all 10 findings approved and applied in auto mode.

1. Replaced substring matching with typed `tmux.ErrNoSuchSession` sentinel + `errors.Is`.
2. Pinned daemon.pid write to `defaultDaemonRun` (cmd/state_daemon.go:70, post-line-290 acquire); added AST-walk assertion test.
3. Component F now explicitly documents env inheritance: respawn preserves session env, no new -e overrides introduced, baseline parity with pre-F behaviour.
4. Component A: residual recycle window acknowledged + tightened (identity-check immediately before kill); no extra mitigation required.
5. Component G placement pinned to new `internal/portaltest/` leaf package.
6. Component F respawn-pane precedent: addressed by Finding 3 edit (RespawnPane method signature referenced).
7. Added "Composite End-to-End Verification" section requiring one integration test that reconstructs the three-daemon scenario.
8. Component D: stale daemon.pid after self-eject documented as intentional; MUST NOT add cleanup.
9. pgrep -fx form pinned as canonical (sole supported form); ps/awk alternative downgraded to illustrative.
10. Component G audit completion criterion pinned (grep returns zero un-tagged call sites; deliverable in PR description or .workflows/ file).

# Review Tracking: slow-open-empty-previews-and-zombie-sessions - Gap Analysis

## Findings

### 1. Component E natural-churn predicate fragile to tmux error-string drift

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Component E (`CaptureStructure` Per-Session Log-and-Continue), "Total-failure guard" + per-session error classification

**Details**:
The classifier says "the error message indicates the session no longer exists — e.g., `\"no such session\"` substring match, or `errors.Is` against a known sentinel if tmux exposes one". This is hedged across two unrelated mechanisms (substring vs sentinel) without specifying which the implementer should use. Worse, errors from `c.ShowEnvironment(name)` are produced by the `tmux.Commander` wrapping layer (`internal/tmux/`), which today produces messages like `exit status 1: no such session: A` (wrapped with the exit-status prefix). A naive substring match on `"no such session"` works against the current wrapping, but:

- The spec does not pin where the substring is searched (raw stderr, wrapped error.Error(), unwrapped via `errors.Unwrap`).
- Tmux's error string is not part of any stable contract; a future tmux upgrade or a Commander rewrite could change the prefix/suffix and silently flip natural-churn classification into anomalous (or vice versa). The classifier mis-firing in either direction has user-visible consequences: extending zombie-state by a tick (false-anomalous on real kill) or skipping a defensive guard (false-churn on real failure).
- No sentinel error is currently exposed by `internal/tmux/`; the "if tmux exposes one" hedge defers a real design decision (introduce `tmux.ErrNoSuchSession` sentinel, or stay with substring) to the implementer.

The cleanest fix is to introduce a typed sentinel at the `internal/tmux/` boundary (e.g., `ErrNoSuchSession` returned by `ShowEnvironment` when stderr contains "no such session"), and have Component E classify via `errors.Is`. Substring matching at the daemon layer is brittle and creates a cross-package coupling on string content.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 2. Component C "runDaemonE" function name not verified against current code

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Component C (Stabilise the `daemon.lock` Singleton), step 4 "Post-acquire daemon.pid write" + Layered Enforcement Note

**Details**:
Step 4 anchors the daemon.pid write location to "`cmd/state_daemon.go`'s `runDaemonE` (or whichever existing function hosts the acquire call)" and says the write must be "the next statement after the successful `AcquireDaemonLock` return … and BEFORE entering the tick loop". The "(or whichever existing function hosts the acquire call)" hedge means the implementer is forced to locate the function rather than the spec naming it. If multiple call sites of `AcquireDaemonLock` exist (production daemon, test helpers, future entry points), the spec's contract that the write happens "as the next statement" is ambiguous — does it apply to all call sites, or only the production one?

This matters because the spec's "layered enforcement" guarantee depends on a narrow startup window between acquire-return and pid-write. If a different daemon entry point (or a future refactor that wraps the acquire in a helper) inserts work between those two statements, the window widens silently and the layered enforcement contract degrades — without any test failing, because no acceptance criterion measures the size of that window.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 3. Component F respawn-pane process environment inheritance unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Component F (Saver Creation Sets destroy-unattached=off BEFORE Daemon Starts), step 3 (respawn-pane)

**Details**:
The new ordering creates `_portal-saver` with a placeholder via `tmux new-session`, then uses `tmux respawn-pane -k -t _portal-saver 'portal state daemon'` to replace the placeholder with the daemon. The spec does not address what environment the respawned daemon process inherits.

Under the current (pre-F) code path, the daemon is the *initial* pane command, so it inherits the environment set at `new-session` time — including any `XDG_CONFIG_HOME`, `PATH`, and other vars that `BootstrapPortalSaver` may have set or relied on. Under `respawn-pane`, the new process inherits from tmux's per-session environment (set via `set-environment` or inherited from the tmux server's launch environment), which may differ in subtle ways from `new-session`'s `-e` overrides.

Specifically:
- If the existing `createPortalSaverWithRetry` passes `-e KEY=VAL` flags to `new-session`, those are baked into the session env. After `respawn-pane`, does the daemon see those? On most tmux versions, yes — session env is preserved across respawn-pane — but the spec doesn't confirm.
- If the developer's shell has `XDG_CONFIG_HOME` set, and the tmux server was launched without that var being propagated, the respawned daemon could resolve a different state dir than the placeholder would have. This is the exact failure mode Component G is meant to prevent in tests; in production it could produce a degraded but functional install with state files in an unexpected location.

The spec should either (a) confirm that current `createPortalSaverWithRetry` does not set any env via `-e` (so respawn inheritance is moot), or (b) specify that `respawn-pane` preserves the session env established by `new-session`, or (c) require an explicit `-e` re-application on respawn.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 4. Components A and B SIGKILL: no test for "PID recycled to unrelated process between identity-check and kill" race

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Component A (Kill-Barrier Escalation) acceptance criteria, Component B (Bootstrap-Time Orphan Sweep) acceptance criteria

**Details**:
Both A and B rely on `state.IdentifyDaemon(pid)` as the gate before sending SIGKILL. The identity-check at time T cannot rule out PID recycling between T and the kill syscall at T+ε. On a busy system, ε can be milliseconds; the OS could reap the daemon (because something else killed it), recycle the PID to a new shell or build tool, and the SIGKILL lands on the unrelated process.

The spec's rationale section in Component A acknowledges PID-recycle risk but only justifies the identity-check, not what happens after the check passes. No acceptance criterion exercises the recycle-between-check-and-kill race because it's genuinely hard to construct deterministically. The practical mitigation strategies (re-identity-check after kill if process still alive; use kill-with-credential checks if OS supports; accept the risk because the window is tiny) are not enumerated.

The risk profile is asymmetric: SIGKILL on an unrelated short-lived process (build tool, sleep, shell) is usually recoverable; SIGKILL on a critical user process (their editor, a long-running compile) is destructive. Without a specified mitigation, the spec accepts that risk silently.

Minimal acceptable resolution: a sentence in Component A/B saying "the residual recycle-between-check-and-kill window is accepted as unmitigated; the window is bounded by syscall latency (~µs) and the OS's PID-recycle pressure", OR a stronger mitigation like a second identity-check immediately before `kill(2)` (still racy but tighter).

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 5. Component G `portaltest` package placement deferred without naming criteria

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Component G (Test Isolation Contract), Fix item 1 "Placement"

**Details**:
The helper is to live in "a new leaf package `internal/portaltest/` (or attach to the existing `portalbintest` package — planning decides)". The decision is deferred to planning without naming a tie-breaker. Both options are plausible:

- New `internal/portaltest/`: cleaner separation (env isolation is orthogonal to binary building); easier to reason about cross-package imports.
- Attach to `portalbintest`: fewer new packages; the existing package already houses subprocess-spawning helpers that need to consume the isolation env.

Either is defensible, but the spec doesn't pin one. The implementer's choice could ripple into test-import-graph decisions (every test that uses both isolation and binary-build helpers now imports one or two packages). Pinning is a low-cost win.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 6. Component F respawn-pane existing-codebase precedent claim is partially overstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Component F, "Why this ordering is safe" — `respawn-pane -k` precedent claim

**Details**:
The spec says: "`respawn-pane -k` is already used elsewhere in the codebase (the hydrate-helper path during Restore — see CLAUDE.md restore section), so the primitive and its `Commander` plumbing exist." Cross-checking CLAUDE.md's `restore` row: "Phase A reconstructs skeleton … via new-session/new-window/split-window with `respawn-pane -k` swapping the default shell for the hydrate helper". So the *flag* is used. But the existing call site swaps shell→hydrate-helper, not placeholder→daemon. The spec's claim that the "primitive and its `Commander` plumbing exist" is technically true (the `RespawnPane` Commander method exists per the `tmux` package row in CLAUDE.md), but the call shape (passing the full command string verbatim and relying on tmux to spawn it as the pane's initial process) needs to be validated by the implementer.

Minor risk: if the existing `RespawnPane` method has a different signature (e.g., takes a structured args struct, or assumes shell-form vs exec-form quoting), Component F's call form needs to adapt. The spec doesn't name the exact method signature it expects.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 7. No consolidated end-to-end test plan tying components to verification scenarios

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: End-State Verification section, all component acceptance criteria

**Details**:
Each component has its own acceptance criteria. The End-State Verification section enumerates user-visible end-states (`portal open` sub-second, previews render, K kills permanently, log is quiet, `pgrep -fxc` returns 1, `daemon.version` tracks). But no single test scenario ties multiple components together — for example, the "three concurrent daemons" trigger described in Root Cause is mentioned in B's acceptance ("Given N concurrent `portal state daemon` processes where N-1 are orphans"), but the same scenario also exercises A's escalation (one is reachable via priorPID), C's pre-check (a fourth spawn must be refused), and D's self-eject (orphans not killed by B should self-eject within 3 ticks).

A single end-to-end integration test that constructs the reporter's three-daemon scenario and asserts post-bootstrap singleton + sub-second open + preview rendering would catch component-composition regressions that per-component tests miss. The spec doesn't require this; planning may decide. But without it, a regression that breaks composition (e.g., B's sweep ordering breaks D's first-tick assumption) could slip through.

This is planning-readiness gap: an implementer would not know whether a composite integration test is required for the work unit to be "done", or whether per-component tests suffice.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 8. Component D self-eject leaves stale `daemon.pid` on disk

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Component D (Daemon Self-Supervision), interaction with Component C pre-check

**Details**:
Component D step 4.ii says: "Skip the final flush. Exit immediately via `os.Exit(0)` (bypassing any deferred shutdown handler)". `os.Exit(0)` skips deferred cleanup, including any defer that would remove or update `daemon.pid`. The orphan daemon's `daemon.pid` (which it wrote post-acquire per Component C step 4) remains on disk after self-eject, pointing at the now-dead PID.

The next `AcquireDaemonLock` call (by the legitimate daemon during a subsequent bootstrap) runs C's pre-check, reads the stale `daemon.pid`, finds the recorded PID is dead, and proceeds — correct. So functionally this works.

But two minor concerns:
1. The spec doesn't explicitly state that leaving a stale `daemon.pid` is safe-by-design and handled by C's pre-check. An implementer reading D in isolation might add cleanup logic ("delete daemon.pid before exit") that itself is racy (another daemon could be mid-pre-check) and inverts the layered-enforcement contract.
2. If a third party (e.g., the user via a debug command) is reading `daemon.pid` to identify the live daemon, the stale value after a self-eject is misleading. Not a correctness issue, but a diagnostic-quality issue worth noting.

A one-line clarification in D ("Stale `daemon.pid` after self-eject is intentional; Component C's pre-check handles the dead-PID case on the next acquire") would close this.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 9. Component B's pgrep alternate enumeration form not equivalent

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Component B (Bootstrap-Time Orphan Sweep), step 1 enumeration alternatives

**Details**:
Step 1 offers two enumeration forms:
- `pgrep -fx '^portal state daemon( |$)'`
- `ps -axo pid=,args= | awk '$2=="portal" && $3=="state" && $4=="daemon" {print $1}'`

These are not behaviourally equivalent. The pgrep form's regex `^portal state daemon( |$)` matches argv starting with `portal state daemon` followed by either a space or end-of-string. The awk form parses `args` field-by-field after whitespace splitting and asserts the first three positional args are exactly `portal`, `state`, `daemon`. The awk form does not anchor against argv-end — `portal state daemon-foo` would NOT match in awk (because field 4 would be `daemon-foo` not `daemon`), so behaviourally similar to the pgrep `( |$)` anchor — but `portal state daemon --flag` matches both, and `portal state daemonize` matches neither.

Edge case where they diverge: if `args` contains embedded whitespace that splits a logical argument across multiple fields (extremely unlikely for `portal state daemon` but possible for a hypothetical `portal "state daemon"`), the two forms diverge. Practically irrelevant for this codebase, but the "acceptable if the implementer prefers a non-pgrep dependency" framing implies behavioural equivalence which is slightly overclaimed.

Minor: pinning one form (preferably the pgrep one, since it's the simpler invocation and CLAUDE.md / the acceptance criteria reference `pgrep -fxc`) would avoid this entirely.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 10. Component G's audit step has no completion criterion

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Component G (Test Isolation Contract), Fix item 2 "Audit existing test helpers"

**Details**:
Item 2 says "Any helper in `internal/portalbintest`, `internal/tmuxtest`, or `internal/restoretest` that spawns `portal` or `portal state daemon` as a subprocess MUST pass the env from `portaltest.NewIsolatedStateEnv` (or equivalent isolation). Helpers currently inheriting `os.Environ()` directly are updated to require the isolated env at their call signature".

The audit's *completion* condition is not specified:
- Does the implementer enumerate every helper and produce a checklist?
- Is the "audit list produced during implementation" (referenced in the acceptance criterion "Verified by code-review of the audit list") a deliverable? Where does it live? PR description?
- What's the criterion for "all helpers updated" — `grep` for `exec.Command(...portal...)` returning zero unprotected sites?

For a "code review verifies the audit" acceptance criterion to be enforceable, the audit's shape needs to be specified (a list of helper functions in a PR description, or a comment block, or a file). Otherwise a reviewer cannot tell whether the audit was performed thoroughly or skimmed.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---
