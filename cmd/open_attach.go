package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/runner"
	"github.com/spf13/cobra"
)

var attachLogger = log.For("attach")

// attachConnector attaches the terminal to a session through the attach
// runner, and each time the attach ends asking for the picker, opens the plain
// picker and attaches its pick, until the picker is quit or an attach ends any
// other way.
type attachConnector struct {
	cmd     *cobra.Command
	backend backend.Backend
	execer  execer
	run     func(context.Context, runner.Config) (runner.Outcome, error)
}

func newSessionConnector(cmd *cobra.Command, b backend.Backend) SessionConnector {
	c := attachConnector{cmd: cmd, backend: b, execer: realExecer{}, run: runner.Run}
	if openDeps != nil && openDeps.Execer != nil {
		c.execer = openDeps.Execer
	}
	if openDeps != nil && openDeps.Runner != nil {
		c.run = openDeps.Runner
	}
	return c
}

func (c attachConnector) Connect(name string) error {
	for {
		out, err := c.attach(name)
		if err != nil {
			return err
		}
		if !out.Picker() {
			return exitStatus(out.ExitCode)
		}
		if name, err = reopenPickerFunc(c.cmd, c.backend, flashOf(out.Err)); err != nil || name == "" {
			return err
		}
	}
}

// attach runs one attach. An exec returns only if it fails, so beside its
// error the outcome is the zero one.
func (c attachConnector) attach(name string) (runner.Outcome, error) {
	ctx := c.cmd.Context()
	command, err := c.backend.AttachCommand(ctx, name)
	if err != nil {
		return runner.Outcome{}, err
	}
	if len(command.Argv) == 0 {
		return runner.Outcome{}, fmt.Errorf("attach session %q: the backend gave no command to run", name)
	}
	// Inside a session the command switches that session's clients and exits
	// at once, which the runner refuses: Portal has nothing left to do.
	if command.Switches {
		return runner.Outcome{}, c.exec(command)
	}
	out, err := c.run(ctx, runner.Config{
		Session: name,
		Command: command,
		In:      os.Stdin,
		Out:     os.Stdout,
		Scanner: attachkeys.NewMatcher(attachBindings(name)),
		Kill:    c.kill,
	})
	if errors.Is(err, runner.ErrNoTerminal) {
		return runner.Outcome{}, c.exec(command)
	}
	return out, err
}

func (c attachConnector) exec(command backend.Command) error {
	logExecHandoff(command.Argv)
	return c.execer.Exec(command.Argv[0], command.Argv, command.Env)
}

// kill is the kill key's: the session goes as the picker's kill takes it, its
// index entry with it, though it is already gone.
func (c attachConnector) kill(ctx context.Context, name string) error {
	return pickerKiller{ctx: ctx, b: goneCountsKilled{c.backend}}.KillSession(name)
}

func flashOf(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// exitStatus ends Portal with code, silently; zero ends it with no error.
func exitStatus(code int) error {
	if code == 0 {
		return nil
	}
	return &ExitCodeError{Code: code}
}

var reopenPickerFunc = reopenPicker

// reopenPicker opens the plain picker an attach's end asked for, on flash, and
// returns its pick: empty when the picker was quit. It runs no bootstrap, which
// the first picker or command already ran.
func reopenPicker(cmd *cobra.Command, b backend.Backend, flash string) (string, error) {
	var picked string
	pick := connectorFunc(func(name string) error {
		picked = name
		return nil
	})
	err := runPicker(cmd, b, pickerLaunch{landing: pickerLanding{flash: flash}}, pick)
	return picked, err
}

// attachBindings reads the attach keys from prefs.json at each attach, so an
// edit applies from the next one. Whatever can't be used keeps its default,
// with a WARN.
func attachBindings(session string) attachkeys.Bindings {
	raw, err := loadAttachKeys()
	if err != nil {
		attachLogger.Warn("key config", "session", session, "error", err)
		return attachkeys.Defaults()
	}
	bindings, warnings := attachkeys.Parse(raw)
	for _, w := range warnings {
		attachLogger.Warn("key config", "session", session, "detail", w.String())
	}
	return bindings
}

func loadAttachKeys() (json.RawMessage, error) {
	store, err := loadPrefsStoreNoMigrate()
	if err != nil {
		return nil, err
	}
	return store.LoadKeys()
}
