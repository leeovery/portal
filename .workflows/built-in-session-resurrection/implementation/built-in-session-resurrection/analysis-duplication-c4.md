---
agent: duplication
cycle: 4
findings_count: 1
---
# Duplication Analysis (Cycle 4)

## Summary

One medium-severity twin-instance finding around the cmd/tui BootstrapWarning emission pair; everything else is out-of-scope, already deferred in c3, or below threshold.

---

## Findings

### FINDING: BootstrapWarning emission and shape duplicated across cmd/ and internal/tui/
- **Severity**: medium
- **Files**: `cmd/bootstrap_warnings.go:55-61`, `internal/tui/bootstrap_warnings.go:38-44`, `cmd/bootstrap/errors.go:49-51`, `internal/tui/bootstrap_warnings.go:19-21`
- **Description**: Two structural twins introduced together by T6-9 / T6-10 and now in lock-step:
  1. `EmitTo` (`cmd/bootstrap_warnings.go:55-61`) and `WriteBootstrapWarnings` (`internal/tui/bootstrap_warnings.go:38-44`) are byte-identical nested loops — `for _, warn := range …; for _, line := range warn.Lines; fmt.Fprintln(w, line)`. Both carry explicit mirror comments — `WriteBootstrapWarnings` says it "mirrors cmd.BootstrapWarningsSink.EmitTo so the CLI path and the TUI path produce identical stderr output for the same warnings." A single Fprintln→Fprintf change for color/prefix/severity must be applied in both bodies.
  2. `cmd/bootstrap.Warning` (`errors.go:49`) and `tui.BootstrapWarning` (`bootstrap_warnings.go:19`) are byte-identical struct shapes (`{Lines []string}`) sitting either side of the cmd→tui import boundary. `drainBootstrapWarningsForTUI` (`cmd/bootstrap_warnings.go:78-88`) is a pure O(n) field-copy whose only job is to bridge them. Either type gaining a Severity/Code/Component field forces the addition twice plus an update to the conversion.
- **Recommendation**: Hoist the Warning shape into a small leaf package (e.g. `internal/warning`) holding the struct and a single `WriteLines(io.Writer, []Warning)`. `cmd/bootstrap` can alias the type for its existing callers; both `cmd/bootstrap_warnings.go` and `internal/tui/bootstrap_warnings.go` consume the shared helper. `drainBootstrapWarningsForTUI`'s conversion copy disappears.
