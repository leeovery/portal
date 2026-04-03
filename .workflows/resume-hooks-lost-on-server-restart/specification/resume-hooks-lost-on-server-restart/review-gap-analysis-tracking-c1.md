---
status: complete
created: 2026-04-02
cycle: 1
phase: Gap Analysis
topic: resume-hooks-lost-on-server-restart
---

# Review Tracking: resume-hooks-lost-on-server-restart - Gap Analysis

## Findings

### 1. SendKeys targeting with structural keys is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component Changes (Hook execution), Behavioral Requirements

**Details**:
The current `ExecuteHooks` calls `tmux.SendKeys(paneID, command)` where `paneID` is `%0`, `%3`, etc. The spec changes hook lookup to use structural keys (`session:window.pane`) but never states how `SendKeys` targets panes after the change. Two approaches exist: (1) use the structural key directly as the tmux `-t` target (tmux accepts `session:window.pane` format), or (2) resolve the structural key back to a pane ID at execution time via tmux query. The implementer needs to know which approach to use, as option 1 is simpler and aligns with tmux-resurrect's own targeting, while option 2 adds unnecessary complexity. This also affects the `MarkerName` function signature (does it take a structural key now?).

**Proposed Addition**:
Added "Design Decisions" section: SendKeys uses structural keys directly as tmux `-t` targets.

**Resolution**: Approved
**Notes**: Auto-approved. Resolved as option 1 — use structural keys directly.

---

### 2. Open design decision for pane querying method

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component Changes (Pane querying)

**Details**:
The spec states: "Either `ListPanes` must return richer data (window index, pane index per pane) or a new method is needed to query panes with their structural position." This leaves a design decision open for the implementer. A planner cannot create concrete tasks without knowing which approach to take. The spec should choose one approach and define the return type. Key considerations: `ListPanes` is used in `ExecuteHooks` to get session panes for matching, `ListAllPanes` is used in `CleanStale` for global pane enumeration. Both need structural key data. Additionally, `hooks set` and `hooks rm` need to query the *current* pane's structural position, which is a different operation (single pane, not a list).

**Proposed Addition**:
Added "Design Decisions" section: ListPanes/ListAllPanes change format strings to return structural keys. New method for current pane key resolution. Updated Component Changes to be definitive.

**Resolution**: Approved
**Notes**: Auto-approved. Resolved as format string change — signatures stay []string.

---

### 3. CleanStale parameter contract unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component Changes (Hook storage), Component Changes (Hook execution)

**Details**:
Currently `CleanStale(livePaneIDs []string)` takes a flat list of pane IDs. The spec says "CleanStale cross-references structural keys against live tmux structure instead of pane IDs" but doesn't specify the new parameter contract. Options: (1) caller builds structural keys and passes `[]string` of structural keys (signature unchanged, semantics changed), (2) `CleanStale` takes richer data and builds keys internally. Similarly, `ListAllPanes` currently returns `[]string` of pane IDs -- does it now return structural keys? This cascading change affects `ExecuteHooks`, `clean.go`, and the `AllPaneLister`/`HookCleaner` interfaces. The spec should clarify who is responsible for building structural keys from tmux output.

**Proposed Addition**:
Added "Design Decisions" section: CleanStale(liveKeys []string) — same signature, parameter renamed. Callers pass structural keys from ListAllPanes.

**Resolution**: Approved
**Notes**: Auto-approved. Resolved as option 1 — callers build keys, CleanStale just cross-references.

---

### 4. Hook registration: how to query current pane's structural position

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Component Changes (Hook registration)

**Details**:
The spec says "query tmux for the current pane's session name, window index, and pane index" during hook registration but doesn't specify the mechanism. Currently `hooks set` reads `$TMUX_PANE` (a single env var). The new approach needs to run a tmux command to get session name, window index, and pane index for the current pane. The spec should specify: (a) what tmux command to use (e.g., `tmux display-message -p -t $TMUX_PANE '#{session_name}:#{window_index}.#{pane_index}'`), or (b) what new `tmux.Client` method provides this (e.g., a `PaneStructuralKey(paneID string)` method). Without this, the implementer must research tmux format strings independently. Similarly for `hooks rm`.

**Proposed Addition**:
Added "Design Decisions" section: New tmux.Client method resolves current pane's structural key from $TMUX_PANE via tmux display-message.

**Resolution**: Approved
**Notes**: Auto-approved. Mechanism specified with tmux format string.

---

### 5. Executor interface changes not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component Changes (Hook execution)

**Details**:
`ExecuteHooks` depends on `TmuxOperator` (which embeds `PaneLister`, `KeySender`, `OptionChecker`, `AllPaneLister`) and `HookRepository` (which embeds `HookLoader`, `HookCleaner`). If `ListPanes` returns richer data, `PaneLister` interface changes. If `ListAllPanes` returns structural keys, `AllPaneLister` interface changes. If `SendKeys` now takes structural keys instead of pane IDs, `KeySender` interface changes. The spec doesn't address which interfaces change and how. Since all existing tests mock these interfaces, the test updates depend on knowing the new interface signatures. A planner needs this to estimate task scope and ordering.

**Proposed Addition**:
Added "Design Decisions" section: Interface signatures remain []string/string — values change semantically. No new interfaces needed. Mocks update to use structural key values.

**Resolution**: Approved
**Notes**: Auto-approved. Dependencies on findings 2 and 3 resolved — all use same approach (semantic change, not signature change).

---

### 6. Volatile marker naming with special characters

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component Changes (Volatile markers)

**Details**:
The spec proposes changing marker names from `@portal-active-%0` to `@portal-active-session:window.pane` (e.g., `@portal-active-my-project-abc:0.0`). Tmux user options (prefixed with `@`) do accept colons and dots in their names, so this should work. However, the spec uses hedging language ("e.g.") rather than defining the exact format. It should either confirm this exact naming convention or define an alternative encoding (e.g., replacing `:` and `.` with `-`). The `MarkerName` function is the single source of truth for this naming, so the spec should state the definitive format.

**Proposed Addition**:

**Proposed Addition**:
Added "Design Decisions" section: Definitive format is `@portal-active-{structural_key}`. Component Changes updated to remove hedging language.

**Resolution**: Approved
**Notes**: Auto-approved. Format confirmed — tmux user options accept colons and dots.
