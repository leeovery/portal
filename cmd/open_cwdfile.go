package cmd

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/spf13/cobra"
)

// cwdFileFlag is open's hidden flag naming the file the x function portal init
// emits reads, once Portal exits, for the directory to leave its shell in.
const cwdFileFlag = "cwd-file"

// leftDirKey carries a run of open's leftDir on its command's context, so
// every connector the run makes, wherever it is made, records into it.
const leftDirKey contextKey = "leftDir"

// cwdStatWait bounds the check that the directory left still exists. A local
// stat answers in microseconds; one on a network mount that has hung never
// does, and Portal's exit must not wait on it.
const cwdStatWait = 200 * time.Millisecond

var cwdFileStat = os.Stat

// leftDir is one run of open's record of the last session this process
// attached itself and left, and the cwd file it writes that session's
// directory to as it exits. With no cwd file it records nothing, so a run
// without one pays nothing for it.
type leftDir struct {
	path    string
	backend backend.Backend
	// session is the last session left, "" until one is; found is the
	// directory its work ran in as its attach began.
	session, found string
	written        bool
}

// withLeftDir hands cmd's context, for this run, the leftDir it returns,
// writing to the cwd file at path.
func withLeftDir(cmd *cobra.Command, path string) *leftDir {
	left := &leftDir{path: path}
	cmd.SetContext(context.WithValue(cmd.Context(), leftDirKey, left))
	return left
}

// leftDirOf is the leftDir cmd's run records into: one with no cwd file when
// the command isn't a run of open.
func leftDirOf(cmd *cobra.Command) *leftDir {
	if ctx := cmd.Context(); ctx != nil {
		if left, ok := ctx.Value(leftDirKey).(*leftDir); ok {
			return left
		}
	}
	return &leftDir{}
}

// leave records that an attach to name through b, which began with found as
// the directory its work ran in, has ended. It reads nothing: the directory
// is read once, as Portal exits, for the last session left alone.
func (l *leftDir) leave(b backend.Backend, name, found string) {
	if l.path == "" {
		return
	}
	l.backend, l.session, l.found = b, name, found
}

// write writes the directory of the last session left to the cwd file, once,
// for the x function to change to: the directory its work runs in now, read
// within reportDirWait, else the one found as its attach began; home when
// that no longer exists; and nothing when no session was left, there is no
// directory to name, or whether it exists can't be told (see judgeDir).
// A write that fails is logged and changes nothing else.
func (l *leftDir) write(ctx context.Context) {
	if l.session == "" || l.written {
		return
	}
	l.written = true
	dir := l.dirNow(ctx)
	if dir == "" {
		return
	}
	switch judgeDir(dir) {
	case dirUnknown:
		return
	case dirGone:
		dir = resolver.ProcessHome()
	}
	if dir == "" {
		return
	}
	if err := os.WriteFile(l.path, []byte(dir), 0o600); err != nil {
		attachLogger.Warn("cwd file write failed", "path", l.path, "error", err)
	}
}

// dirNow is the directory the last session left's work runs in now, or the
// one found as its attach began when the read finds none, as for a session
// that has ended.
func (l *leftDir) dirNow(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, reportDirWait)
	defer cancel()
	if dir, err := l.backend.WorkDir(ctx, l.session); err == nil && dir != "" {
		return dir
	}
	return l.found
}

// dirVerdict is what a check of the directory left found.
type dirVerdict int

const (
	// dirUnknown is a stat that failed for another reason than the path's
	// absence, such as EACCES or ELOOP, or didn't answer within cwdStatWait.
	dirUnknown dirVerdict = iota
	dirPresent
	// dirGone is a path that no longer exists, or isn't a directory.
	dirGone
)

// judgeDir checks dir within cwdStatWait. A stat that never answers is left
// running: Portal is exiting.
func judgeDir(dir string) dirVerdict {
	result := make(chan dirVerdict, 1)
	go func() {
		info, err := cwdFileStat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			result <- dirGone
		case err != nil:
			result <- dirUnknown
		case !info.IsDir():
			result <- dirGone
		default:
			result <- dirPresent
		}
	}()
	select {
	case verdict := <-result:
		return verdict
	case <-time.After(cwdStatWait):
		return dirUnknown
	}
}
