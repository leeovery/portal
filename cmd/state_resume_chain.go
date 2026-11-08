package cmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	resumeFlagTyped         = "typed"
	resumeFlagRepliesOwed   = "replies-owed"
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

	// DropInput is set by the hand-off that opens the confirmation, and by the
	// helper that parks the session, since input queued before a screen is
	// first drawn answers nothing on it, and cleared by the draw that honours
	// it, so a later draw of the same screen never discards a key the user
	// typed while reading it.
	DropInput bool

	// InputArriving tells a waiter its session was handed over while input
	// was still arriving, as a draw whose input drop failed hands it over, or
	// one that read more than a key of it, so what arrives first belongs to
	// that input and answers nothing.
	InputArriving bool

	// Typed is the key a draw read while it asked the terminal its background,
	// handed to the waiter as the first byte of an arrival, so it answers as
	// it would have had it been typed after the draw. A draw hands on one byte
	// at most here, and more as InputArriving, so nothing pasted is ever
	// carried in the waiter's argv.
	Typed string

	// RepliesOwed is the background questions draws gave up on with no reply
	// read, as a terminal over a slow link answers late. It rides each
	// hand-off, so the hand-off to a hook or a shell reads the replies off
	// while one is owed.
	RepliesOwed replyDebt
}

// resumeReplyWindow is how long a background question is owed its reply. Only
// a terminal attached when it was asked answers it, within its round trip, and
// a phone over SSH answers well within this, so a question older than it is
// owed nothing.
const resumeReplyWindow = time.Second

// replyDebtKept bounds a debt to its newest questions. More go unanswered
// within one window only while the terminal answers none, as a drag's redraws
// can, and the newest is the one the wait before a hand-off lasts for.
const replyDebtKept = 4

// replyDebt is the background questions a session's draws asked and have read
// no reply to, each by when it was asked, in unix milliseconds, oldest first.
// A reply read settles the newest question owed: a terminal answers in the
// order it is asked, but only a terminal that saw a question answers it, so
// one asked as a terminal came or went may never be answered, and settled
// oldest first, it would take the next real reply and leave the question that
// reply answered owed in its place. Settled newest first, a question never
// answered is owed only until its own window closes. An array, not a slice, so
// a payload compares whole.
type replyDebt struct {
	asked [replyDebtKept]int64
	n     int
}

// replyDebtFrom is the debt of the questions asked, oldest first, keeping the
// newest replyDebtKept.
func replyDebtFrom(asked []int64) replyDebt {
	var d replyDebt
	d.n = copy(d.asked[:], asked[max(0, len(asked)-replyDebtKept):])
	return d
}

// replyDebtFromFlag reads the replies-owed flag's comma-separated times, a
// time later than now read as now, as live reads one. A value Portal didn't
// write is read as one question asked now, so a reply it may stand for is
// still waited for, a window at most.
func replyDebtFromFlag(value string, now time.Time) replyDebt {
	if value == "" {
		return replyDebt{}
	}
	fields := strings.Split(value, ",")
	asked := make([]int64, len(fields))
	for i, field := range fields {
		ms, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return replyDebt{}.owe(now)
		}
		asked[i] = min(ms, now.UnixMilli())
	}
	return replyDebtFrom(asked)
}

func (d replyDebt) owing() bool {
	return d.n > 0
}

func (d replyDebt) count() int {
	return d.n
}

// owe adds a question asked at at, letting the oldest go past the bound.
func (d replyDebt) owe(at time.Time) replyDebt {
	if d.n == replyDebtKept {
		d = replyDebtFrom(d.asked[1:d.n])
	}
	d.asked[d.n] = at.UnixMilli()
	d.n++
	return d
}

// settle drops the k newest questions, one for each reply read.
func (d replyDebt) settle(k int) replyDebt {
	return replyDebtFrom(d.asked[:d.n-min(max(k, 0), d.n)])
}

// live drops the questions asked a window or more before now, which are owed
// nothing, and reads a question the clock says is yet to be asked as asked
// now, so a clock set back, or a time Portal didn't write, keeps none owed
// for longer than a window.
func (d replyDebt) live(now time.Time) replyDebt {
	var kept replyDebt
	for _, asked := range d.asked[:d.n] {
		asked = min(asked, now.UnixMilli())
		if now.Sub(time.UnixMilli(asked)) < resumeReplyWindow {
			kept.asked[kept.n] = asked
			kept.n++
		}
	}
	return kept
}

// lastDue is when the newest question's window closes.
func (d replyDebt) lastDue() time.Time {
	return time.UnixMilli(d.asked[d.n-1]).Add(resumeReplyWindow)
}

func (d replyDebt) flagValue() string {
	asked := make([]string, d.n)
	for i, a := range d.asked[:d.n] {
		asked[i] = strconv.FormatInt(a, 10)
	}
	return strings.Join(asked, ",")
}

func (p resumeChainPayload) logAttrs(attrs ...any) []any {
	return resumeLogAttrs(p.Session, attrs...)
}

// carryTyped hands the waiter what a draw read as typed: a key alone as Typed,
// the byte it answers as, and any more as input still arriving, which answers
// nothing, as a paste read whole would not.
func (p *resumeChainPayload) carryTyped(typed []byte) {
	if key, ok := resumeKeyIn(typed); ok {
		p.Typed = string(key)
		return
	}
	if len(typed) > 0 {
		p.InputArriving = true
	}
}

// typedFromFlag reads the typed flag's hex. A value that isn't one byte's is
// input the waiter can't name, so it is taken as input still arriving and
// answers nothing.
func typedFromFlag(hexed string) (typed string, arriving bool) {
	if hexed == "" {
		return "", false
	}
	key, err := hex.DecodeString(hexed)
	if err != nil || len(key) != 1 {
		return "", true
	}
	return string(key), false
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
	if p.RepliesOwed.owing() {
		argv = append(argv, flagArg(resumeFlagRepliesOwed), p.RepliesOwed.flagValue())
	}
	// Only the waiter takes a key: a draw handed one would refuse its argv,
	// and the session would lose its panel to the chain's recovery.
	if p.Typed != "" && subcommand == resumeWaitSubcommand {
		argv = append(argv, flagArg(resumeFlagTyped), hex.EncodeToString([]byte(p.Typed)))
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
