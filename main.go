// Portal is a Go CLI that provides an interactive session picker for zmx.
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

// Test seams; production never reassigns them.
var (
	executeFunc           = cmd.Execute
	errOut      io.Writer = os.Stderr
)

func main() {
	// Must run before any other portal code so every logger routes through the
	// configured handler. Both errors are tolerated: logging must never block
	// startup, and an empty stateDir degrades Init to a stderr handler.
	stateDir, _ := state.Dir()
	processRole := log.ResolveProcessRole(os.Args[1:])
	_ = log.Init(stateDir, cmd.Version(), processRole)

	code, panicked := run()

	// Close emits the terminal exit marker, so the panic path must skip it —
	// run() already emitted that path's sole marker.
	if !panicked {
		log.Close(code)
	}
	os.Exit(code)
}

// run executes the CLI inside a panic-recovering closure. It deliberately never
// exits, leaving main to own process termination.
func run() (code int, panicked bool) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				// The sole terminal marker on this path; main skips Close.
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

// classify maps an Execute error to a process exit code, printing it to stderr
// unless the error is one whose message has already been surfaced elsewhere.
func classify(err error) int {
	if _, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		// Execute already wrote the user-facing message.
		return 1
	}

	if !cmd.IsSilentExitError(err) {
		// The final diagnostic sink: a write failure here has nowhere to go.
		_, _ = fmt.Fprintln(errOut, err)
	}

	if exit, ok := errors.AsType[*cmd.ExitCodeError](err); ok {
		return exit.Code
	}
	if _, ok := errors.AsType[*cmd.UsageError](err); ok {
		return 2
	}
	return 1
}
