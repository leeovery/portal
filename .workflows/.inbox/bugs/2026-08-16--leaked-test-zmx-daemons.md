# Test zmx daemons outlive an interrupted test run, and nothing sweeps them

Test zmx daemons created by `zmxtest.New`, with the portal processes under them, can survive the test run that spawned them, and nothing reaps them afterwards.

Cleanup is purely per test: `zmxtest.New` (`internal/zmxtest/zmxtest.go`) registers a `t.Cleanup` that ends the session daemons it started and removes its `/tmp/zx-*` directory. When the test binary is killed outright (a package timeout, Ctrl-C on `go test`, a panic), `t.Cleanup` never fires. The daemons stay, reparented to PID 1, and their `/tmp/zx-*` socket directories stay with them, along with the `/tmp/ps-*` and `/tmp/rr-*` state directories of runners that registered.

Three things make this bite harder than an ordinary temp-file leak. The orphan is invisible: it lives on a private `ZMX_DIR` under `/tmp`, so it never appears in the developer's own session list and there is no reason to go looking. It adds load: leaked daemons and the portal processes under them manufacture timing flakes in the daemon-timing suites, and those flakes get blamed on the code under test. And nothing reaps it: there is no run-level or repo-level sweep of stray `/tmp/zx-*` directories, so every interrupted run adds to the pile permanently.

CLAUDE.md documents only a by-hand sweep ("Cleaning up after a test run"): list the bound processes by their `ZMX_DIR`, run `zmx kill '*'` per leaked directory, and remove the directories. The repo already guards test machinery elsewhere (`RegisterSubprocessCleanup`, `RegisterStateDirTeardownGuard`, the daemon-pgrep sandbox), but a killed test binary has no equivalent guard on its zmx daemons.

Worth settling in triage: a sweep that runs at the start of the next test run, proving ownership the way `zmxtest` does before any signal (`ZMX_DIR=<dir>` in the process's environment, or an ancestor's), so it can never reach the developer's own daemons.
