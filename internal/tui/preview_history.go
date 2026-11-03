package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	previewNotAnswering       = "(session not answering)"
	previewHistoryUnavailable = "(history unavailable)"
)

// previewHistory is where the preview reads its body from.
type previewHistory struct {
	reader  HistoryReader
	ceiling time.Duration
}

func newPreviewHistory(reader HistoryReader) previewHistory {
	return previewHistory{reader: reader, ceiling: previewHistoryCeiling}
}

// body is the preview body for session: its whole history, drawn, or a line
// saying why there is none. ok is false when there is no session to preview,
// so the caller opens nothing, as it does with no reader at all.
func (h previewHistory) body(session string) (body string, ok bool) {
	if h.reader == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeoutCause(context.Background(), h.ceiling,
		fmt.Errorf("preview read got no answer in %s: %w", h.ceiling, backend.ErrTimeout))
	defer cancel()

	vt, err := h.reader.History(ctx, session, 0)
	switch {
	case err == nil:
		return renderHistory(vt), true
	case errors.Is(err, backend.ErrNoSuchSession):
		return "", false
	case errors.Is(err, backend.ErrTimeout):
		return previewNotAnswering, true
	default:
		return previewHistoryUnavailable, true
	}
}
