# The cmd/bootstrap integration lane fails whole-package with no established cause

Running `go test -tags integration -p 1 ./cmd/bootstrap` fails the package, and nobody has established why. Three agents hit it independently during phase 9 of the resume-hooks-silently-lost work, and all three reported the same shape.

The failing set moves between runs. One run failed on `TestCompositeBootstrap_FObservables`. Another, minutes later on the same tree, failed on a different three: `FreshAcquireDaemonLockRefusesPostBootstrap`, `ScrollbackDirPathSetStableAcross10Observations` and `ExternalSaverKillTriggersSelfEject`. Every one of those named tests passes when run in isolation, on both the edited tree and a stashed one.

The symptom in each case is the saver readiness stall — the assertion reports `saver daemon did not come up: pid N is dead`.

It predates the phase-9 work. One agent stashed all local changes and re-ran at HEAD; the package still failed, on a different test than the run with its edits applied. A second agent, whose change was comment-only, established structurally that its diff could not reach the failure at all — Go does not compile comments, and no comment-reading guard in the repo scans that package.

Machine load is the leading hypothesis and is not yet more than a hypothesis. One failing run was measured at load average 47 across 10 cores; a later check during this session showed 23. That is consistent with contention deciding the verdict, and CLAUDE.md already documents this failure class for the daemon-timing suites — which is precisely why `-p 1` is load-bearing on this lane. But no one has run the package on an idle machine, so the hypothesis is untested in both directions.

One detail rules out a nearby suspect. The daemon involved is respawned as a bare binary through `respawn-pane` (`internal/tmux/portal_saver.go`), with no shell in the chain, so none of the shell-environment pinning in `internal/portaltest/isolated_env.go` can reach it. The isolation work landed in that file during the same phase is not implicated.

Two further pieces of context. Reviewers deliberately declined to re-run this lane during the phase, on the grounds that doing so spawns the daemons and tmux servers whose leakage manufactures exactly this class of failure — so the observations above come from runs made for other reasons. And the machine carries debris from earlier runs: 171 `portaltest-self-sandbox-*` directories and 21 `ptl-bin-*` build directories in `$TMPDIR` at the time of writing.

The next step is a clean run on an idle machine. If the package passes repeatedly, the budget is too tight under load and the fix is a bounded wait. If it still fails, there is a real race that load merely exposes, and the readiness barrier is where to start looking.
