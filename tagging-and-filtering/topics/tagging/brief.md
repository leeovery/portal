# Discovery Brief — Tagging

Drawn from discovery session 001.

## Soft decisions

- Tags stay on projects (the directory) only. There are no per-session tags. Sessions are ephemeral — one can last a minute or a month — so tagging them is not useful; every session is tagged indirectly through its project (a Portal session links to the portal project, which carries the user's "personal" tag).
- In scope, all accepted by the user: suggestions while typing a tag, drawn from the tags already in use across all projects and narrowing as you type; bulk tagging — mark several projects and add or remove a tag on all of them at once; tag management in one place — rename or delete a tag everywhere, and see how many projects carry each; and tagging a session's project from the sessions list, without going to the projects edit screen.

## Rejected paths

- Per-session tags layered on top of project tags (a session's tags being its project's plus its own), proposed to tag what a session is for — a review, a migration — and to tag sessions whose folder is not a project. Rejected on the ephemerality argument. It would also have widened the separate reboot bugfix to restore session-level tags across a reboot; with project-only tags that bugfix stays scoped to the project link.

## Open questions

- Where bulk tagging happens. Marking several items exists only on the sessions list today, where it is used to open several sessions at once.
- Whether a new project inherits the tags of the project whose folder encloses it — raised by Claude, not taken up. New projects start untagged: three carry no tag today (switchboard, acom/mach2, ~/code), all saved since 2026-10-01. The user's tags follow the folder layout: of the 34 projects inside a tagged project's folder, 32 carry exactly their parent's tags; the two exceptions are fabric/flowx/flowx-api and fabric/packages/laravel-workflow, tagged personal inside the fabric-tagged tree. Inheritance would have tagged acom/mach2 with acom with no action.
- Tag case. Tags are case-sensitive by deliberate choice ("Work" and "work" are two tags). Suggestions put near-duplicates side by side, and renaming a tag into one that already exists is a merge that has to be handled.
- Which key tagging takes: `t` opens the theme panel on both pages. The sessions page already uses Enter, Space, `/`, `k`, `r`, `s`, `t`, `m`, `x` and `?`, and new actions compete for footer space.
- Tagging an Unknown session's project from the sessions list has nowhere to put the tag until the session is linked to a project — depends on the project-link topic's answer.
- Each new surface needs designing in the current look, including the no-colour mode: the suggestion list, tag management, bulk tagging.
- The narrow-width overflow bug (inbox) touches the edit screen where tag suggestions would appear; it was left in the inbox, not brought into the epic.
