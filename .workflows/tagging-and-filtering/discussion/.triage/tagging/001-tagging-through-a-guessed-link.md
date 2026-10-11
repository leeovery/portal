### Tagging a session's project from the sessions list when its project link is a guess
*From: project-link · discussion · 2026-10-11*

The project-link discussion decided that a session with no recorded project keeps a live guess of its project — the folder its active pane is in now, resolved to the repository it belongs to — for its heading, its tags and the tag and project filters. A wrong guess was accepted as silent because it corrects itself: when the pane moves back, the guess follows. The picker also gains a "link to project" action on every session row, which records the session's project by the user's choice and replaces any guess; its chooser is the projects list as the projects page draws it, with the session's current project marked and labelled as a guess when it is one.

Tagging a session's project from the sessions list (this topic's flow) writes a tag onto the session's project. When that project is a guess, the tag lands durably on whatever project the guess chose. Live example on 2026-10-11: `laravel-workflow-PGR3jA`'s pane sits in `~`, so its guess is the home-folder project; tagging it from the sessions list would tag the home-folder project — and with it every session the guess files there — while the session itself belongs to `~/Code/fabric/packages/laravel-workflow`. Unlike the guess, the tag does not correct itself when the pane moves.

Options in sight from project-link: treat a guessed link like a recorded one; route the tag action through the link chooser first when the link is a guess (the user confirms the project, which records it, then tags); or refuse with a pointer to linking. How the sessions-list tagging flow invokes the link action, and which key it takes, were already left to this topic.

The ask: whether sessions-list tagging treats a session whose project is a guess the same as one whose project is recorded.
