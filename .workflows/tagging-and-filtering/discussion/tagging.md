# Discussion: Tagging

## Context

Tags in Portal live on projects (a saved folder), and every session is tagged through the project it belongs to. Today the only way to put a tag on a project is the projects page's edit screen: open the projects page, pick the project, open edit, move to the TAGS field and type the tag in full as a chip. Nothing suggests the tags already in use, nothing tags several projects at once, there is no place that shows every tag or how many projects carry it, and nothing renames or deletes a tag across projects. A session's project cannot be tagged from the sessions list. Once this epic makes tags the basis of filtering, the cost of getting tags onto projects — and keeping them consistent — matters more than it did while tags only fed the By Tag grouping view.

Measured on the live install 2026-10-26 (read-only): 66 saved projects, 5 tags in use, no project carrying more than one tag, 7 projects untagged.

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort | uniq -c | sort -rn` → 27 personal, 22 fabric, 5 acom, 3 curated, 2 detaillab

`jq '[.projects[]|select((.tags//[])|length>1)]|length' ~/.config/portal/projects.json` → 0

`jq -r '.projects[]|select((.tags//[])|length==0)|.path' ~/.config/portal/projects.json` → 7 rows: `~/Code/switchboard`, `~/Code/acom/mach2`, `~/code`, `~/.config/kit`, `~/Code/packages`, `~/Code/kit`, `~/Code/terrace`

### Inherited position from discovery

Discovery reached these with the user; they carry forward as working ground, open to movement only where this discussion surfaces something that contradicts them:

- **Tags stay on projects only.** No per-session tags: sessions are ephemeral — one can last a minute or a month — so tagging them is not useful. Every session is tagged indirectly through its project.
- **In scope, all accepted by the user:** suggestions while typing a tag, drawn from the tags already in use across all projects and narrowing as you type; bulk tagging — mark several projects and add or remove a tag on all of them at once; tag management in one place — rename or delete a tag everywhere, and see how many projects carry each; tagging a session's project from the sessions list, without going to the projects edit screen.
- **Tags are case-sensitive by deliberate choice** — "Work" and "work" are two tags (`project.NormaliseTag` trims whitespace and preserves case).

### Rejected paths from discovery

- Per-session tags layered on project tags (a session's tags being its project's plus its own), proposed to tag what a session is for and to tag sessions whose folder is not a project. Rejected on the ephemerality argument; it would also have widened the reboot bugfix to restore session-level tags.

### Ground settled by the project-link discussion

- A session records its project apart from the folder it started in; a worktree session's project is the repository the worktree was made from. Sessions with no record keep a live guess of their project, which is grouping-and-filtering only and silent when wrong.
- Every session row offers an action that links the session to a project: a chooser that is the projects list as the projects page draws it, narrowed by typing, with the session's current project marked (labelled as a guess when it is one), and "save `<folder>` as a project" at the top when the session's project is not a saved project. A link applies to the one session it is made on. Project-link left the key this action takes, and how sessions-list tagging invokes it, to this topic.
- Projects stand alone: no project takes in the projects nested inside it, and a family of projects (folio with folio/api and folio/sdk) is expressed by tagging its members. Project-link anticipated this topic's inheritance question as what would make a family tag cost nothing to maintain.
- Tagged projects are never removed by housekeeping when their folder disappears; they are kept, marked missing, and re-pointed or deleted by the user. Re-pointing a project at a folder that is already a project merges the two with their tags combined, as does two spellings of one folder turning out to be one project.

### Open questions carried in

- Where bulk tagging happens. Marking several items exists only on the sessions list today (`m`, multi-select), where it opens several sessions at once.
- Whether a new project inherits the tags of the project whose folder encloses it. Measured 2026-10-26 by nearest enclosing saved project (folder compared without case): 36 projects sit directly under a tagged project, and 31 carry exactly its tags; the five that do not are `~/.config/kit` and `~/code` (untagged under the home-folder project, tagged personal), `acom/mach2` (untagged under acom), and `fabric/flowx/flowx-api` and `fabric/packages/laravel-workflow` (personal under fabric). Four untagged top-level projects (`~/Code/kit`, `packages`, `switchboard`, `terrace`) sit directly under the untagged `~/code` project, which itself sits under the home folder.

  `python3 -I -c "import json,os; ps={p['path']:p for p in json.load(open(os.path.expanduser('~/.config/portal/projects.json')))['projects']}; enc=lambda q: max((k for k in ps if k!=q and (q.lower()+'/').startswith(k.lower()+'/')), key=len, default=None); r=[(q,e) for q in ps for e in [enc(q)] if e and ps[e].get('tags')]; print(len(r), sum(sorted(ps[q].get('tags') or [])==sorted(ps[e]['tags']) for q,e in r))"` → `36 31`
- Tag case once suggestions and merging put near-duplicates side by side, and renaming a tag into one that already exists — a merge that has to be handled.
- Which key tagging takes: `t` opens the theme panel on both pages. The sessions page already uses Enter, Space, `/`, `s`, `n`, `r`, `k`, `q`, `x`, `t`, `m` and `?`; the projects page Enter, `x`, `e`, `/`, `t`, `d`, `n`, `q`, Esc and `?`. New actions compete for footer space. The link action's key is this topic's too.
- Tagging an Unknown session's project from the sessions list has nowhere to put the tag until the session is linked to a project; the link action now exists (project-link), and this topic decides how tagging invokes it.
- Each new surface needs designing in the current look, including the no-colour mode: the suggestion list, tag management, bulk tagging.
- The narrow-width overflow bug (inbox) touches the edit screen where tag suggestions would appear; discovery left it in the inbox, not in the epic.

### References

- [Discovery brief — Tagging](../discovery/briefs/tagging.md)
- [Discovery session 001](../discovery/sessions/session-001.md)
- [Discussion — Project Link](project-link.md)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent. The Discussion Map (which subtopics exist and their states) lives in the manifest, not this file.*

---

## Tag Case

### Context

Today a tag is stored and shown exactly as typed, and two tags differing only in case are two tags: `Work` and `work` give two By Tag headings and two separate sets of projects (`project.NormaliseTag` trims and preserves case; every comparison goes through it). Suggestions put such near-duplicates side by side, tag management would show them as two rows with separate counts, a tag filter on one misses projects carrying the other, and renaming a tag into one that already exists has to be handled as a merge. How tag identity treats case decides all of these.

The original tagging work (session-tagging-and-grouping specification) made the canonical form lower-cased: `Work`, `WORK` and `work` were one tag, stored and shown as `work`. A later fix changed it — commit `e0e41a13d` (2026-06-08), "preserve tag casing": `NormaliseTag` now trims only, "so a tag is stored and displayed exactly as typed ("Personal" no longer renders "personal"). Matching/dedup/grouping are consequently case-sensitive". The aim of that change was display casing; case-sensitive identity followed from it rather than being chosen for itself. Discovery recorded the current behaviour as case-sensitive "by deliberate choice".

On the live install no tag carries a capital and no two tags differ only by case (2026-10-26):

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort -u | awk '{print tolower($0)}' | sort | uniq -d | wc -l` → 0

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort -u | grep -c '[A-Z]'` → 0

### Options Considered

**Case-sensitive (today)** — `Work` typed where `work` exists becomes a second tag.
- Cons: two By Tag headings, a filter on one misses the other, suggestions and tag management show both; the repair is a rename that merges.

**One tag whatever the case, keeping the casing it was first written with** — typing `Work` matches the existing `work`; the suggestion highlights it and adding it adds `work`.
- Pros: keeps what the June fix was for (the user's casing is shown as written) and removes its side effect; two tags differing only in case cannot arise.
- Cons: a tag's casing is set by whichever project used it first; changing it is a deliberate rename.

**Lower-case everything** (the original rule) — not reopened: the June fix rejected it for erasing the user's casing.

### Journey

The history reframed discovery's open question: the deliberate choice was display casing, not case-sensitive identity. Suggestions make the question sharper than it is today — once tags are picked from a list, a stray capital is the main way a duplicate gets made, and under case-sensitive identity it would silently split a tag in two (two headings, a filter that misses half the projects). Nothing on the install needs merging at ship. The test that would have kept case-sensitivity — a real case where `API` and `api` should mean different things — has no instance.

### Decision

#### 2026-10-26 — revised
*Trigger: settled call — case-insensitive identity left unsaid which casing shows when two casings of one tag are already on disk.*

**Settled by derivation** — not discussed. Determined by the case-insensitive identity below and the paths that can still put two casings on disk: a hand edit of `projects.json`, and project-link's merges (re-pointing a project at a folder that is already a project, and two spellings of one folder becoming one project), which combine two projects' tags.

The entry below stands. Where two casings of one tag are already on disk — `Work` on one project, `work` on another — they are one tag, shown in the casing carried by more projects; a tie keeps the casing of the most recently used project. Any project that gains the tag afterwards takes that casing.

#### 2026-10-26

**One tag whatever the case, shown in the casing it was first written with.** Tag identity ignores case everywhere a tag is compared — adding a tag to a project, suggestions, grouping, tag management, and tag filters. Typing a tag that matches an existing one in another case adds the existing tag, in its existing casing. Changing a tag's casing everywhere (`work` → `Work`) is an ordinary rename in tag management, never a merge. The user's casing is still shown as written — the June fix's aim stands; only its side effect is dropped.

Sibling check: filtering, command-line-filters (discovery briefs) — neither says how a tag filter or `x --tag` treats case; both compare tags, so they inherit this identity. No overlap.

---

## Tag Suggestions

### Context

Adding a tag today means typing it in full into the edit screen's TAGS field (Enter on the `+ add` slot opens a text box; Enter commits it as a chip; Esc discards). Nothing shows which tags already exist, so reusing a tag depends on remembering its exact spelling — and a misremembered spelling silently makes a new tag. Discovery accepted suggestions drawn from the tags already in use across all projects, narrowing as you type. While a tag is being typed, ↑ and ↓ do nothing in that box today (they move between fields only when no text box is open), so they are free to move through a suggestion list.

### Options Considered

The list's content and behaviour were proposed as one shape and stood: it opens as soon as a tag is being typed — before any text — showing tags in use on other projects that this project does not already carry, each with how many projects carry it, most-used first; typing narrows it, matching anywhere in the tag and ignoring case (Tag Case); ↑↓ move through it; it serves editing an existing chip as well as adding one, so replacing one tag with another on a project is a pick, not a retype.

The fork was what Enter does with `fab` typed while `fabric` is listed:

**Enter takes the highlighted suggestion** — the best match is highlighted from the start; creating a new tag means moving to a `+ new tag "fab"` row, present only when the typed text is not already a tag.
- Pros: the easy keystroke reuses a tag; a near-duplicate takes a deliberate move.
- Cons: one extra ↓ to create a tag that happens to be a prefix of an existing one.

**Enter adds what was typed** — nothing is highlighted until ↓.
- Pros: creating a tag is a keystroke shorter.
- Cons: `fab` + Enter creates `fab` beside `fabric` — the duplicate trap Tag Case closed for capitals, reopened for prefixes.

### Journey

Reuse is the common case by a wide margin — five tags on the install after months of use — so the default keystroke belongs to it. The user then pushed back on the list showing every tag: with a hundred tags it would run off the bottom of the screen. The list is a scrollable dropdown of bounded height that fits the space available, scrolling for the rest; its look is to be spiked in Paper, in the existing Portal design file, starting from one of the existing designs.

### Decision

**Suggestions open with the tag box, and Enter takes the highlighted suggestion.** While a tag is being typed — added or edited — a dropdown below the box lists the tags in use on other projects that this project does not carry, each with its project count, most-used first, narrowed as the user types by a case-insensitive match anywhere in the tag. The best match is highlighted from the start and Enter adds it; ↑↓ move the highlight. When the typed text is not already a tag, a `+ new tag "…"` row closes the list and is the way to create one; when nothing matches, it is the only row and is highlighted, so Enter creates the tag. The dropdown has a bounded height that fits the space available and scrolls beyond it — it never runs off the screen however many tags exist.

---

## Tag Management

### Context

Tags have no home of their own: a tag exists only as the union of what projects carry (no registry — the session-tagging-and-grouping design), and the only way to see which projects carry a tag is the By Tag view of the sessions list, which shows only projects with a live session. Renaming a tag means editing every project that carries it one by one; deleting a tag likewise. Discovery accepted tag management in one place: rename or delete a tag everywhere, and see how many projects carry each. Tag Case already settled that renaming into an existing tag (ignoring case) is a merge, and that a case-only rename is a rename.

### Options Considered

Proposed as one surface with bulk tagging (see Bulk Tagging), and it stood: a **Tags list** opened by a key on the projects page — one row per tag with its project count; `r` renames a tag everywhere, a rename into an existing tag being a merge confirmed with the counts; `d` deletes a tag from every project, confirmed with the count; Enter opens the tag's project checklist (Bulk Tagging); a `+ new tag` row starts a new tag.

```text
Tags                          acom — 5 projects
  personal   27                 [x] acom
  fabric     22                 [x] acom/mach2/api
