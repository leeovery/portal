---
status: complete
created: 2026-07-17
cycle: 4
phase: Gap Analysis
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Gap Analysis

## Findings

### 1. `doctor`'s "no stale entries" check + `doctor --fix` prune are undefined when the tmux server is down — an empty live-pane enumeration false-positives every hook as stale, and `--fix` would then prune valid hooks (breaking the "reversible-by-reconstruction" premise)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `doctor` — Diagnostics & Repair (check catalog "no stale entries (dead-pane hooks, gone-dir projects)"; `--fix` repair list; "Exit-code contract" — the down-server case); Bootstrap Exemption — `doctor` & `uninstall`

**Details**:
The spec deliberately makes `doctor` bootstrap-exempt and treats a **down server** as a first-class, valid state: it "observes raw state, starts nothing … a down server is reported honestly." The exit-code contract enumerates which checks fail on a down server as "daemon / saver / hooks checks fail." It never addresses what the **"no stale entries"** check does when the server is down — and that check has a hazardous failure mode the others don't.

Detecting a stale (dead-pane) hook means comparing each `hooks.json` entry's hook key against the set of **live pane hook keys**, which requires enumerating panes on a running tmux server (`tmux list-panes -a`, the `ListAllPaneHookKeys` path). With the server down, that enumeration yields the empty set, so **every** hook entry looks orphaned:

- In plain `portal doctor`, the "no stale entries" check would report all hooks as stale — a false alarm layered on top of the (correctly) non-zero exit from daemon-down.
- In `portal doctor --fix` — which runs its filesystem repairs (`prune stale hooks`, `prune stale projects`, `sweep logs`) and does **not** start a server (bootstrap-exempt) — the stale-hook prune would then **delete the user's entire `hooks.json` content**. This directly violates the spec's stated premise that `--fix` repairs are "low-stakes, **reversible-by-reconstruction**": a pruned user-authored on-resume command is *not* reconstructable by Portal.

This is a plausible real path (a user sees `doctor` reporting problems on a down/rebooted server and runs `--fix`), and it is distinct from the already-resolved cycle-1 down-server *exit-code* ruling (which only decided that a down server counts as unhealthy → non-zero, not how the stale-entries check/prune behaves). The stale-**project** prune is not affected — it tests directory existence on disk (`gone-dir projects`), which needs no server — so the hazard is specific to the dead-pane-hook prune's dependency on a live pane enumeration. An implementer has no spec guidance and must choose (guard/skip the stale-entries check when the server is down vs. run it against an empty live set), with a data-loss branch on the wrong choice.

**Proposed Addition**:
State that the "no stale entries" check — and the `--fix` stale-hook prune it drives — are **guarded against a down server**: when the tmux server is not running, dead-pane-hook staleness cannot be determined (there are no live panes to compare against), so the check is reported as *not-evaluable* / skipped rather than "all stale," and `--fix` performs **no** hook pruning in that state (the stale-project prune, being filesystem-only, may still run). This keeps the "reversible-by-reconstruction" guarantee intact and prevents `--fix` on a down/rebooted server from wiping valid `hooks.json` entries. (Confirms the safe behavior; the concrete probe stays planning's, per the catalog note.)

**Resolution**: Approved
**Notes**: Auto-approved — genuine data-loss hazard. Added a "down-server guard on the stale-hook prune" bullet: dead-pane-hook staleness is not-evaluable when the server is down, `--fix` does no hook pruning in that state (protects the reversible-by-reconstruction premise), stale-project prune (filesystem-only) may still run. Logged to spec.

---

### 2. Session-domain resolution (exact name, session glob, `-s`) does not state which session set it matches against — the user-visible/filtered set vs. raw tmux — leaving the load-bearing internal `_portal-saver` / `_portal-bootstrap` sessions reachable by `open`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Target resolution precedence" — exact session name; glob pre-check); `portal open` — Flags & Command Passthrough (`-s/--session`); Multi-Target Burst Mechanics ("Glob targets"); Tab Completion (`open` bare positional / `-s`)

**Details**:
Session-domain resolution is the core of the redesign (`open` absorbs `attach`; exact-name matching becomes part of `open`'s public grammar; session globs are new). The spec describes glob expansion as matching "against the finite set of **live session names**" and exact-name resolution as "exact session name → attach," but never states **which enumeration** defines that set: the user-visible, **leading-underscore-filtered** `ListSessions` set (which excludes the load-bearing `_portal-saver` and `_portal-bootstrap` sessions), or the raw `tmux list-sessions` / `HasSession` view (which includes them).

This is consequential because the two internal sessions are explicitly designed to be **invisible** to every user-facing surface (filtered from `ListSessions`, excluded from the picker and from `sessions.json` capture) — yet `open`'s new resolution grammar is silent on inheriting that filter, and the natural implementations diverge:

- **Exact name** is most naturally implemented with `HasSession(name)` (a raw tmux `has-session` check), which returns true for `_portal-saver` / `_portal-bootstrap`. So `open _portal-saver` would **attach the user's terminal to the daemon's saver session** (or the bootstrap anchor) — a footgun and a direct contradiction of their "invisible / never a user surface" contract.
- **Session globs** implemented against a fresh `tmux list-sessions -F '#{session_name}'` enumeration (unfiltered) would make `open '*'` (or a scripted `-s '*'`) match and try to attach the internal sessions; implemented against the filtered `ListSessions` they would not.

Tab completion ("complete session names") would naturally use the filtered `ListSessions`, so completion and resolution can silently disagree about the eligible session set unless the spec pins one answer. An implementer has no spec basis to choose, and the wrong choice attaches users to Portal's own plumbing.

**Proposed Addition**:
State that `open`'s session-domain resolution — exact-name match, session-glob expansion, and the `-s/--session` pin — operates only against the **user-visible session set** (the same leading-underscore-filtered `ListSessions` view used by the picker and completion), so the internal `_portal-saver` / `_portal-bootstrap` (and any future `_`-prefixed) sessions are never matchable as `open` targets. A bare/`-s` name or glob that would resolve only to a filtered internal session is treated as a miss (falls through / hard-fails) exactly as if the session did not exist.

**Resolution**: Approved
**Notes**: Auto-approved — footgun/contract-violation. Added a "Session set — user-visible only" note pinning exact-name / glob / `-s` resolution to the leading-underscore-filtered `ListSessions` view, so `_portal-saver` / `_portal-bootstrap` are never matchable; a name/glob resolving only to a filtered internal session is a miss. Logged to spec.

---
