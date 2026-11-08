package cmd

import (
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bgreply"
)

// A terminal over a slow link, a phone over SSH among them, answers the draw's
// background question after the draw has given up on it, so the reply lands in
// the waiter's input, or in whatever the session is handed to next. Only a
// terminal attached when a question was asked answers it, within its round
// trip, so a question is owed its reply for resumeReplyWindow and no longer.

const (
	lateDarkReply = "\x1b]11;rgb:0b0b/0c0c/1414\a"
	lateSTReply   = "\x1b]11;rgb:0b0b/0c0c/1414\x1b\\"
	lateC1Reply   = "\x1b]11;rgb:e1e1/e2e2/e7e7\x9c"
)

var lateReplies = []struct {
	name  string
	reply string
}{
	{"BEL-terminated", lateDarkReply},
	{"ESC \\-terminated", lateSTReply},
	{"C1 ST-terminated", lateC1Reply},
	{"two terminals' replies", lateDarkReply + lateC1Reply},
}

// A reply split where a link might split it: its head, then its rest.
const (
	lateReplyHead = "\x1b]11;rgb:0b0b/0c"
	lateReplyTail = "0c/1414\a"
)

// resumeTestEpoch is the instant every test clock reads, or starts at.
var resumeTestEpoch = time.UnixMilli(1_762_560_000_000)

func testClockAtEpoch() time.Time { return resumeTestEpoch }

// testClock is a wall clock a test moves on, read from any goroutine.
type testClock struct{ nanos atomic.Int64 }

func newTestClock() *testClock {
	c := new(testClock)
	c.nanos.Store(resumeTestEpoch.UnixNano())
	return c
}

func (c *testClock) now() time.Time { return time.Unix(0, c.nanos.Load()) }

