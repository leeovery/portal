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

  Discussion Map — CLI Verb Surface Redesign (12 subtopics — 8 decided · 1 exploring · 3 pending)

  ┌─ ✓ Mental model & verb taxonomy [decided]
  │  ├─ ✓ open vs attach reconciliation [decided]
  │  ├─ ✓ spawn: distinct verb vs variadic attach [decided → superseded by the fold]
  │  └─ ✓ Where the picker sits (open, no args) [decided]
  ├─ ✓ Input domain legibility (universal target resolution) [decided]
  ├─ ✓ Verb naming (open stays — portal metaphor; verb B name dissolved) [decided]
  ├─ ✓ The open fold (spawn absorbed; absorb/net-N as rule) [decided]
  │  ├─ ✓ Arg resolution (universal, atomic pre-flight) [decided]
  │  ├─ ✓ Attach-vs-mint dichotomy (session=attach, dir=always new) [decided]
  │  ├─ ✓ Miss handling (fallback stripped — hard fail; -f explicit) [decided]
  │  ├─ ✓ Domain-pinning flags (-s/-p/-z/-a/-f) [decided]
  │  ├─ ✓ Glob targets (session-domain; shell globs paths; -a globs) [decided]
  │  └─ ○ --detect home [pending]
  ├─ ✓ attach disposition (retired — open --session + hidden --ack) [decided]
  ├─ ○ Utility command audit (kill, list, hooks, clean, state, alias, init) [pending]
  └─ ✓ Back-compat & deprecation story (none — deliberate reversal of the seed) [decided]

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

> **Superseded in part**: the two-public-verbs shape was later revised by **The Open Fold** (below) — verb B was absorbed into `open` once the absorb/net-N rule was recognized as making arity continuous. The underlying decisions survive: outcome-thinking, attach → plumbing, multi-open stays public (now as variadic `open`), universal resolution.

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

User: "i dont know tbh" — genuine fork. Perspective agents dispatched (Formal Systems ↔ Incentive Realist: what the coherent model demands vs how users actually behave). Synthesis surfaced four tensions:

