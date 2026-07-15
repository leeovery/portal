# Discussion: CLI Verb Surface Redesign

## Context

Portal's CLI grew by accretion — commands were added as they were needed, without a holistic design pass. The symptom that surfaced this: even the author is now fuzzy on the difference between `open` and `attach`. When the person who built it can't cleanly recall which verb does what, the surface has drifted past coherent.

The current shape:

- `portal open` — no args launches the TUI picker; one arg resolves a path/query through path → alias → zoxide → session, then attaches in place
- `portal attach <session>` — attaches in place to a named session (also carries the internal `--spawn-ack` flag used by spawned windows)
- `x` — alias (for `open`)
- `portal spawn <sessions…>` — provisionally named; opens each session in its own host-terminal window (`--detect` dry-run)
- Utility commands: `hooks`, `clean`, `init`, `state`, `alias`, `version`

The core problem: overlapping, blurry verbs with illegible input domains (path/query vs single session name vs multi-session). Bolting `spawn` on in isolation just lengthens an organically-grown list without fixing the underlying incoherence.

The goal is one intentional, coherent design pass over the **full** command list (the user explicitly chose a full audit — `hooks`, `clean`, `state`, `alias`, `init` and friends included, not just the three overlapping verbs). The output is a ship-able design: rename/restructure commands plus a back-compat/deprecation story, since existing commands live in user muscle memory and scripts.

A live design question carried from the seed: should the window-spawn operation stay a distinct `spawn`, or fold into a variadic `attach foo bar baz` where argument count decides attach-in-place vs spawn-new-windows? The author likes variadic-attach (it matches the session-name input domain) but notes the count-dependent behaviour split.

### References

- Seed: `.workflows/cli-verb-surface-redesign/seeds/2026-07-09-cli-verb-surface-redesign.md`
- Discovery log: `.workflows/cli-verb-surface-redesign/discovery/sessions/session-001.md`
- Origin discussion: `restore-host-terminal-windows` (named `spawn` provisionally; CLI verb is a secondary surface, cheap to rename)

## Discussion Map

A living index of subtopics tracked during the discussion. This is the structural backbone — it grows as the conversation branches, and converges as decisions land.

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — CLI Verb Surface Redesign (6 subtopics: 6 pending)

  ┌─ ○ Mental model & verb taxonomy [pending]
  │  ├─ ○ open vs attach reconciliation [pending]
  │  ├─ ○ spawn: distinct verb vs variadic attach [pending]
  │  └─ ○ Where the picker sits [pending]
  ├─ ○ Input domain legibility (path/query vs session vs multi-session) [pending]
  ├─ ○ Utility command audit (hooks, clean, state, alias, init, version) [pending]
  └─ ○ Back-compat & deprecation story (aliases, muscle memory, scripts) [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Summary

### Key Insights

(none yet)

### Open Threads

(none yet)

### Current State

- Discussion initialized; no subtopics explored yet.

## Triage

(none)
