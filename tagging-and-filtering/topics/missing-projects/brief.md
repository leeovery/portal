# Discovery Brief — Missing Projects

Drawn from the discovery-gap analysis, read out of project-link.md, tagging.md.

## The gap

Project-link decided that a tagged project whose folder disappears — renamed, moved or deleted — is kept and marked missing on the projects page, where the user points it at a new folder or deletes it; re-pointing carries its tags and sessions, merges it with a project already at that folder, renames it when its name still follows the old folder, and rewrites the recorded folder of every session started under it. Tagging then counts missing projects toward their tags and marks them in a tag's checklist. Neither discussion decided the surface the repair happens on: Portal has no way today to enter or choose a folder — projects are only ever saved by starting a session — so how the user gives the new folder is open (typing a path, or picking from the folders the project's own sessions now sit in, the way the link chooser already offers a session pane's current folder after a rename). Also open: which key re-points on the projects page (tagging's key decisions cover `t`, `c` and `l` only), how a missing project is marked there and wherever else projects are listed (the link chooser reuses the projects list), what Enter — start a session — and edit do on a project whose folder is gone, and whether merging into a project already at the chosen folder asks first, as renaming one tag into another does.

Without this, the keep-tagged rule protects the user's tags from silent deletion but leaves no way to get them back onto the renamed folder, so the goosemates case discovery started from would still end with sessions under a missing project and nothing to do but delete it. The surface needs designing in Paper alongside tagging's new surfaces, under the same no-colour rule.

## Why it matters

An emergent gap: project-link's keep-tagged rule introduced a repair that no topic gives a surface, keys or design to. Without it, a renamed folder's project keeps its tags but can only be deleted, so the rule protects the tags without ever returning them to use.

## Open questions

- How the user gives a missing project its new folder — a typed path, a pick from the folders the project's sessions now sit in, or both.
- Which key re-points a project on the projects page, and whether it shows in the footer or only in help.
- How a missing project is marked on the projects page, in the link chooser (which reuses the projects list), and consistently with a tag checklist's missing marker, including under no-colour.
- What starting a session (Enter) and edit do on a project whose folder is gone.
- Whether re-pointing onto a folder that is already a project asks before merging, and what it shows.
- What re-pointing does when the chosen folder is itself inside a worktree, another project, or a bare repository.
