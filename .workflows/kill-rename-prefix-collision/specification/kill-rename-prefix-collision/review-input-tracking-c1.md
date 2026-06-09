---
status: in-progress
created: 2026-06-09
cycle: 1
phase: Input Review
topic: kill-rename-prefix-collision
---

# Review Tracking: kill-rename-prefix-collision - Input Review

## Findings

### 1. User-facing exposed callers (the real-world blast radius) not documented

**Source**: Investigation §Analysis › Code Trace › "Callers (blast radius of the destructive paths)" (lines 115-126)
**Category**: Enhancement to existing topic
**Affects**: "Required Behaviour & The Fix" › §2 "Fix the two destructive callers" (the paragraph beginning "The fix lives entirely at the Client-method chokepoint…")

**Details**:
The investigation enumerates the concrete caller surfaces and marks three as **exposed** (the user-facing, collision-prone paths):
- `KillSession`: `cmd/kill.go:37` (`portal kill <name>`) and `internal/tui/model.go:2171` (TUI kill key) — both **exposed**
- `RenameSession`: `internal/tui/model.go:2225` (TUI rename) — **exposed**

The specification mentions only the *internal* `_portal-saver` callers (`cmd/state_cleanup.go`, `internal/tmux/portal_saver.go`) when discussing callers — and frames them as the ones that "gain the `=` prefix harmlessly." It never names the user-facing entry points (`portal kill <name>` CLI and the TUI kill/rename keys) that are the *actual* exposure for this bug. While the fix is at the Client chokepoint and needs no caller-side change, naming the exposed surfaces grounds the severity ("which real user actions can trigger the wrong-session kill") and helps the implementer/tester understand what to manually verify. The asymmetry is notable: the spec documents the harmless internal callers but omits the dangerous exposed ones.

**Current**:
> The fix lives entirely at the Client-method chokepoint. Both methods are the single argv-construction point, so fixing the argv inside them covers every caller uniformly — **no caller-side change anywhere**. This includes the internal `_portal-saver` `KillSession` callers (`cmd/state_cleanup.go`, `internal/tmux/portal_saver.go`), which gain the `=` prefix harmlessly (fixed literal name, no possible prefix collision).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. Pane-ID `display-message -t <paneID>` (line 324) explicitly excluded as not a collision concern

**Source**: Investigation §Analysis › Blast Radius (lines 198-200): "(`display-message -t <paneID>` at 324 targets a unique `%N` pane ID — not a prefix-collision concern.)"
**Category**: Enhancement to existing topic
**Affects**: "Migration Scope & Out of Scope" › §"Explicitly out of scope"

**Details**:
The investigation explicitly reasons about `display-message -t <paneID>` at line 324 and excludes it on a *different* basis than every other out-of-scope item: it targets a unique `%N` pane ID, so it is categorically immune to prefix collision (not merely "non-destructive" or "lower exposure"). The specification's out-of-scope section omits this site entirely. Because the spec's stated goal is that "no inline `"="+name` session-target strings remain" and it carefully catalogs the surrounding `-t` surface, an implementer auditing `tmux.go` line-by-line will encounter line 324 and may wonder whether it needs the prefix. Recording the investigation's explicit "unique pane ID → not a collision concern" rationale forecloses that question and prevents an accidental (harmful — `=%N` would break) attempt to "fix" it.

**Current**:
> - **Caller-supplied pane/window-target writers** — `SendKeys`, `RespawnPane`, `CapturePane`, `NewWindow`, `SplitWindow`, `SelectLayout`. Lower collision exposure (not session names directly).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
