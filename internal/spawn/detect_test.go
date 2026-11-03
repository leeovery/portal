package spawn

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// Declared independently of the production constants so the test locks the
// catalog wording rather than mirroring it.
const (
	wantMsgResolved   = "detection resolved host terminal"
	wantMsgNullBundle = "detection resolved no host-local terminal"
	wantMsgUnsure     = "detection could not tell the host terminal"
	wantMsgTransient  = "detection transient failure"
)

var phase1ForbiddenAttrKeys = []string{"resolution", "session", "ack", "opened", "total", "batch"}

func assertNoWarn(t *testing.T, sink *logtest.Sink) {
	t.Helper()
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn {
			t.Errorf("unexpected WARN record emitted: %q %+v", r.Msg, r.Attrs)
		}
	}
}

// outsideDetector detects outside any session, with env as the environment.
func outsideDetector(t *testing.T, env map[string]string, logger *slog.Logger) *Detector {
	return &Detector{
		sessions: fakeSessions{},
		getenv:   mapGetenv(env),
		selfPID:  100,
		walker:   failWalker{t},
		reader:   failReader{t},
		logger:   logger,
	}
}

func TestDetectorDetect(t *testing.T) {
	t.Run("it emits a resolved INFO with terminal and bundle_id and returns the identity", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		d := outsideDetector(t, map[string]string{"__CFBundleIdentifier": "com.apple.Terminal"}, logger)

		got := d.Detect()

		if got.BundleID != "com.apple.Terminal" || got.Name != "Terminal" {
			t.Fatalf("identity = %+v, want the resolved Apple Terminal identity", got)
		}
		rec := sink.Records().Only(t, "log record")
		if rec.Level != slog.LevelInfo {
			t.Errorf("record level = %v, want INFO", rec.Level)
		}
		if rec.Msg != wantMsgResolved {
			t.Errorf("record message = %q, want %q", rec.Msg, wantMsgResolved)
		}
		if v := rec.AttrString(t, "terminal"); v != "Terminal" {
			t.Errorf("terminal attr = %q, want %q", v, "Terminal")
		}
		if v := rec.AttrString(t, "bundle_id"); v != "com.apple.Terminal" {
			t.Errorf("bundle_id attr = %q, want %q", v, "com.apple.Terminal")
		}
		if v := rec.AttrString(t, "detail"); v != "outside-session env/self walk" {
			t.Errorf("detail = %q, want the outside-session route", v)
		}
		assertNoWarn(t, sink)
	})

	t.Run("it emits a NULL-bundle INFO with no WARN for a remote leader", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		d, _, _ := insideDetector(leaderWith(), local(601))
		d.logger = logger

		got := d.Detect()

		if !got.IsNull() || got.Unsure {
			t.Fatalf("identity = %+v, want a plain NULL for a remote leader", got)
		}
		rec := sink.Records().Only(t, "log record")
		if rec.Level != slog.LevelInfo || rec.Msg != wantMsgNullBundle {
			t.Errorf("record = %v %q, want INFO %q", rec.Level, rec.Msg, wantMsgNullBundle)
		}
		if v := rec.AttrString(t, "detail"); v != "inside-session raw client walk" {
			t.Errorf("detail = %q, want the raw client route", v)
		}
		if rec.HasAttr("terminal") || rec.HasAttr("bundle_id") {
			t.Errorf("NULL-bundle record must carry neither terminal nor bundle_id: %+v", rec.Attrs)
		}
		assertNoWarn(t, sink)
	})

	t.Run("it folds an uncertain leader to NULL with an INFO saying why", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		d, _, _ := insideDetector(leaderWith(), local(501), local(502))
		d.logger = logger

		got := d.Detect()

		if !got.IsNull() || !got.Unsure {
			t.Fatalf("identity = %+v, want an unsure NULL for two raw clients", got)
		}
		rec := sink.Records().Only(t, "log record")
		if rec.Level != slog.LevelInfo || rec.Msg != wantMsgUnsure {
			t.Errorf("record = %v %q, want INFO %q", rec.Level, rec.Msg, wantMsgUnsure)
		}
		want := `inside-session raw client walk: no one terminal leads the session: 2 of 2 zmx attach processes on session "dev" can be its leader, want 1`
		if v := rec.AttrString(t, "detail"); v != want {
			t.Errorf("detail = %q, want %q", v, want)
		}
		assertNoWarn(t, sink)
	})

	t.Run("it folds a transient detection error to NULL and emits a spawn WARN", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		failure := errors.New("no answer in 1s")
		d, _, _ := insideDetector(nil)
		d.sessions = fakeSessions{current: "dev", err: failure}
		d.logger = logger

		got := d.Detect()

		if !got.IsNull() || !got.Unsure {
			t.Fatalf("identity = %+v, want an unsure NULL folded from the transient error", got)
		}
		rec := sink.Records().Only(t, "log record")
		if rec.Level != slog.LevelWarn || rec.Msg != wantMsgTransient {
			t.Errorf("record = %v %q, want WARN %q", rec.Level, rec.Msg, wantMsgTransient)
		}
		if detail := rec.AttrString(t, "detail"); !strings.Contains(detail, failure.Error()) {
			t.Errorf("WARN detail = %q, want it to carry the underlying error %q", detail, failure.Error())
		}
		if rec.HasAttr("terminal") || rec.HasAttr("bundle_id") {
			t.Errorf("transient WARN must carry neither terminal nor bundle_id: %+v", rec.Attrs)
		}
	})

	t.Run("it emits only the terminal, bundle_id and detail attr keys", func(t *testing.T) {
		for name, detector := range map[string]func(*slog.Logger) *Detector{
			"resolved": func(logger *slog.Logger) *Detector {
				return outsideDetector(t, map[string]string{"__CFBundleIdentifier": "com.apple.Terminal"}, logger)
			},
			"clean NULL": func(logger *slog.Logger) *Detector {
				d, _, _ := insideDetector(leaderWith(), local(601))
				d.logger = logger
				return d
			},
			"uncertain": func(logger *slog.Logger) *Detector {
				d, _, _ := insideDetector(nil)
				d.logger = logger
				return d
			},
			"transient": func(logger *slog.Logger) *Detector {
				d, _, _ := insideDetector(nil)
				d.sessions = fakeSessions{current: "dev", err: errors.New("boom")}
				d.logger = logger
				return d
			},
		} {
			t.Run(name, func(t *testing.T) {
				logger, sink := logtest.NewCaptureLogger(t)
				detector(logger).Detect()
				for _, rec := range sink.Records() {
					for _, forbidden := range phase1ForbiddenAttrKeys {
						if rec.HasAttr(forbidden) {
							t.Errorf("record %q carries forbidden attr key %q: %+v", rec.Msg, forbidden, rec.Attrs)
						}
					}
				}
			})
		}
	})

	t.Run("outside a session it reads the environment and asks no leader", func(t *testing.T) {
		logger, _ := logtest.NewCaptureLogger(t)
		d := outsideDetector(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, logger)
		d.runners = &fakeRunners{err: errors.New("asked a runner outside a session")}

		if got := d.Detect(); got.BundleID != "com.mitchellh.ghostty" {
			t.Errorf("BundleID = %q, want the environment's Ghostty", got.BundleID)
		}
	})
}

func TestNewDetectorWiresProductionSeams(t *testing.T) {
	sessions, runners, clients := &fakeSessions{current: "dev"}, &fakeRunners{}, &fakeClients{}
	d := NewDetector(sessions, runners, clients)

	if d.sessions != sessions || d.runners != runners || d.clients != clients {
		t.Error("NewDetector did not keep the seams it was given")
	}
	if d.getenv == nil || d.selfPID == 0 || d.walker == nil || d.reader == nil {
		t.Error("NewDetector left a production seam unwired")
	}
	if d.logger == nil {
		t.Error("NewDetector left the logger unwired")
	}
}
