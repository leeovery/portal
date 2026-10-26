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

**One tag whatever the case, shown in the casing it was first written with.** Tag identity ignores case everywhere a tag is compared — adding a tag to a project, suggestions, grouping, tag management, and tag filters. Typing a tag that matches an existing one in another case adds the existing tag, in its existing casing. Changing a tag's casing everywhere (`work` → `Work`) is an ordinary rename in tag management, never a merge. The user's casing is still shown as written — the June fix's aim stands; only its side effect is dropped.

Sibling check: filtering, command-line-filters (discovery briefs) — neither says how a tag filter or `x --tag` treats case; both compare tags, so they inherit this identity. No overlap.

---

## Summary

### Key Insights

### Open Threads

### Current State
