package cmd

import (
	"errors"
	"io"
	"slices"
	"strconv"
	"testing"
)

// A terminal over a slow link, a phone over SSH among them, answers the draw's
// background question after the draw has given up on it, so the reply lands in
// the waiter's input, or in whatever the session is handed to next.

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

// owing is a waiting panel whose draw's question is still owed its reply.
func owing(p resumeChainPayload) resumeChainPayload {
	p.ReplyOwed = true
	return p
}

// lateReply delivers reply to a wait listening for it after an answer, a byte
// to each listen, as a terminal's reply reaches the session's input.
func (h *arrivalHarness) lateReply(reply string) {
	h.t.Helper()
	go func() { _, _ = h.writer.Write([]byte(reply)) }()
	for range len(reply) {
		h.awaitListen("a listen for the reply owed") <- true
	}
}

// silentFor answers n of the wait's listens with no input arriving.
func (h *arrivalHarness) silentFor(n int) {
	h.t.Helper()
	for range n {
		h.awaitListen("a listen for the reply owed") <- false
	}
}

// answerWith types key alone and lets the session fall quiet, leaving the wait
// to answer it, which waits on for a reply owed before handing on.
func (h *arrivalHarness) answerWith(key string) {
	h.t.Helper()
	h.arrive(key)
	h.answerListen(false)
}

var lateReplyListens = int(resumeLateReplyWait / resumeInputQuiet)

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
						_ = h.writer.Close()
						if err := h.wait(); !errors.Is(err, io.EOF) {
							t.Fatalf("runResumeWait() error = %v, want the EOF that ended a wait still going", err)
						}
						if h.probe.backCalls != 1 {
							t.Errorf("BackToPicker called %d times, want 1", h.probe.backCalls)
						}
						assertNoHandOff(t, h.probe)
					case "d":
						h.answered()
						want := opened(payload)
						want.ReplyOwed = false
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
				h.arrive("\r")
				h.answerListen(false)

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
			})
		}
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
				h.lateReply(tc.reply)

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
				if got, want := h.reader.reads.Load(), int64(1+len(tc.reply)); got != want {
					t.Errorf("the wait read %d bytes, want Enter and the reply's %d read off before the hook", got, len(tc.reply))
				}
			})
		}
	})

	t.Run("the confirmation's y waits for the reply owed before the shell", func(t *testing.T) {
		payload := owing(confirmationPayload())
		h := startArrivals(t, payload)

		h.answerWith("y")
		h.lateReply(lateDarkReply)

		h.answered()
		assertDiscardAnswered(t, h.probe, payload)
	})

	t.Run("Enter hands on once the wait for a reply that never comes is spent", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.silentFor(lateReplyListens)

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("a key typed in the wait ends it", func(t *testing.T) {
		payload := owing(drawnPayload())
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.answerWith("\r")
		h.lateReply("x")

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
		if !slices.Contains(h.probe.execArgs, flagArg(resumeFlagReplyOwed)) {
			t.Errorf("confirmation argv = %q, want the reply still owed: %s", h.probe.execArgs, flagArg(resumeFlagReplyOwed))
		}
	})
}

func TestResumeLateReply_TheDraw(t *testing.T) {
	cases := []struct {
		name       string
		owed       bool
		unanswered bool
		want       bool
	}{
		{"a question answered", false, false, false},
		{"a question unanswered", false, true, true},
		{"a reply owed from before, its question answered", true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := samplePayload()
			payload.ReplyOwed = tc.owed
			probe := &resumeDrawProbe{unanswered: tc.unanswered}

			if err := runResumeDraw(newResumeDrawConfig(t, probe, payload, fixedSize(100, 30))); err != nil {
				t.Fatalf("runResumeDraw() error = %v", err)
			}

			if got := waiterOf(t, probe.execArgs).ReplyOwed; got != tc.want {
				t.Errorf("waiter's ReplyOwed = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("the reply owed round-trips through a redraw's argv", func(t *testing.T) {
		payload := owing(samplePayload())
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
}
