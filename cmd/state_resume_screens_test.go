package cmd

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

// onScreen is the payload a hand-off to screen carries: the key press that
// moves between screens ends any report the waiter was launched holding.
func onScreen(p resumeChainPayload, screen string) resumeChainPayload {
	p.Screen = screen
	p.Report = ""
	return p
}

func reportedPayload(screen string) resumeChainPayload {
	p := samplePayload()
	p.Width, p.Height = 100, 30
	p.Report = "could not remove the resume command"
	p.Screen = screen
	return p
}

// swallowed runs the waiter over input that ends in EOF and asserts it acted on
// none of it: no hand-off, no write, no store or marker touched, and a read
// taken past the input so the loop was still going round when it ran dry.
func swallowed(t *testing.T, payload resumeChainPayload, input string) {
	t.Helper()
	var probe resumeWaitProbe
	reader := keystrokes(t, input)

	err := runResumeWait(queuedKeysConfig(t, &probe, payload, reader))

	if !errors.Is(err, io.EOF) {
		t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait after the swallowed input", err)
	}
	if want := len(input) + 1; reader.reads != want {
		t.Errorf("the wait read %d times, want %d: every byte and then the EOF", reader.reads, want)
	}
	assertNoHandOff(t, &probe)
	assertNoStateTouched(t, &probe)
}

func assertNoStateTouched(t *testing.T, p *resumeWaitProbe) {
	t.Helper()
	if p.clearCalls != 0 {
		t.Errorf("ClearMarker called %d times, want 0", p.clearCalls)
	}
	if len(p.lookupKeys) != 0 {
		t.Errorf("the store was read for %q, want it never opened", p.lookupKeys)
	}
	if len(p.discardKeys) != 0 {
		t.Errorf("a discard was written for %q, want the store never written", p.discardKeys)
	}
}

// assertDiscardAnswered asserts the confirmation's y reached the discard: one
// removal of the pane's registration and the pane handed to a plain shell.
func assertDiscardAnswered(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload) {
	t.Helper()
	if !slices.Equal(p.discardKeys, []string{payload.HookKey}) {
		t.Errorf("discard written for %q, want one write for %q", p.discardKeys, payload.HookKey)
	}
	assertShellHandOff(t, p)
}

func answered(t *testing.T, payload resumeChainPayload, input string) *resumeWaitProbe {
	t.Helper()
	var probe resumeWaitProbe
	if err := runResumeWait(queuedKeysConfig(t, &probe, payload, keystrokes(t, input))); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
	return &probe
}

// queuedKeysConfig is for input that is already queued in full and then ends,
// so every byte of it arrives together and the end of the input is what judges
// it. A resize settle is refused: with no size change delivered and the pane at
// its drawn size, none can start.
func queuedKeysConfig(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload, in io.Reader) resumeWaitConfig {
	t.Helper()
	cfg := newResumeWaitConfig(t, p, payload, in)
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		t.Errorf("window = %v armed over queued keys, want none", d)
		return nil
	}
	return cfg
}

func assertArgvLacks(t *testing.T, argv []string, flag string) {
	t.Helper()
	if slices.Contains(argv, flagArg(flag)) {
		t.Errorf("hand-off argv = %q, want no %s", argv, flagArg(flag))
	}
}

// backOut runs a bare Escape on the confirmation through to its hand-off.
func backOut(t *testing.T, payload resumeChainPayload) *resumeWaitProbe {
	t.Helper()
	h := startArrivals(t, payload)
	h.typed("\x1b")
	h.answered()
	return h.probe
}

// inertEscape runs a bare Escape on the waiting panel, then ends the input and
// asserts the loop was still reading when it did.
func inertEscape(t *testing.T, payload resumeChainPayload) *resumeWaitProbe {
	t.Helper()
	h := startArrivals(t, payload)
	h.typed("\x1b")
	h.stillWaiting()
	if got := h.reader.reads.Load(); got < 2 {
		t.Errorf("the wait read %d times, want it to read on past the escape", got)
	}
	return h.probe
}

