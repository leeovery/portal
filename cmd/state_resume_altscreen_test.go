package cmd

import (
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
)

// pinProbe answers the leave confirmation and the unpin for a waiter or a
// recover tail, naming each in the caller's order log.
type pinProbe struct {
	// reads answers each #{alternate_on} read in turn, repeating the last; none
	// reads as "0".
	reads     []string
	readErr   error
	readCalls int

	unpinErr   error
	unpinCalls int

	pauses []time.Duration
}

func (p *pinProbe) seams(order *[]string) altScreenPin {
	return altScreenPin{
		AlternateOn: func() (string, error) {
			*order = append(*order, "confirm")
			p.readCalls++
			if p.readErr != nil {
				return "", p.readErr
			}
			if len(p.reads) == 0 {
				return "0", nil
			}
			return p.reads[min(p.readCalls, len(p.reads))-1], nil
		},
		Unpin: func() error {
			*order = append(*order, "unpin")
			p.unpinCalls++
			return p.unpinErr
		},
		Pause: func(d time.Duration) { p.pauses = append(p.pauses, d) },
	}
}

type answerRoute struct {
	name string
	run  func(t *testing.T, arrange func(*pinProbe)) (order []string, pin *pinProbe, sink *logtest.Sink, execCalls int)
}

func pinReleasingRoutes() []answerRoute {
	return []answerRoute{
		{name: "Enter", run: func(t *testing.T, arrange func(*pinProbe)) ([]string, *pinProbe, *logtest.Sink, int) {
			var probe resumeWaitProbe
			arrange(&probe.pin)
			probe.lookup = foundHook("make deploy")
			answerEnter(t, &probe, samplePayload())
			return probe.order, &probe.pin, probe.sink, probe.execCalls
		}},
		{name: "a confirmed discard", run: func(t *testing.T, arrange func(*pinProbe)) ([]string, *pinProbe, *logtest.Sink, int) {
			var probe resumeWaitProbe
			arrange(&probe.pin)
			answerDiscard(t, &probe, confirmationPayload())
			return probe.order, &probe.pin, probe.sink, probe.execCalls
		}},
		{name: "the recover tail", run: func(t *testing.T, arrange func(*pinProbe)) ([]string, *pinProbe, *logtest.Sink, int) {
			var probe resumeRecoverProbe
			probe.markerValue = "1"
			arrange(&probe.pin)
			sink := logtest.Install(t)
			if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
				t.Fatalf("runResumeRecover() error = %v", err)
			}
			return probe.order, &probe.pin, sink, probe.execCalls
		}},
	}
}

func TestReleaseAltScreenPin(t *testing.T) {
	for _, route := range pinReleasingRoutes() {
		t.Run(route.name+" polls until the pane is off the alternate screen before it unpins", func(t *testing.T) {
			order, pin, sink, execs := route.run(t, func(p *pinProbe) { p.reads = []string{"1", "1", "0"} })

			if pin.readCalls != 3 || pin.unpinCalls != 1 {
				t.Errorf("reads = %d, unpins = %d; want 3 reads then one unpin", pin.readCalls, pin.unpinCalls)
			}
			confirmed := slices.Index(order, "confirm")
			unpinned := slices.Index(order, "unpin")
			if confirmed < 0 || unpinned < confirmed || order[unpinned-1] != "confirm" {
				t.Errorf("order = %q, want the unpin straight after the confirming read", order)
			}
			if clear := slices.Index(order, "clear"); clear < 0 || clear > confirmed {
				t.Errorf("order = %q, want the confirmation after the marker clear", order)
			}
			if !slices.Equal(pin.pauses, []time.Duration{altScreenLeavePoll, altScreenLeavePoll}) {
				t.Errorf("pauses = %v, want one poll interval between each read", pin.pauses)
			}
			if warned := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warned) != 0 {
				t.Errorf("records at or above WARN = %v, want none", warned)
			}
			if execs != 1 {
				t.Errorf("exec calls = %d, want the hand-over", execs)
			}
		})

		t.Run(route.name+" keeps the pin and records the pane when the leave is never confirmed", func(t *testing.T) {
			_, pin, sink, execs := route.run(t, func(p *pinProbe) { p.reads = []string{"1"} })

			if pin.readCalls != altScreenLeaveAttempts {
				t.Errorf("reads = %d, want the bound's %d", pin.readCalls, altScreenLeaveAttempts)
			}
			if pin.unpinCalls != 0 {
				t.Errorf("unpins = %d, want none while the pane may still show the alternate screen", pin.unpinCalls)
			}
			rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the unconfirmed leave's record")
			if rec.Msg != "alternate screen leave unconfirmed" {
				t.Errorf("WARN = %q, want the unconfirmed leave", rec.Msg)
			}
			if got := rec.AttrOrEmpty("pane_key"); got != "proj-a1b2:0.1" {
				t.Errorf("WARN pane_key = %q, want the pane", got)
			}
			if execs != 1 {
				t.Errorf("exec calls = %d, want the hand-over to go ahead", execs)
			}
		})

		t.Run(route.name+" keeps polling through a read that fails", func(t *testing.T) {
			_, pin, _, execs := route.run(t, func(p *pinProbe) { p.readErr = errors.New("no server") })

			if pin.readCalls != altScreenLeaveAttempts || pin.unpinCalls != 0 {
				t.Errorf("reads = %d, unpins = %d; want the bound's %d reads and no unpin", pin.readCalls, pin.unpinCalls, altScreenLeaveAttempts)
			}
			if execs != 1 {
				t.Errorf("exec calls = %d, want the hand-over to go ahead", execs)
			}
		})

		t.Run(route.name+" records a refused unpin and still hands the pane over", func(t *testing.T) {
			unpinErr := errors.New("can't find pane: %7")
			_, _, sink, execs := route.run(t, func(p *pinProbe) { p.unpinErr = unpinErr })

			rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the refused unpin's record")
			if rec.Msg != "unset alternate-screen pin failed" {
				t.Errorf("WARN = %q, want the refused unpin", rec.Msg)
			}
			if got := rec.AttrOrEmpty("pane_key"); got != "proj-a1b2:0.1" {
				t.Errorf("WARN pane_key = %q, want the pane", got)
			}
			if err := rec.ErrorAttr(t, "error"); !errors.Is(err, unpinErr) {
				t.Errorf("WARN error = %v, want %v", err, unpinErr)
			}
			if execs != 1 {
				t.Errorf("exec calls = %d, want the hand-over to go ahead", execs)
			}
		})
	}

	t.Run("its records are worded apart from the recover tail's failed clear", func(t *testing.T) {
		counts := warnMessages(t)
		for _, message := range []string{"alternate screen leave unconfirmed", "unset alternate-screen pin failed", "lift alternate-screen pin failed"} {
			if counts[message] != 1 {
				t.Errorf("%q is emitted by %d WARN sites, want 1", message, counts[message])
			}
		}
	})
}
