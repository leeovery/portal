package bootstrap

import "log/slog"

type Option func(*defaultsConfig)

type defaultsConfig struct {
	orphanSweeper OrphanSweeper
	saver         SaverBootstrapper
	restore       Restorer
}

func WithOrphanSweeper(s OrphanSweeper) Option {
	return func(c *defaultsConfig) { c.orphanSweeper = s }
}

func WithSaver(s SaverBootstrapper) Option {
	return func(c *defaultsConfig) { c.saver = s }
}

func WithRestore(r Restorer) Option {
	return func(c *defaultsConfig) { c.restore = r }
}

// NewWithDefaults fills every degradable step seam with a NoOp, so no seam is
// ever nil. restoring is mandatory: its steps are fatal and have no NoOp form.
func NewWithDefaults(logger *slog.Logger, restoring RestoringMarker, opts ...Option) *Orchestrator {
	cfg := defaultsConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.orphanSweeper == nil {
		cfg.orphanSweeper = NoOpOrphanSweeper{}
	}
	if cfg.saver == nil {
		cfg.saver = NoOpSaver{}
	}
	if cfg.restore == nil {
		cfg.restore = NoOpRestorer{}
	}

	return &Orchestrator{
		Restoring:     restoring,
		OrphanSweeper: cfg.orphanSweeper,
		Saver:         cfg.saver,
		Restore:       cfg.restore,
		Logger:        logger,
	}
}