func (c *testClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

// askedAgo is a debt of questions, oldest first, each asked so long before the
// epoch.
func askedAgo(ago ...time.Duration) replyDebt {
	var d replyDebt
	for _, a := range ago {
		d = d.owe(resumeTestEpoch.Add(-a))
	}
	return d
}

// owing is a waiting panel whose draw's question, asked as its wait starts, is
// still owed its reply.
func owing(p resumeChainPayload) resumeChainPayload {
	p.RepliesOwed = askedAgo(0)
	return p
}

// listensLeftAfterEnter is how many quiet listens are left of the window of a
// question asked at the epoch once Enter's own quiet window has passed.
var listensLeftAfterEnter = int(resumeReplyWindow/resumeInputQuiet) - 1

// inTheWait delivers input to a wait listening before a hand-off, a byte to
// each listen, as it reaches the session's input.
func (h *arrivalHarness) inTheWait(input string) {
	h.t.Helper()
	go func() { _, _ = h.writer.Write([]byte(input)) }()
	h.arriving(len(input))
}

// arriving answers n of the wait's listens with a byte arriving.
func (h *arrivalHarness) arriving(n int) {
	h.t.Helper()
	for range n {
		h.awaitListen("a listen for the input arriving").reply <- true
	}
}

// silentFor answers n of the wait's listens with no input arriving.
func (h *arrivalHarness) silentFor(n int) {
	h.t.Helper()
	for range n {
		h.awaitListen("a listen for the reply owed").reply <- false
	}
}

// answerWith types key alone and lets the session fall quiet, a window passing
// on the wait's clock, leaving the wait to answer it, which waits on for a
// reply owed before handing on.
func (h *arrivalHarness) answerWith(key string) {
	h.t.Helper()
	h.arrive(key)
	h.answerListen(false)
	h.pending, h.heldFor = nil, 0
}

// held lets the pane fall quiet after a reply's head and returns once the wait
// listens on for its rest, the arrival still open.
func (h *arrivalHarness) held() {
	h.t.Helper()
	h.answerListen(false)
	h.heldFor++
	h.live = h.awaitListen("a listen held open for the rest of a reply")
}

func (h *arrivalHarness) holdSpent() {
	h.t.Helper()
	for h.heldFor < resumeReplyTailHolds {
		h.held()
	}
}

func (h *arrivalHarness) assertRead(want string) {
	h.t.Helper()
	if got := h.reader.reads.Load(); got != int64(len(want)) {
		h.t.Errorf("the wait read %d bytes, want %d, all of %q", got, len(want), want)
	}
}

func TestResumeLateReply_InAnArrival(t *testing.T) {
	t.Run("a reply arriving behind the key the draw read leaves the key to answer", func(t *testing.T) {
		for _, tc := range lateReplies {
			t.Run(tc.name, func(t *testing.T) {
				payload := waiterOf(t, drawReadingUnanswered(t, "\r"))
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.arrive(tc.reply)
				h.quiet()

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
			})
		}
	})

	t.Run("a reply arriving around a key typed leaves the key to answer", func(t *testing.T) {
		for _, key := range []string{"\r", "d", "\x1b"} {
			for _, tc := range lateReplies {
				t.Run(strconv.Quote(key)+"/"+tc.name, func(t *testing.T) {
					payload := owing(drawnPayload())
					h := startArrivals(t, payload)
					h.probe.lookup = foundHook(payload.Command)

					h.arrive(tc.reply + key + tc.reply)
					h.quiet()

					switch key {
					case "\x1b":
						if err := h.endInput(); !errors.Is(err, io.EOF) {
							t.Fatalf("runResumeWait() error = %v, want the EOF that ended a wait still going", err)
						}
						if h.probe.backCalls != 1 {
							t.Errorf("BackToPicker called %d times, want 1", h.probe.backCalls)
						}
						assertNoHandOff(t, h.probe)
					case "d":
						h.answered()
						want := opened(payload)
						want.RepliesOwed = replyDebt{}
						assertHandOff(t, h.probe, want)
					default:
						h.answered()
						assertHookHandOff(t, h.probe, payload.Command)
					}
				})
			}
		}
	})

	t.Run("a reply arriving alone answers nothing, and settles the one owed", func(t *testing.T) {
		for _, tc := range lateReplies {
			t.Run(tc.name, func(t *testing.T) {
				payload := owing(drawnPayload())
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.arrive(tc.reply)
				h.quiet()
				h.answerWith("\r")

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
			})
		}
	})

	t.Run("a reply settles a question still in its window, never one past it", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(resumeReplyWindow-2*resumeInputQuiet, 0)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.typed("x")
		h.typed("x")
		h.arrive(lateDarkReply)
		h.quiet()
		h.answerWith("\r")

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	// A question no terminal saw is never answered, so a reply is the newest
	// question's: settled oldest first, that question would wait out a window
	// of its own after its reply had come.
	t.Run("a reply settles the newest question owed, leaving an older one to its own window", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(resumeReplyWindow-3*resumeInputQuiet, 0)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive(lateDarkReply)
		h.quiet()
		h.answerWith("\r")
		h.silentFor(1)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a reply split across quiet windows is held for its rest, never judged as keys", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)

		h.arrive(lateReplyHead)
		h.held()
		h.arrive(lateReplyTail + "d")
		h.quiet()

		h.answered()
		want := opened(payload)
		want.RepliesOwed = replyDebt{}
		assertHandOff(t, h.probe, want)
	})

	t.Run("an arrival ending in what may open a reply is held no longer than a reply's rest takes", func(t *testing.T) {
		payload := drawnPayload()
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive("\x1b]")
		h.quiet()
		h.answerWith("\r")

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("an arrival longer than an arrival keeps answers nothing, whatever the bytes it kept hold", func(t *testing.T) {
		reply := "\x1b]11;" + strings.Repeat("0", resumeArrivalKept-len("\r\x1b]11;\a")) + "\a"
		h := startArrivals(t, drawnPayload())

		h.arrive("\r" + reply + "x")
		h.quiet()

		h.stillWaiting()
	})
}

func TestResumeLateReply_BeforeTheHandOff(t *testing.T) {
	t.Run("Enter waits for the reply owed, reads it off and hands on as it comes", func(t *testing.T) {
		for _, tc := range lateReplies[:3] {
			t.Run(tc.name, func(t *testing.T) {
				payload := owing(drawnPayload())
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.answerWith("\r")
				h.silentFor(2)
				h.inTheWait(tc.reply)

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
				h.assertRead("\r" + tc.reply)
			})
		}
	})

	t.Run("the confirmation's y waits for the reply owed before the shell", func(t *testing.T) {
		payload := owing(confirmationPayload())
		h := startArrivals(t, payload)

		h.answerWith("y")
		h.inTheWait(lateDarkReply)

		h.answered()
		assertDiscardAnswered(t, h.probe, payload)
	})

	t.Run("Enter hands on once the question's window has closed, its reply never come", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.silentFor(listensLeftAfterEnter)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("two questions owed, replies coming one at a time: the hand-off waits for the second", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(2*resumeInputQuiet, 0)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.inTheWait(lateDarkReply)
		h.silentFor(2)
		h.inTheWait(lateC1Reply)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r" + lateDarkReply + lateC1Reply)
	})

	t.Run("the wait lasts until the newest question's window closes", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(resumeReplyWindow/2, 0)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.silentFor(listensLeftAfterEnter)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a question past its window owes nothing, so Enter hands on at once", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(resumeReplyWindow)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a question past its window as the wait begins is settled by no reply", func(t *testing.T) {
		clock := newTestClock()
		input := keystrokes(t, lateDarkReply)
		cfg := newResumeWaitConfig(t, new(resumeWaitProbe), drawnPayload(), input)
		cfg.RepliesOwed = askedAgo(resumeReplyWindow, 0)
		cfg.Now = clock.now
		quietListens := 0
		cfg.AwaitInput = func(d time.Duration) (bool, error) {
			if input.reads < len(lateDarkReply) {
				return true, nil
			}
			quietListens++
			clock.advance(d)
			return false, nil
		}

		cfg.awaitOwedReplies()

		if quietListens != 0 || cfg.RepliesOwed.owing() {
			t.Errorf("the wait listened %d times past the reply, owing %v; want the reply to settle the question in its window, and the wait over", quietListens, cfg.RepliesOwed)
		}
	})

	t.Run("a question with one listen left of its window once Enter is judged is waited for that listen", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(resumeReplyWindow - 2*resumeInputQuiet)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.silentFor(1)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a key typed in the wait is lost whole, and the wait reads on for the reply", func(t *testing.T) {
		for _, key := range []string{"x", "\x1b", "\x1b[A", "\x1b[13u", "\x1b[200~echo hi\r\x1b[201~"} {
			t.Run(strconv.Quote(key), func(t *testing.T) {
				payload := owing(drawnPayload())
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.answerWith("\r")
				h.inTheWait(key)
				h.silentFor(1)
				h.inTheWait(lateDarkReply)

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
				h.assertRead("\r" + key + lateDarkReply)
			})
		}
	})

	t.Run("a key typed ahead of the reply is read on to its quiet once the reply has settled the debt", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.inTheWait("\x1b[A" + lateDarkReply)
		h.silentFor(1)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r\x1b[A" + lateDarkReply)
	})

	t.Run("a reply cut off at the end of Enter's arrival is finished in the wait", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive("\r" + lateReplyHead)
		h.holdSpent()
		h.answerListen(false)
		h.inTheWait(lateReplyTail)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a paste longer than an arrival keeps, typed in the wait, is lost whole", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)
		paste := strings.Repeat("x", resumeArrivalKept+10)

		h.answerWith("\r")
		h.inTheWait(paste)
		h.silentFor(1)
		h.inTheWait(lateDarkReply)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r" + paste + lateDarkReply)
	})

	t.Run("a reply's head, the pane falling quiet, then its rest, all in the wait, settle the question owed", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.inTheWait(lateReplyHead)
		h.silentFor(1)
		h.inTheWait(lateReplyTail)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r" + lateReplyHead + lateReplyTail)
	})

	t.Run("two replies back to back in the wait settle two questions, and it waits on for the third", func(t *testing.T) {
		payload := drawnPayload()
		payload.RepliesOwed = askedAgo(2*resumeInputQuiet, resumeInputQuiet, 0)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.inTheWait(lateDarkReply + lateC1Reply)
		h.silentFor(1)
		h.inTheWait(lateSTReply)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r" + lateDarkReply + lateC1Reply + lateSTReply)
	})

	t.Run("a reply cut off at the end of Enter's arrival is finished with nothing owed", func(t *testing.T) {
		payload := drawnPayload()
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive("\r" + lateReplyHead)
		h.holdSpent()
		h.answerListen(false)
		h.inTheWait(lateReplyTail)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
		h.assertRead("\r" + lateReplyHead + lateReplyTail)
	})

	t.Run("a reply cut off with nothing owed is waited on for a reply's rest, and no longer", func(t *testing.T) {
		payload := drawnPayload()
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive("\r" + lateReplyHead)
		h.holdSpent()
		h.answerListen(false)
		h.silentFor(int(bgreply.TailWait / resumeInputQuiet))

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("nothing owed, Enter hands on at once", func(t *testing.T) {
		payload := drawnPayload()
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a reply owed holds no hand-off to another screen", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)

		h.answerWith("d")

		h.answered()
		assertHandOff(t, h.probe, opened(payload))
		if !slices.Contains(h.probe.execArgs, flagArg(resumeFlagRepliesOwed)) {
			t.Errorf("confirmation argv = %q, want the reply still owed: %s", h.probe.execArgs, flagArg(resumeFlagRepliesOwed))
		}
	})

	t.Run("a hand-off refused after the wait redraws owing nothing it read off", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			payload resumeChainPayload
			key     string
		}{
			{"Enter", drawnPayload(), "\r"},
			{"the confirmation's y", onScreen(drawnPayload(), resumeScreenDiscard), "y"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h := startArrivals(t, owing(tc.payload))
				h.probe.clearErr = errors.New("flag stuck")

				h.answerWith(tc.key)
				h.inTheWait(lateDarkReply)

				h.answered()
				if h.probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want one redraw", h.probe.execCalls)
				}
				assertArgvLacks(t, h.probe.execArgs, resumeFlagRepliesOwed)
			})
		}
	})
}

