---
agent: standards
cycle: 2
findings_count: 2
---
# Standards Analysis (Cycle 2)

## Summary

Two drifts. CLAUDE.md still documents the pre-resurrection world (ExecuteHooks, @portal-active markers, bootstrapWait, WaitForSessions) — high-impact because it misguides future contributors and Claude sessions. Spec text describing the hydrate helper as the pane's initial process needs updating to match the implemented `respawn-pane -k` arming mechanism — medium-impact spec-vs-code wording drift.

---

## Findings

### FINDING: CLAUDE.md describes removed mechanisms (ExecuteHooks, @portal-active markers, bootstrapWait, WaitForSessions)
- **Severity**: high
- **Files**: `CLAUDE.md:42`, `CLAUDE.md:49`, `CLAUDE.md:63`, `CLAUDE.md:65-67`
- **Description**: The project conventions doc still documents the pre-resurrection design. It lists `WaitForSessions` in the tmux package methods (L42); it describes the `hooks` package as containing `ExecuteHooks` plus `@portal-active-<pane>` volatile markers (L49); it claims `PersistentPreRunE` calls `bootstrapWait()` with a "1–6s window" (L63); and an entire "Resume hooks" section (L65-67) describes send-keys-driven attach-time firing with `@portal-active-<pane>` dual-level tracking. All explicitly deleted by the spec. Verified `grep -rn "ExecuteHooks\|WaitForSessions\|bootstrapWait" --include="*.go"` returns zero hits in product code. CLAUDE.md is the first thing a new contributor or future Claude session reads; leaving it describing deleted internals will misguide refactors.
- **Recommendation**:
  1. L42: drop `WaitForSessions` from the tmux Client method list. Optionally add new methods (`RespawnPane`, `SetSessionOption`, `IsRestoringSet`, hook-register helpers).
  2. L49: rewrite the `hooks` row to drop `ExecuteHooks` and `@portal-active-<pane>`; describe the package as the JSON-backed `Store` only — firing now lives in the hydrate helper.
  3. L63: rewrite the server-bootstrap section to describe the 8-step `bootstrap.Orchestrator` and the 1.2s minimum TUI loading-page pad; remove the "1–6s polling window" claim.
  4. L65-67: replace the Resume-hooks paragraph to state hooks fire only inside the hydrate helper's exec chain after skeleton-restore (reboot recovery), not on every detach/reattach; drop the `@portal-active-<pane>` dual-level tracking description.
  5. Add a brief mention of the new `internal/state` and `internal/restore` packages and the `_portal-saver` detached session hosting `portal state daemon`.

### FINDING: Specification text still describes the hydrate helper as the pane's INITIAL process; chosen implementation arms it via `respawn-pane -k`
- **Severity**: medium
- **Files**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:632`, `:730-740`, `:1022`; `internal/restore/session.go:163-221`, `:491-508`
- **Description**: Per the cycle 2 input note (T7-9 pivot), restore now creates each pane with the default shell and dispatches the helper via `tmux respawn-pane -k <hydrate-cmd>`. Implementation comments at session.go:165-170 and session.go:491-495 already acknowledge that respawn-pane is "load-bearing for the spec's 'helper pre-shell' contract" — i.e., the spec is being preserved semantically but not literally. The spec text still describes the helper as the pane's *initial* process in three places: § Skeleton-Eager + Scrollback-Lazy ("`new-session -d -s <name> -c <root_cwd> "<hydrate command for first pane>"`" L632); § Scrollback Restore Mechanics → Injection Path ("Each skeleton-restored pane is created with a shell-pipeline command **as its initial process**" L730-733; "the shell does not exist yet when the bytes are written" L750); § Bootstrap Flow step 5.2 (L1022). Code is correct; spec wording lags.
- **Recommendation**: Update the spec to describe respawn-pane -k arming. Specifically:
  1. L632: change the new-session command from `"<hydrate command for first pane>"` to default shell (or omitted command), and add a sentence stating the helper is dispatched via `respawn-pane -k` immediately after pane creation.
  2. L730-740: rephrase "Each skeleton-restored pane is created with a shell-pipeline command **as its initial process**" to "Each skeleton-restored pane is **respawned** immediately after creation with a shell-pipeline command via `respawn-pane -k`. respawn-pane atomically kills the default shell and replaces the pane's process with the helper, so the shell does not produce output before the helper runs." The L750 "shell does not exist yet when the bytes are written" claim survives because respawn-pane -k is atomic.
  3. L1022: replace the literal `new-session ... "sh -c 'portal state hydrate ...'"` with the default-shell form, and add a step describing the post-creation `respawn-pane -k` invocation.
  4. Alternatively, append a brief "Implementation Note: respawn-pane arming" subsection.
