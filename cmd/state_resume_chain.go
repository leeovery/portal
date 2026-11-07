package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/shellquote"
)

// The flags the resume chain's subcommands are addressed by. --session names
// the waiting session: the key of its resume hook and of its resume-pending
// flag, and the name the chain's records carry.
const (
	resumeFlagCommand       = "command"
	resumeFlagReport        = "report"
	resumeFlagSession       = "session"
	resumeFlagWidth         = "width"
	resumeFlagHeight        = "height"
	resumeFlagScreen        = "screen"
	resumeFlagDropInput     = "drop-input"
	resumeFlagInputArriving = "input-arriving"
)

// The screens one draw of the waiting session can put up. The panel is the
// chain's default and names no flag; any value the selector does not recognise
// draws the panel.
const (
	resumeScreenPanel   = ""
	resumeScreenDiscard = "discard"
)

const (
	resumeDrawSubcommand    = "resume-draw"
	resumeWaitSubcommand    = "resume-wait"
	resumeRecoverSubcommand = "resume-recover"
)

// resumeChainPayload is everything one screen of the waiting session needs,
// carried forward across each hand-off so no later process re-reads the store
// to decide whether to draw.
type resumeChainPayload struct {
	Command string
	Report  string
	Session string
	Width   int
	Height  int
	Screen  string

	// DropInput is set by the hand-off that opens the confirmation and
	// cleared by the draw that honours it, so a later draw of the same screen
	// never discards a key the user typed while reading it.
	DropInput bool

	// InputArriving tells a waiter its session was handed over while input
	// was still arriving, so what arrives first belongs to that input and
	// answers nothing.
	InputArriving bool
}

func (p resumeChainPayload) logAttrs(attrs ...any) []any {
	return resumeLogAttrs(p.Session, attrs...)
}

// resumeLogAttrs is how the chain's records name the waiting session.
func resumeLogAttrs(session string, attrs ...any) []any {
	return append([]any{"session", session}, attrs...)
}

func resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string {
	argv := []string{exe, "state", subcommand}
	if subcommand == resumeRecoverSubcommand {
		return append(argv, flagArg(resumeFlagSession), p.Session)
	}

	argv = append(argv, flagArg(resumeFlagCommand), p.Command)
	if p.Report != "" {
		argv = append(argv, flagArg(resumeFlagReport), p.Report)
	}
	argv = append(argv, flagArg(resumeFlagSession), p.Session)
	if p.Width > 0 {
		argv = append(argv, flagArg(resumeFlagWidth), strconv.Itoa(p.Width))
	}
	if p.Height > 0 {
		argv = append(argv, flagArg(resumeFlagHeight), strconv.Itoa(p.Height))
	}
	if p.Screen == resumeScreenDiscard {
		argv = append(argv, flagArg(resumeFlagScreen), resumeScreenDiscard)
	}
	if p.DropInput {
		argv = append(argv, flagArg(resumeFlagDropInput))
	}
	if p.InputArriving {
		argv = append(argv, flagArg(resumeFlagInputArriving))
	}
	return argv
}

func flagArg(name string) string {
	return "--" + name
}

var osExecutable = os.Executable

// An empty path is an error: exec'ing it would replace the process image with
// nothing. Both failures carry the whole clause, so no consumer wraps them
// again and a session's stderr names the resolution once.
func resumeChainExe() (string, error) {
	exe, err := osExecutable()
	if err != nil {
		return "", fmt.Errorf("resolve portal executable: %w", err)
	}
	if exe == "" {
		return "", errors.New("resolve portal executable: empty path")
	}
	return exe, nil
}

// resumeHandOff replaces this process's image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the
// exec: the writer behind it is unbuffered, so it reaches the kernel before the
// image is replaced, and a marker emitted after the exec is never written at
// all. The trailing return is unreachable in production.
func resumeHandOff(logger *slog.Logger, execSelf func(prog string, args []string), subcommand string, p resumeChainPayload) error {
	exe, err := resumeChainExe()
	if err != nil {
		return err
	}
	argv := resumeChainArgv(exe, subcommand, p)

	logger.Info("exec", "target", exe, "args", shellquote.Readable(argv))
	execSelf(exe, argv)
	return nil
}

// execHandOff is the same hand-off under the same ordering rule, for the
// sites whose marker also records whether the session carried a registered
// command.
func execHandOff(logger *slog.Logger, execShell func(prog string, args []string), prog string, args []string, hookPresent bool) {
	logger.Info("exec", "target", prog, "args", shellquote.Readable(args), "hook_present", hookPresent)
	execShell(prog, args)
}

