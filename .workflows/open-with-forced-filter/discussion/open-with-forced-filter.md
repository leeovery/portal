# Discussion: Open With Forced Filter

## Context

What the thing after `portal open` means — the whole argument surface, not just
one addition to it.

Two threads arrive here together, and discovery committed them as one feature
because they are two facets of the same question.

**The filter shortcut.** The seed describes the user's dominant real-world
Portal flow: type `x`, wait for the picker, press `/`, type three or four
characters, `Enter`, `Space` through the two or three survivors to see which is
which, `Enter` to attach. Everything before the last two steps is ceremony —
the shape of the target is already known before the picker paints. The ask is a
single shell-level form (`x /port`, or a no-space `x/port` if possible) that
lands straight in the sessions list with `port` already in the filter box, the
list narrowed, cursor on the first match, and the rest of the flow unchanged.

The seed's stated hard property: the term must be **forced as filter text** and
must never enter the bare-positional resolution grammar, which is too eager and
too fuzzy — it will decide `port` names a zoxide directory or an alias and mint
a new session, when what was wanted is "show me the live sessions whose names
contain this, and let me pick". The driving scenario is three live Portal
sessions, one or two custom-named and the rest `portal-<nanoid>`: the goal is
not to resolve to one session, it is to narrow to the right handful fast and
choose by preview.

**The bare-positional grammar.** Discovery widened past the shortcut. The user
raised `portal open xxx` itself as "not quite right" after living with Portal
for an extended period. Two things came out of it:

*Predictability.* A bare argument walks an ordered chain of guesses and the
first hit wins, with a zoxide hit minting a new session. The user does not
remember the order, cannot predict which branch will answer for a given word,
and has consequently stopped using the bare form altogether — the habit is now
`x`, then the picker, every time. One of the command's main surfaces is dead in
practice.

