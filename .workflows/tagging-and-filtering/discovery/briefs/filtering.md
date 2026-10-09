# Discovery Brief — Filtering

Drawn from discovery session 001.

## Soft decisions

- In scope, all accepted by the user: narrowing the sessions list to one or more tags, with a choice between matching any of them and matching all of them; narrowing to a project; excluding a tag ("everything except archive"); keeping the group headings while filtering (today typing a filter flattens the list); and showing the active filter as chips in the header so it is visible what is narrowing the list.
- A tag filter matches through projects: tags live on projects only, and a session carries its project's tags.
- The picker row rework (each session's folder beside its name, the right-hand status strip) stays a separate roadmap item. Display work tied to filtering, such as the chips, belongs here.

## Rejected paths

(none)

## Open questions

- How the new filters sit beside the existing `/` text filter and the `s` grouping cycle (Flat, By Project, By Tag) — one more control that stacks on them, or merged with them — including how a tag filter relates to the By Tag view.
- Whether a filter persists between launches, as the grouping mode does.
- Whether the projects list filters too, or only the sessions list — the user said "projects / sessions".
- Whether typing a tag name in the `/` text filter matches that tag's sessions.
- Whether a project filter takes in projects nested inside the chosen one (Folio and its api and sdk) — decided in the project-link topic.
- Sessions with no project drop out of any tag or project filter, so filtering is only as reliable as the session-to-project link and the separate reboot bugfix.
- Keys and footer space for the filter controls; the sessions page already uses Enter, Space, `/`, `k`, `r`, `s`, `t`, `m`, `x` and `?`.
- Filter chips add a row to the same vertical space where the keymap footer already disappears on short terminals (ten rows or fewer), and the section header where chips would go overflows at very narrow widths. Both are inbox bugs left out of the epic.
- Designing the chips and filter controls in the current look, including the no-colour mode.
- The switch to a grouped view is slow because each session's folder is looked up one at a time; project and tag filtering take the same path. Most of it goes once the reboot fix lands.
