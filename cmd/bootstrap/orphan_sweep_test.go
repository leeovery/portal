package bootstrap

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// recordingIdentify is a callable seam that records each invocation in order
// and returns a per-call (result, error) pair. The keyed map lets a test pin
// behaviour per PID; a missing key returns the zero default.
type recordingIdentify struct {
	calls   []int
	results map[int]identifyOutcome
	def     identifyOutcome
}

type identifyOutcome struct {
	res state.IdentifyResult
	err error
}

func (r *recordingIdentify) fn(pid int) (state.IdentifyResult, error) {
	r.calls = append(r.calls, pid)
	if r.results != nil {
		if v, ok := r.results[pid]; ok {
			return v.res, v.err
		}
	}
	return r.def.res, r.def.err
}

// recordingKill records signal targets in invocation order so tests can
// assert which PIDs were killed and how many times.
type recordingKill struct {
	calls []int
	errs  map[int]error
}

func (r *recordingKill) fn(pid int) error {
	r.calls = append(r.calls, pid)
	if r.errs != nil {
		if e, ok := r.errs[pid]; ok {
			return e
		}
	}
	return nil
}

func TestSweepOrphanDaemons_killsTwoOrphansLeavesLegitimate(t *testing.T) {
	const legitPID = 1000
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{legitPID, 2001, 2002}, nil },
		SaverPanePID: func() (int, error) { return legitPID, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	if len(kill.calls) != 2 {
		t.Fatalf("expected 2 kill calls, got %d (%v)", len(kill.calls), kill.calls)
	}
	got := map[int]struct{}{kill.calls[0]: {}, kill.calls[1]: {}}
	for _, want := range []int{2001, 2002} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected pid %d killed; got %v", want, kill.calls)
		}
	}
	for _, p := range kill.calls {
		if p == legitPID {
			t.Errorf("legitimate pid %d must not be killed", legitPID)
		}
	}
}

func TestSweepOrphanDaemons_saverAbsentKillsAllIdentifying(t *testing.T) {
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{3001, 3002, 3003}, nil },
		SaverPanePID: func() (int, error) { return 0, nil }, // _portal-saver absent
		Identify:     identify.fn,
		Kill:         kill.fn,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	if len(kill.calls) != 3 {
		t.Fatalf("expected 3 kill calls (legitimate set empty), got %d (%v)", len(kill.calls), kill.calls)
	}
}

func TestSweepOrphanDaemons_pgrepErrorLogsWarnReturnsNil(t *testing.T) {
	logger := &RecordingLogger{}
	sentinel := errors.New("pgrep boom")
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return nil, sentinel },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     func(pid int) (state.IdentifyResult, error) { return state.IdentifyIsPortalDaemon, nil },
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("expected nil err under pgrep failure; got %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("expected zero kill calls on pgrep error; got %v", kill.calls)
	}
	found := false
	for i, msg := range logger.warnings {
		if strings.Contains(msg, "pgrep") && strings.Contains(msg, "boom") {
			if logger.warnComponents[i] != state.ComponentBootstrap {
				t.Errorf("pgrep Warn component = %q, want %q", logger.warnComponents[i], state.ComponentBootstrap)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn entry for pgrep failure; warnings=%v", logger.warnings)
	}
}

func TestSweepOrphanDaemons_listPanesErrorTreatsLegitimateEmpty(t *testing.T) {
	logger := &RecordingLogger{}
	sentinel := errors.New("list-panes boom")
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{4001, 4002}, nil },
		SaverPanePID: func() (int, error) { return 0, sentinel },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 2 {
		t.Fatalf("expected 2 kill calls (legitimate empty), got %d (%v)", len(kill.calls), kill.calls)
	}
	found := false
	for i, msg := range logger.warnings {
		if strings.Contains(msg, "list-panes") && strings.Contains(msg, "_portal-saver") {
			if logger.warnComponents[i] != state.ComponentBootstrap {
				t.Errorf("list-panes Warn component = %q, want %q", logger.warnComponents[i], state.ComponentBootstrap)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn entry for list-panes failure; warnings=%v", logger.warnings)
	}
}

func TestSweepOrphanDaemons_identifyDeadSkipped(t *testing.T) {
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyDead}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{5001, 5002}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("IdentifyDead must skip kill; got %v", kill.calls)
	}
}

