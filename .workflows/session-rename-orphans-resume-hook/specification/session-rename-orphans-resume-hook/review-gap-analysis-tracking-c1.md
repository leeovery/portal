---
status: complete
created: 2026-07-01
cycle: 1
phase: Gap Analysis
topic: session-rename-orphans-resume-hook
---

# Review Tracking: session-rename-orphans-resume-hook - Gap Analysis

## Findings

### 1. Capture assembly path: how the per-pane `#{@portal-id}` reaches the session-level `PortalID` field is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Cross-Reboot Persistence of `@portal-id` → §2 Capture

**Details**:
The spec says "Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it. `#{@portal-id}` resolves per-pane to the owning session's option value, so it is present on every pane row for that session; the parser takes it when assembling the session."

But the actual assembly path is more involved than "the parser takes it," and the spec leaves the concrete threading unstated. In `capture.go` the `Session` value is not built from the pane rows at all — it is assembled from the `keep` name set:

```go
sessions = append(sessions, Session{
    Name:        name,
    Environment: parseShowEnvironment(envRaw),
    Windows:     buildWindows(name, grouped[name]),
})
```

The new per-pane field is parsed into `paneRow` (which today carries only pane/window-scoped fields, no session-scoped field), and `paneRow` is consumed by `buildWindows`/`buildPanes` — neither of which surfaces a session-level attribute back to the `Session{...}` literal above. So an implementer has to decide, unspecified by the spec:
- whether to add a `portalID` field to `paneRow`,
- how to lift a per-pane value up to the session level (e.g. take it from `grouped[name][0]`, the first row), and
- what "the first/canonical row" is when a session has multiple windows/panes (they should all agree, but the spec should say "take from any row / first row, they are identical per session" so the implementer doesn't invent a reconciliation rule).

This is the load-bearing persistence step; leaving the assembly mechanism to guesswork risks the id being parsed but never landing on `Session.PortalID`.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. Capture §2 now specifies: parse into a `portalID` field on `paneRow`, lift `Session.PortalID` from the first row of `grouped[name]` (all rows identical per session).

---

### 2. Capture edge case: session in `keep` with zero pane rows yields no source for `PortalID`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Cross-Reboot Persistence of `@portal-id` → §2 Capture

**Details**:
Because the spec sources `#{@portal-id}` from pane rows ("it is present on every pane row for that session"), the value has no source when a session appears in the `keep` set but contributes no pane rows to `grouped` (`grouped[name]` is nil/empty). This is a real state the code already tolerates — `buildWindows(name, grouped[name])` is called with a possibly-empty group, and the capture loop separately handles vanished/churned sessions. In that case `PortalID` would silently be `""` even for a stamped session.

The spec should state the expected behavior: either (a) this window is acceptable because a session with no captured panes is not restored/hydrated anyway, or (b) the id should be sourced independently of pane rows (e.g. a per-session `show-options` read) to avoid the pane-row dependency entirely. Right now an implementer cannot tell whether the empty-group case is a defect to guard or a benign no-op.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. Documented as benign — zero-row (churn) session yields empty id + empty Windows and is rejected by Restore, so the empty id is never consumed; no guard needed.

---

### 3. Registration: behavior when the new `ResolveHookKey` live read fails is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Hook-Key Derivation → Stage 1 — Registration (`cmd/hooks.go`)

**Details**:
The spec replaces `resolveCurrentPaneKey()`'s call to `ResolveStructuralKey($TMUX_PANE)` with a new client read (`ResolveHookKey(paneID)` → `display-message -p -t <pane> <HookKeyFormat>`). The current `ResolveStructuralKey` returns an error on failure, and the caller (`portal hooks set`/`rm`) presumably surfaces it. The spec does not state the failure contract for the new read:
- Does `portal hooks set` abort with an error (parity with today)?
- Does it fall back to a name-based key on read failure?

This matters for correctness: a silent name-based fallback on a transient read failure for a *stamped* session would register the hook under the name key, re-introducing exactly the orphaning this fix removes if the session is later renamed. The spec's central invariant ("every key-producing site derives the same key") depends on registration never quietly diverging, so the failure path should be pinned (recommended: abort/propagate the error, no fallback — a stamped session must key by id or not register).

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. Stage 1 now pins the failure contract: on read failure, abort/propagate (parity with ResolveStructuralKey); never synthesize a name-based key.

---

### 4. Token generation contract: uniqueness guarantee vs. fire-and-forget, and the consequence of a collision

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Overview → "Its value: a fresh opaque id"; Risks → "Token collision"

**Details**:
The spec frames token collision as a purely quantitative "implementation detail" ("width chosen so birthday-collision across the whole session population is negligible") and reuses "the existing `NewNanoIDGenerator` scheme, widened if warranted." But it does not specify the *generation contract*, which is a behavioral (not just sizing) decision:
- `GenerateSessionName` today actively *guarantees* uniqueness (it loops until the name is new). The spec is silent on whether `@portal-id` generation is a guaranteed-unique loop or a single fire-and-forget draw.
- The spec does not state what happens on the (rare) collision: two sessions sharing an `@portal-id` collide in the `hooks.json` key namespace, so a hook registered under one session could fire in the other's pane — a correctness/cross-talk outcome, not merely a probability figure.

At minimum the spec should state: (a) whether generation checks for uniqueness among live sessions or relies solely on width, and (b) that a collision (if unchecked) is an accepted residual risk with the described cross-talk consequence — so an implementer knows whether to build a uniqueness check.

Note: the current `NewNanoIDGenerator` is 6 chars alphanumeric; the spec's "widened if warranted" leaves the actual width undecided, which is acceptable as an implementation detail *only if* the generation contract above is pinned.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. Identity section now states fire-and-forget generation (no uniqueness check), width-only correctness, and the accepted cross-talk residual; ties to QuickStart's seam-less argv chain.

---

### 5. QuickStart token value: where `<token>` is generated for the chained ExecArgs is unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Overview → "Where it is stamped" (QuickStart.Run)

**Details**:
For `CreateFromDir` the spec is concrete (a Go-level `SetSessionOption` call after `NewSession`, error swallowed). For `QuickStart.Run` the spec shows only the argv fragment `; set-option -t <name> @portal-id <token>` in the chained `ExecArgs`. Because QuickStart hands off to a single chained tmux invocation (create-detached → stamp → attach) via `syscall.Exec`, `<token>` must be materialized as a literal argv value *before* the exec — there is no Go error-handling seam inside the chain. The spec does not say where/when the token is generated in the QuickStart path (e.g. in `QuickStart.Run` before assembling `ExecArgs`, via the same generator as `CreateFromDir`). Minor, but an implementer must otherwise infer the generation site and confirm it uses the same generator/width as the other path (relevant to Finding 4's uniqueness contract).

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. QuickStart bullet now states the token is generated in Go inside Run before ExecArgs, interpolated as a literal; generation failure omits the stamp step (best-effort).

---

### 6. `captureFieldCount` field placement / parser index updates are described but not pinned to a position

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Cross-Reboot Persistence → §2 Capture

**Details**:
The spec correctly flags that `captureFormat` is fixed-arity, that `captureFieldCount` must bump 10 → 11, and that "the parser's field-index reads must update in lockstep." It does not specify *where* in the `|||`-delimited format the new `#{@portal-id}` field is appended (end vs. inline), which determines which existing `parts[N]` indices shift. Appending at the end (new `parts[10]`) leaves all existing indices untouched and is the lowest-risk placement; inserting inline shifts every downstream index and is a larger, more error-prone change. Since the spec already treats the arity/index lockstep as load-bearing, it should state the placement (recommend: append as the trailing field) so the implementer doesn't re-order existing reads unnecessarily. Minor — the "update in lockstep" instruction covers correctness, but placement guidance de-risks the change.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Approved via auto mode. Folded into Finding 1's Capture addition — append `#{@portal-id}` as the last column so existing indices are unchanged.

---
