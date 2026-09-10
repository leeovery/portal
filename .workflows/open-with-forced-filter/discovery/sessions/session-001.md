# Discovery Session 001

Date: 2026-09-10
Work unit: open-with-forced-filter

## Description (as of session)

Rethink what an argument to `portal open` means — the bare-positional resolution
grammar plus a `/text` shortcut that forces the picker open pre-filtered.

## Seed

- seeds/2026-08-17-open-with-forced-filter.md (inbox:idea)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The seed asked for one thing: a shell-level shorthand — `x /port` — that launches
the picker with `port` already in the filter box, the list narrowed to matching
sessions, and the cursor on the first match. From there the user carries on as
today: `Space` to quick-preview each candidate, `Enter` to attach. The seed's
stated key property is that the search term must be forced as filter text and
must never enter the bare-positional resolution grammar, which is too eager and
too fuzzy for the job — it will decide the text names a zoxide directory or an
alias and mint a new session when what was wanted was "show me the live sessions
whose names contain this, and let me pick". The seed itself notes that
`portal open -f/--filter <text>` already provides the underlying picker
behaviour, leaving the ergonomic shell-level form as the missing piece.

The conversation widened past that. The user raised the bare-positional form
itself — `portal open xxx` — as "not quite right" after an extended period of
living with Portal. Two things came out of pulling on that.

First, the predictability problem. Today a bare argument walks an ordered chain
of guesses (exact session name, then glob, then a directory path, then an alias,
then zoxide) and the first hit wins, with a zoxide hit minting a new session.
The user does not remember that order, cannot predict which of the five branches
will answer for a given word, and has consequently stopped using the bare form
altogether — the habit is now `x`, then the picker, every time. That is workable
but it means one of the command's main surfaces is dead in practice.

Second, an intent question the user surfaced themselves and was explicitly
unsure about: when they type something like `x portal`, they believe they mean
"open a new session in the Portal project directory" and say they would never
expect it to attach to an existing session — because the default session naming
(`{project}-{nanoid}`) makes existing sessions unrecognisable by name. They
flagged their own uncertainty about that ("I might be wrong there"), so it is a
belief to test in discussion rather than a settled requirement.

The user framed the desired outcome loosely on purpose: they want to discuss the
argument surface as a whole, and the result may be nothing more than adding the
`/filter` shortcut while leaving the resolution chain untouched. That is a
legitimate landing, not a reason to shrink the work — the debate is real either
way, and the shortcut is being designed as one route within a grammar rather
than as a bolt-on.

Shape signals: one coherent scope (what the thing after `open` means), real
behaviour debate, nothing broken to diagnose, and no second topic pulling away
from the first — the filter shortcut and the bare-positional chain are two
facets of the same question. Committed as a feature.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to discussion.
