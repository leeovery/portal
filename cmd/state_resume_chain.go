package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/shellquote"
)

// The flags the resume chain's subcommands are addressed by. A pane is named
// twice: --pane is the pane id the marker writes need, --pane-key the positional
// key every hydrate record already names its pane by.
const (
	resumeFlagCommand = "command"
	resumeFlagReport  = "report"
	resumeFlagHookKey = "hook-key"
	resumeFlagPane    = "pane"
	resumeFlagPaneKey = "pane-key"
	resumeFlagWidth   = "width"
	resumeFlagHeight  = "height"
)

const (
	resumeDrawSubcommand    = "resume-draw"
	resumeWaitSubcommand    = "resume-wait"
	resumeRecoverSubcommand = "resume-recover"
)

// resumeChainPayload is everything one screen of the waiting pane needs, carried
// forward across each hand-off so no later process re-reads the store to decide
// whether to draw.
type resumeChainPayload struct {
	Command string
	Report  string
	HookKey string
	Pane    string
	PaneKey string
	Width   int
	Height  int
}

// resumeChainArgv composes one chain command's argv, emitting only the flags
// that subcommand registers: a flag a subcommand does not know fails its parse,
// and on the tail's path a failed parse closes the pane the tail exists to keep
// open.
func resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string {
	argv := []string{exe, "state", subcommand}
	if subcommand == resumeRecoverSubcommand {
		return append(argv, flagArg(resumeFlagPane), p.Pane, flagArg(resumeFlagPaneKey), p.PaneKey)
	}

	argv = append(argv, flagArg(resumeFlagCommand), p.Command)
	if p.Report != "" {
		argv = append(argv, flagArg(resumeFlagReport), p.Report)
	}
	argv = append(argv,
		flagArg(resumeFlagHookKey), p.HookKey,
		flagArg(resumeFlagPane), p.Pane,
		flagArg(resumeFlagPaneKey), p.PaneKey,
	)
	if p.Width > 0 {
		argv = append(argv, flagArg(resumeFlagWidth), strconv.Itoa(p.Width))
	}
	if p.Height > 0 {
		argv = append(argv, flagArg(resumeFlagHeight), strconv.Itoa(p.Height))
	}
	return argv
}

// shellWords renders an argv as one shell command word-for-word, so a value
// holding spaces, quotes, an expansion or a newline reaches the command it is
// composed into as the single argument it left as.
func shellWords(argv []string) string {
	words := make([]string, len(argv))
	for i, arg := range argv {
		words[i] = shellquote.Single(arg)
	}
	return strings.Join(words, " ")
}

func flagArg(name string) string {
	return "--" + name
}

var osExecutable = os.Executable

// An empty path is an error: exec'ing it would replace the process image with
// nothing. Both failures carry the whole clause, so no consumer wraps them
// again and a pane's stderr names the resolution once.
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

// resumeHandOff replaces the pane's process image with the chain's next
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

	logger.Info("exec", "target", exe, "args", strings.Join(argv, " "))
	execSelf(exe, argv)
	return nil
}

// execHandOff is the same hand-off under the same ordering rule, for the
// sites whose marker also records whether the pane carried a registered
// command.
func execHandOff(logger *slog.Logger, execShell func(prog string, args []string), prog string, args []string, hookPresent bool) {
	logger.Info("exec", "target", prog, "args", strings.Join(args, " "), "hook_present", hookPresent)
	execShell(prog, args)
}

// hookExecArgs composes the argv a pane's registered command is run as. The
// command occupies its own argv slot so sh's parser handles any embedded quotes
// - Portal never interpolates it - and the trailing exec leaves the pane on its
// own shell, so it closes on the first exit.
func hookExecArgs(command, shell string) (prog string, args []string) {
	return "/bin/sh", []string{"sh", "-c", command + "; exec " + shell}
}
