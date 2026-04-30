---
status: complete
created: 2026-04-30
cycle: 2
phase: Gap Analysis
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Gap Analysis

## Findings

### 1. End-to-end test asserts on `Client.ListSessions` — same pipeline being filtered

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements — End-To-End — No `_*` Sessions Visible Post-Bootstrap

**Details**:
The e2e test contract says "the session list visible via `Client.ListSessions` contains no entries whose names begin with `_`." But after Fix A, `Client.ListSessions` filters `_*` at the chokepoint by construction — asserting that its output has no `_*` is asserting the filter ran, which the unit test in `tmux_test.go` already covers.

The end-to-end value is supposed to be catching tmux-level regressions (e.g. Fix B's rename arg silently dropped). To do that the test needs to read tmux's actual session state — either via a raw enumeration helper or by shelling to `tmux list-sessions -F '#{session_name}'` directly — and assert:

1. No `_*` sessions visible to user-facing surfaces (current contract).
2. Plus: tmux's raw session list contains exactly the expected internal names (`_portal-bootstrap`, `_portal-saver`) and nothing else (no leftover `0`).

Without (2), a regression to Root Cause 2 (rename arg dropped, `0` returns) would still pass the test because `0` has no `_*` prefix and `Client.ListSessions` would still report empty. The test as currently worded does not catch the very root cause it's intended to guard.

**Proposed Addition**:
Tighten the e2e test contract to assert on raw tmux state (via a new test-only helper or direct `tmux list-sessions` shell-out), not on `Client.ListSessions` output. Two assertions:
  - Raw tmux session names are a subset of `{_portal-bootstrap, _portal-saver, <expected restored sessions>}`. Specifically: no `0` and no other unexpected names.
  - `Client.ListSessions` returns the expected user-facing slice (i.e. excludes both reserved names).
Together these catch both Root Cause 1 (filter regression) and Root Cause 2 (rename regression).

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 2. `StartServer` behaviour when `_portal-bootstrap` already exists

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Behaviour Contract; Lifecycle After The Rename

**Details**:
`tmux new-session -d -s _portal-bootstrap` fails ("duplicate session") if a session by that name already exists. Spec assumes `StartServer` runs only when the server is not running (which is how production calls it today, gated by `ServerRunning`). But the contract on `StartServer` itself is silent on what happens if called against a server where `_portal-bootstrap` exists — e.g. a future caller, a test harness re-using a server, or an unusual recovery path.

Not strictly needed for the bugfix, but the spec elevates `_portal-bootstrap` to a reserved, exported constant and treats it as a Portal-wide invariant. The behaviour of the call when the precondition is violated is unspecified.

**Proposed Addition**:
Add a one-line note to Fix B's Behaviour Contract: `StartServer` is contracted to be called only when no tmux server is running (the existing precondition via `ServerRunning`). Behaviour when called against a server already hosting a `_portal-bootstrap` session is undefined and not in scope for this bugfix — callers must check `ServerRunning` first, as today.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 3. `StartServer` reference to `PortalBootstrapName` constant — implicit, not explicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Behaviour Contract; Naming Constraint

**Details**:
Spec mandates the constant `PortalBootstrapName = "_portal-bootstrap"` and says tests "reference the constant rather than the literal string." But for production code in `StartServer` itself, the spec only says the implementation invokes `tmux new-session -d -s _portal-bootstrap` — using the literal string. Whether `StartServer`'s implementation should also reference the constant (rather than hardcode the literal) is unspecified.

This is a small point but matters for "the constant is the canonical reference" claim under Naming Constraint — if production hardcodes the string and only tests use the constant, the canonical-reference claim is partly aspirational.

**Proposed Addition**:
Clarify in Fix B's Behaviour Contract that `StartServer`'s implementation MUST reference `PortalBootstrapName` (not the literal string) when constructing the tmux args, so the constant is genuinely the canonical reference for production and test code alike.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 4. `ListSessionNames` "thin wrapper" assumption is load-bearing but not pinned in the contract

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Interaction With The Capture Path

**Details**:
The chosen capture-path strategy (Strategy 2 from cycle 1) relies on `ListSessionNames` being a thin wrapper around `ListSessions` — so the filter applies transitively and double-filtering is a no-op. Spec states this as an observation ("verified to be a thin wrapper") but does not pin it as a contract going forward.

If a future change makes `ListSessionNames` go to tmux directly (bypassing `ListSessions`), the capture path silently loses the underscore filter and Root Cause 1 partially regresses for capture. This is exactly the "single source of truth" argument made for Fix A's chokepoint placement, but only enforced in one direction.

**Proposed Addition**:
Add a one-line invariant to Fix A's Interaction With The Capture Path subsection: `ListSessionNames` MUST remain a delegation to `ListSessions` — any future change that decouples them must re-evaluate the capture-path filter. Optionally seed a unit test that asserts `ListSessionNames` returns a subset of `ListSessions`.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 5. Inter-commit visibility window between Fix A and Fix B

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Rollout

**Details**:
Rollout splits the work into two commits: commit 1 ships Fix A (filter), commit 2 ships Fix B (rename). Between these commits — i.e. on a developer build of just commit 1 — the bootstrap session is still named `0` (Fix B not yet applied). Fix A's filter only excludes `_*`, so `0` remains visible. The end-to-end test, if added in commit 1, would pass against `0`-still-visible because the assertion is on `_*` prefixes only.

The spec says the two commits "MUST land in the same release," which addresses user-visible shipping. But for CI / bisect / git-blame review, the intermediate commit has the bug it claims to fix (for `0`).

This may be acceptable — small commits with a test for each is good hygiene — but the spec should explicitly call out that the e2e test (which guards both root causes) should land in commit 2 (or later) so it is not accidentally green against partial fixes. Currently spec says "MAY ship in either commit," which permits a misleading-green commit 1.

**Proposed Addition**:
Update Rollout to specify that the End-To-End test SHOULD ship in commit 2 (or in a third commit after both fixes land), not commit 1, so the test is never green against an incomplete fix. If shipped in commit 1, it must be paired with the Fix B work in the same commit.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 6. Empty-List Behaviour assumes both Fix A and Fix B have landed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Empty-List Behaviour

**Details**:
Empty-List Behaviour says: "on a freshly bootstrapped server with no restorable state, `Client.ListSessions` returns an empty slice (only `_portal-bootstrap` and `_portal-saver` exist, both filtered)." This is true only after Fix B has landed; under Fix A alone, the bootstrap session is still `0`, which is not filtered, and `ListSessions` would return `["0"]`, not empty.

This is a minor descriptive issue rather than an implementation gap — the Empty-List Behaviour describes the post-shipping state, and shipping requires both fixes per Rollout. But a casual reader of Fix A in isolation (e.g. a planning agent breaking work into tasks) could misinterpret this as Fix A's standalone behaviour.

**Proposed Addition**:
Clarify in Fix A's Empty-List Behaviour: prefix the sentence with "After both Fix A and Fix B have landed, on a freshly bootstrapped server..." so the precondition is explicit and the section is correctly contextualised.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 7. `cmd/list.go` empty-output behaviour — claim that nothing changes is unverified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Empty-List Behaviour

**Details**:
Spec says `portal list` "prints nothing (silent exit, no 'no sessions' message). This is the existing behaviour — no change required." The word "existing" implies a verification was done. Today, before Fix A, `portal list` always has at least `0` and `_portal-saver` to print, so the empty-input branch of the current implementation may never have been exercised in production.

If `cmd/list.go` happens to print something on empty input (e.g. an "iterating over zero items" path that emits a trailing newline, or a future hypothetical message), the post-fix behaviour would not match spec. The "no change required" claim should be verifiable from the current code, not assumed.

**Proposed Addition**:
Either (a) drop the "no change required" framing and instead state "Implementation MUST emit no output when the slice is empty — verify and adjust `cmd/list.go` if necessary," or (b) confirm via a quick read of `cmd/list.go` that the empty-input path is truly silent and tighten the spec wording. Recommend (a) — it makes the contract enforceable rather than reliant on an unverified premise.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 8. Doc-comment cleanup placement when Fix A ships before Fix B

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Doc-Comment Cleanup; Rollout

**Details**:
Rollout pairs each doc-comment with a specific commit:
- Commit 1 (Fix A): doc-comment cleanup on `tmux.PortalSaverName`.
- Commit 2 (Fix B): doc-comment cleanup on `tmux.StartServer`.

But the `tmux.PortalSaverName` directive (Doc-Comment Cleanup section) says the comment "MUST be re-read in the post-fix context and revised so it correctly references the chokepoint — `Client.ListSessions`." The chokepoint exists after Fix A — that part lines up. However, the `tmux.StartServer` directive says the comment must "Document that the session is created with the reserved name `PortalBootstrapName`" and "Document that the session is hidden from user-facing listings by the underscore-prefix filter in `Client.ListSessions`." The latter clause references Fix A's filter, which by Rollout has already landed in commit 1 — fine.

So commit 1 lands a `PortalSaverName` doc-comment that says "filtered by `Client.ListSessions`" (true after Fix A) and a `StartServer` body unchanged from current (still creates session `0`). Commit 1's doc-comment is referencing the new filter while the bootstrap session is still `0` — internally consistent for commit 1 in isolation, but a future reader bisecting at commit 1 sees a doc-comment claim about `_*`-filter-hides-bootstrap while bootstrap is still `0`. Not a bug, just a momentarily-inconsistent intermediate state.

This is fine as long as the two commits are reviewed together. Worth a small note in Rollout to encourage reviewing them as a pair, not in isolation.

**Proposed Addition**:
Add a brief note to Rollout: "The two commits should be reviewed as a unit — commit 1's doc-comment references Fix A's filter behaviour, which is fully realised only after commit 2's rename. Reviewers landing only commit 1 should expect a transient state where `_portal-saver` is hidden but `0` remains visible."

**Resolution**: Approved
**Notes**: Auto-approved.

---
