# Discussion: Filtering

## Context

The sessions list can be narrowed today only by the `/` text filter, which matches each session's name and its recorded folder, and it can be arranged by the `s` grouping cycle — Flat, By Project, By Tag — whose choice is remembered between launches. Discovery accepted a set of new filters for the picker: narrowing the sessions list to one or more tags with a choice between matching any of them and matching all of them; narrowing to a project; excluding a tag ("everything except archive"); keeping the group headings while filtering, since today typing a filter flattens the list; and showing the active filter as chips in the header, so it is visible what is narrowing the list. This topic settles how those filters behave and how they sit beside what exists.

`grep -n 'func (i SessionItem) FilterValue' -A2 internal/tui/session_item.go` → returns `resolver.SearchFields(i.Session.Name, i.Session.Dir)`: the session's name and its recorded folder (abbreviated under `~`), the recorded folder only when one exists (2026-10-26).

### Inherited position from discovery

Discovery reached these with the user; they carry forward as working ground, open to movement only where this discussion surfaces something that contradicts them:

- **In scope, all accepted:** tag filters with an any/all choice, a project filter, tag exclusion, group headings kept while filtering, and the active filter shown as chips in the header.
- **A tag filter matches through projects.** Tags live on projects only, and a session carries its project's tags.
- **The picker row rework stays a separate roadmap item** — each session's folder beside its name, the right-hand status strip. Display work tied to filtering, such as the chips, belongs here.

### History in the original tagging work

The session-tagging-and-grouping specification deferred two of these explicitly:

- **Live-grouped filtering** — keeping group headings while the `/` filter is active was the user's first preference then, deferred because the list widget re-ranks matches into a relevance-sorted flat list, so keeping groups would mean Portal owning the filter outright (its own input state, matching, re-grouping per keystroke, cursor and pagination), with a large interaction matrix (an active filter plus a view switch, the preview, an external kill refreshing the list). The payoff was judged small and transient, since filtering is usually "find one session fast". Nothing in that work locked it out.
- **Tag exclusion** — deferred as a power feature, acceptable because the `/` filter covered basic narrowing; to be revisited if a concrete "tag I never want to see" pain appeared.

That specification also fixed the text filter's scope as name-based, with the tag dimension served by the By Tag view rather than by the filter. Since then the filter has come to match the recorded folder too, and group headings have become real, non-selectable rows of the list whose empty filter text makes them vanish the moment a query is typed — the mechanism by which the list flattens today.

### Ground settled by sibling discussions

From project-link:

- A session records the folder it was started in and, separately, the project it belongs to; a worktree session's project is the repository the worktree was made from. The folder keeps every job it had apart from grouping — the session row, the `/` text filter, `x /term` — and the project drives the By Project heading, the session's tags, and every tag or project filter.
- A session with no record keeps a live guess of its project from its active pane's folder; the guess feeds headings, tags and the tag and project filters only, and is silent when wrong. Being missing from a filter was weighed as just as much a failure as being misplaced in one.
- Projects stand alone: a project filter matches the sessions of that project only, never those of projects nested inside it. A family of projects (folio with folio/api and folio/sdk) is a tag put on its members.
- A tagged project whose folder disappears is kept and marked missing; sessions recorded under it keep grouping under it and stay in its filters.
- Switching to a grouped view, or applying a tag or project filter, shows no delay however many sessions are guessed.
- Every session row offers `l`, linking the session to a project; a link applies to that one session.

From tagging:

- Tag identity ignores case everywhere a tag is compared — explicitly including tag filters — and a tag has one casing on disk.
- The sessions page's keys become: Enter, `/`, Space, `s`, `n`, `r`, `k`, `q`, `x`, `t` (tagging a session's project, replacing the theme key among the footer's core keys), `m`, `?`, plus `c` (theme) and `l` (link) in help only. Footer core keys on the sessions page today: Enter, `/`, Space, `s`, `x`, `t`, `m`, `?` (`internal/tui/keymap.go`).
- New surfaces use the existing colour tokens, keep every state glyph- or word-backed under `NO_COLOR`, and are designed in Paper, in the existing Portal design file, starting from an existing design.
- The projects page gains a Tags list with each tag's project count.

### Open questions carried in