- **T1 (the core split)** — Formal Systems: rename A → `go` (mac `open` connotes new-window; `open` is the verb whose contract changes most — keeping the name while moving behaviour is silent drift). Incentive Realist: keep `open` (`x` absorbs nearly all invocations; the *reported* confusion is cured by demoting attach, so the rename chases an unreported problem; `go` collides with the Go toolchain in this author's world).
- **T2** — verb B replacement: `windows` (guessable noun) vs `launch` (grammar-consistent verb); both lenses converged on retiring `spawn` (mechanism jargon, provisional, no muscle memory — only consumers are author-owned tests).
- **T3** — names are a coupled set: `open`+`launch` is the one forbidden pair (near-synonyms for opposite poles).
- **T4** — alias lifecycle: permanent silent aliases vs deprecated-with-sunset. (Still live; feeds the back-compat subtopic.)

**Resolution of T1**: the user rejected the Realist's *frame* outright — "I don't care about the impacts of the rename; if the rename is the right thing to do, we do it. That's the whole point of this task." Migration cost is not a criterion here. `open` stays **on semantic merits**: "Portal open" is the tool's founding metaphor — you are *opening a portal to a session* (the play on words is why the tool is named Portal). The parameter doesn't change what you're doing, only how the destination is derived. Under the portal metaphor the mac new-window connotation dissolves — `open` never meant "window", it means the portal. `x` is acknowledged as a personal convenience, not the design.

**T2/T3 dissolved** rather than resolved: The Open Fold (below) eliminates verb B entirely — there is no second verb to name. `spawn` the *word* is retired with it.

### Decision

**Verb A is `open`, decided on semantic-fit grounds (portal metaphor), explicitly not on migration-cost grounds.** No second verb exists post-fold, so no other public session-verb name is needed. Confidence: high.

---

## The Open Fold (spawn absorbed into open)

### Context

With naming under discussion, the user pushed the shape further: "It's doing nothing different from the user's perspective as Portal open, except you're opening one or more… I feel like it's all just Portal open." Re-examining the earlier one-verb objection showed it was conditional — and the condition is settleable.

### Journey

- The earlier case for two verbs rested on one inexpressible sentence: "open a window for that session but leave my terminal alone" — a count-driven single verb can't say it. The fold survives this because of the **absorb/net-N rule**: "open portals to these N sessions; this terminal is one of them" is *continuous in N* — at N=1 your terminal is the only surface (plain open-here), at N=3 it's one of three. No behaviour cliff. Today's `spawn` already self-attaches the Nth, so the semantics are proven in production.
- The cliff only appears if multi-open *stays put* while single-open connects — so **choosing the fold is choosing absorb-as-rule**. Stay-put isn't lost: it becomes an explicit future flag on `open` (`--detach`-ish) — the exceptional behaviour gets the flag, the natural behaviour gets the bare verb.
- The user proposed **domain-pinning flags**: `--session` / `--path` on `open` pin the input domain instead of guessing — "wouldn't really be used by humans, but it is something that's scriptable." This relocates the script-determinism role from hidden `attach` to a **public, documented** home on `open`.
- Consequence: the entire verb-B naming debate (T2/T3) evaporates — there is no verb B.

### Decision

**`portal open` is the single public session verb.**

- `portal open` → picker
- `portal open <target>` → resolve (session → path → alias → zoxide), connect this terminal
- `portal open <t1> <t2> … <tN>` → N portals; this terminal becomes one of them (absorb/net-N rule), N−1 host windows spawn
- Domain-pinning flags (see below) → no guessing; script-safe
- Atomic resolution pre-flight for multi-target: any target unresolvable ⇒ nothing opens
- Stay-put multi-open: future explicit flag, deliberately deferred scope (not designed here)
- `spawn` retired as a public verb; `kill`, `list`, utilities unchanged by this decision
- Picker placement (formerly its own open question) is settled by the same sentence: no parameters → the picker is how you choose the destination

Confidence: high on the shape.

### The Attach-vs-Mint Dichotomy (second axiom)

Code verification (`internal/session/quickstart.go:72-76`, `create.go`) corrected an earlier mis-statement in this doc ("creates when the target is directory-shaped and no session exists" — implied find-or-create). The real model, confirmed and **locked as the design's second axiom**:

- **Session-domain hit** (exact name, glob) → **attach to an existing session**
- **Directory-domain hit** (path, alias, zoxide) → **mint a brand-new session there, always**

There is no find-or-create: `GenerateSessionName` guarantees a fresh `{project}-{nanoid}` name and the former `new-session -A` attach-to-existing was deliberately removed as unreachable. Multiple sessions per project is the designed workflow. The precedence chain is therefore semantic, not just disambiguation: "is this an existing session I should surface, or a place I should open a new portal to?"

**Accepted consequence (ruled on explicitly):** bare project shorthand does *not* reattach — `open api` never exactly-matches `api-x7Kd9a`, falls to zoxide → dir → new session, even while an `api-*` session runs. Existing sessions are reached via the picker, a glob (`'api-*'`), or `-s`. **Rejected alternative:** project-prefix session matching (`api` attaches to the sole `api-*` session) — reintroduces attach-vs-create guessing with an ambiguity cliff the moment a second `api-*` session exists.

### Miss Handling & the Filter Flag

**Total miss ⇒ hard fail, every arity, every form.** The TUI-fallback-with-filter (today's terminal step of the chain) is **stripped**. The error message suggests the escape hatch (`nothing resolved for 'blog' — try -f blog`).

Journey: the user's first instinct was fail, but they liked the filter mechanic and floated an alternative — a total-miss target opens a *filtered picker in its surface* (uniform across arities: "every unresolvable target becomes a filtered picker"). Pushback killed it on three grounds: (1) **interactive-vs-scripted** — scripts are the CLI multi-open's constituency, and `open` can't know its context; a picker quietly waiting in window 3 of a scripted burst is the surprise scripts can't tolerate; (2) **the ack machinery breaks** — a picker-window writes no `--ack` receipt (nothing attaches until a human picks), so the burst misclassifies it failed and a retry opens a second picker; (3) **the user's own rarity observation** — with zoxide installed nearly every token resolves to something, so the implicit fallback is "a lottery you occasionally lose into", not a dependable feature (user: seen it a couple of times; the attempts to use it deliberately failed because zoxide ate the word). The explicit flag is what makes the filter mechanic *reliable* for the first time. User reversed with conviction.

- **`-f/--filter <text>`** → skips resolution entirely; opens the picker with the filter pre-filled. Mutually exclusive with positional targets and with other pin flags (usage error).

### Domain-Pinning Flags (locked set)

| Flag | Pins to | Semantics on that domain |
|---|---|---|
| `-s/--session <name-or-glob>` | exact session / session glob | attach; hard fail on miss; never mints |
| `-p/--path <dir>` | directory path | mint new session; dir must exist |
| `-z/--zoxide <query>` | zoxide best match | mint at matched dir; hard fail on no match; **explicit error if zoxide not installed** (pinned ≠ silently skipped — unlike the guessing chain, where zoxide absence just falls through: `resolver/zoxide.go` `ErrZoxideNotInstalled`, `query.go` treats any zoxide error as continue) |
| `-a/--alias <key-or-glob>` | alias key / key glob | mint at aliased dir; hard fail on unknown key |
| `-f/--filter <text>` | (none) | picker, pre-filtered |

`-a` was added for symmetry (the fourth resolution domain; also the only way to reach an alias shadowed by a same-named session). User rationale for the full set: open-source project — flags earn their keep for scripting and other users even where the author won't use them personally. "The fact that portal open is multi-dextrous allows us to keep the same command but focus its intent." Note `-e` is already taken by open's existing run-command flag.

### Glob Targets

- **A bare target containing glob metacharacters (`*`, `?`, `[…]`) is session-domain by construction** — patterns match against the finite set of live session names and skip path/alias/zoxide entirely. Expansion produces K targets that join the target list (`open 'agentic-workflows-*' blog` → K+1 surfaces; absorb rule unchanged). **Zero matches ⇒ unresolvable ⇒ atomic hard fail** — no special case. Glob, not regex.
- **Shell-quoting caveat (accepted, documented):** unquoted `*` is expanded by the shell against cwd files first — so session globs are typed quoted (`x 'api-*'`). Same wart as git/docker pattern args.
- **The quote is the domain switch — path globs are already free:** *unquoted* `x ~/Code/skill*` is expanded by the shell into N path args before Portal sees them → N minted sessions in N windows, zero Portal code. This answers "why couldn't globs work for paths?" — they do, via the shell, today.
- **`-a` accepts key globs** (alias keys are a finite Portal-owned namespace, same shape as session names — `-a 'workflow-*'`).
- **Zoxide has no glob support** (ordered-keyword/subsequence scoring; last term weighted to the final path component). It does have `zoxide query --list` (all matches, ranked) — multi-match zoxide ("mint sessions for everything frecency-matching *skill*") is **deferred**: a shotgun that mints N sessions for possibly-stale dirs; not designed now.

Fine print still open: `--detect`'s new home; `-e`/`--` command passthrough under the merged verb (review F1, being raised).

---

## Attach Disposition

### Context

Post-fold, `attach`'s remaining jobs: (1) the exec target of every spawned host window (`portal attach <session> --spawn-ack <batch>:<token>` — a *separate process* in a fresh window must be handed some invocable command line; in-process functions can't serve this), and (2) exact/no-guessing attach for scripts — now covered publicly by `open --session`. Clarified for the user: today `open` is *not* sugar around the `attach` command — both call the same internal Go functions in-process (`connect()` = exec `tmux attach-session` outside tmux / `switch-client` inside); the command form exists only for cross-process callers. Go's `internal/` packages are language-enforced module-private (service class vs artisan command, in Laravel terms).

An architectural constraint independent of spelling: the spawned window must **not** re-run open's guessing chain — the burst resolved all N targets at pre-flight, and re-resolution inside each window could diverge if state changed mid-burst. The exec target keeps exact semantics.

### Options Considered

**A. Keep `attach` as a hidden command** (initially proposed)
- Pros: serves the spawn exec target; **free back-compat** — every existing `portal attach foo` script keeps working with zero alias machinery; exact semantics preserved verbatim.
- Cons: one more (hidden) command in the tree; duplicates what `open --session` now expresses.

**B. Fold into open flags** (`open --session <name> --<ack> …`)
- Pros: smallest command tree; one verb carries the whole session surface, including the machine path.
- Cons: back-compat for `portal attach` then needs a separate alias shim (rerouted to the back-compat subtopic); ack flag rides on open's flag surface (hidden).

### Journey

The initial proposal was A, argued on two grounds: the spawn exec target needs an invocable command, and keeping `attach` gives free back-compat. The user challenged: "why can't we just use Portal Open with a session flag, and pass through an open-acknowledge?" — and specifically recalled that the abridged bootstrap fast-path was deliberately built as a **marker/latch, not tied to certain commands**. Verified against the code: `cmd/root.go:173` — `BootstrappedLatchSatisfied` is consulted once in `PersistentPreRunE`, command-agnostically; any bootstrap-needing command (open included) takes the abridged path when the `@portal-bootstrapped` version-stamped latch is satisfied. So the bootstrap argument for `attach` was void. `attach`'s actual body is tiny (has-session check → best-effort ack write → connect), and every piece has an `open` equivalent. The "free back-compat" argument is real but belongs to the back-compat story (an alias artifact), not the design. Option A's proposer conceded.

### Decision

**`attach` is retired from the design.** Option B:

- Spawned host windows exec `portal open --session <name> --<ack-flag> <batch>:<token>`.
- **Pinned-domain contract:** `--session` (and `--path`) invocations hard-fail on unresolvable, **never** fall back to the TUI picker — a spawned window or script must not pop a TUI. `--session` never mints (a bare name has no directory to mint from); `--path` mints per the attach-vs-mint dichotomy.
- **Burst determinism preserved:** session vanished mid-burst ⇒ pinned open hard-fails ⇒ no ack written ⇒ the burst classifies that window failed, exactly as today.
- **The ack flag is `--ack <batch>:<token>`, marked hidden via Cobra `MarkHidden`** (decided; the user asked about private-flag conventions — there is no `---`/underscore convention; hiding is the mechanism, spelling stays plain. Today's `--spawn-ack` is only *labelled* "internal:" in help text, not actually hidden — the redesign hides it properly. It remains visible in `ps` when a spawned window runs; acceptable — internal, not secret.) Rejected: `--on-open` (reads as a hook trigger, collides with `--on-resume` hooks vocabulary); `--open-ack` (redundant on `open`); `--receipt` (unusual CLI vocabulary). What it does: the burst generates a `<batch>:<token>` per window and bakes it into the spawned command; the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent polls for (~8s/window); no receipt ⇒ window classified failed. Internal names (`internal/spawn` package, `spawn` log component, `@portal-spawn-*` marker prefix) are out of this redesign's scope.
- `portal attach` is **deleted outright** — see Back-Compat & Deprecation Story: the user explicitly wants no compat surface.

Confidence: high.

---

## Back-Compat & Deprecation Story

### Context

The seed called for a compatibility/deprecation story (back-compat aliases) because `open`/`attach`/`spawn` live in muscle memory and scripts. The synthesis's T4 tension (permanent silent aliases vs deprecated-with-sunset) sat here.

### Decision

**There is no back-compat story — deliberately.** User, verbatim intent: "I'm not interested in backwards compatibility here." Consistent with their earlier frame: "I don't care about the impacts of the rename; if the rename is the right thing to do, we do it — that's the whole point of this task."

- `attach` and `spawn` are **removed**, not aliased, not deprecated-with-warning.
- Broken scripts are the owner's to fix (single-digit user base; the author owns the known scripts).
- The `x`/`xctl` shell functions re-emit from `portal init` and keep working untouched (`x` already maps to `portal open`).
- This is a **deliberate reversal of the seed's assumption**, not an omission — recorded so specification doesn't reintroduce aliases.
- Synthesis T4 (alias lifecycle) is moot: no aliases exist to have a lifecycle.

Confidence: high.

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

- `--detect`'s new home (spawn verb retiring).
- Bare `portal` (no subcommand) behaviour — related to but distinct from the settled picker placement.
- Stay-put multi-open flag — deliberately deferred scope.
- Utility command audit.
- Remaining review findings queued: -e/-- passthrough (F1, in flight), wrong-guess feedback (F2), kill/list resolution scope (F4), completion UX (F5), bare-portal vs xctl (F7).

### Current State

- Decided: `open` is the single public session verb (fold, absorb/net-N rule, universal resolution, domain-pinning flags --session/--path, hidden --ack, picker at no-args); `open` name kept on portal-metaphor grounds; `attach`/`spawn` deleted outright — no back-compat surface (deliberate seed reversal).
- Exploring: open's remaining flag surface (-e/-- passthrough).

## Triage

(none)
