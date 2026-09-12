# Review Tracking: Open With Forced Filter - Integrity

## Findings

### 1. The `--` separator rule is pinned by a row that fails against correct code

**Severity**: Important
**Plan Reference**: Phase 4, task 4-4 (`open-with-forced-filter-4-4`) — Do, the `TestIsTUIPath` step
**Category**: Task Self-Containment / Task Template Compliance (Tests)
**Move**: settled
**Change Type**: update-task

**Problem**:
The one property that keeps an ordinary mint line off the picker route — that a trailing command's own `/word` argument is never read as a search — is pinned by a test row the task directs the implementer to build out of helpers that cannot express it. `openProbeCmd` / `openProbeCmdWithFlags` hand back a command whose flag set has never been parsed, and an unparsed pflag set reports `ArgsLenAtDash() == -1`; the scan then reads every word it is given, so `isTUIPath` over `{"~/Code/api", "ls", "/tmp"}` answers true and the row fails against production code that is behaving correctly. The implementer meets a red test they wrote exactly as directed, and the cheapest way to make it green is to weaken the scan — at which point `x ~/Code/app -- npm start` on a cold server stops minting and opens a session search for `tmp`, on the loading page, with the command silently dropped.

**Proposal**:
State the setup the row needs: build its command by parsing the whole line and pass `c.Flags().Args()` alongside it, so the separator is recorded in the flag set the scan consults. This is determined rather than chosen — the scan reads `cmd.ArgsLenAtDash()` by the task's own design (task 1-5), pflag initialises that field to `-1` and only `Parse` moves it, and cobra hands `PersistentPreRunE` the post-parse `argWoFlags` with the `--` itself removed, so a parsed command plus its own `Args()` is the only pairing that reproduces what production sees. The other rows are unaffected: they carry no separator, where `-1` and "scan everything" agree.

**Current**:
```
- Extend `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go:28`) with the search-form rows, reusing its `openProbeCmd` / `openProbeCmdWithFlags` helpers, including a parity row asserting the same verdict for `-f port` and `/port`.
```

**Proposed Text**:
```
- Extend `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go:28`) with the search-form rows, reusing its `openProbeCmd` / `openProbeCmdWithFlags` helpers, including a parity row asserting the same verdict for `-f port` and `/port`.
- Build the `--` separator row's command by parsing the whole line rather than handing a helper a hand-written args slice — `c := openProbeCmd()`, `_ = c.ParseFlags([]string{"~/Code/api", "--", "ls", "/tmp"})`, then `isTUIPath(c, c.Flags().Args())`. Both existing helpers return a command whose flag set has never been parsed, where `ArgsLenAtDash()` is pflag's `-1` default and the scan therefore reads every word it is given, the command's own `/tmp` included; parsing is what records the separator, and `Args()` is the same post-parse slice cobra hands `PersistentPreRunE` (the `--` itself is not in it). The rows carrying no separator are unaffected and keep the plain helper call.
```

**Resolution**: Fixed
**Notes**: Applied to task open-with-forced-filter-4-4 (detail file and tick record) under auto mode; the probe-command edge-case bullet now records why the separator row must parse.
