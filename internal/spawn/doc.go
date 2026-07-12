// Package spawn is Portal's shared host-terminal spawn service: it detects the
// host terminal a Portal process is running under, resolves that terminal to a
// window-spawning adapter, and drives the adapter to open new terminal windows
// for restored sessions.
//
// It is a single service reached in-process by two callers — the TUI picker
// (which spawns windows as part of its restore/attach flow) and the `portal
// spawn` CLI (the thin command that mirrors the picker's commit and exposes the
// `--detect` dry-run). Both callers share one detection and resolution path so
// their behaviour cannot drift.
//
// Detection produces an Identity: the host terminal's macOS bundle id plus a
// friendly display name, or a NULL identity when there is no host-local
// terminal (a remote/mosh client, or an unsupported/transient outcome).
// Adapter resolution matches an Identity's bundle id against a bundle-id family
// glob. Phase 1 of the feature provides only the Identity value type and the
// standalone family-matching primitive; the process-tree walk, the env
// fast-path, the inside-tmux client enumeration, the detect orchestrator, and
// the adapter layer are layered in by later phases.
package spawn
