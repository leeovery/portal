## missing-projects
summary: Seeing a project whose folder has gone, and pointing it at its new folder from the projects page
description: |
  Project-link decided that a tagged project whose folder disappears — renamed, moved or deleted — is kept and marked missing on the projects page, where the user points it at a new folder or deletes it; re-pointing carries its tags and sessions, merges it with a project already at that folder, renames it when its name still follows the old folder, and rewrites the recorded folder of every session started under it. Tagging then counts missing projects toward their tags and marks them in a tag's checklist. Neither discussion decided the surface the repair happens on: Portal has no way today to enter or choose a folder — projects are only ever saved by starting a session — so how the user gives the new folder is open (typing a path, or picking from the folders the project's own sessions now sit in, the way the link chooser already offers a session pane's current folder after a rename). Also open: which key re-points on the projects page (tagging's key decisions cover `t`, `c` and `l` only), how a missing project is marked there and wherever else projects are listed (the link chooser reuses the projects list), what Enter — start a session — and edit do on a project whose folder is gone, and whether merging into a project already at the chosen folder asks first, as renaming one tag into another does.

  Without this, the keep-tagged rule protects the user's tags from silent deletion but leaves no way to get them back onto the renamed folder, so the goosemates case discovery started from would still end with sessions under a missing project and nothing to do but delete it. The surface needs designing in Paper alongside tagging's new surfaces, under the same no-colour rule.
routing: discussion
source: gap-analysis
source_artifacts: project-link.md, tagging.md
gap_type: emergent