func TestResumeLateReply_TheWaitsLength(t *testing.T) {
	cases := []struct {
		name string
		ago  time.Duration
		want time.Duration
	}{
		{"a question asked 30 ms ago", 30 * time.Millisecond, resumeReplyWindow - 30*time.Millisecond},
		{"a question the clock says is yet to be asked", -5 * time.Second, resumeReplyWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is waited for no longer than is left of its window, from now at most", func(t *testing.T) {
			clock := newTestClock()
			cfg := newResumeWaitConfig(t, new(resumeWaitProbe), drawnPayload(), keystrokes(t, ""))
			cfg.RepliesOwed = askedAgo(tc.ago)
			cfg.Now = clock.now
			var waited time.Duration
			cfg.AwaitInput = func(d time.Duration) (bool, error) {
				if d <= 0 || d > resumeInputQuiet {
					t.Errorf("listen window = %v, want one no longer than %v", d, resumeInputQuiet)
				}
				waited += d
				clock.advance(d)
				return false, nil
			}

			cfg.awaitOwedReplies()

			if waited != tc.want {
				t.Errorf("waited %v for the reply, want %v", waited, tc.want)
			}
		})
	}
}

// Auto-repeat, or a paste arriving a byte at a time: input that never falls
// quiet.
func TestResumeLateReply_TheWaitsBound(t *testing.T) {
	t.Run("keys typed without a pause hold the wait a window from its start at most", func(t *testing.T) {
		const repeat = 30 * time.Millisecond
		clock := newTestClock()
		cfg := newResumeWaitConfig(t, new(resumeWaitProbe), drawnPayload(), keystrokes(t, strings.Repeat("x", 1000)))
		cfg.RepliesOwed = askedAgo(0)
		cfg.Now = clock.now
		cfg.AwaitInput = func(time.Duration) (bool, error) {
			clock.advance(repeat)
			return true, nil
		}

		cfg.awaitOwedReplies()

		if waited := clock.now().Sub(resumeTestEpoch); waited > resumeReplyWindow+repeat {
			t.Errorf("the wait lasted %v with a key arriving every %v, want no more than its window, %v", waited, repeat, resumeReplyWindow)
		}
	})

	t.Run("a reply cut off as the wait's window closes holds it no longer", func(t *testing.T) {
		clock := newTestClock()
		input := keystrokes(t, lateReplyHead)
		cfg := newResumeWaitConfig(t, new(resumeWaitProbe), drawnPayload(), input)
		cfg.RepliesOwed = askedAgo(0)
		cfg.Now = clock.now
		headAt := resumeTestEpoch.Add(resumeReplyWindow - resumeInputQuiet)
		cfg.AwaitInput = func(d time.Duration) (bool, error) {
			if !clock.now().Before(headAt) && input.reads < len(lateReplyHead) {
				return true, nil
			}
			clock.advance(d)
			return false, nil
		}

		cfg.awaitOwedReplies()

		if waited := clock.now().Sub(resumeTestEpoch); waited != resumeReplyWindow {
			t.Errorf("the wait lasted %v with a reply's head arriving %v before its window closed, want its window, %v", waited, resumeInputQuiet, resumeReplyWindow)
		}
	})
}