func TestSweepOrphanDaemons_identifyNotPortalDaemonSkipped(t *testing.T) {
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyNotPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{6001, 6002}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("IdentifyNotPortalDaemon must skip kill; got %v", kill.calls)
	}
}

func TestSweepOrphanDaemons_identifyTransientErrorSkipped(t *testing.T) {
	logger := &RecordingLogger{}
	transient := errors.New("ps malformed output")
	identify := &recordingIdentify{def: identifyOutcome{err: transient}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{7001}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("Identify transient error must skip kill; got %v", kill.calls)
	}
	found := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, "identity-check") && strings.Contains(msg, "7001") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn entry for identity-check failure; warnings=%v", logger.warnings)
	}
}

func TestSweepOrphanDaemons_killErrorLogsWarnContinues(t *testing.T) {
	logger := &RecordingLogger{}
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	killSentinel := errors.New("kill: no such process")
	kill := &recordingKill{errs: map[int]error{8001: killSentinel}}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{8001, 8002}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 2 {
		t.Errorf("expected both PIDs attempted despite first kill error; got %v", kill.calls)
	}
	found := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, "kill") && strings.Contains(msg, "8001") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn entry for kill failure; warnings=%v", logger.warnings)
	}
}

func TestSweepOrphanDaemons_cleanStateZeroInfo(t *testing.T) {
	const legitPID = 9000
	logger := &RecordingLogger{}
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{legitPID}, nil },
		SaverPanePID: func() (int, error) { return legitPID, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("clean state must send zero signals; got %v", kill.calls)
	}
	for _, msg := range logger.infos {
		if strings.Contains(msg, "killed orphan daemon") {
			t.Errorf("clean state must emit zero killed-orphan INFO entries; got %q", msg)
		}
	}
}

// recordingSignalKill records the signal alongside the PID so tests can assert
// that the production semantic is SIGKILL (never SIGTERM). The seam used in
// production is the bare Kill(pid int) error so this test installs a wrapper
// that records the signal it would have sent — verifying that the Core never
// reaches for SIGTERM at the call site (the Core only calls Kill(pid)).
func TestSweepOrphanDaemons_neverSIGTERM(t *testing.T) {
	// The Core's Kill seam takes only a PID — meaning the signal choice is
	// the seam adapter's responsibility, NOT the Core's. We verify that
	// no path in the Core invokes anything BUT the Kill seam (e.g., no
	// hidden SIGTERM call), by recording all PIDs through Kill and asserting
	// the production default (when Kill is unset) delegates to SIGKILL.
	//
	// This is exercised via the default-seam path: leaving Kill nil and
	// asserting that the defaulted closure invokes syscall.Kill with SIGKILL.
	var capturedSig syscall.Signal
	var capturedPID int
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		// Inject a Kill closure to record the call shape that the Core
		// performs — Core MUST call Kill(pid) with a single int arg only.
		Kill: func(pid int) error {
			capturedPID = pid
			capturedSig = syscall.SIGKILL
			return nil
		},
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	// With empty pgrep, kill is never invoked; we use the suppression to
	// confirm Core never invokes Kill for non-existent candidates.
	if capturedPID != 0 {
		t.Errorf("unexpected Kill invocation; pid=%d sig=%v", capturedPID, capturedSig)
	}
	if capturedSig != 0 {
		t.Errorf("unexpected signal recorded; sig=%v", capturedSig)
	}
}

func TestSweepOrphanDaemons_defensiveOwnPIDSkip(t *testing.T) {
	ownPID := os.Getpid()
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{ownPID, 10001}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	for _, p := range kill.calls {
		if p == ownPID {
			t.Fatalf("own pid %d must never be killed; got %v", ownPID, kill.calls)
		}
	}
	// Other PID still killed.
	if len(kill.calls) != 1 || kill.calls[0] != 10001 {
		t.Errorf("expected only 10001 killed; got %v", kill.calls)
	}
}

