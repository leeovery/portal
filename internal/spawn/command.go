package spawn

// ExecutableResolver resolves the picker's own binary path. It is the seam over
// os.Executable so command composition is unit-testable without depending on the
// real running binary; production callers pass os.Executable directly.
type ExecutableResolver func() (string, error)

// composeAttachArgv is the pure builder for the env-self-sufficient attach argv.
// It returns a real argv (never shell syntax), each fragment load-bearing:
//
//   - /usr/bin/env … : prefixes a minimal explicit env in front of the exec.
//   - -u TMUX -u TMUX_PANE : the explicit strip. A picker triggered from INSIDE
//     tmux must not leak TMUX/TMUX_PANE into the spawned window, or its `portal
//     attach` would take the switch-client path instead of a clean out-of-tmux
//     exec-attach — the spawned N−1 MUST run out of tmux.
//   - PATH=<path> : the ONLY injected var — no whole-env snapshot. PATH is the
//     sole var the spawned attach needs to find tmux in a bare host env.
//   - exePath : the picker's own absolute binary (not a bare "portal" PATH
//     lookup) so the version-gated warm-command latch stays satisfied and each
//     spawned attach takes the abridged fast-path.
//   - attach, session : the session is a discrete argv element, so a name with a
//     space never needs shell quoting.
//   - --spawn-ack <batch>:<token> : TWO discrete argv elements (never a joined
//     "--spawn-ack=<value>" and never quoted). The colon-joined value is
//     unambiguous because option-safe ids are colon-free (see
//     FormatSpawnAckFlag).
func composeAttachArgv(exePath, path, session, batch, token string) []string {
	return []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"PATH=" + path,
		exePath, "attach", session,
		"--spawn-ack", FormatSpawnAckFlag(batch, token),
	}
}
