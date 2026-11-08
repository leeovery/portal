package cmd

import (
	"errors"
	"io"
	"slices"
	"strconv"
	"testing"
)

// A draw of an adaptive pair asks the terminal its background and reads the
// session's input for the reply, so a key typed meanwhile, as zmx's redraw on
// attach makes likely, is read with it. These run the draw's hand-off through
// the waiter's own command line, as the exec does.

// drawReading runs a draw whose appearance query read typed beside its reply,
// and returns the argv it hands the waiter.
func drawReading(t *testing.T, typed string) []string {
	t.Helper()
	return drawThrough(t, &resumeDrawProbe{typed: []byte(typed)})
}

// drawReadingUnanswered is drawReading for a query no reply answered in time.
func drawReadingUnanswered(t *testing.T, typed string) []string {
	t.Helper()
	return drawThrough(t, &resumeDrawProbe{typed: []byte(typed), unanswered: true})
}

func drawThrough(t *testing.T, probe *resumeDrawProbe) []string {
	t.Helper()
	if err := runResumeDraw(newResumeDrawConfig(t, probe, samplePayload(), fixedSize(100, 30))); err != nil {
		t.Fatalf("runResumeDraw() error = %v", err)
	}
	if probe.execCalls != 1 {
		t.Fatalf("the draw exec'd %d times, want one hand-off to the waiter", probe.execCalls)
	}
	return probe.execArgs
}

// waiterOf parses argv through the waiter's command and returns the payload it
// would wait with.
func waiterOf(t *testing.T, argv []string) resumeChainPayload {
	t.Helper()
	var got resumeChainPayload
	ran := false
	withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
		got, ran = cfg.resumeChainPayload, true
		return nil
	})
	executeChainArgv(t, argv[1:])
	if !ran {
		t.Fatalf("argv %q ran no waiter", argv)
	}
	return got
}

// handedOn is the payload the waiter hands on, which carries nothing typed.
func handedOn(p resumeChainPayload) resumeChainPayload {
	p.Typed, p.InputArriving = "", false
	return p
}

func TestResumeTypedDuringTheDraw(t *testing.T) {
	t.Run("a key the draw read answers the panel as if typed after it", func(t *testing.T) {
		t.Run("Enter", func(t *testing.T) {
			payload := waiterOf(t, drawReading(t, "\r"))
			h := startArrivals(t, payload)
			h.probe.lookup = foundHook(payload.Command)

			h.quiet()

			h.answered()
			assertHookHandOff(t, h.probe, payload.Command)
		})
		t.Run("d", func(t *testing.T) {
			payload := waiterOf(t, drawReading(t, "d"))
			h := startArrivals(t, payload)

			h.quiet()

			h.answered()
			assertHandOff(t, h.probe, opened(handedOn(payload)))
			assertNoStateTouched(t, h.probe)
		})
		t.Run("Escape", func(t *testing.T) {
			payload := waiterOf(t, drawReading(t, "\x1b"))
			h := startArrivals(t, payload)

			h.quiet()
			_ = h.writer.Close()

			if err := h.wait(); !errors.Is(err, io.EOF) {
				t.Fatalf("runResumeWait() error = %v, want the EOF that ended a wait still going", err)
			}
			if h.probe.backCalls != 1 {
				t.Errorf("BackToPicker called %d times, want 1", h.probe.backCalls)
			}
			assertNoHandOff(t, h.probe)
		})
	})

	t.Run("a paste the draw read answers nothing, and carries none of its text", func(t *testing.T) {
		for _, paste := range []string{"echo hi\r", "\r\r", "d\r", "\x1b[A"} {
			t.Run(strconv.Quote(paste), func(t *testing.T) {
				argv := drawReading(t, paste)
				if !slices.Contains(argv, flagArg(resumeFlagInputArriving)) {
					t.Errorf("waiter argv = %q, want %s", argv, flagArg(resumeFlagInputArriving))
				}
				assertArgvLacks(t, argv, resumeFlagTyped)
				h := startArrivals(t, waiterOf(t, argv))

				h.quiet()

				h.stillWaiting()
			})
		}
	})

	// Enter after it would answer alone, so only the key the draw read, as the
	// arrival's first byte, keeps it from answering.
	t.Run("a key the draw read with Enter arriving after it answers nothing", func(t *testing.T) {
		for _, key := range []string{"\r", "d", "\x1b", "x"} {
			t.Run(strconv.Quote(key), func(t *testing.T) {
				payload := waiterOf(t, drawReading(t, key))
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.arrive("\r")
				h.quiet()

				h.stillWaiting()
			})
		}
	})

	t.Run("a draw that read nothing typed hands nothing on", func(t *testing.T) {
		argv := drawReading(t, "")

		assertArgvLacks(t, argv, resumeFlagTyped)
		assertArgvLacks(t, argv, resumeFlagInputArriving)
	})

	t.Run("the waiter hands on no key it was given", func(t *testing.T) {
		payload := drawnPayload()
		payload.Typed = "x"
		h := startArrivals(t, payload)

		h.quiet()
		h.resizeSettled()

		h.answered()
		assertHandOff(t, h.probe, handedOn(payload))
	})
}

func TestResumeTypedFlag(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		typed    string
		arriving bool
	}{
		{"one byte", "0d", "\r", false},
		{"Escape", "1b", "\x1b", false},
		{"more than a byte", "0d0a", "", true},
		{"no hex at all", "zz", "", true},
		{"half a byte", "d", "", true},
	}
	for _, tc := range cases {
		t.Run("the waiter reads "+tc.name, func(t *testing.T) {
			argv := resumeChainArgv("portal", resumeWaitSubcommand, samplePayload())
			argv = append(argv, flagArg(resumeFlagTyped), tc.value)

			got := waiterOf(t, argv)

			if got.Typed != tc.typed || got.InputArriving != tc.arriving {
				t.Errorf("--typed %s waits with Typed %q and InputArriving %v, want %q and %v", tc.value, got.Typed, got.InputArriving, tc.typed, tc.arriving)
			}
		})
	}

	t.Run("only the waiter's argv carries a key, so a draw handed one still parses", func(t *testing.T) {
		payload := samplePayload()
		payload.Typed = "\r"

		if argv := resumeChainArgv("portal", resumeWaitSubcommand, payload); !slices.Contains(argv, flagArg(resumeFlagTyped)) {
			t.Errorf("waiter argv = %q, want %s", argv, flagArg(resumeFlagTyped))
		}
		argv := resumeChainArgv("portal", resumeDrawSubcommand, payload)
		assertArgvLacks(t, argv, resumeFlagTyped)
		ran := false
		withFuncSeam(t, &resumeDrawRunFunc, func(resumeDrawConfig) error {
			ran = true
			return nil
		})
		executeChainArgv(t, argv[1:])
		if !ran {
			t.Errorf("draw argv %q ran no draw", argv)
		}
	})

	t.Run("a payload's key round-trips through the waiter's argv", func(t *testing.T) {
		payload := samplePayload()
		payload.Typed = "\r"

		if got := waiterOf(t, resumeChainArgv("portal", resumeWaitSubcommand, payload)); got != payload {
			t.Errorf("payload = %+v, want %+v", got, payload)
		}
	})
}