func TestResumeLateReply_TheDebt(t *testing.T) {
	const before = 100 * time.Millisecond

	t.Run("a reply settles the newest question owed", func(t *testing.T) {
		if got, want := askedAgo(3*before, 2*before, before).settle(1), askedAgo(3*before, 2*before); got != want {
			t.Errorf("one reply leaves %v owed, want %v", got, want)
		}
	})

	t.Run("a question the clock says is yet to be asked is owed a window from now, and no longer", func(t *testing.T) {
		owed := askedAgo(-5 * time.Second).live(resumeTestEpoch)

		if due := resumeTestEpoch.Add(resumeReplyWindow); !owed.owing() || !owed.lastDue().Equal(due) {
			t.Fatalf("owes %v, want one question due at %v", owed, due)
		}
		if later := owed.live(resumeTestEpoch.Add(resumeReplyWindow)); later.owing() {
			t.Errorf("a window on, owes %v, want nothing", later)
		}
	})
}

func TestResumeLateReply_TheDraw(t *testing.T) {
	const before = 100 * time.Millisecond
	cases := []struct {
		name    string
		owed    replyDebt
		asked   bool
		replies int
		handed  int
		want    replyDebt
	}{
		{"nothing owed, its question answered", replyDebt{}, true, 1, 0, replyDebt{}},
		{"nothing owed, its question unanswered", replyDebt{}, true, 0, 0, askedAgo(0)},
		{"nothing owed, no question asked, as with no terminal to answer", replyDebt{}, false, 0, 0, replyDebt{}},
		{"one owed, that one's reply and its own read", askedAgo(before), true, 2, 1, replyDebt{}},
		{"one owed, one reply read, which settles its own question", askedAgo(before), true, 1, 1, askedAgo(before)},
		{"one owed, no reply read", askedAgo(before), true, 0, 1, askedAgo(before, 0)},
		{"one owed, no question asked, and its reply read", askedAgo(before), false, 1, 1, replyDebt{}},
		{"one owed, no question asked, and no reply read", askedAgo(before), false, 0, 1, askedAgo(before)},
		{"one owed past its window, as after a boot, its own answered", askedAgo(resumeReplyWindow), true, 1, 0, replyDebt{}},
		{"as many owed as are kept, its own unanswered", askedAgo(4*before, 3*before, 2*before, before), true, 0, 4, askedAgo(3*before, 2*before, before, 0)},
		{"as many owed as are kept, one reply read", askedAgo(4*before, 3*before, 2*before, before), true, 1, 4, askedAgo(3*before, 2*before, before)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := samplePayload()
			payload.RepliesOwed = tc.owed
			probe := &resumeDrawProbe{asked: tc.asked, replies: tc.replies}

			if err := runResumeDraw(newResumeDrawConfig(t, probe, payload, fixedSize(100, 30))); err != nil {
				t.Fatalf("runResumeDraw() error = %v", err)
			}

			if !slices.Equal(probe.owed, []int{tc.handed}) {
				t.Errorf("the query was told of %v questions owed, want [%d]", probe.owed, tc.handed)
			}
			if got := waiterOf(t, probe.execArgs).RepliesOwed; got != tc.want {
				t.Errorf("waiter owes %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("the replies the drop read settle questions as the query's do", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			replies int
			want    replyDebt
		}{
			{"one", 1, askedAgo(before)},
			{"two", 2, replyDebt{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				payload := openingPayload()
				payload.RepliesOwed = askedAgo(before)
				probe := &resumeDrawProbe{asked: true}
				cfg := newResumeDrawConfig(t, probe, payload, fixedSize(100, 30))
				cfg.DropInputQueue = func() (int, error) { return tc.replies, nil }

				if err := runResumeDraw(cfg); err != nil {
					t.Fatalf("runResumeDraw() error = %v", err)
				}

				if got := waiterOf(t, probe.execArgs).RepliesOwed; got != tc.want {
					t.Errorf("waiter owes %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("the replies owed round-trip through a redraw's argv", func(t *testing.T) {
		payload := samplePayload()
		payload.RepliesOwed = askedAgo(before, 0)
		var got resumeChainPayload
		withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
			got = cfg.resumeChainPayload
			return nil
		})

		executeChainArgv(t, resumeChainArgv("portal", resumeDrawSubcommand, payload)[1:])

		if got != payload {
			t.Errorf("payload = %+v, want %+v", got, payload)
		}
	})

	t.Run("an argv owing more than are kept is read as the newest", func(t *testing.T) {
		argv := resumeChainArgv("portal", resumeWaitSubcommand, samplePayload())
		argv = append(argv, flagArg(resumeFlagRepliesOwed), "1,2,3,4,5")

		if got, want := waiterOf(t, argv).RepliesOwed, replyDebtFrom([]int64{2, 3, 4, 5}); got != want {
			t.Errorf("waiter owes %v, want %v", got, want)
		}
	})

	t.Run("an argv owing a question the clock says is yet to be asked is read as one asked now", func(t *testing.T) {
		argv := resumeChainArgv("portal", resumeWaitSubcommand, samplePayload())
		argv = append(argv, flagArg(resumeFlagRepliesOwed), "99999999999999")

		before := time.Now().UnixMilli()
		got := waiterOf(t, argv).RepliesOwed
		after := time.Now().UnixMilli()

		if got.count() != 1 || got.asked[0] < before || got.asked[0] > after {
			t.Errorf("waiter owes %v, want one question asked between %d and %d", got, before, after)
		}
	})

	t.Run("an argv owing what Portal didn't write is read as one question asked now", func(t *testing.T) {
		for _, value := range []string{"soon", "1,x", "1,,2"} {
			t.Run(strconv.Quote(value), func(t *testing.T) {
				argv := resumeChainArgv("portal", resumeWaitSubcommand, samplePayload())
				argv = append(argv, flagArg(resumeFlagRepliesOwed), value)

				before := time.Now().UnixMilli()
				got := waiterOf(t, argv).RepliesOwed
				after := time.Now().UnixMilli()

				if got.count() != 1 || got.asked[0] < before || got.asked[0] > after {
					t.Errorf("waiter owes %v, want one question asked between %d and %d", got, before, after)
				}
			})
		}
	})
}