*Intent.* The user believes that when they type `x portal` they mean "open a new
session in the Portal project directory" and would never expect an attach —
because the default `{project}-{nanoid}` naming makes existing sessions
unrecognisable by name. They flagged their own uncertainty ("I might be wrong
there"), so this is a belief to test here, not a settled requirement.

Discovery framed the desired outcome loosely on purpose: the landing may be
nothing more than adding the filter shortcut and leaving the resolution chain
untouched. That is a legitimate result, not a reason to shrink the work — the
shortcut is being designed as one route within a grammar rather than as a
bolt-on.

### Current state of the code

Measured against the tree at the start of this discussion.

- The picker behaviour the seed wants already exists behind a flag.
  `portal open -f/--filter <text>` skips resolution and opens the picker
  pre-filtered (`cmd/open.go`), and the model applies it as a *committed*
  filter — filter text set and filter state `FilterApplied`, not focused — then
  re-anchors the cursor, so arrows/`Space`/`Enter` work immediately on the
  narrowed list (`internal/tui/model.go:1261`). `-f` is mutually exclusive with
  a target and with every domain pin.
- The shell function is a pass-through, so any new syntax is parsed by Portal,
  not by the shell. `portal init` emits `x() { portal open "$@"; }` for bash,
  zsh and fish (`cmd/init.go`) — `x /port` reaches `portal open` as the single
  positional `/port`. A no-space `x/port` is not a shell function call at all,
  so it cannot work without a different mechanism.
- The bare chain is exact session → path → alias → zoxide → miss
  (`internal/resolver/query.go`), with globs handled separately (a glob is not
  expanded by `Resolve`; it falls through to a miss on the single-target path,
  and expands only on the multi-target/burst path and under `-s`). A total miss
  hard-fails — there is no implicit picker fallback.
- **A leading `/` is currently a path argument.** `IsPathArgument` returns true
  for any value containing `/`, or beginning with `.` or `~`
  (`internal/resolver/path.go:10`), so `/port` resolves in the path domain and
  errors `Directory not found: /port`. Absolute paths are a live, legitimate
  bare-positional input, which is a direct collision with a `/`-prefix filter
  sigil.
- Domain pins already exist and hard-fail rather than falling through:
  `-s/--session` (attach only, never mints), `-p/--path`, `-a/--alias`,
  `-z/--zoxide`.
- Sessions are named `{project}-{nanoid}` by `internal/session/naming.go`, which
  is the naming the user cites as making the attach branch unusable by name.

### References

- `.workflows/open-with-forced-filter/seeds/2026-08-17-open-with-forced-filter.md` — the seed (inbox:idea)
- `.workflows/open-with-forced-filter/discovery/sessions/session-001.md` — discovery session log (the carrier)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. The Discussion Map lives in the manifest.*

---

## bare-positional-grammar

### Context

Discovery arrived with two complaints about `portal open <word>`: that the
outcome is unpredictable because a bare argument walks five branches whose order
nobody remembers, and that the user's own intent when typing `x portal` is "mint
a session in the Portal directory, never attach" — a belief they flagged as
possibly wrong.

Both were tested against the code and against the decision of record before
anything else was discussed, because a conversation reasoned over a false
premise stays poisoned however well the document is corrected later.

### Journey

**The intent belief is already the behaviour, and was decided deliberately.**
`x portal` mints in the Portal directory today. `portal` is not an exact session
name — sessions are `{project}-{nanoid}` — so the session branch cannot answer
and the chain falls through to a directory domain, which always mints. The
`cli-verb-surface-redesign` specification locked this as **Axiom 2 (no
find-or-create)**, recorded the consequence under its own heading ("Bare project
shorthand does not reattach"), and **explicitly rejected** project-prefix session
matching — `api` resolving to the sole live `api-*` session — on the grounds that
it "reintroduces attach-vs-create guessing with an ambiguity cliff the moment a
second `api-*` session exists". The user was right about their own intent, and
the product already serves it.

**The chain is three live branches for a plain word, not five.** A bare word can
reach neither the path domain nor the glob branch:

`sed -n '13,15p' internal/resolver/path.go` → `return strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`

A path argument needs a `/`, or a leading `.` or `~`; a glob needs one of
`*?[` (`internal/resolver/glob.go`, `globMeta`). So for `portal` the live set is
exact session name → alias → zoxide, and the first of those never fires for a
project word by naming convention. **Both survivors mint at a directory.**

That matters because it relocates the unpredictability. The *outcome* of a bare
word is invariant — a new session at some directory. The only thing the user
cannot predict is **which directory zoxide picked**, which is zoxide's frecency
ranking rather than Portal's branch ordering. "I can't remember which of five
branches answers" is largely a mirage.

**What is actually missing.** Stripping the mirage leaves a real gap, and it is
not in the chain: the argument surface has three routes that mint and **no route
that searches live sessions**. Reaching an existing session requires its exact
`{project}-{nanoid}` name under `-s`, or the picker. Since the names are
unmemorable by design, the picker is the only practical route — which is exactly
the ceremony the seed set out to remove. The bare form is not broken; it is
complete for minting and deliberately blind to attaching.

This reframes the feature. It is not "add a filter shortcut, and separately
consider fixing the chain". It is: **the argument surface is missing its search
route, and this feature adds it.**

The user confirmed the reframe, and named two things they want from the result:
a shortened syntax for opening the picker pre-filtered, and going straight to a
session when the term is unambiguous.

### Decision

*(provisional — the chain itself stands unchanged; what replaces the missing
route is being worked under the sigil subtopics)*

The bare-positional resolution chain is **not** altered by this feature. Its
ordering, its domains, and Axiom 2 stand as the `cli-verb-surface-redesign`
specification set them. Project-prefix session matching stays rejected for the
reason that specification gave.

Sibling check: `cli-verb-surface-redesign` specification — holds Axiom 2 (no
find-or-create), the accepted consequence that bare project shorthand does not
reattach, and the explicit rejection of project-prefix session matching. This
discussion ratifies all three rather than contradicting them; no correction is
owed.

---

## Summary

### Key Insights

*(to be captured)*

### Open Threads

*(to be captured)*

### Current State

*(to be captured)*
