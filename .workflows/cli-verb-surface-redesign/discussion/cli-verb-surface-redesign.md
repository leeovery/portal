# Discussion: CLI Verb Surface Redesign

## Context

Portal's CLI grew by accretion — commands were added as they were needed, without a holistic design pass. The symptom that surfaced this: even the author is now fuzzy on the difference between `open` and `attach`. When the person who built it can't cleanly recall which verb does what, the surface has drifted past coherent.

The current shape (verified against the codebase at discussion start — the seed's inventory was incomplete):

**Session verbs:**
- `portal open [-e cmd] [destination] [-- cmd args...]` — no args launches the TUI picker; one arg resolves a path/query through path → alias → zoxide → session, then attaches in place; can carry a command to run in the new session
- `portal attach <session>` — attaches in place to a named session (also carries the internal `--spawn-ack` flag used by spawned windows)
- `portal spawn [sessions...]` — provisionally named; opens each session in its own host-terminal window (`--detect` dry-run)
- `portal kill [name]` — kill a tmux session
- `portal list` — list running tmux sessions

**Utility commands:**
- `portal alias {set,rm,list}` — path aliases
- `portal hooks {set,rm,list}` — resume hooks
- `portal clean [--logs]` — remove stale projects / sweep logs
- `portal state {status,cleanup}` — user-facing; plus six **hidden** internal subcommands (`daemon`, `hydrate`, `signal-hydrate`, `notify`, `commit-now`, `migrate-rename`)
- `portal init [shell] [--cmd name]` — shell integration
- `portal version`, `portal completion` (cobra built-in)

**Shell layer (from `portal init`):** `x` is not a cobra alias — it's a shell function `x() { portal open "$@" }`, paired with `xctl() { portal "$@" }`. So a two-tier surface already exists: `x` = the launcher (hardwired to `open`), `xctl` = the full control plane. `--cmd` renames the pair.

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

  Discussion Map — CLI Verb Surface Redesign (10 subtopics — 5 decided · 2 exploring · 3 pending)

  ┌─ ✓ Mental model & verb taxonomy [decided]
  │  ├─ ✓ open vs attach reconciliation [decided]
  │  ├─ ✓ spawn: distinct verb vs variadic attach [decided]
  │  └─ ○ Where the picker sits [pending]
  ├─ ✓ Input domain legibility (universal target resolution) [decided]
  ├─ ◐ Verb naming (keep incumbents vs rename) [exploring]
  ├─ ◐ Verb B contract [exploring]
  │  ├─ ✓ Arg resolution (universal, atomic pre-flight, create-on-miss) [decided]
  │  ├─ ○ Absorb vs stay-put [pending]
  │  └─ ○ --detect home [pending]
  ├─ ○ Utility command audit (kill, list, hooks, clean, state, alias, init) [pending]
  └─ ○ Back-compat & deprecation story (aliases, muscle memory, scripts) [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Mental Model & Verb Taxonomy

### Context

The parent decision everything else hangs off: what's the sentence a user says to themselves to pick a verb? Today's surface muddles this — `open` and `attach` produce the identical outcome (your terminal ends up in a session) and differ only by what the argument looks like (path/query vs session name), while `spawn` differs by outcome (new host windows).

### Options Considered

Three candidate mental models were put on the table first:

**A. One-verb funnel** — a single primary verb accepts anything (nothing → picker, path, query, session name, maybe several); everything else is management. Maximum muscle-memory simplicity; input domains unified by resolution.
- Cons: quietly re-creates the blur *inside* one verb's resolution chain; can't express "open a window over there without moving me".

**B. Two verbs by input domain** — "where"-shaped input → `open` (resolving, create-or-attach); "which"-shaped input → `attach` (exact, existing-only, variadic — spawn folds in).
- Pros: domains legible by verb.
- Cons: verb choice encodes *how you'll spell the destination*, not intent — both verbs produce the same outcome. (This is the split the user called weird, and it is: no beloved CLI splits verbs by argument type — `cd` has no sibling `cd-by-inode`.)

**C. Three verbs by operation (status quo + docs)** — keep open/attach/spawn, fix with naming/docs only.
- Cons: preserves exactly the blur that motivated the work; even the author can't recall open-vs-attach.

The discussion then reframed the space as **three possible axes to split verbs on** — by input type (B), by action/outcome, by determinism (resolving vs exact) — observing that today's surface muddles all three.

### Journey

- Started from the input-domain framing (the seed's): "open guesses, attach doesn't" — attach's value is precision for callers that must not be guessed at (the spawn burst composes `portal attach <session> --spawn-ack` for every spawned window; scripts want determinism). User confirmed they *never* type `attach` manually (session names are `{project}-{nanoid}` — not hand-typed currency) but felt it has value. That located attach's constituency: machines and deliberate exactness, not fingers.
- User pushed back on the input-domain split itself: "attach is for session names and open is for everything else… feels weird. we need a better split." The reframe landed: split the *human* surface **by action/outcome**, and demote exactness to **plumbing** (porcelain/plumbing precedent already exists — six of `state`'s subcommands are hidden).
- The variadic-attach idea from the seed (fold spawn into `attach foo bar baz`) dissolved rather than being decided against: once attach leaves the public surface, there's nothing public to fold spawn into. Notably, the seed's objection to variadic attach (count-dependent behaviour split) had already been weakened: the picker multi-select's Enter is a *continuous* rule — "net N surfaces, your terminal is one of them" — where N=1 is just the degenerate case with zero external windows. The fold died for a different reason than the seed's worry.
- Why two public verbs and not one count-driven verb: a single count-driven verb cannot express *"open a window for that session but leave my terminal alone"* (N=1 always attaches in place). Two outcome-verbs make every combination typeable.
- Whether window-opening deserves to be public *at all* was challenged (the picker calls the spawn package in-process; the CLI verb is a test seam + provisional). The deciding argument for public: **scriptability** — the morning-after-reboot script that rebuilds a standard window layout without the picker; a hidden verb is one nobody discovers. User confirmed: multi-opening stays public.

### Decision

**Split the public surface by outcome; demote exactness to plumbing; input domains unify inside resolution.**

- **Verb A ("take me there")** — picker at no-args; universal target resolution (exact session match → path → alias → zoxide); connects the current terminal in place; creates when the target is directory-shaped and no session exists.
- **Verb B ("open windows for these")** — variadic, public (scriptability argument), currently `spawn`.
- **`attach` leaves the public surface but cannot be deleted** — it is the exec target of every spawned window (`--spawn-ack`) and the exact/no-guessing command for scripts. It demotes to hidden-but-working plumbing, same trajectory as `state daemon`. Functionally nothing is lost: verb A accepts session names, so every current `attach` invocation has a verb-A equivalent.

Names for both verbs deliberately deferred to the Verb Naming subtopic. Confidence: high on the split; the contracts' fine print (verb B arg resolution, absorb-vs-stay-put) tracked separately.

---

## Input Domain Legibility

### Context

The seed framed illegible input domains (path/query vs single session name vs multi-session) as a problem to make *legible by verb*. The mental-model decision inverted this.

### Decision

Input domains are **not** made legible by verb — they are **unified inside resolution**. Any target argument to a public verb accepts a session name, path, alias, or zoxide query, resolved with a precedence order: **exact session match → path → alias → zoxide**. Collisions between a session name and a directory name are rare (`{project}-{nanoid}` names don't look like paths) and precedence-resolvable. Exactness (no-guessing) remains available in plumbing (`attach`) for scripts and the spawn machinery. Folded from the parent decision — no separate debate.

---

## Verb Naming

### Context

With contracts settled (verb A "take me there", verb B "open windows for these"), are the incumbent names right? `open` is the incumbent for A — but in mac/desktop convention `open` connotes *a new window* (verb B's job), and `open` is the name whose contract changes most (gains session names). `spawn` is the incumbent for B — accurate but jargon-y, describes the mechanism (create windows) not the outcome, and was explicitly shipped as provisional. Candidates floated: A — keep `open`, or `go`, `enter`, `jump`; B — keep `spawn`, or `windows`, `launch`, `pop`.

Key facts bearing on the decision:
- `x` is a shell function emitted by `portal init` (`x() { portal open "$@" }`) — it remaps to whatever verb A becomes with zero muscle-memory cost. `xctl() { portal "$@" }` is the control-plane twin.
- `portal open` / `portal attach` / `portal spawn` exist in scripts and muscle memory today → back-compat aliases required either way.

### Journey

User: "i dont know tbh" — genuine fork. Perspective agents dispatched (Formal Systems ↔ Incentive Realist: what the coherent model demands vs how users actually behave). Awaiting synthesis.

### Decision

(pending)

---

## Verb B Contract

### Context

Verb B (currently `spawn`) inherited a contract designed for the picker burst: exact session names only, `has-session` pre-flight with atomic abort, spawn N−1 external windows then self-attach the trigger terminal to the Nth (net N, never N+1), `--detect` diagnostic riding on the command. Going public-by-design (the scriptability decision) raises which parts of that contract should change.

### Arg Resolution (decided)

**Verb B's args get the same universal resolution as verb A** — exact session match → path → alias → zoxide, including create-on-miss for directory-shaped targets with no session. User's framing: "same precedence/ordering — any reason not to?" Two consequences examined and accepted:

- **The all-or-nothing gate survives intact.** Resolution is read-only, so all N args resolve atomically *before* anything is created or opened; "any target unresolvable ⇒ nothing opens" replaces the `has-session` string check. Same abort semantics, better inputs.
- **Guess-risk enters bursts.** A typo that zoxide-matches an unrelated dir opens a wrong window (and creates a session for it). Recoverable via `kill`; identical risk profile to verb A today — no new failure class.

Rationale for create-on-miss: the morning-after-reboot script (`portal <B> api blog infra`) shouldn't care whether restore already made the sessions; "exact names only" was another instance of the input-domain split the mental-model decision killed. Post-resolution partial-failure semantics stay leave-what-opened, as today.

### Open

- **Absorb vs stay-put** — today the trigger terminal always becomes one of the N (net-N rule, inherited from the picker where the leftover picker window would be junk). From a shell, "open windows but leave me here" is inexpressible; that headless mode was deferred (not rejected) in restore-host-terminal-windows.
- **`--detect` home** — a diagnostic dry-run currently riding on the spawn command.

---

## Summary

### Key Insights

1. Verb splits feel right when they split by *what happens*, not by what the argument looks like — the open/attach blur was an input-type split masquerading as a verb pair.
2. The porcelain/plumbing distinction Portal already uses for `state` internals generalizes: exact/no-guessing commands (`attach`) serve machines and scripts and can be hidden without losing function.
3. The picker multi-select's "net N surfaces, your terminal is one of them" rule is continuous in N — the seed's count-dependent-split worry about variadic verbs was a framing artifact.

### Open Threads

- Verb naming — perspectives in flight.
- Verb B contract fine print: universal resolution for its args; absorb-the-terminal (today's net-N) vs stay-put; where `--detect` lives.
- Where the picker sits (verb A no-args vs bare `portal`).
- Utility command audit; back-compat/deprecation story.

### Current State

- Decided: outcome-split mental model, attach → plumbing, spawn-op stays public, universal target resolution.
- Exploring: verb naming (perspective agents dispatched).

## Triage

(none)
