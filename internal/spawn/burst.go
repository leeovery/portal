package spawn

// SpawnOutcome pairs a session with the Result of attempting to open its
// external host-terminal window. SpawnWindows returns one per window it tried,
// in list order, so the caller can report per-session detail and name the
// failed session.
type SpawnOutcome struct {
	Session string
	Result  Result
}

// SpawnWindows opens one host-terminal window per session, sequentially in list
// order, each running the composed env-self-sufficient attach argv through
// adapter.OpenWindow. It is the N−1 external half of the spawn burst; the Nth
// self-attach is the caller's concern.
//
// The picker's own executable is resolved ONCE up front (via exe): an
// unresolvable executable aborts the whole burst before any window opens
// (return nil, err). PATH is likewise read once (via getenv) and each per-session
// argv is composed from those two fixed values, so every window still runs the
// exact env-self-sufficient attach form without re-resolving the executable.
//
// Iteration is strictly sequential — one OpenWindow completes before the next
// fires — and stops on the first non-success Result, returning the outcomes
// collected so far with the failed one last. An empty sessions slice (the N=1
// external set) is a no-op returning (nil, nil) without resolving the executable.
func SpawnWindows(adapter Adapter, sessions []string, exe ExecutableResolver, getenv func(string) string) ([]SpawnOutcome, error) {
	if len(sessions) == 0 {
		return nil, nil
	}

	exePath, err := exe()
	if err != nil {
		return nil, err
	}
	path := getenv("PATH")

	outcomes := make([]SpawnOutcome, 0, len(sessions))
	for _, session := range sessions {
		argv := composeAttachArgv(exePath, path, session)
		result := adapter.OpenWindow(argv)
		outcomes = append(outcomes, SpawnOutcome{Session: session, Result: result})
		if !result.OK() {
			break
		}
	}
	return outcomes, nil
}
