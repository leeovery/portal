package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backend"
)

// HistoryReader reads a session's screen and scrollback, as backend.Backend's
// History does.
type HistoryReader interface {
	History(ctx context.Context, name string, rows int) ([]byte, error)
}

// previewHistoryCeiling bounds the preview's read, so a wedged session holds
// the picker no longer than this before the preview says it isn't answering.
// zmx answers its whole 10,000-line history in tens of milliseconds.
const previewHistoryCeiling = time.Second

const (
	previewLoading            = "(reading history…)"
	previewNotAnswering       = "(session not answering)"
	previewHistoryUnavailable = "(history unavailable)"
	previewSessionGone        = "(session gone)"
)

// previewHistory is where the preview reads its body from.
type previewHistory struct {
	reader  HistoryReader
	ceiling time.Duration
}

func newPreviewHistory(reader HistoryReader) previewHistory {
	return previewHistory{reader: reader, ceiling: previewHistoryCeiling}
}

// previewBodyMsg carries a preview's body back from its read. Gen is the
// open it was read for, so a body that lands after the user has stepped on
// or left is dropped rather than drawn over the session now showing.
type previewBodyMsg struct {
	gen  uint64
	body string
}

// read is the command that reads and draws session's body off the update
// loop, where a 10,000-line history or a wedged session would hold every key.
func (h previewHistory) read(session string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		return previewBodyMsg{gen: gen, body: h.body(session)}
	}
}

// body is the preview body for session: its whole history, drawn, or a line
// saying why there is none.
func (h previewHistory) body(session string) string {
	ctx, cancel := context.WithTimeoutCause(context.Background(), h.ceiling,
		fmt.Errorf("preview read got no answer in %s: %w", h.ceiling, backend.ErrTimeout))
	defer cancel()

	vt, err := h.reader.History(ctx, session, 0)
	switch {
	case err == nil:
		return renderHistory(vt)
	case errors.Is(err, backend.ErrNoSuchSession):
		return previewSessionGone
	case errors.Is(err, backend.ErrTimeout):
		return previewNotAnswering
	default:
		return previewHistoryUnavailable
	}
}
