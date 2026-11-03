package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

var attachLogger = log.For("attach")

// attachConnector attaches the terminal to a session through the attach
// runner, listed in the runner registry while it is attached. Each time the
// attach ends asking for the picker, it opens the plain picker and attaches
// its pick; each time a switch ends it, it attaches the session switched to;
// until the picker is quit or an attach ends any other way. Inside a session
// it switches instead: through the registry when a runner's terminal leads
// the session.
type attachConnector struct {
	cmd     *cobra.Command
	backend backend.Backend
	execer  execer
	run     func(context.Context, runner.Config) (runner.Outcome, error)
	runners runnerRegistry
	// acked writes the receipt --ack asks for, once: as the first attach
	// begins, or just before an exec takes the attach's place.
	acked func()
}

func newSessionConnector(cmd *cobra.Command, b backend.Backend) SessionConnector {
	c := attachConnector{cmd: cmd, backend: b, execer: realExecer{}, run: runner.Run, runners: stateRunners{}, acked: ackOnce(cmd)}
	if openDeps != nil && openDeps.Execer != nil {
		c.execer = openDeps.Execer
	}
	if openDeps != nil && openDeps.Runner != nil {
		c.run = openDeps.Runner
	}
	if openDeps != nil && openDeps.Runners != nil {
		c.runners = openDeps.Runners
	}
	return c
}

func (c attachConnector) Connect(name string) error {
	listing := c.runners.Listing(c.backend)
	defer listing.Close()
	out, err := c.attach(name, listing)
	for err == nil {
		if out.Switch != "" {
			out, err = c.attachSwitched(out.Switch, listing)
			continue
		}
		if name, err = c.next(out, listing); err != nil || name == "" {
			return err
		}
		out, err = c.attach(name, listing)
	}
	return err
}

// attachSwitched attaches the session a switch named. One gone by then ends
// as a failed kill does: the picker follows, showing why. Any other failure
// is returned, as a first attach's is.
func (c attachConnector) attachSwitched(name string, listing runnerListing) (runner.Outcome, error) {
	out, err := c.attach(name, listing)
	if errors.Is(err, backend.ErrNoSuchSession) {
		return runner.Outcome{Action: attachkeys.Detach, Err: err}, nil
	}
	return out, err
}

// next is the session to attach after an attach that ended with out other
// than by a switch: the picker's pick. It is empty when Portal exits instead,
// with the error to exit with. The runner is unlisted meanwhile.
func (c attachConnector) next(out runner.Outcome, listing runnerListing) (string, error) {
	listing.Close()
	if !out.Picker() {
		return "", exitStatus(out.ExitCode)
	}
	return reopenPickerFunc(c.cmd, c.backend, flashOf(out.Err))
}

// attach runs one attach. An exec returns only if it fails, so beside its
// error the outcome is the zero one.
func (c attachConnector) attach(name string, listing runnerListing) (runner.Outcome, error) {
	ctx := c.cmd.Context()
	command, err := c.backend.AttachCommand(ctx, name)
	if err != nil {
		return runner.Outcome{}, err
	}
	if len(command.Argv) == 0 {
		return runner.Outcome{}, fmt.Errorf("attach session %q: the backend gave no command to run", name)
	}
	// Inside a session the command switches that session's clients and exits
	// at once, which the runner refuses: the terminal moves through switchTo.
	if command.Switches {
		return runner.Outcome{}, c.switchTo(name, command)
	}
	cfg := runner.Config{
		Session: name,
		Command: command,
		In:      os.Stdin,
		Out:     os.Stdout,
		Scanner: attachkeys.NewMatcher(attachBindings(name)),
		Kill:    c.kill,
	}
	listing.Attach(&cfg)
	listed := cfg.Began
	cfg.Began = func() {
		if listed != nil {
			listed()
		}
		c.acked()
	}
	defer listing.Ended()
	out, err := c.run(ctx, cfg)
	if errors.Is(err, runner.ErrNoTerminal) {
		return runner.Outcome{}, c.exec(command)
	}
	return out, err
}

// switchTo moves the terminal this command was typed in to name, from inside
// a session: through the runner whose terminal leads the session, else by the
// command, which zmx routes to the leader. The session it was typed in needs
// no move, and gets none: a switch would detach and attach it again.
func (c attachConnector) switchTo(name string, command backend.Command) error {
	ctx, current := c.cmd.Context(), c.backend.Current()
	if name == current {
		return nil
	}
	if err := c.runners.Switch(ctx, c.leader(ctx, current), current, name); err == nil {
		return nil
	}
	return c.exec(command)
}

// leader is the registry token the client leading session carries: empty for
// a raw client, no client leading, or a leader the backend can't tell.
func (c attachConnector) leader(ctx context.Context, session string) string {
	leader, err := runnertoken.ReadLeader(ctx, c.backend, session)
	if err != nil {
		attachLogger.Warn("leader unknown", "session", session, "error", err)
	}
	return leader.Token
}

func (c attachConnector) exec(command backend.Command) error {
	c.acked()
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

// spawnAck is the batch and token of the receipt the line's --ack names, and
// whether they are ones a burst could have minted.
func spawnAck(cmd *cobra.Command) (batch, token string, ok bool) {
	value, _ := cmd.Flags().GetString("ack")
	return spawn.ParseSpawnAckFlag(value)
}

// ackOnce writes the line's --ack receipt the first time it is called, and
// does nothing for a line with none. A failed write still attaches, which the
// burst counts as a window that failed rather than one left unaccounted for.
func ackOnce(cmd *cobra.Command) func() {
	batch, token, ok := spawnAck(cmd)
	if !ok {
		return func() {}
	}
	writer := buildAckWriter()
	return sync.OnceFunc(func() {
		if err := writer.Write(batch, token); err != nil {
			spawnLogger.Warn("spawn ack write failed", "batch", batch, "error", err)
		}
	})
}

func buildAckWriter() spawn.AckWriter {
	if openDeps != nil && openDeps.AckWriter != nil {
		return openDeps.AckWriter
	}
	return stateAcks{}
}