- How the new filters sit beside the `/` text filter and the `s` grouping cycle — one more control stacking on them, or merged with them — including how a tag filter relates to the By Tag view.
- Whether a filter persists between launches, as the grouping mode does.
- Whether the projects list filters too, or only the sessions list — the user said "projects / sessions".
- Whether typing a tag name in the `/` text filter matches that tag's sessions.
- Sessions with no project drop out of any tag or project filter, so filtering is only as reliable as the session-to-project link and the separate reboot bugfix.
- Keys and footer space for the filter controls.
- Filter chips add a row to the vertical space where the keymap footer already disappears on short terminals (ten rows or fewer), and the section header where chips would go overflows at very narrow widths. Both are inbox bugs discovery left out of the epic.
- Designing the chips and filter controls in the current look, including the no-colour mode.

Settled elsewhere and not reopened here: whether a project filter takes in nested projects (project-link — it does not), and the lookup cost of tag and project filtering (project-link — no delay).

### References

- [Discovery brief — Filtering](../discovery/briefs/filtering.md)
- [Discovery session 001](../discovery/sessions/session-001.md)
- [Discussion — Project Link](project-link.md)
- [Discussion — Tagging](tagging.md)
- [Specification — session-tagging-and-grouping](../../session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md) (Non-Goals; Filter Composition)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent. The Discussion Map (which subtopics exist and their states) lives in the manifest, not this file.*

---

## Filter Model

### Context

The new tag, project and exclusion filters need a place beside the two controls the sessions list already has: the `/` text filter and the `s` grouping cycle. The worked case: an afternoon where only work should show, across 33 live sessions (`tmux list-sessions -F '#{session_name}' | grep -vc '^_portal'` → 33, 2026-10-26). Today that means switching to By Tag and scrolling to the `fabric` heading, or typing `/fabric` — which catches sessions whose recorded folder sits under `~/Code/fabric`, an approximation of the tag rather than the tag. Either way, typing `/folio` to find one session then drops the headings and re-ranks the list flat.

The framing that drove the choice: two different jobs. **Search** (`/`) is "find one session now" — transient: type, pick, gone. **Scope** is "this is my working set for a while" — sticky: it should survive a search, a view switch, perhaps a relaunch.

### Options Considered

**A. Scope apart from search** — the tag/project/exclusion filter is set on its own, stays put, and shows as chips in the header; `/` searches within it and clearing a search leaves the scope alone; `s` arranges whatever is in scope.
- Pros: each control keeps one lifetime; the scope persists naturally if persistence is wanted; a command-line way to open the picker pre-narrowed has an obvious thing to set.
- Cons: a new control to learn and to find a key for.

**B. One box with a syntax** — `/` accepts tokens such as `t:fabric -t:personal folio` beside free text; chips are the parsed tokens.
- Pros: one control; fast to type.
- Cons: the scope lives and dies with the search box — clearing a search clears the scope; persisting it means opening the picker with text already in the box.

**C. Filter through the grouped views** — in By Tag, hide or isolate tag groups; in By Project, isolate a project.
- Pros: no new control.
- Cons: the filter is tied to a view — switching to Flat loses or hides it; "match all" has no group to map onto, since a session with two tags shows under both headings.

```text
 Sessions 9 — by project    fabric · −personal          / to filter
   folio ··· 3
 ▌ folio-a1b2
   folio-x9y8
   finder ··· 2
   …
```

(Illustration of A only; the chips' look and placement are Filter Chips'.)

### Journey

The deciding point was lifetime: under B a transient search wipes out a lasting choice, and under C the choice depends on which view is showing. The question put to the user was whether they reach for filtering mostly as "find one session" — which would have favoured B's single box. They agreed with A.

### Decision

**The tag and project filter is a scope, separate from the `/` search and from the `s` grouping cycle.** Three independent controls:

- **Scope** — which sessions are in the list at all: tags included or excluded, a project. It stays in place until the user changes it, and is shown as chips in the header.
- **Search** (`/`) — finds sessions within the scope, as today; clearing the search returns to the scoped list, never to the unscoped one.
- **Arrangement** (`s`) — Flat, By Project or By Tag over whatever is in scope; switching views leaves the scope as it is.

How the scope is set is Setting The Filter's; whether it survives a relaunch is Filter Persistence's.

Sibling check: command-line-filters (discovery brief) — whether `x --tag work` opens the picker narrowed is open there; a scope separate from the search box gives that flag something to set without pre-filling `/`, as `-f` does. No overlap.

---

## Summary

### Key Insights

### Open Threads

### Current State
