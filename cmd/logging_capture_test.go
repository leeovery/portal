package cmd

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

func newCaptureLoggerForComponent(t *testing.T, component string) (*slog.Logger, *logtest.Sink) {
	t.Helper()
	sink := &logtest.Sink{}
	return slog.New(sink).With("component", component), sink
}
