# Restore reopened windows to their origin macOS Space

Follow-up to the `restore-host-terminal-windows` feature, which reopens and re-attaches the terminal windows that were attached on this host after a reboot. That feature deliberately stops at reopening the windows — it does not control *where* on the desktop they land.

This idea extends it: when Portal re-spawns a terminal window, drop it back onto the macOS Space it was originally on. The motivation comes straight from how the user works — they run roughly 14 macOS Spaces, one project "zone" per Space, with a few sessions per window. After a crash/reboot, even once the windows reopen, they'd still be piled onto whatever Space the terminal happens to open on, so the user would have to drag each one to the right zone by hand. Placing reopened windows back on their origin Space would complete the host-state restore and remove the last manual step in rebuilding a working layout.

This was split off from the main feature on purpose because it is **very Mac-specific**. Reopening windows is comparatively portable; binding a window to a particular macOS Space is a different, platform-bound problem (macOS Spaces are notoriously awkward to address programmatically — historically private/undocumented APIs, with the usual workarounds being AppleScript, window-manager tooling like yabai, or accessibility APIs). Feasibility is genuinely unknown and should be researched on its own rather than dragging the windows-reopen work into Spaces territory.

Dependencies and open questions to revisit when this is picked up:
- It builds on whatever host-local tracking `restore-host-terminal-windows` establishes — the per-window record would need to also capture which Space the window was on at snapshot time.
- How reliably can a Space be identified and re-targeted across reboots, given Spaces have no stable user-facing IDs and can be reordered?
- Should this stay Ghostty-specific or generalise across terminals?

Deferred from the `restore-host-terminal-windows` discovery session as a separate job, to be shaped on its own once the windows-reopen capability exists.
