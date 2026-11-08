# A Portal menu inside a session

The user drives their terminal setup almost entirely through a menu rather than key bindings: nothing has to be remembered, every action is a labelled item, bound to a single shortcut. Key chords that must be remembered are unreliable in practice, so everything they do inside a session should be reachable through a menu or through Portal.

The idea is a Portal-owned menu that does the same job: a floating menu opened from a single shortcut inside any session, styled in Portal's own Modern Vivid look, offering Portal's session actions in one place. The actions the user has in mind are renaming the session, pausing it into the waiting state described in `2026-10-05--pause-session-to-waiting-state.md`, sending it back to waiting, and whatever else turns out to belong there. Beyond the actions themselves, the goal is presence: today Portal is something you pass through on the way into a session, and once you are in, nothing of it is visible. A menu like this would make Portal an on-brand layer around the session rather than only the picker in front of it.

The user flags that this needs discussion before anything else, because it changes what Portal is. Portal is a session manager that runs, hands the terminal to the session and gets out of the way (apart from the background daemon); a shortcut that brings up a Portal menu from inside a running session means Portal has some presence inside the session that it does not have today. Whether and how that fits is the open question, not a detail.

On zmx the seam already exists: Portal's attach runner owns the pty the session is attached through and already acts on bound keys (`internal/attachkeys`), so a menu key would be one more bound key the runner handles, with the menu drawn by the runner over the session rather than by anything inside it.

Related prior material: the `lazy-resume-on-attach` discussion recorded why the overlay facilities it considered were rejected for drawing the resume panel (`.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md`, around the section on the panel's drawing surface).
