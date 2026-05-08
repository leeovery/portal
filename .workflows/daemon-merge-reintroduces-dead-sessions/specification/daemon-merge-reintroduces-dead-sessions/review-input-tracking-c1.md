---
status: complete
created: 2026-05-08
cycle: 1
phase: Input Review
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Input Review

## Findings

### 1. "Why It Wasn't Caught" missing from specification

**Source**: `investigation/daemon-merge-reintroduces-dead-sessions.md` lines 107-112 ("Why It Wasn't Caught" section)
**Category**: New topic
**Affects**: Specification has no equivalent section; would slot near "Testing Requirements" or as a sibling to "Scope and Risk"

**Details**:
The investigation has a dedicated "Why It Wasn't Caught" section enumerating four reasons:
1. Existing unit test (`capture_test.go:570-617`) explicitly asserts the buggy behaviour as correct — codifies the wrong invariant.
2. Original spec for `built-in-session-resurrection` framed merge intent around hydrate-in-progress only, did not model marker-staleness adversarial cases.
3. Feature integration tests exercise happy-path skeleton → hydrate flow, not the killed-mid-flight path.
4. Reproducing in the wild requires hydrate failure (hard to engineer in CI) or manual marker injection.

The specification mentions item 1 in passing ("the test codifies the buggy behaviour as correct and must be replaced") but loses items 2-4. These rationale points matter for the planning phase: they justify why new test categories (adversarial marker-staleness, killed-mid-flight) need to be added beyond simply replacing the existing test.

**Proposed Addition**:
New section "Why This Bug Wasn't Caught" added before "Scope and Risk" enumerating all four reasons and noting they justify the new adversarial/regression test categories.

**Resolution**: Approved
**Notes**: Approved via auto mode. Section added before "Scope and Risk".

---

### 2. Blast radius — downstream consumers of `sessions.json` not enumerated

**Source**: `investigation/daemon-merge-reintroduces-dead-sessions.md` lines 114-122 ("Blast Radius" section)
**Category**: Enhancement to existing topic
**Affects**: "Impact" section (lines 26-29 of specification)

**Details**:
Investigation enumerates downstream consumers affected by the polluted `sessions.json`:
- `internal/state` — committed `sessions.json` becomes inconsistent with live tmux.
- `internal/restore` — reconstructs ghost sessions on bootstrap.
- "Any consumer that reads `sessions.json` (CLI list commands, TUI session picker after a restart) sees the ghost session."

The specification's Impact section captures the bootstrap/restore path but does not explicitly call out the "CLI list commands" and "TUI session picker after a restart" surface. This matters for acceptance/manual-test planning — the verification surface includes those user-facing entry points, not just `sessions.json` content.

**Current**:
> ### Impact
>
> - **Severity:** High — silent corruption of persisted state; user-visible "zombie" sessions; eroded trust that `kill-session` is permanent.
> - **Scope:** All users running `portal state daemon`; triggers under any path producing a stale `@portal-skeleton-*` marker.
> - **Manifestation:** Killed session reappears in `~/.config/portal/state/sessions.json` within one daemon tick (≤30s). No error or warning surfaces.

**Proposed Addition**:
Added a new bullet under Impact: "User-visible surfaces affected" listing internal/restore, CLI list commands, and TUI session picker after restart.

**Resolution**: Approved
**Notes**: Approved via auto mode. Added as new "User-visible surfaces affected" bullet in Impact.

---

### 3. Test file line range — `capture_test.go:570-617` vs `:570`

**Source**: `investigation/daemon-merge-reintroduces-dead-sessions.md` line 81 (`capture_test.go:570-617`)
**Category**: Enhancement to existing topic
**Affects**: "Existing Tests to Replace" (line 102-104) and "Files Touched" (line 152) of specification

**Details**:
Investigation cites the buggy test's full line range (`capture_test.go:570-617`); the specification cites only the start line (`:570`). The end line is useful for the implementer to know the full extent of the test body that will be replaced. Minor but a free precision improvement.

**Current**:
> **`internal/state/capture_test.go:570`** — The test `TestCaptureStructureMergeSkippedPanes/merges a skipped pane's session and window from prev when missing from fresh` codifies the buggy behaviour as correct and **must be replaced** with its inverse:

**Proposed Addition**:
Updated reference to `internal/state/capture_test.go:570-617`.

**Resolution**: Approved
**Notes**: Approved via auto mode. Line range updated.

---

### 4. "Business impact" framing missing from specification

**Source**: `investigation/daemon-merge-reintroduces-dead-sessions.md` line 42 ("Business impact: Trust regression on a core product promise (user controls their session list).")
**Category**: Enhancement to existing topic
**Affects**: "Impact" section (lines 26-29 of specification)

**Details**:
The investigation explicitly frames a "Business impact" beyond severity/scope/manifestation: "Trust regression on a core product promise (user controls their session list)." The specification compresses this into "eroded trust that `kill-session` is permanent" inside the Severity bullet. The investigation framing — that this is a regression against a *core product promise* — is stronger and more explicit. Worth deciding whether to preserve verbatim or accept the compressed form.

**Current**:
> - **Severity:** High — silent corruption of persisted state; user-visible "zombie" sessions; eroded trust that `kill-session` is permanent.

**Proposed Addition**:
Updated Severity bullet to surface "Business impact: Trust regression on a core product promise (user controls their session list)" as an explicit sub-frame.

**Resolution**: Approved
**Notes**: Approved via auto mode. Severity bullet updated to preserve "Business impact" framing.

---