func TestRunResumeWait_Screens(t *testing.T) {
	t.Run("it opens the confirmation on d", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)

		probe := answered(t, payload, "d")

		assertHandOff(t, probe, opened(payload))
		assertArgvLacks(t, probe.execArgs, resumeFlagReport)
		assertNoStateTouched(t, probe)
	})

	t.Run("it backs out to the waiting panel on Escape", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)

		probe := backOut(t, payload)

		assertHandOff(t, probe, onScreen(payload, resumeScreenPanel))
		assertArgvLacks(t, probe.execArgs, resumeFlagScreen)
		assertArgvLacks(t, probe.execArgs, resumeFlagReport)
		assertNoStateTouched(t, probe)
	})

	t.Run("it swallows Enter on the confirmation", func(t *testing.T) {
		for _, key := range []string{"\r", "\n"} {
			t.Run(keyName(key), func(t *testing.T) {
				swallowed(t, reportedPayload(resumeScreenDiscard), key)
			})
		}
	})

	t.Run("it swallows d on the confirmation", func(t *testing.T) {
		swallowed(t, reportedPayload(resumeScreenDiscard), "d")
	})

	t.Run("it swallows uppercase Y on the confirmation", func(t *testing.T) {
		swallowed(t, reportedPayload(resumeScreenDiscard), "Y")
	})

	t.Run("it leaves Escape inert on the waiting panel", func(t *testing.T) {
		inertEscape(t, reportedPayload(resumeScreenPanel))
	})

	t.Run("it swallows an escape sequence on the confirmation", func(t *testing.T) {
		for _, seq := range []string{"\x1b[A", "\x1b[3~", "\x1bOD"} {
			t.Run(strings.TrimPrefix(seq, "\x1b"), func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)
				swallowed(t, payload, seq)

				assertDiscardAnswered(t, answeredAfter(t, payload, seq, "y"), payload)
			})
		}
	})

	t.Run("it swallows an escape sequence on the waiting panel", func(t *testing.T) {
		for _, seq := range []string{"\x1b[A", "\x1b[3~", "\x1bOD"} {
			t.Run(strings.TrimPrefix(seq, "\x1b"), func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)
				swallowed(t, payload, seq)

				assertHandOff(t, answeredAfter(t, payload, seq, "d"), opened(payload))
			})
		}
	})

	t.Run("it swallows a sequence whose final byte is an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1b[?1;2y"},
			{"d on the waiting panel", resumeScreenPanel, "\x1b[5d"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}
	})

	t.Run("it swallows an SS3 whose final byte is an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1bOy"},
			{"d on the waiting panel", resumeScreenPanel, "\x1bOd"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}

		payload := reportedPayload(resumeScreenDiscard)
		assertDiscardAnswered(t, answeredAfter(t, payload, "\x1bOy", "y"), payload)
	})

	t.Run("it swallows a terminal background-colour reply whole", func(t *testing.T) {
		replies := []string{
			"\x1b]11;rgb:1d1d/1f1f/2121",
			"\x1b]11;rgb:2d2d/2d2d/2d2d",
		}
		terminators := []struct {
			name string
			end  string
		}{
			{"BEL", "\x07"},
			{"ST", "\x1b\\"},
		}
		for _, screen := range []string{resumeScreenPanel, resumeScreenDiscard} {
			for _, reply := range replies {
				for _, term := range terminators {
					t.Run(screenName(screen)+"/"+reply[len(reply)-4:]+"/"+term.name, func(t *testing.T) {
						payload := reportedPayload(screen)
						swallowed(t, payload, reply+term.end)

						if screen == resumeScreenDiscard {
							assertDiscardAnswered(t, answeredAfter(t, payload, reply+term.end, "y"), payload)
							return
						}
						assertHandOff(t, answeredAfter(t, payload, reply+term.end, "d"), opened(payload))
					})
				}
			}
		}
	})

	t.Run("it backs out on a bare Escape", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)
		probe := backOut(t, payload)
		assertHandOff(t, probe, onScreen(payload, resumeScreenPanel))

		inertEscape(t, reportedPayload(resumeScreenPanel))
	})

	t.Run("it resumes on the Enter typed after a chord on the waiting panel", func(t *testing.T) {
		for _, chord := range altChords {
			t.Run(chord.name, func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)

				probe := answeredAfter(t, payload, chord.keys, "\r")

				if probe.clearCalls != 1 {
					t.Errorf("ClearMarker called %d times, want 1", probe.clearCalls)
				}
				if !slices.Equal(probe.lookupKeys, []string{payload.HookKey}) {
					t.Errorf("the store was read for %q, want one read for %q", probe.lookupKeys, payload.HookKey)
				}
				assertHookHandOff(t, probe, payload.Command)
			})
		}
	})

	t.Run("it confirms on the y typed after a chord on the confirmation", func(t *testing.T) {
		for _, chord := range altChords {
			t.Run(chord.name, func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)

				assertDiscardAnswered(t, answeredAfter(t, payload, chord.keys, "y"), payload)
			})
		}
	})

	t.Run("it backs out on a lone Escape typed after a chord on the confirmation", func(t *testing.T) {
		for _, chord := range append(slices.Clone(altChords), struct{ name, keys string }{"an Escape pair", "\x1b\x1b"}) {
			t.Run(chord.name, func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)

				probe := answeredAfter(t, payload, chord.keys, "\x1b")

				assertHandOff(t, probe, onScreen(payload, resumeScreenPanel))
				assertNoStateTouched(t, probe)
			})
		}
	})

	t.Run("it swallows a two-byte chord of an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1by"},
			{"d on the waiting panel", resumeScreenPanel, "\x1bd"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}
	})

	t.Run("it redraws on a resize that settles after a chord", func(t *testing.T) {
		for _, chord := range altChords {
			t.Run(chord.name, func(t *testing.T) {
				payload := drawnPayload()
				h := startArrivals(t, payload)

				h.arrive(chord.keys)
				h.quiet()
				h.sizes.deliver(h.winch)
				h.awaitSettle() <- time.Now()

				h.answered()
				assertHandOff(t, h.probe, payload)
				assertNoStateTouched(t, h.probe)
			})
		}
	})

	t.Run("it swallows every non-acting key on both screens", func(t *testing.T) {
		shared := []string{"\x03", "\x04", "\x1a", "D", "q", "a", " ", "cat README"}
		perScreen := map[string][]string{
			resumeScreenPanel:   append(slices.Clone(shared), "\x1b", "y", "Y"),
			resumeScreenDiscard: append(slices.Clone(shared), "\r", "\n", "d", "Y"),
		}
		for _, screen := range []string{resumeScreenPanel, resumeScreenDiscard} {
			for _, key := range perScreen[screen] {
				t.Run(screenName(screen)+"/"+keyName(key), func(t *testing.T) {
					swallowed(t, reportedPayload(screen), key)
				})
			}
		}
	})

	t.Run("it clears the report on both hand-offs", func(t *testing.T) {
		cases := []struct {
			name     string
			run      func(t *testing.T, payload resumeChainPayload) *resumeWaitProbe
			from     string
			wantTo   string
			wantDrop bool
		}{
			{
				name:     "d from a reported panel",
				run:      func(t *testing.T, p resumeChainPayload) *resumeWaitProbe { return answered(t, p, "d") },
				from:     resumeScreenPanel,
				wantTo:   resumeScreenDiscard,
				wantDrop: true,
			},
			{
				name:   "Escape from a reported confirmation",
				run:    backOut,
				from:   resumeScreenDiscard,
				wantTo: resumeScreenPanel,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := reportedPayload(tc.from)

				probe := tc.run(t, payload)

				want := onScreen(payload, tc.wantTo)
				want.DropInput = tc.wantDrop
				assertHandOff(t, probe, want)
				assertArgvLacks(t, probe.execArgs, resumeFlagReport)
			})
		}
	})

	t.Run("it restores the terminal before every hand-off", func(t *testing.T) {
		cases := []struct {
			name string
			run  func(t *testing.T) *resumeWaitProbe
		}{
			{"Enter on the panel", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenPanel), "\r")
			}},
			{"d on the panel", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenPanel), "d")
			}},
			{"y on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenDiscard), "y")
			}},
			{"Escape on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return backOut(t, reportedPayload(resumeScreenDiscard))
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				probe := tc.run(t)

				if probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
				}
				if probe.restoredAtExec != 1 {
					t.Errorf("%d restores had run when the hand-off exec'd, want 1: the next process image must inherit a cooked tty", probe.restoredAtExec)
				}
			})
		}
	})

	t.Run("it acts on both keys at a size below the card's", func(t *testing.T) {
		sizes := []struct {
			name string
			w, h int
		}{
			{"a single cell", 1, 1},
			{"narrower than the card", 20, 6},
			{"unmeasured", 0, 0},
			{"negative", -4, -2},
		}
		for _, size := range sizes {
			sized := func(screen string) resumeChainPayload {
				p := reportedPayload(screen)
				p.Width, p.Height = size.w, size.h
				return p
			}
			t.Run(size.name+"/panel/Enter", func(t *testing.T) {
				var probe resumeWaitProbe
				payload := sized(resumeScreenPanel)
				probe.lookup = foundHook(payload.Command)
				if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, keystrokes(t, "\r"))); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				assertHookHandOff(t, &probe, payload.Command)
			})
			t.Run(size.name+"/panel/d", func(t *testing.T) {
				payload := sized(resumeScreenPanel)
				assertHandOff(t, answered(t, payload, "d"), opened(payload))
			})
			t.Run(size.name+"/discard/y", func(t *testing.T) {
				payload := sized(resumeScreenDiscard)
				assertDiscardAnswered(t, answered(t, payload, "y"), payload)
			})
			t.Run(size.name+"/discard/Escape", func(t *testing.T) {
				payload := sized(resumeScreenDiscard)
				assertHandOff(t, backOut(t, payload), onScreen(payload, resumeScreenPanel))
			})
		}
	})

	t.Run("it answers the confirmation's y with the discard", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)

		assertDiscardAnswered(t, answered(t, payload, "y"), payload)
	})
}

// altChords are Alt chords as a terminal sends them, each arriving as an Escape
// and the key it modifies, including those whose second byte would open a
// sequence.
var altChords = []struct{ name, keys string }{
	{"Alt-b", "\x1bb"},
	{"Alt-P", "\x1bP"},
	{"Alt-_", "\x1b_"},
	{"Alt-[", "\x1b["},
	{"Alt-Shift-O", "\x1bO"},
	{"Alt-]", "\x1b]"},
}

// answeredAfter delivers first as one arrival and then key typed alone, the
// pane falling quiet after each.
func answeredAfter(t *testing.T, payload resumeChainPayload, first, key string) *resumeWaitProbe {
	t.Helper()
	h := startArrivals(t, payload)
	h.probe.lookup = foundHook(payload.Command)

	h.arrive(first)
	h.quiet()
	h.typed(key)

	h.answered()
	return h.probe
}

func screenName(screen string) string {
	if screen == resumeScreenDiscard {
		return "discard"
	}
	return "panel"
}
