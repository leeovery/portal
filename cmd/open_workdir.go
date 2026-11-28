package cmd

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// reportDirWait bounds the find and the report of a session's work directory
// at an attach, which waits on them, and the read of the last session left's
// as Portal exits, which the exit waits on. On a daemon that answers, both
// take a few milliseconds: two Info probes, one listing of the process table
// and one Output frame waited back. Under load they take longer, so the bound
// leaves a margin of a hundred times that, yet is short enough that an attach
// to a session slow to answer, which the driver's probe ceiling would hold a
// second per call, starts without a delay the user notices.
const reportDirWait = 300 * time.Millisecond

// reportWorkDir finds the directory the session name's work runs in and
// tells the session it, so every client attached to it, and every one that
// attaches later, learns it; and returns the directory found, "" for none.
// It is best effort, within reportDirWait: a failure is logged at DEBUG and
// changes nothing else.
func reportWorkDir(ctx context.Context, b backend.Backend, name string) string {
	ctx, cancel := context.WithTimeout(ctx, reportDirWait)
	defer cancel()
	dir, err := b.WorkDir(ctx, name)
	if err != nil {
		attachLogger.Debug("dir report failed", "session", name, "detail", "read", "error", err)
		return ""
	}
	if dir == "" {
		return ""
	}
	if err := b.ReportDir(ctx, name, dir); err != nil {
		attachLogger.Debug("dir report failed", "session", name, "detail", "report", "error", err)
	}
	return dir
}
