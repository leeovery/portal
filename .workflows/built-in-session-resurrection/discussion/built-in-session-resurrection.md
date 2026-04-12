# Discussion: Built-in Session Resurrection

## Context

Portal should own the full session lifecycle: server start → session restoration → resume hook execution. Currently the middle step depends on tmux-resurrect/continuum, which has a 100% failure rate — sessions never come back after reboot. The resume hook feature is effectively broken end-to-end despite the code being correct, because the session structure it depends on doesn't exist.

Research has confirmed full technical feasibility. tmux provides all the APIs needed for capture (`list-panes -a -F`) and restore (`new-session`, `split-window`, `select-layout`). The question is no longer *can we do this* but *how should we design it*.

Key design principles established in research:
- Portal's hook system is generic — no awareness of what consumers do with it
- Portal doesn't maintain a separate session registry — reads tmux directly
- Portal captures all sessions (Portal-created and native tmux), consistent with existing behavior
- Portal is always the entry point — bootstrap is the natural place for restoration

### References

- [Research: Built-in Session Resurrection](./../research/built-in-session-resurrection.md)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Hook Lifecycle Redesign [decided]
  ├─ One-shot vs persistent hooks [decided]
  └─ Per-hook configurability [decided]

  Save Content & Scope [decided]
  ├─ Structural state capture [decided]
  ├─ Scrollback / pane contents capture [decided]
  ├─ Ephemeral interaction state exclusion [decided]
  ├─ History size policy (no artificial caps) [decided]
  └─ Security / file permissions [decided]

  Save-Side Architecture [pending]
  ├─ Execution model (daemon vs subprocess-per-event) [pending]
  ├─ Trigger mechanism (which tmux events to hook) [pending]
  ├─ Crash safety / periodic save cadence [pending]
  ├─ Debouncing / serialization strategy [pending]
  ├─ Save format and schema [pending]
  ├─ save-state CLI surface and contract [pending]
  └─ tmux hook registration lifecycle (install/uninstall/upgrade) [pending]

  Restore-Side Architecture [pending]
  ├─ Bootstrap integration [pending]
  ├─ Fate of WaitForSessions / bootstrapWait [pending]
  ├─ Shell readiness detection [pending]
  └─ Layout restoration approach [pending]

  Failure Modes & Recovery [pending]
  ├─ Corrupt / partial saved state [pending]
  ├─ Missing working directories on restore [pending]
  ├─ Layout fit failures (terminal size drift) [pending]
  └─ User feedback on partial restore [pending]

  Observability & Diagnostics [pending]
  ├─ Save-state introspection command [pending]
  ├─ Logging strategy [pending]
  └─ Health signals for silent failures [pending]

  CleanStale Guard Behavior [pending]
  ├─ Guard rationale change post-restoration [pending]
  └─ Stale-hook detection criteria (binary/dir/project missing) [pending]

  Session & Project Store Interaction [pending]
  ├─ Restored session naming [pending]
  └─ projects.json timestamp handling [pending]

  Ephemeral Session Opt-Out [pending]

  Scope Boundaries [pending]
  ├─ Environment / shell state (explicit non-goal) [pending]
  └─ tmux version compatibility [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Hook Lifecycle Redesign

### Context

The resume hook system fires stored commands when a user enters a session. Research flagged "one-shot vs persistent" as a design decision — do hooks fire once and delete themselves, or persist across reboots until explicitly removed?

Current implementation is effectively persistent: entries live in `hooks.json` and survive reboots; volatile markers (`@portal-active-<pane>`) prevent duplicate runs within a tmux server lifetime. The research proposed making this configurable per-hook.

### Options Considered

**A: Add a `mode` field — configurable per-hook (`once` vs `always`)**
- Pros: Expressive; user declares intent; `once` auto-cleans after firing so broken hooks fail only once instead of every reboot.
- Cons: Adds API surface, CLI flag, storage field, docs, test matrix. Two more states for users to reason about.

**B: Single behavior — persistent only (status quo)**
- Pros: Minimal mechanism. Matches Portal's generic-hook principle. No new fields.
- Cons: Callers wanting one-shot semantics must implement it themselves at the command level.

### Journey

Initial framing was that `once` and `always` solve different use cases — `once` for dynamic commands like `claude --resume <uuid>` (where a wrapper re-registers on each start), `always` for static commands like `npm start`. Proposed adding a mode field, with the semantic anchor being "survives reboot yes/no."

First clarification: "session alive" meant the *logical* session (same named session across reboots), not the tmux session instance. Detach/reattach within a server lifetime was raised as an edge case but is a non-issue — existing volatile markers handle it correctly because processes are still running and nothing needs restarting. The only scenario where `once` and `always` would behave differently is reboot recovery.

Naming settled early: `once` / `always` — maps cleanly to `--mode=once` CLI flag, matches user mental model ("run this once" vs "always run this when I come back").

Then the user pushed back with a use case audit. For their Claude setup, *both* modes work — the Claude wrapper re-registers a `once` hook on resume, AND a separate exit hook removes the `always` version on explicit Claude exit. That prompted the pivotal question: if both modes work for the flagship use case, what is `once` actually for?

Audit of use cases:
- **Static dev commands** (`npm start`, `tail -f`, file watchers): `always` only; `once` makes no sense.
- **Claude resume** (dynamic UUID): both work.
- **Ephemeral one-time tasks**: `once` slightly cleaner, `always` + manual removal works.
- **Stale hook hygiene** (broken hook fails once vs every reboot): minor win for `once`.

No slam-dunk use case for `once`. The decisive argument came from re-reading the generic-hook design principle from research:

> Portal's hook system is generic. No awareness of what consumers do with it. Portal stores and fires a command string — it's the caller's responsibility to make that command correct.

One-shot vs persistent is *policy*. Portal provides the *mechanism*. If a caller wants one-shot behavior, they implement it at the command level — not inside Portal.

### False path: `&&` chaining

An initial framing proposed that one-shot callers could self-remove via shell chaining:

```
portal hooks set --on-resume "my-cmd && portal hooks rm --on-resume"
```

**This doesn't work for the flagship use case.** The canonical hook commands are long-running processes — `claude --resume <uuid>`, `npm start`, `tail -f`. These never exit, so the `&&` clause never fires, and the hook never removes itself. The proposed pattern was architecturally broken for the exact class of commands hooks exist to serve.

Verified against the codebase: the actual CLI is `portal hooks set --on-resume "..."` and `portal hooks rm --on-resume`, both inferring the current pane from `TMUX_PANE` and keying hooks by structural key (`session:window.pane`). The API shape is fine; shell chaining is not.

### The actual caller pattern: wrapper-script lifecycle management

The correct model — and the one the user already described from their Claude setup — is that long-running processes are invoked by a wrapper script which *owns* the hook lifecycle:

- Wrapper registers a Portal hook when the process starts (using current state, e.g., resume UUID)
- Wrapper re-registers on each resume if the hook command is dynamic
- Wrapper removes the hook on explicit process exit (via exit trap or explicit teardown)

Portal is never involved in deciding when to remove; it just exposes `set`/`rm` primitives that the wrapper calls at the appropriate lifecycle moments. This keeps Portal fully generic while giving callers precise control.

### Decision

**Do not add a `mode` field.** Portal keeps its single behavior: hooks persist in the store across reboots until explicitly removed via `portal hooks rm`. Callers that want one-shot or bounded-lifetime semantics manage it from a wrapper script around the target process — using set/rm as primitives at start/exit points.

**Trade-off accepted**: callers of long-running processes shoulder the responsibility of wiring up wrapper-script hook management. This is consistent with the rest of Portal's hook design — callers already own the command string entirely, and wrapping a long-running process is standard operational practice.

**Confidence**: high. YAGNI-compliant; a mode field can be added later if a concrete use case emerges where wrapper-script management is genuinely impractical.

**False paths documented**:
1. *"One-shot vs persistent as two viable models"* (original research framing) — overstated the design space. `always` (current behavior) handles every real use case with caller-side wrapping.
2. *"`&&` chaining for self-removal"* — architecturally broken for long-running processes, the exact class of commands hooks serve.

---

## Save Content & Scope

### Context

Before any save-side architecture decisions (when to save, how to write, by what mechanism), we need to decide *what* gets saved. The initial discussion had been progressing toward plumbing (daemon vs subprocess, debouncing strategy) without first nailing down the content profile — a gap the user caught by asking whether scrollback was in scope.

The answer reframes the whole save-side discussion. "Structural resurrection" (sessions + windows + panes + layouts + cwds) is functional but hollow. Every pane comes back empty; history continuity is lost. Zellij's session persistence captures *pane contents by default* and is consistently cited as one of its best features. If Portal is going to own the full lifecycle, it has to at least match that standard — otherwise the feature name is aspirational and users who know Zellij will rightly feel shortchanged.

### Journey

Initial framing implicitly excluded scrollback. I was deep in architectural plumbing and never stopped to enumerate content. User corrected with an unambiguous product directive: *"Scrollback 100% MUST be captured. This is useless without it. I want Zellij but in tmux!! Whatever we can save we should."*

That directive became the organizing principle: **capture everything that persists as meaningful state, exclude only ephemeral interaction state, accept the uncapturable as out of scope.**

**Main screen vs alternate screen — a phantom problem resolved**: A follow-up tangent worried that `capture-pane -p -S -` returns "stale" content for panes running `vim`, `htop`, `less`, `man`, etc. — because those programs use tmux's *alternate screen buffer*, not the main screen. Initial (wrong) concern: "a pane in vim for 3 hours returns empty/stale scrollback because vim's content isn't captured."

Resolution: tmux distinguishes two separate buffers per pane:
- **Main screen buffer** — the normal terminal output that scrolls. This *is* scrollback. `capture-pane -p -S -` captures this.
- **Alternate screen buffer** — what alt-screen programs draw into. It temporarily replaces the visible area while the program runs, then disappears when the program exits and the main screen becomes visible again. It is *not* part of scrollback.

So the capture is correct: the main screen buffer *is* the real shell history, just temporarily hidden by the alt-screen overlay. A pane that's been in vim for 3 hours still has the actual scrollback (everything up to and including `vim main.go`) in its main screen buffer — and that's what gets captured and restored. There is no "stale content" — there is the scrollback as it exists.

**Items removed from inventory post-review:**
- **Marks** (`<prefix>m`) — initially listed as "position markers." In reality, tmux's `<prefix>m` sets a *pane-level* marked state (used by `swap-pane -m`, one pane at a time across the server) — not a scrollback position bookmark. The useful thing (copy-mode position marks) has no tmux API to capture or restore. Neither version justifies the complexity. Removed.
- **"Deviating session options"** — initially listed as "session names and deviating session options." On inspection, nearly all tmux options are set globally via `~/.tmux.conf` and apply on restore automatically. Per-session/per-window overrides (e.g., `synchronize-panes`, `monitor-activity`) are niche. Capturing them generically requires diffing `show-options` against global defaults — complexity not worth it. Also carried a recursion risk if Portal's own `set-hook -g` definitions were captured. Dropped generic options capture entirely. If a specific flag (like `synchronize-panes`) is missed, it can be added as an explicit per-window boolean later. YAGNI.
- **Last-pane tracking** — no confirmed tmux format variable exposes "which pane is 'last' for this window." To verify during implementation; dropped from the guaranteed inventory for now.

**Implication: no special handling for alt-screen panes.** Portal captures scrollback. Programs like vim are *not* scrollback. If a user wants vim auto-relaunched on restore, they register a hook — same as Claude, same as any other process. Portal doesn't guess, doesn't infer, doesn't try to capture alt-screen contents. The user's framing: *"If I was to start something that overtook the window, like a special command like vim, I wouldn't expect you to capture that because it's outside of the scrollback."* Correct.

### Options Considered

**A: Structural-only** (original implicit framing)
- Pros: Smallest save files, fastest, simplest security story.
- Cons: Panes come back empty. No history continuity. "Resurrection" in name only. Zellij users would scoff.

**B: Structural + scrollback, opt-in** (resurrect's model)
- Pros: Safety-conscious default, users opt in if they want it.
- Cons: Most users don't opt in and never experience the full benefit. The feature exists but doesn't feel right out of the box. Fails the "Zellij in tmux" product goal.

**C: Everything capturable, on by default, ephemeral excluded** (user's directive)
- Pros: Resurrection actually feels like resurrection. Matches Zellij UX standard. Simple mental model ("whatever was there, comes back").
- Cons: Larger save files, more data on disk, security consideration for sensitive output.

### Content Inventory

**IN SCOPE** (captured on save, restored on resurrection):

*Structural:*
- Session names
- Window indices, names, layout strings, active/zoom flags
- Pane indices, current working directories, active flag

*Content:*
- Full pane scrollback with ANSI escape sequences — colors, attributes, formatting preserved via `tmux capture-pane -e -p -S - -t <pane>`
- tmux per-session environment via `show-environment -t <session>` (the tmux-level env used for initializing new panes, not live shell env). Restored in full without filtering — tmux's own `update-environment` mechanism automatically refreshes stale values (`SSH_AUTH_SOCK`, `DISPLAY`, etc.) from the attaching client's env on session attach. No Portal-side filtering needed.

*Already stored:*
- Resume hooks (already in `hooks.json`, not new)

**OUT OF SCOPE — explicitly ephemeral:**
- Copy mode state
- Active selections
- Paste buffers
- Cursor position within panes
- Scroll position within scrollback
- Per-client state (which client has which pane focused, client-specific dimensions)

**OUT OF SCOPE — uncapturable by tmux** (research-confirmed, not Portal's problem to solve):
- Live shell environment variables — tmux can't observe shell-side `export`. Callers can compensate via resume hooks if they care.
- Running process state (REPL state, interactive sessions) — hence the resume hook system exists at all
- Open file descriptors, pipes, sockets, ptrace state, etc.

### Decision

**Capture everything tmux exposes that persists as meaningful state. On by default. No opt-in.**

- Scrollback capture is non-negotiable and always on
- History size: no artificial Portal cap — save whatever tmux has in the history buffer (respects user's `history-limit`). A cap can be added later if storage becomes a real issue. YAGNI.
- Security: saved state lives in `~/.config/portal/` alongside existing config, with `0600` permissions on any file containing scrollback. Same local-filesystem trust model as shell history (`~/.bash_history`, `~/.zsh_history`). No encryption at rest — overkill, adds key management complexity, matches neither resurrect nor Zellij.
- Per-session opt-out for sensitive sessions is handled separately under the Ephemeral Session Opt-Out subtopic — that gives users a safety valve without compromising the default experience.

### Capture feasibility (tmux APIs)

What tmux actually exposes for each item on the in-scope list:

**Verified against research / tmux docs:**

| Content | tmux mechanism |
|---|---|
| Session/window/pane structure | `list-panes -a -F` with format variables |
| Window layout strings | `#{window_layout}` (pre-zoom form, research-verified) |
| Pane working directory | `#{pane_current_path}` |
| Pane active / zoom state | `#{pane_active}`, `#{window_zoomed_flag}` |
| Pane current command (short name) | `#{pane_current_command}` (research-verified: short name only, no args — not a Portal problem) |
| Main-screen scrollback with ANSI | `capture-pane -e -p -S - -t <pane>` (research-verified) |
| tmux per-session environment | `show-environment -t <session>` (standard tmux) |
| Session/window/pane options | `show-options -s`, `show-options -w`, `show-options -p` |

**All items on the in-scope list are verified as capturable via standard tmux APIs.** Three soft-spot items (marks, deviating session options, last-pane tracking) were removed from the inventory during review — see Journey notes above.

### Impact on Save-Side Architecture (flagged, not decided here)

Saves are now content-heavy (scrollback per pane + structural), not lightweight JSON. Implications:

- Each save does N `capture-pane` calls + a JSON write — still fast (~ms per pane), but not negligible at burst frequency.
- Debouncing matters more — avoiding storms of large saves is valuable.
- Format probably wants per-pane scrollback files referenced from a main state JSON, rather than one giant state blob. Debuggable, selectively restorable, partial-corruption tolerant.
- These concerns feed into the upcoming Save-Side Architecture and Failure Modes subtopics — noted here, decided there.

**Confidence**: High. Product direction is unambiguous, tmux capture APIs are verified, architectural ripple effects are understood and manageable.

---

## Summary

### Key Insights
*(To be completed during discussion)*

### Open Threads
*(To be completed during discussion)*

### Current State
- Hook Lifecycle Redesign: **decided** — no mode field; single persistent behavior; one-shot is a caller-level policy via wrapper-script lifecycle management (not `&&` chaining)
- Save Content & Scope: **decided** — capture everything tmux exposes as meaningful state (structural + scrollback + env + marks), on by default, no opt-in. Target is "Zellij in tmux" resurrection quality. Ephemeral interaction state excluded.
- Remaining: Save-Side Architecture, Restore-Side Architecture, Failure Modes & Recovery, Observability & Diagnostics, CleanStale Guard Behavior, Session & Project Store Interaction, Ephemeral Session Opt-Out, Scope Boundaries
