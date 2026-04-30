---
phase: 1
phase_name: Filter `_*` Sessions in Client.ListSessions
total: 2
---

## hidden-sessions-showing-on-startup-1-1 | approved

### Task 1-1: Add underscore-prefix filter to `Client.ListSessions` with unit test

**Problem**: The `built-in-session-resurrection` spec mandates that sessions whose names begin with `_` are hidden Portal-wide. The capture path filters them, but `internal/tmux.Client.ListSessions` does not — so every downstream consumer (TUI session picker via `internal/tui/model.go.filteredSessions`, `cmd/list.go`, future callers) leaks `_portal-saver` (and, after Phase 2, `_portal-bootstrap`) into user-visible output. This is Root Cause 1 of the bug.

**Solution**: Apply the underscore-prefix filter at the chokepoint — inside `Client.ListSessions` in `internal/tmux/tmux.go` — as the final post-processing step before return. Single source of truth: every current and future caller inherits the invariant without per-consumer code changes. Ship a unit test in `internal/tmux/tmux_test.go` that drives mocked `Commander` output with a mix of `_*` and non-`_*` names and asserts only non-`_*` names survive (plus a fully-filtered case asserting non-nil empty slice).

**Outcome**: After this task, `Client.ListSessions` never returns a session whose name starts with `_`; the returned slice is always non-nil (empty when fully filtered); `ListSessionNames` continues to delegate to `ListSessions` with no new low-level enumeration; the capture path's existing `keepSessionNames` filter (`internal/state/capture.go:218-228`) double-filters `_*` names as a verifiable no-op; the new unit test guards against regression of Root Cause 1; `go test ./...` is green.

**Do**:
- In `internal/tmux/tmux.go`, edit `Client.ListSessions` (currently at lines 108-150). After the existing parsing loop builds the `sessions` slice, add a final post-processing pass that filters out any `Session` whose `Name` satisfies `strings.HasPrefix(s.Name, "_")`. The filter is unconditional (no flag, no escape hatch) and runs **last** — after parsing and any future ordering/enrichment — so the contract "the returned slice never contains a `_*` name" survives further pipeline evolution.
- Preserve the existing non-nil-slice contract: when every parsed session is filtered, return a non-nil empty `[]Session{}` (never `nil`). The simplest way is to allocate a fresh `filtered := make([]Session, 0, len(sessions))` and append survivors; do not return the input slice short-circuited to nil.
- Do **not** touch `ListSessionNames` (lines 157-167). It is a thin wrapper around `ListSessions` and the spec mandates it stay that way ("`ListSessionNames` MUST remain a delegation to `ListSessions` — it MUST NOT bypass `ListSessions` to query tmux directly"). Leave the existing delegation in place; it inherits the filter for free.
- In `internal/tmux/tmux_test.go`, add a new top-level test (suggested name: `TestListSessionsFiltersUnderscorePrefixed`) that uses `MockCommander` (already defined at lines 12-42) to drive `list-sessions` output. Use the existing table-driven style of `TestListSessions` (lines 44+).
- The new test must include at minimum these table entries:
  1. **Mixed names** — mocked output `dev|2|0\n_portal-saver|1|0\nwork|3|1\n_portal-bootstrap|1|0`; assert returned slice contains exactly `dev` and `work`, in that order, with all metadata preserved.
  2. **All filtered** — mocked output `_portal-saver|1|0\n_portal-bootstrap|1|0`; assert returned slice is non-nil and `len == 0`. Use `result == nil` (the `nil`-check) and `len(result) == 0` as separate assertions so a regression to `return nil` is caught explicitly.
  3. **Boundary literal** — confirm the match is `strings.HasPrefix(name, "_")` with no trimming: an entry with a leading space (e.g. ` _underscore|1|0`) is **not** filtered (whitespace before `_`), because `strings.HasPrefix(" _underscore", "_")` is false. (Note: tmux session names cannot legitimately contain leading spaces; this case is asserted to nail down the literal-prefix semantics, not because it is reachable.)
- Do **not** add or modify capture-path tests. `internal/state/capture_test.go` is the explicit regression gate: run it (via `go test ./...`) and verify it still passes — that proves the double-filter no-op claim from the spec.

