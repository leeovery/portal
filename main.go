// Portal is a Go CLI that provides an interactive session picker for tmux.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// executeFunc and errOut are test seams. Production wires executeFunc to
// cmd.Execute and errOut to os.Stderr; tests swap them via package-level
// assignment (restored with t.Cleanup), mirroring the cmd-package DI idiom.
// Tests that mutate these must not use t.Parallel().
var (
	executeFunc           = cmd.Execute
	errOut      io.Writer = os.Stderr
)

func main() {
	// log.Init must run before any other portal code so every For-created
	// logger routes through the configured handler. stateDir resolution is
	// tolerated: logging must never block startup, so a Dir() error degrades
	// to an empty stateDir (Init falls back to a stderr handler). The Init
	// error is advisory and likewise tolerated.
	stateDir, _ := state.Dir()
	processRole := log.ResolveProcessRole(os.Args[1:])
	_ = log.Init(stateDir, cmd.Version(), processRole)

	code, panicked := run()

	// Close is the process: exit marker-emitter for the non-panic path only. A
	// recovered panic deliberately skips it — run() already emitted process:
	// panic as that path's sole terminal marker, so calling Close here would
	// double-emit and break the four-way classification. os.Exit owns control flow.
	if !panicked {
		log.Close(code)
	}
	os.Exit(code)
}

// run executes the CLI inside a panic-recovering closure and returns the
// process exit code plus whether a panic was recovered. It owns no control
// flow (no os.Exit) so main remains the single owner of process termination
// and so Close can run on the non-panic path.
func run() (code int, panicked bool) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				// process: panic is the SOLE terminal marker on the recovered-panic
				// path — emitted BEFORE we set panicked so main's !panicked gate
				// skips Close (no process: exit double-emit). ERROR per the spec's
				// "line immediately preceding panic/exit -> Error"; reason carries
				// the recovered value verbatim (r is any; slog renders it). reason
				// is the cross-listed Lifecycle key, not a new Process key.
				log.For("process").Error("panic", "reason", r)
				code = 2
				panicked = true
			}
		}()

		if err := executeFunc(); err != nil {
			code = classify(err)
		}
	}()
	return code, panicked
}

// classify maps an Execute error to a process exit code, preserving the exact
// ordering and stderr-suppression contract of the original main.go:
//
//  1. *bootstrap.FatalError -> code 1, no stderr (Execute already wrote the
//     single user-facing line; duplicating it would double-print).
//  2. otherwise print the error to stderr unless it is a silent-exit sentinel
//     (cmd.ErrStatusUnhealthy / the wrapped commit-now failure).
//  3. *cmd.UsageError -> code 2.
//  4. anything else -> code 1.
func classify(err error) int {
	var fatal *bootstrap.FatalError
	if errors.As(err, &fatal) {
		// cmd.Execute already wrote fatal.UserMessage to stderr. Avoid
		// duplicating it; just exit non-zero.
		return 1
	}

	// Some errors are intentional silent-exit sentinels — e.g.
	// cmd.ErrStatusUnhealthy (status output already rendered to stdout) and
	// the wrapped errCommitNowFailed returned by `portal state commit-now`
	// (a tmux hook subprocess with nowhere meaningful to surface stderr).
	// cmd.IsSilentExitError compile-time-links the suppression contract
	// across the cmd and main packages so neither side drifts on a brittle
	// string-compare. The wrapped commit-now error still preserves the
	// underlying cause via errors.Unwrap for any consumer inspecting the chain.
	if !cmd.IsSilentExitError(err) {
		// errOut is the final diagnostic sink (stderr in production); a write
		// failure here has nowhere meaningful to surface as we head to exit.
		_, _ = fmt.Fprintln(errOut, err)
	}

	var usageErr *cmd.UsageError
	if errors.As(err, &usageErr) {
		return 2
	}
	return 1
}