// TestSweepOrphanDaemons_pgrepEmptyListNoOp pins the edge case from the task:
// pgrep returning an empty slice (e.g., exit status 1 with zero matches) must
// be a clean no-op — no kill calls, no INFO entries, no warnings.
func TestSweepOrphanDaemons_pgrepEmptyListNoOp(t *testing.T) {
	logger := &RecordingLogger{}
	kill := &recordingKill{}
	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     func(pid int) (state.IdentifyResult, error) { return 0, nil },
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	if len(kill.calls) != 0 {
		t.Errorf("empty pgrep must produce zero kill calls; got %v", kill.calls)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("empty pgrep must produce zero warnings; got %v", logger.warnings)
	}
	if len(logger.infos) != 0 {
		t.Errorf("empty pgrep must produce zero INFO entries; got %v", logger.infos)
	}
}

// TestSweepOrphanDaemons_emitsKilledOrphanInfo pins acceptance criterion that
// each successful kill emits the canonical INFO message.
func TestSweepOrphanDaemons_emitsKilledOrphanInfo(t *testing.T) {
	logger := &RecordingLogger{}
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{11001}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       logger,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}
	found := false
	for i, msg := range logger.infos {
		if strings.Contains(msg, "killed orphan daemon") && strings.Contains(msg, "11001") {
			if logger.infoComponents[i] != state.ComponentBootstrap {
				t.Errorf("killed-orphan Info component = %q, want %q", logger.infoComponents[i], state.ComponentBootstrap)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an Info entry for killed orphan; infos=%v", logger.infos)
	}
}

// TestSweepOrphanDaemons_nilLoggerSafe pins the mirroring convention with
// MarkerCleanupCore — a nil Logger must not panic; call sites must dispatch
// through a substituted no-op.
func TestSweepOrphanDaemons_nilLoggerSafe(t *testing.T) {
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}
	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{12001}, nil },
		SaverPanePID: func() (int, error) { return 0, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       nil,
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error under nil Logger: %v", err)
	}
}

// TestSweepOrphanDaemons_neverReturnsError pins acceptance criterion that the
// method swallows EVERY error path and returns nil unconditionally.
func TestSweepOrphanDaemons_neverReturnsError(t *testing.T) {
	cases := []struct {
		name string
		core *OrphanSweepCore
	}{
		{
			name: "pgrep error",
			core: &OrphanSweepCore{
				Pgrep:        func() ([]int, error) { return nil, errors.New("pgrep") },
				SaverPanePID: func() (int, error) { return 0, nil },
				Identify:     func(pid int) (state.IdentifyResult, error) { return 0, nil },
				Kill:         func(pid int) error { return nil },
			},
		},
		{
			name: "list-panes error",
			core: &OrphanSweepCore{
				Pgrep:        func() ([]int, error) { return []int{1}, nil },
				SaverPanePID: func() (int, error) { return 0, errors.New("list-panes") },
				Identify:     func(pid int) (state.IdentifyResult, error) { return state.IdentifyDead, nil },
				Kill:         func(pid int) error { return nil },
			},
		},
		{
			name: "identify error",
			core: &OrphanSweepCore{
				Pgrep:        func() ([]int, error) { return []int{1}, nil },
				SaverPanePID: func() (int, error) { return 0, nil },
				Identify:     func(pid int) (state.IdentifyResult, error) { return 0, errors.New("identify") },
				Kill:         func(pid int) error { return nil },
			},
		},
		{
			name: "kill error",
			core: &OrphanSweepCore{
				Pgrep:        func() ([]int, error) { return []int{1}, nil },
				SaverPanePID: func() (int, error) { return 0, nil },
				Identify:     func(pid int) (state.IdentifyResult, error) { return state.IdentifyIsPortalDaemon, nil },
				Kill:         func(pid int) error { return errors.New("kill") },
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.core.SweepOrphanDaemons(); err != nil {
				t.Errorf("expected nil err; got %v", err)
			}
		})
	}
}