// handOffToHookOrShell gives a session answered on the panel or recovered by
// the tail the same invocation as one that never waited. An empty command
// means no registration. A session handed /bin/sh gets it as its $SHELL too,
// as the backstop's tail has it, so one handed it in place of a $SHELL that
// can't be found, and whatever it starts, inherit a $SHELL that runs: the exec
// hands on this process's environment.
func handOffToHookOrShell(logger *slog.Logger, execShell func(prog string, args []string), command string) {
	shell := resolveShell(logger)
	if shell == fallbackShell {
		// Setenv fails only for a malformed name.
		_ = os.Setenv("SHELL", fallbackShell)
	}
	if command == "" {
		execHandOff(logger, execShell, shell, loginShellArgv(shell), false)
		return
	}
	prog, args := hookExecArgs(command, shell)
	execHandOff(logger, execShell, prog, args, true)
}

// fallbackShell is the shell a session is handed when $SHELL names none: the
// one zmx starts a fresh session's with.
const fallbackShell = "/bin/sh"

// resolveShell is the shell a session is handed: $SHELL, or else /bin/sh, as
// zmx picks a fresh session's. A $SHELL naming no path is found on PATH, as
// zmx's execvpe finds it: the exec it is handed to searches nothing. One that
// can't be found, at its path or on PATH, is /bin/sh, with one WARN: an exec
// that failed would end the session, and the saver would drop it and its
// transcript with it.
func resolveShell(logger *slog.Logger) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return fallbackShell
	}
	found, err := exec.LookPath(shell)
	if err != nil {
		logger.Warn("shell not found, so the session gets the fallback shell", "path", shell, "target", fallbackShell, "error", err)
		return fallbackShell
	}
	return found
}

// loginShellArgv is the argv a session's shell is exec'd with: a login shell,
// as zmx starts a fresh session's, named by its base name after a dash.
func loginShellArgv(shell string) []string {
	return []string{"-" + filepath.Base(shell)}
}

// loginShellFlag asks for a login shell where a shell script execs one: sh
// has no way to set the dash argv[0] zmx starts a session's shell with, and
// every shell that can log in takes this flag.
const loginShellFlag = "-l"

// resumeRegistrationOrLog answers the zero value for a registration carrying no
// command as it does for a miss, so a session with nothing to run never waits.
func resumeRegistrationOrLog(logger *slog.Logger, lookup func(hookKey string) (hooks.OnResume, error), hookKey string) hooks.OnResume {
	onResume, err := lookup(hookKey)
	if err != nil {
		logger.Debug("hook lookup", "hook_key", hookKey, "result", "error", "error", err)
		logger.Warn("lookup on-resume hook failed", "hook_key", hookKey, "error", err)
		return hooks.OnResume{}
	}
	if !onResume.Found || onResume.Command == "" {
		logger.Debug("hook lookup", "hook_key", hookKey, "result", "miss")
		return hooks.OnResume{}
	}
	logger.Debug("hook lookup", "hook_key", hookKey, "result", "hit")
	return onResume
}

// catchSIGTERM keeps a waiting session's draw and waiter running through a
// shutdown's SIGTERM, so the session is saved still waiting: a waiter that
// ended would be recovered, its flag cleared, and the saver could capture the
// panel over the session's transcript. It must stay a catch: an ignored signal
// stays ignored across exec, and an answered session's hook program and shell
// would inherit it. The relay is never read; a caught SIGTERM is dropped.
func catchSIGTERM() {
	signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)
}

// hookShellTrap keeps the shell running a resume hook alive through a SIGTERM,
// as an interactive shell is, so the session outlasts a shutdown's SIGTERM
// until its daemon ends it. It must stay a caught trap: an ignored signal
// stays ignored across exec, and the hook and the user's shell would inherit
// it. SIGHUP stays uncaught so a kill still ends the session.
const hookShellTrap = "trap : TERM; "

// hookExecArgs composes the argv a session's registered command is run as. The
// command is spliced into the -c script unquoted, so sh's own parser handles
// any embedded quotes, and the trailing exec leaves the session on its own
// login shell, so it closes on the first exit.
func hookExecArgs(command, shell string) (prog string, args []string) {
	return "/bin/sh", []string{"sh", "-c", hookShellTrap + command + "; exec " + shellquote.Single(shell) + " " + loginShellFlag}
}

// A session whose kill keys could not be given back is still handed on: a
// hook that ignores Ctrl-C is the lesser failure against a session left on its
// panel.
func enableTTYSignalsOrLog(logger *slog.Logger, enable func() error, session string) {
	if err := enable(); err != nil {
		logger.Warn("enable terminal signals failed", resumeLogAttrs(session, "error", err)...)
	}
}

func enableTTYEchoOrLog(logger *slog.Logger, enable func() error, session string) {
	if err := enable(); err != nil {
		logger.Warn("enable terminal echo failed", resumeLogAttrs(session, "error", err)...)
	}
}
