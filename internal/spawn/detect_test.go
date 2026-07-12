package spawn

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// Expected spawn closed-event-catalog message strings. Declared independently
// of the production constants so the test locks the catalog wording rather than
// tautologically mirroring it.
const (
	wantMsgResolved   = "detection resolved host terminal"
	wantMsgNullBundle = "detection resolved no host-local terminal"
	wantMsgTransient  = "detection transient failure"
)

// phase1ForbiddenAttrKeys are the spawn attr keys reserved for later phases
// (adapter resolution / burst). No Phase-1 detection record may carry any of
// them — only terminal, bundle_id, and the opaque detail are in scope.
var phase1ForbiddenAttrKeys = []string{"resolution", "session", "ack", "opened", "total", "batch"}

func assertNoWarn(t *testing.T, sink *logtest.Sink) {
	t.Helper()
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn {
			t.Errorf("unexpected WARN record emitted: %q %+v", r.Msg, r.Attrs)
		}
	}
}

func TestDetectorDetect(t *testing.T) {
	t.Run("it emits a resolved INFO with terminal and bundle_id and returns the identity", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		d := &Detector{
			insideTmux: func() bool { return false },
			getenv:     mapGetenv(map[string]string{"__CFBundleIdentifier": "com.apple.Terminal"}),
			selfPID:    100,
			walker:     failWalker{t}, // env fast-path resolves; the walk must not run
			reader:     failReader{t},
			logger:     logger,
		}

		got := d.Detect()

		if got.BundleID != "com.apple.Terminal" || got.Name != "Terminal" {
			t.Fatalf("identity = %+v, want the resolved Apple Terminal identity", got)
		}
		rec := sink.OnlyRecord(t)
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
		if !rec.HasAttr("detail") || rec.AttrString(t, "detail") == "" {
			t.Errorf("resolved record missing a non-empty opaque detail attr: %+v", rec.Attrs)
		}
		assertNoWarn(t, sink)
	})

	t.Run("it emits a NULL-bundle INFO with no WARN for a clean remote-only detection", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		walker, reader := localWalkSeams()
		d := &Detector{
			insideTmux:     func() bool { return true },
			currentSession: func() (string, error) { return "dev", nil },
			lister: &fakeClientLister{clients: []ClientActivity{
				{PID: 601, Activity: 100}, // mosh — walks to NULL
				{PID: 602, Activity: 200}, // mosh — walks to NULL
			}},
			walker: walker,
			reader: reader,
			logger: logger,
		}

		got := d.Detect()

		if !got.IsNull() {
			t.Fatalf("identity = %+v, want NULL for a remote-only client set", got)
		}
		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("record level = %v, want INFO", rec.Level)
		}
		if rec.Msg != wantMsgNullBundle {
			t.Errorf("record message = %q, want %q", rec.Msg, wantMsgNullBundle)
		}
		if rec.HasAttr("terminal") || rec.HasAttr("bundle_id") {
			t.Errorf("NULL-bundle record must carry neither terminal nor bundle_id: %+v", rec.Attrs)
		}
		assertNoWarn(t, sink)
	})

	t.Run("it folds a transient detection error to NULL and emits a spawn WARN", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		listFailure := errors.New("list-clients: server not found")
		d := &Detector{
			insideTmux:     func() bool { return true },
			currentSession: func() (string, error) { return "dev", nil },
			lister:         &fakeClientLister{err: listFailure},
			walker:         &fakeWalker{procs: map[int]fakeProc{}},
			reader:         &fakeReader{bundles: map[string]fakeBundle{}},
			logger:         logger,
		}

		got := d.Detect()

		if !got.IsNull() {
			t.Fatalf("identity = %+v, want NULL folded from the transient error", got)
		}
		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("record level = %v, want WARN", rec.Level)
		}
		if rec.Msg != wantMsgTransient {
			t.Errorf("record message = %q, want %q", rec.Msg, wantMsgTransient)
		}
		detail := rec.AttrString(t, "detail")
		if !strings.Contains(detail, listFailure.Error()) {
			t.Errorf("WARN detail = %q, want it to carry the underlying error %q", detail, listFailure.Error())
		}
		if rec.HasAttr("terminal") || rec.HasAttr("bundle_id") {
			t.Errorf("transient WARN must carry neither terminal nor bundle_id: %+v", rec.Attrs)
		}
	})

	t.Run("it emits only the terminal, bundle_id and detail attr keys in phase 1", func(t *testing.T) {
		walker, reader := localWalkSeams()
		scenarios := []struct {
			name     string
			detector func(logger *slog.Logger) *Detector
		}{
			{
				name: "resolved",
				detector: func(logger *slog.Logger) *Detector {
					return &Detector{
						insideTmux: func() bool { return false },
						getenv:     mapGetenv(map[string]string{"__CFBundleIdentifier": "com.apple.Terminal"}),
						selfPID:    100,
						walker:     failWalker{t},
						reader:     failReader{t},
						logger:     logger,
					}
				},
			},
			{
				name: "clean NULL",
				detector: func(logger *slog.Logger) *Detector {
					return &Detector{
						insideTmux:     func() bool { return true },
						currentSession: func() (string, error) { return "dev", nil },
						lister:         &fakeClientLister{clients: []ClientActivity{{PID: 601, Activity: 100}}},
						walker:         walker,
						reader:         reader,
						logger:         logger,
					}
				},
			},
			{
				name: "transient",
				detector: func(logger *slog.Logger) *Detector {
					return &Detector{
						insideTmux:     func() bool { return true },
						currentSession: func() (string, error) { return "dev", nil },
						lister:         &fakeClientLister{err: errors.New("boom")},
						walker:         &fakeWalker{procs: map[int]fakeProc{}},
						reader:         &fakeReader{bundles: map[string]fakeBundle{}},
						logger:         logger,
					}
				},
			},
		}

		for _, sc := range scenarios {
			t.Run(sc.name, func(t *testing.T) {
				logger, sink := logtest.NewCaptureLogger(t)
				sc.detector(logger).Detect()
				for _, rec := range sink.Records() {
					for _, forbidden := range phase1ForbiddenAttrKeys {
						if rec.HasAttr(forbidden) {
							t.Errorf("record %q carries forbidden Phase-1 attr key %q: %+v", rec.Msg, forbidden, rec.Attrs)
						}
					}
				}
			})
		}
	})

	t.Run("it branches to inside-tmux detection when TMUX is set", func(t *testing.T) {
		logger, _ := logtest.NewCaptureLogger(t)
		sessionAsked := false
		lister := &fakeClientLister{clients: []ClientActivity{{PID: 501, Activity: 100}}}
		walker, reader := localWalkSeams()
		d := &Detector{
			insideTmux: func() bool { return true },
			getenv: func(string) string {
				t.Fatalf("getenv called: inside-tmux detection must not use the env fast-path")
				return ""
			},
			currentSession: func() (string, error) { sessionAsked = true; return "dev", nil },
			lister:         lister,
			walker:         walker,
			reader:         reader,
			logger:         logger,
		}

		got := d.Detect()

		if got.BundleID != "com.mitchellh.ghostty" {
			t.Errorf("BundleID = %q, want the inside-tmux walk's %q", got.BundleID, "com.mitchellh.ghostty")
		}
		if !sessionAsked {
			t.Error("currentSession was not consulted; inside-tmux branch did not run")
		}
		if len(lister.calls) != 1 || lister.calls[0] != "dev" {
			t.Errorf("lister calls = %v, want exactly [dev]", lister.calls)
		}
	})
}

func TestNewDetectorWiresProductionSeams(t *testing.T) {
	// NewDetector must compile against a real *tmux.Client and wire every seam.
	// A typed-nil client is a valid *tmux.Client for construction (no method is
	// invoked here), so this exercises the wiring without a live tmux server.
	d := NewDetector(nil)

	if d == nil {
		t.Fatal("NewDetector returned nil")
	}
	if d.insideTmux == nil || d.getenv == nil || d.currentSession == nil {
		t.Error("NewDetector left a function seam unwired")
	}
	if d.walker == nil || d.reader == nil || d.lister == nil {
		t.Error("NewDetector left a walk/list seam unwired")
	}
	if d.logger == nil {
		t.Error("NewDetector left the logger unwired")
	}
}