**Acceptance Criteria**:
- [ ] `Client.ListSessions` excludes any session whose name satisfies `strings.HasPrefix(name, "_")` as the final post-processing step before return.
- [ ] When all underlying sessions are filtered, `ListSessions` returns a non-nil empty slice (never `nil`); JSON-serialisation of the result produces `[]`, not `null`.
- [ ] `ListSessionNames` is unchanged and continues to delegate to `ListSessions` (no new low-level enumeration introduced); the delegation invariant from the spec's "Interaction With The Capture Path" section is preserved.
- [ ] New unit test in `internal/tmux/tmux_test.go` asserts that mocked `Commander` output containing a mix of `_*` and non-`_*` names yields a returned slice containing only the non-`_*` names with metadata intact.
- [ ] New unit test includes a fully-filtered case asserting the returned slice is non-nil and `len == 0`.
- [ ] Existing `TestListSessions` cases continue to pass unchanged.
- [ ] Existing `internal/state/capture_test.go` regression tests pass unchanged (capture-path regression guard from spec → Test Requirements → Capture-Path Regression Guard).
- [ ] `go test ./...` is green.

**Tests**:
- `"it returns only non-underscore-prefixed sessions when input contains a mix"` — mocked tmux output `dev|2|0\n_portal-saver|1|0\nwork|3|1\n_portal-bootstrap|1|0`; result slice equals `[{Name:"dev", Windows:2, Attached:false}, {Name:"work", Windows:3, Attached:true}]`.
- `"it returns a non-nil empty slice when every session is underscore-prefixed"` — mocked output `_portal-saver|1|0\n_portal-bootstrap|1|0`; assert `result != nil && len(result) == 0`.
- `"it returns a non-nil empty slice when tmux reports no sessions"` (existing case, must still pass) — mocked output `""`; assert `result != nil && len(result) == 0`.
- `"it preserves session metadata for survivors"` — `dev|7|2` survives with `Windows:7, Attached:true`.
- `"it matches the underscore prefix literally with no trimming"` — entry ` _x|1|0` (leading space) is **not** filtered; entry `_x|1|0` is filtered.
- `"it preserves order of surviving sessions as tmux reported them"` — input `a|1|0\n_b|1|0\nc|1|0\n_d|1|0\ne|1|0`; result names are `[a, c, e]` in that order.
- Capture-path regression: existing tests in `internal/state/capture_test.go` (notably the case at line 135 referenced by the spec) pass unchanged when run via `go test ./...`.

**Edge Cases**:
- All sessions filtered → non-nil, `len == 0` slice (NOT `nil`). Callers may rely on `len(result) == 0` and on JSON-`[]` rather than `null`.
- Zero sessions reported by tmux (server has no sessions, or `list-sessions` returns empty) → existing path already returns `[]Session{}`; filter is a no-op on empty input.
- `ListSessionNames` delegation invariant: the chosen filter strategy depends on `ListSessionNames` calling `ListSessions`; do not introduce a sibling raw enumeration, even as an "optimisation" — that would silently bypass the filter and partially regress Root Cause 1 for the capture path.
- Capture-path double-filter: `internal/state/capture.go:218-228` applies `keepSessionNames` on top of `ListSessionNames`. Once `ListSessions` filters `_*`, the `keepSessionNames` set difference is identical (set difference of an already-disjoint subset). The capture-path tests are the regression gate proving this is a no-op.
- Match is on the literal session name as reported by tmux: no trimming, no case-folding. `strings.HasPrefix(name, "_")` is the exact predicate. A name starting with whitespace then `_` is **not** filtered.

