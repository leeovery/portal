package cmd

import (
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/spf13/cobra"
)

// ackRecorder records each receipt written, and how far the attach had got
// as it was: the runs begun and the execs made.
type ackRecorder struct {
	rec    *attachRecorder
	err    error
	writes []ackWrite
}

type ackWrite struct {
	receipt     string
	runs, execs int
}

func (a *ackRecorder) Write(batch, token string) error {
	a.writes = append(a.writes, ackWrite{receipt: batch + ":" + token, runs: len(a.rec.runs), execs: len(a.rec.execs)})
	return a.err
}

// stageAck stages attaches of api-x7Kd9a and blog-c3d4, through the
// recorder, from an open line carrying --ack value, and records the receipts
// written.
func stageAck(t *testing.T, value string) (*cobra.Command, *backendtest.Fake, *attachRecorder, *ackRecorder) {
	t.Helper()
	fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
	acks := &ackRecorder{rec: rec}
	withOpenDeps(t, OpenDeps{Execer: rec, Runner: rec.Run, Runners: &rec.runners, AckWriter: acks})
	cmd := contextCommand()
	cmd.Flags().String("ack", "", "")
	if value != "" {
		if err := cmd.Flags().Set("ack", value); err != nil {
			t.Fatal(err)
		}
	}
	return cmd, fake, rec, acks
}

func TestOpen_AckReceipt(t *testing.T) {
	t.Run("it is written once the attach has begun", func(t *testing.T) {
		cmd, _, rec, acks := stageAck(t, "b1:t1")

		if err := openSession(cmd, "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}

		if want := []ackWrite{{receipt: "b1:t1", runs: 1}}; !slices.Equal(acks.writes, want) {
			t.Errorf("receipts = %+v, want %+v: one, as the run began", acks.writes, want)
		}
		if !slices.Equal(rec.runners.events, []string{"listed api-x7Kd9a", "ended", "unlisted"}) {
			t.Errorf("listing = %q, want the attach listed as ever", rec.runners.events)
		}
	})

	t.Run("an attach that never begins writes none", func(t *testing.T) {
		cmd, _, rec, acks := stageAck(t, "b1:t1")
		rec.answer(runResult{err: errors.New("pty: boom")})

		_ = openSession(cmd, "api-x7Kd9a")

		if len(acks.writes) != 0 {
			t.Errorf("receipts = %+v, want none", acks.writes)
		}
	})

	t.Run("with no terminal to own, it is written just before the exec", func(t *testing.T) {
		cmd, _, rec, acks := stageAck(t, "b1:t1")
		rec.answer(runResult{err: runner.ErrNoTerminal})

		if err := openSession(cmd, "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}

		if want := []ackWrite{{receipt: "b1:t1", runs: 1}}; !slices.Equal(acks.writes, want) || len(rec.execs) != 1 {
			t.Errorf("receipts = %+v, execs %d; want %+v before the one exec", acks.writes, len(rec.execs), want)
		}
	})

	t.Run("it is written once, however many attaches the picker loop makes", func(t *testing.T) {
		cmd, _, rec, acks := stageAck(t, "b1:t1")
		stagePicker(t, "blog-c3d4")
		rec.answer(outcome(attachkeys.Detach))

		if err := openSession(cmd, "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}

		if len(rec.runs) != 2 || len(acks.writes) != 1 {
			t.Errorf("runs %d, receipts %+v; want two attaches and one receipt", len(rec.runs), acks.writes)
		}
	})

	t.Run("a mint writes it as its attach begins", func(t *testing.T) {
		cmd, fake, _, acks := stageAck(t, "b1:t1")
		stageMint(t, fake, time.Now())

		if err := openPath(cmd, t.TempDir(), nil); err != nil {
			t.Fatalf("openPath: %v", err)
		}

		if want := []ackWrite{{receipt: "b1:t1", runs: 1}}; !slices.Equal(acks.writes, want) {
			t.Errorf("receipts = %+v, want %+v", acks.writes, want)
		}
	})

	t.Run("a write that fails still attaches, and says so at DEBUG", func(t *testing.T) {
		sink := logtest.Install(t)
		cmd, _, rec, acks := stageAck(t, "b1:t1")
		acks.err = errors.New("read-only file system")

		if err := openSession(cmd, "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}

		if len(rec.runs) != 1 {
			t.Errorf("runs = %d, want the attach", len(rec.runs))
		}
		rec2 := sink.Records().Matching("spawn", "spawn ack write failed").AtExactLevel(slog.LevelDebug).Only(t, "ack write record")
		if rec2.AttrString(t, "batch") != "b1" || rec2.AttrString(t, "error") != acks.err.Error() {
			t.Errorf("record = %+v, want the batch and the error", rec2)
		}
	})

	t.Run("a line with no --ack writes none", func(t *testing.T) {
		cmd, _, _, acks := stageAck(t, "")

		if err := openSession(cmd, "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}

		if len(acks.writes) != 0 {
			t.Errorf("receipts = %+v, want none", acks.writes)
		}
	})
}
