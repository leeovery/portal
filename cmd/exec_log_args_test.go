package cmd

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// An exec marker's args attr renders the argv shell-quoted, so where an
// argument holding a space ends stays readable.
func TestExecLogArgs_QuoteAnArgumentHoldingASpace(t *testing.T) {
	t.Run("the attach hand-off's marker", func(t *testing.T) {
		sink := logtest.Install(t)

		logExecHandoff([]string{"zmx", "attach", "my session"})

		got := sink.Records().Matching("process", "exec").Only(t, "process exec record").AttrString(t, "args")
		if want := "attach 'my session'"; got != want {
			t.Errorf("args = %q, want %q", got, want)
		}
	})

	t.Run("a failed exec's WARN", func(t *testing.T) {
		sink := logtest.Install(t)
		withFuncSeam(t, &hydrateLogger, slog.New(sink).With("component", "hydrate"))
		withOsExitFake(t, func(int) { panic("osExit invoked") })

		func() {
			defer func() { _ = recover() }()
			defaultExecShell("/nonexistent/portal-exec-args-probe", []string{"sh", "-c", "echo hi"})
		}()

		got := sink.Records().WithMessage("exec handoff failed").Only(t, "the exec failure WARN").AttrString(t, "args")
		if want := "sh -c 'echo hi'"; got != want {
			t.Errorf("args = %q, want %q", got, want)
		}
	})
}
