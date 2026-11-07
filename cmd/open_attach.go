package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

var attachLogger = log.For("attach")

// attachConnector attaches the terminal to a session through the attach
// runner, listed in the runner registry while it is attached, taking the
// session from every other terminal first. Each time the attach ends asking
// for the picker, it opens the plain picker, on the kill prompt for the
// session when the kill key asked, or on the screen saying another terminal
// took it, and attaches its pick; each time a switch ends it, it attaches the
// session switched to; until the picker is quit or an attach ends any other
// way. Inside a session it switches instead: through the registry when a
// runner's terminal leads the session.
type attachConnector struct {
	cmd     *cobra.Command
	backend backend.Backend
	execer  execer
	run     func(context.Context, runner.Config) (runner.Outcome, error)
	runners runnerRegistry
	// acked writes the receipt --ack asks for, once: as the first attach
	// begins, or just before an exec takes the attach's place.
	acked func()
	// takeWait bounds each take's wait for the session to have no other
	// client.
	takeWait time.Duration
}

func newSessionConnector(cmd *cobra.Command, b backend.Backend) attachConnector {
	c := attachConnector{cmd: cmd, backend: b, execer: realExecer{}, run: runner.Run, runners: stateRunners{}, acked: ackOnce(cmd), takeWait: takeWait}
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

// Connect attaches name, a session the command line named: one that can't be
// attached ends Portal with why.
func (c attachConnector) Connect(name string) error {
	return c.connect(name, c.attach)
}

// ConnectPick attaches name, a session picked in the picker, as every later
// session of the loop is attached (see attachOrPicker).
func (c attachConnector) ConnectPick(name string) error {
	return c.connect(name, c.attachOrPicker)
}

// connect attaches name through first, then loops while the picker or a
// switch follows each attach.
func (c attachConnector) connect(name string, first func(string, runnerListing) (runner.Outcome, error)) error {
	listing := c.runners.Listing(c.backend)
	defer listing.Close()
	out, err := first(name, listing)
	for err == nil {
		if out.Switch != "" {
			name = out.Switch
		} else if name, err = c.next(name, out, listing); err != nil || name == "" {
			return err
		}
		out, err = c.attachOrPicker(name, listing)
	}
	return err
}

// attachOrPicker attaches a session chosen from inside Portal: a pick, or the
// session a switch named. One that can't be attached for a fault of its own,
// as kill words them, ends as a detach does, the picker following to show
// why. Any other failure is returned, as a first attach's is.
func (c attachConnector) attachOrPicker(name string, listing runnerListing) (runner.Outcome, error) {
	out, err := c.attach(name, listing)
	if fault, ok := sessionFault(name, err, "nothing was opened"); ok {
		return runner.Outcome{Action: attachkeys.Detach, Err: fault}, nil
	}
	return out, err
}

// next is the session to attach after the attach of name ended with out
// other than by a switch: the picker's pick, the picker opening on the kill
// prompt for name when the kill key asked, and on the taken screen for name
// when another terminal took it. It is empty when Portal exits
// instead, with the error to exit with. The runner is unlisted meanwhile.
func (c attachConnector) next(name string, out runner.Outcome, listing runnerListing) (string, error) {
	listing.Close()
	if !out.Picker() {
		return "", exitStatus(out.ExitCode)
	}
	landing := pickerLanding{flash: flashOf(out.Err)}
	if out.AsksKill() {
		landing.kill = name
	}
	if out.Taken {
		landing.taken = tui.Taken{Session: name, By: out.By}
	}
	return reopenPickerFunc(c.cmd, c.backend, landing)
}

// attach runs one attach, once the take has left no other terminal on the
// session, holding the session's take lock until the attach leads it. An
// exec returns only if it fails, so beside its error the outcome is the zero
// one.
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
	}
	listing.Attach(&cfg)
	release := c.take(ctx, name)
	defer release()
	attached, ended := context.WithCancel(ctx)
	defer ended()
	listed := cfg.Began
	cfg.Began = func() {
		if listed != nil {
			listed()
		}
		c.acked()
		go c.awaitLead(attached, name, cfg.Confirm, release)
	}
	defer listing.Ended()
	began := time.Now()
	out, err := c.run(ctx, cfg)
	if errors.Is(err, runner.ErrNoTerminal) {
		release()
		return runner.Outcome{}, c.exec(command)
	}
	return c.takenAway(name, out, began), err
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

// reopenPicker opens the plain picker an attach's end asked for, landing as
// landing says, and returns its pick: empty when the picker was quit. It runs
// no bootstrap, which the first picker or command already ran.
func reopenPicker(cmd *cobra.Command, b backend.Backend, landing pickerLanding) (string, error) {
	var picked string
	pick := connectorFunc(func(name string) error {
		picked = name
		return nil
	})
	err := runPicker(cmd, b, pickerLaunch{landing: landing}, pick)
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
