---
status: in-progress
created: 2026-04-21
cycle: 3
phase: Gap Analysis
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Gap Analysis

## Findings

### 1. `portal state hydrate` CLI signature omits required `--hook-key` flag

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save Format & Schema — Canonical paneKey / Helper hook lookup under index drift (line 354); Restore-Side Architecture — Skeleton-Eager (line 632); Scrollback Restore Mechanics — Injection Path (line 732), Helper Behavior on Startup (line 774); Bootstrap Flow step 5.2 (line 1017); CLI Surface — `portal state hydrate` (line 1251).

**Details**:
Line 354 is load-bearing: it mandates that the helper be invoked with `--hook-key "<raw-session>:<saved-window>.<saved-pane>"` so the helper can look up hooks in `hooks.json` by the saved structural identity, preserving hooks across `base-index` / `pane-base-index` drift between save and restore. This was the cycle-1/cycle-2 resolution to index-drift correctness.

Every other place in the spec that shows the helper's command line omits the flag:
- Line 632: `portal state hydrate --fifo <F> --file <S>`
- Line 732: `sh -c 'portal state hydrate --fifo <FIFO> --file <SCROLLBACK>; exec $SHELL'`
- Line 774 (pseudocode header): `portal state hydrate --fifo F --file S:`
- Line 1017 (Bootstrap Flow, the most planner-facing spot): `sh -c 'portal state hydrate --fifo <F> --file <scrollback>; exec $SHELL'`
- Line 1251 (CLI Surface, the canonical command reference): `portal state hydrate --fifo F --file S` — no mention of `--hook-key`.

A planner breaking this into tasks from the CLI Surface section or Bootstrap Flow would not know the flag exists. The helper pseudocode in Section "Helper Behavior on Startup" (step f: "look up this pane's resume hook by structural key") is then ambiguous: is the structural key derived from environment (live position) or from the `--hook-key` argument? Line 354 resolves it, but only inside the Save Format & Schema section — not where the command is defined.

This is a genuinely load-bearing inconsistency: hooks will be silently lost on any `base-index` / `pane-base-index` change if the helper is implemented per the CLI Surface signature instead of per line 354.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Skeleton marker set/unset commands are inconsistent and the unset form does not actually remove the option

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Scrollback Restore Mechanics — Helper Behavior on Startup (lines 784, 797); Bootstrap Flow step 5.6 (line 1021); Marker Coordination — `@portal-skeleton-<paneKey>` (lines 672–683).

**Details**:
Two related problems in how the `@portal-skeleton-<paneKey>` marker is manipulated:

(a) **Scope inconsistency.** Marker Coordination (line 672) states the marker is at server-option scope, and the daemon enumerates via `show-options -sv` (line 679). Helper pseudocode unsets via `tmux set-option -s @portal-skeleton-<paneKey> ""` (lines 784, 797) — `-s` is the server-scope flag, consistent. But Bootstrap Flow step 5.6 (line 1021) **sets** the marker via `set-option @portal-skeleton-<paneKey> 1` with no scope flag — defaults to session scope. A planner following the Bootstrap Flow literally would set session-scope options; the daemon's `show-options -sv` enumeration would then return nothing and all skeleton-restored panes would be captured (and overwritten) on the next tick before the helper ever runs. This contradicts the entire lazy-hydration mechanism.

(b) **Unset is a no-op-ish on the enumeration path.** The helper unsets with `tmux set-option -s @portal-skeleton-<paneKey> ""` (line 784). In tmux, setting a user option to an empty string does **not** remove the option — it remains present in `show-options -sv` output with an empty value. The daemon's enumeration (line 679) is described as filtering "keys prefixed with `@portal-skeleton-`" — if it filters by presence of the key, empty-value markers still match, and the pane continues to be skipped after hydration completes. To actually remove the user option, the helper must use `set-option -su @portal-skeleton-<paneKey>` (unset flag). If the daemon instead filters on non-empty value, line 679 should state that explicitly so the enumeration contract matches the unset convention.

An implementer must decide: does "marker cleared" mean (i) user option removed via `-u`, (ii) user option present with empty value and enumeration filters on value-non-empty, or (iii) some other convention? Without clarification, the save loop can either permanently skip hydrated panes (never capturing real scrollback post-hydration) or permanently capture skeleton-marked panes (clobbering saved scrollback before the helper runs).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