▸ acom        5     ⏎ ──▶       [ ] acom/mach2
  curated     3                 [ ] switchboard
  detaillab   2                 …
  + new tag
```

### Journey

The projects page is the natural parent: tags live on projects, and the page already owns the edit screen where single-project tagging happens. Management and bulk tagging collapsed into one surface once membership was seen as the thing both act on — the tag's row says how many, its checklist says which, and changing either is the same act.

### Decision

#### 2026-10-26 — revised
*Trigger: settled call — the Tags list needed an order.*

**Settled by derivation** — not discussed. Determined by Tag Suggestions, which orders tags by how many projects carry them.

The entry below stands, and the Tags list is ordered most-used first, ties alphabetical — the order the suggestions dropdown uses, so a tag sits in the same place on both surfaces.

#### 2026-10-26

**A Tags list off the projects page manages tags.** A key on the projects page (Keys And Footer) opens it: every tag in use, each with how many projects carry it. `r` renames a tag on every project that carries it; renaming into a tag that already exists, ignoring case, merges the two after a confirmation showing the counts involved, and a project carrying both ends up with the one; a rename that changes only the casing is a plain rename (Tag Case). `d` removes a tag from every project after a confirmation showing how many projects carry it. Enter on a tag opens its project checklist (Bulk Tagging). A `+ new tag` row asks for a name — offering the existing tags as the suggestions dropdown does, so a name that already exists lands on that tag — and opens its empty checklist; a new tag with nothing ticked when the checklist closes leaves nothing behind, since a tag exists only while a project carries it.

---

## Bulk Tagging

### Context

Discovery accepted bulk tagging — add or remove a tag on several projects at once — and left open where it happens: marking several items exists only on the sessions list (`m`, multi-select), where it opens several sessions at once. The families project-link left to tags (folio with folio/api and folio/sdk) are the motivating case: tagging a family is one tag across several projects.

### Options Considered

**Tag first, then tick projects** — inside the Tags list, a tag's checklist shows every saved project, ticked where it carries the tag; Space ticks and unticks; typing filters the list.
- Pros: bulk add and bulk remove in one place, with the tag's current membership visible while it changes; reuses the Tags list rather than adding a second mode to the projects page.
- Cons: the checklist holds every saved project (66 on the install), so finding the ones wanted relies on typing to filter.

**Mark projects first, then pick a tag** — multi-select on the projects page, like `m` on the sessions list, then an add-tag or remove-tag action on the marked set.
- Pros: suits "these three new projects all get `personal`".
- Cons: needs separate add and remove paths, and shows nothing of who already carries the tag.

### Journey

The mark-first case is served tag-first at the same cost — open `personal`, tick three rows — while the reverse does not hold: mark-first cannot show a tag's membership. Single-project tagging stays in the edit screen, so a projects-page multi-select would add a mode for nothing the checklist lacks.

### Decision

#### 2026-10-26 — revised
*Trigger: settled call — the checklist needed a save model.*

**Settled by derivation** — not discussed. Determined by the edit screen's existing behaviour: each chip is saved the moment it is added or removed, and closing the screen never discards.

The entry below stands, and each tick or untick in a tag's checklist is written at once, with no save step; Esc closes the checklist.

#### 2026-10-26

**Bulk tagging is the tag's project checklist in the Tags list; the projects page gains no multi-select.** Enter on a tag opens a checklist of every saved project, ticked where it carries the tag; Space ticks or unticks the highlighted project, adding or removing the tag on it; typing narrows the list. A new tag (Tag Management's `+ new tag`) opens on an empty checklist.

---

## Tagging a session's project from the sessions list when its project link is a guess

### Context

*From: project-link · discussion · 2026-10-11*

The project-link discussion decided that a session with no recorded project keeps a live guess of its project — the folder its active pane is in now, resolved to the repository it belongs to — for its heading, its tags and the tag and project filters. A wrong guess was accepted as silent because it corrects itself: when the pane moves back, the guess follows. The picker also gains a "link to project" action on every session row, which records the session's project by the user's choice and replaces any guess; its chooser is the projects list as the projects page draws it, with the session's current project marked and labelled as a guess when it is one.

Tagging a session's project from the sessions list (this topic's flow) writes a tag onto the session's project. When that project is a guess, the tag lands durably on whatever project the guess chose. Live example on 2026-10-11: `laravel-workflow-PGR3jA`'s pane sits in `~`, so its guess is the home-folder project; tagging it from the sessions list would tag the home-folder project — and with it every session the guess files there — while the session itself belongs to `~/Code/fabric/packages/laravel-workflow`. Unlike the guess, the tag does not correct itself when the pane moves.

Options in sight from project-link: treat a guessed link like a recorded one; route the tag action through the link chooser first when the link is a guess (the user confirms the project, which records it, then tags); or refuse with a pointer to linking. How the sessions-list tagging flow invokes the link action, and which key it takes, were already left to this topic.

The ask: whether sessions-list tagging treats a session whose project is a guess the same as one whose project is recorded.

### Options Considered

**Treat a guess like a recorded link** — the tag goes straight onto the guessed project.
- Cons: a wrong guess puts a durable tag on an unrelated project, and through it on every other session filed there, with nothing shown.

**Chooser first when the link is a guess** — the tag action opens the link chooser with the guessed project highlighted; confirming records the link, then the tag editor opens.
- Pros: a correct guess costs one Enter and leaves the session with a recorded link it keeps; a wrong guess is caught at the moment a tag would make it durable.
- Cons: one extra step on every guessed session tagged.

**Refuse, pointing at the link action.**
- Cons: a detour that reaches the same chooser with nothing gained over opening it directly.

### Journey

The asymmetry decided it: the guess is tolerable because it corrects itself, and a tag does not — so the moment a tag would be written is exactly when a guess has to become a choice. Opening the chooser with the guess highlighted makes the right-guess case nearly free, and turns a guessed session into a recorded one as a side effect, which is the outcome project-link wants for every repair. Refusing was strictly worse than the chooser: the same destination behind an extra hop.

### Decision

**A guessed link goes through the link chooser before anything is tagged.** Tagging from the sessions list on a session whose project is a guess opens the link-to-project chooser with the guessed project highlighted; picking a project (the guess or another) records the session's link, and the tag editor then opens on that project. A session whose project is recorded goes straight to the tag editor. Tagging never writes onto a guessed project.

Sibling check: project-link — Linking From Picker decided the chooser (the projects list, the current project marked and labelled as a guess) and that a picked link replaces the guess for that session; it left how the sessions-list tagging flow invokes it to this topic. Consistent; this is that invocation.

---

## Summary

### Key Insights

### Open Threads

### Current State