**Context**:
> From the specification's Fix A → Behaviour Contract: "`internal/tmux.Client.ListSessions` MUST exclude any session whose name begins with `_` from its returned slice. The exclusion is unconditional and applies to every caller."
>
> From Fix A → Placement Rationale: chokepoint placement was deliberate over per-consumer placement. Future consumers cannot forget the rule.
>
> From Fix A → Interaction With The Capture Path: "`ListSessionNames` MUST remain a delegation to `ListSessions` — it MUST NOT bypass `ListSessions` to query tmux directly. The chosen strategy depends on this delegation: if a future change decouples them, the capture path silently loses the underscore filter and Root Cause 1 partially regresses for capture."
>
> From Fix A → Filter Application Order: "The filter is applied as the **final** post-processing step before return — after parsing tmux output and any current ordering."
>
> From Fix A → Return-Value Contract: "`ListSessions` returns an empty (non-nil) slice when all underlying sessions are filtered. … The implementation MUST NOT return `nil` to express 'no visible sessions'."
>
> From Fix A → Filter Definition: "A session is filtered when `strings.HasPrefix(name, "_")` is true. The match is on the literal session name as reported by tmux. No trimming, no case-folding."
>
> From Test Requirements → Unit — `Client.ListSessions` Excludes `_*` Sessions: "Drive the test with mocked `Commander` output containing a mix of `_*` and non-`_*` names; assert that only the non-`_*` names appear in the returned slice."
>
> From Test Requirements → Capture-Path Regression Guard: "The capture-path tests (`internal/state/capture_test.go:135` and related) MUST continue to pass unchanged."
>
> Existing `Client.ListSessions` lives at `internal/tmux/tmux.go:108-150`. Existing `ListSessionNames` lives at `internal/tmux/tmux.go:157-167`. Existing `TestListSessions` and `MockCommander` live at `internal/tmux/tmux_test.go:12-100+`. Existing `TestStartServer` lives at `internal/tmux/tmux_test.go:404+`.

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` — sections "Fix A — Filter `_*` Sessions In `Client.ListSessions`" and "Test Requirements — Unit — `Client.ListSessions` Excludes `_*` Sessions" and "Test Requirements — Capture-Path Regression Guard".

---

## hidden-sessions-showing-on-startup-1-2 | approved

### Task 1-2: Verify `cmd/list.go` empty-input contract and refresh `tmux.PortalSaverName` doc-comment

**Problem**: Two follow-on housekeeping items must ride with Phase 1's commit per the spec's Rollout section: (a) `cmd/list.go` must be **explicitly verified** against the empty-input contract that Phase 1 unlocks (the spec forbids assuming "no change required" without verification); (b) the `tmux.PortalSaverName` doc-comment encodes a claim ("filtered from the TUI picker and from `sessions.json` capture") that becomes accurate only once the chokepoint filter from Task 1-1 lands — it must be re-read in the post-fix context and either revised to correctly reference the chokepoint or explicitly noted as "reviewed, no change required" in the commit message. Phase 1 ships as one targeted commit (Fix A + unit test + this verification + doc-comment), so neither item can be deferred.

**Solution**: Read `cmd/list.go:66-68` (`if len(sessions) == 0 { return nil }`) against the spec's empty-input contract and confirm — or correct — that the existing path emits no output when `ListSessions` returns an empty slice. Read the `tmux.PortalSaverName` doc-comment at `internal/tmux/portal_saver.go:10-13` against the post-Task-1-1 reality, and either edit it to reference `Client.ListSessions` as the chokepoint for TUI-picker filtering, or record an explicit "reviewed, no change required" verification line in the commit message. Both verifications, plus Task 1-1's filter and unit test, ship as a single commit.

**Outcome**: After this task, `cmd/list.go`'s empty-input behaviour is verifiably aligned with the spec contract (silent exit on empty slice — no "no sessions" message, no spurious newline); the verification result is recorded (either as a code change or as a commit-message note); the `tmux.PortalSaverName` doc-comment correctly describes how `_portal-saver` is hidden post-fix (or is documented as already accurate via a commit-message note); Phase 1 ships as one targeted commit per the Rollout section.

**Do**:
- **Step 1 — Verify `cmd/list.go` empty-input contract.** Read `cmd/list.go` lines 51-93 (the `RunE` body). Confirm that when `lister.ListSessions()` returns an empty slice, the `if len(sessions) == 0 { return nil }` guard at lines 66-68 returns immediately without emitting any output to `cmd.OutOrStdout()`. The current code is compliant: the empty-slice branch returns `nil` before reaching the formatting loop at lines 79-90, so no `fmt.Fprintln` runs and no trailing newline is emitted. Record this as the verification outcome — **no code change required**. Document the verification in the commit message body (e.g. "Verified `cmd/list.go:66-68` empty-input guard satisfies the post-fix contract: silent exit, no 'no sessions' message, no extraneous newline.").
- **Step 2 — If the verification finds non-compliance** (e.g. a future edit slipped in a "no sessions" message before this phase lands): adjust `cmd/list.go` so the empty-slice branch produces zero bytes of stdout output. In that case the commit message records the adjustment instead of the verification-only note.
- **Step 3 — Do not touch `internal/tui/model.go`.** The spec defers TUI empty-state UX explicitly: "Adding a dedicated empty-state UX is **out of scope** for this bugfix." The existing `filteredSessions` empty rendering is accepted as-is.
- **Step 4 — Refresh `tmux.PortalSaverName` doc-comment.** Open `internal/tmux/portal_saver.go` and re-read lines 10-13 (the `// PortalSaverName …` doc-comment) in the post-Task-1-1 context. The current text is:
  ```
  // PortalSaverName is the tmux session name that hosts the long-running save daemon.
  // The leading underscore marks the session as Portal-internal so it is filtered
  // from the TUI picker and from sessions.json capture.
  ```
  After Task 1-1, the TUI-picker filtering happens at the chokepoint `Client.ListSessions` (not via any per-consumer logic in `internal/tui`). The current wording "filtered from the TUI picker" is technically true post-fix but does not point at the chokepoint that enforces the invariant. Edit the comment to make the chokepoint explicit. Suggested replacement (final wording is the implementer's call, but it MUST name `Client.ListSessions` as the enforcement point):
  ```
  // PortalSaverName is the tmux session name that hosts the long-running save daemon.
  // The leading underscore marks the session as Portal-internal: Client.ListSessions
  // applies a chokepoint underscore-prefix filter so this session is excluded from
  // every user-facing listing (the TUI picker, `portal list`, and any future
  // ListSessions consumer). It is also excluded from sessions.json capture by the
  // separate keepSessionNames pass in internal/state/capture.go.
  ```
- **Step 5 — Alternative path: explicit "reviewed, no change required" note.** If the implementer judges the existing wording already accurate against the post-fix code (it is "filtered from the TUI picker" — true; "and from sessions.json capture" — true), they may leave the comment unchanged. In that case the commit message MUST contain an explicit line such as: "Reviewed `tmux.PortalSaverName` doc-comment against post-fix reality: existing wording remains accurate; no change required." The spec is explicit that this option exists ("ships a deliberate edit OR an explicit 'reviewed, no change required' code comment in the commit message") — preferred outcome is the deliberate edit, because pointing at the chokepoint is more useful to future readers, but either is acceptable.
- **Step 6 — Do not touch the `tmux.StartServer` doc-comment.** That cleanup is owned by Phase 2 (per the spec's "Doc-Comment Cleanup → `tmux.StartServer`" section and the planning file's Phase 2 acceptance criteria). Phase 1 touches only `tmux.PortalSaverName`.
- **Step 7 — Single-commit shape.** Per the Rollout section, Phase 1 is one targeted commit containing: Task 1-1's filter + unit test, Task 1-2's verification of `cmd/list.go`, and Task 1-2's doc-comment refresh (or commit-message verification note). No staged commits within the phase.

**Acceptance Criteria**:
- [ ] `cmd/list.go:66-68` (`if len(sessions) == 0 { return nil }`) is verified to emit no output when `ListSessions` returns an empty slice; the verification result is recorded in the commit message.
- [ ] If the verification finds the existing path non-compliant, `cmd/list.go` is adjusted so the empty-slice branch produces zero bytes of stdout output; otherwise no code change is made to `cmd/list.go`.
- [ ] `internal/tui/model.go` is **not** modified (TUI empty-state UX is explicitly out of scope per spec).
- [ ] `tmux.PortalSaverName` doc-comment at `internal/tmux/portal_saver.go:10-13` is reviewed against the post-Task-1-1 reality and either: (a) updated to reference `Client.ListSessions` as the chokepoint for user-facing listing filtering, or (b) left unchanged with an explicit "reviewed, no change required" verification line in the commit message.
- [ ] `tmux.StartServer` doc-comment is **not** touched in this phase (owned by Phase 2).
- [ ] Phase 1 ships as a single targeted commit containing Task 1-1's filter + unit test together with this task's verification + doc-comment outcome, per the Rollout section's "two small targeted commits, each with its own test" shape.
- [ ] `go test ./...` remains green (this task introduces no behavioural code change beyond the optional doc-comment edit).

**Tests**:
- No new automated tests are introduced by this task. The verification of `cmd/list.go`'s empty-input behaviour is a reading-and-recording exercise; the existing `cmd/list_test.go` (and any test exercising `len(sessions) == 0`) is the regression gate. Run `go test ./cmd/...` to confirm existing list-command tests pass unchanged.
- The doc-comment edit (or explicit verification note) is non-executable and is reviewed at commit time, not at test time.
- Manual verification (commit-message-recorded): with Task 1-1 applied, on a freshly bootstrapped tmux server hosting only `_portal-saver` (and, post-Phase-2, `_portal-bootstrap`), `portal list` produces zero bytes of stdout output and exits 0. (Optional smoke check; not required for the commit to merge — the spec does not mandate an automated end-to-end test in Phase 1; that lives in Phase 2.)

**Edge Cases**:
- Existing empty-input path already compliant — verification-only outcome with a commit-message note recording the verification. This is the expected default.
- Doc-comment already accurate after Task 1-1 lands — commit-message-note path; the spec explicitly permits this. Preferred outcome remains a deliberate edit pointing at `Client.ListSessions`, because chokepoint visibility helps future readers.
- No per-consumer code change is required in `cmd/list.go` (existing guard suffices) or `internal/tui` (TUI empty-state UX is out of scope per spec → "Empty-List Behaviour"). Resist the urge to add a "no sessions" message — the spec forbids it: "MUST emit no output when `ListSessions` returns an empty slice (silent exit, no 'no sessions' message, no trailing newline beyond what the existing iteration produces)."
- Phase 1 must not touch `tmux.StartServer` or its doc-comment — that work is owned by Phase 2's commit.

**Context**:
> From the specification's Fix A → Empty-List Behaviour: "**`portal list`:** MUST emit no output when `ListSessions` returns an empty slice (silent exit, no 'no sessions' message, no trailing newline beyond what the existing iteration produces). The implementer MUST verify `cmd/list.go` against this contract and adjust if the existing empty-input path produces output; do not assume 'no change required' without the verification."
>
> From Fix A → Empty-List Behaviour (TUI clause): "**TUI session picker (`internal/tui`):** verify the existing empty-list rendering (`filteredSessions` returning an empty slice) is acceptable. Adding a dedicated empty-state UX is **out of scope** for this bugfix."
>
> From Doc-Comment Cleanup → `tmux.PortalSaverName`: "After Fix A lands the claim becomes accurate. The comment MUST be re-read in the post-fix context and revised so it correctly references the chokepoint — `Client.ListSessions` — as the source of TUI-picker filtering rather than implying any per-consumer filter. If the existing wording is already accurate against the post-fix code, leave it; otherwise update it. The implementer ships a deliberate edit OR an explicit 'reviewed, no change required' code comment in the commit message."
>
> From Rollout: "Commit shape: two small targeted commits, each with its own test: 1. Fix A (filter in `Client.ListSessions`) plus the `Client.ListSessions` unit test and the doc-comment cleanup on `tmux.PortalSaverName`."
>
> Existing `cmd/list.go` empty-input guard lives at `cmd/list.go:66-68`. Existing `tmux.PortalSaverName` doc-comment lives at `internal/tmux/portal_saver.go:10-13`. The `tmux.StartServer` doc-comment (NOT touched in this phase) lives at `internal/tmux/tmux.go:169-174` — that one is rewritten in Phase 2.

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` — sections "Fix A — Empty-List Behaviour", "Doc-Comment Cleanup — `tmux.PortalSaverName`", and "Rollout".
