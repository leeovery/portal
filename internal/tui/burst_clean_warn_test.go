package tui

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func TestBurstRunner_ACleanThatFailsWarns(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	ack := &spawntest.FakeAckChannel{FailClean: errors.New("read-only file system")}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	burster := spawn.NewBurster(adapter, ack, func() (string, error) { return "/abs/portal", nil }, func() (spawn.WindowEnvironment, error) {
		return spawn.WindowEnvironment{Set: []string{"PATH=/usr/bin"}}, nil
	})
	runner := burstRunner{
		burster:       burster,
		ackChannel:    ack,
		sessionExists: func(string) bool { return true },
		external:      []string{"alpha"},
		trigger:       "bravo",
		logger:        logger,
	}

	runner.run(context.Background(), func(burstProgress) {})

	rec := sink.Records().WithMessage("spawn ack clean failed").AtExactLevel(slog.LevelWarn).Only(t, "clean record")
	if rec.AttrString(t, "batch") != ack.Cleaned[0] || rec.AttrString(t, "error") != ack.FailClean.Error() {
		t.Errorf("record = %+v, want the batch and the error", rec)
	}
}
