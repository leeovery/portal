## Attempt 1

ISSUES:
- `cmd/state_resume_wait_test.go:388` — the signal guard reads `call.Args[1:]`, so a `signal.Notify(ch)` call naming no signal at all iterates zero arguments and passes green. That form is Go's "relay every signal" registration: it diverts SIGHUP, SIGTERM and SIGINT from their default disposition, which is exactly the refusal §4.3 forbids. The failure it admits is the one the guard exists to prevent — a waiter that outlives its own pane, leaving one Portal process per culled session, noticed by a user culling sessions from the picker and finding orphan `portal` processes with nothing to attach to. Task 4.6 edits this very call site to arm SIGWINCH, so the hole sits where the next hand will be. (Secondary, same line: a zero-argument callee named `Notify` would panic the slice expression rather than report — loud rather than silent, but the same fix removes it.)
  FIX: before the argument loop, fail a `Notify` call that names no signal:
  ```go
  if len(call.Args) < 2 {
      t.Errorf("%s: the wait path notifies on every signal; only syscall.SIGWINCH may be watched — the default disposition must end the waiter when tmux tears the pane down",
          source.Position(call.Pos()))
      return true
  }
  ```
  ALTERNATIVE: additionally reject the callee names `Ignore` and `Reset`, which decline a signal without a `Notify` at all. The acceptance criterion names `signal.Notify` alone, so this is a widening rather than a correction — worth taking in the same edit since it is one more clause in the existing `CalleeName` check, but the `len(call.Args) < 2` arm is the part that closes the stated criterion.
  CONFIDENCE: high

NOTES:
- The guard scans `state_resume_wait.go` only (`PackageSource(t, ".", "state_resume_wait.go")`). That is correct while the wait path is one file; if 4.6's resize seam lands in a sibling file the guard's reach has to move with it, or the property goes unpoliced without anything going red.
- `Stdout` is carried and wired to `os.Stdout` but written by nothing on this path. It is the seam the "writes nothing while waiting" assertion observes and the task's field list names it, so it is deliberate rather than dead.
- The "unbuffered writer" claim in the comment above the exec INFO (`cmd/state_resume_wait.go:105-106`) checks out against `internal/log/sink.go:68-71`, which states the same property at the source and forbids wrapping it.
- No comment corrections: every comment the diff introduced carries something the code cannot (the field's population contract, the non-tty spin, the one-byte-no-read-ahead rule, the exec-ordering constraint), and no claim the code falsifies was found.
- Coverage observations that did not rise to issues: (a) the "a `MakeRaw` that succeeded followed by any later failure" restore path is covered only through the read-error case, since `resumeChainExe()` has no seam to fail through — the `defer` makes it structural anyway; (b) the no-timer assertion is a 150ms idle window, which is the pragmatic form the task asked for and would not catch a long-armed timer, but the file imports no `time` at all, so the property is visible statically.
