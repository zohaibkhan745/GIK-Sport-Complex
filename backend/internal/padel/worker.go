package padel

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// CleanerConfig configures the periodic background worker that releases expired holds.
type CleanerConfig struct {
	// Interval specifies the ticker duration between consecutive cleanup sweeps.
	// Defaults to 30 seconds if <= 0.
	Interval time.Duration

	// BatchTimeout bounds the execution duration of a single cleanup query sweep.
	// Defaults to 5 seconds if <= 0.
	BatchTimeout time.Duration
}

// HoldCleanerWorker runs a periodic background ticker routine to transition lapsed held slots
// (now >= hold_expires_at) back to AVAILABLE (SE-423 §2 Table 1 No. 4, FR-03).
//
// Concurrency & Lifecycle:
// - Thread-safe start and stop.
// - Idempotent Start() and Stop() methods.
// - Clean goroutine termination upon shutdown signal without leaking resources.
type HoldCleanerWorker struct {
	repo    Repository
	clock   TimeProvider
	cfg     CleanerConfig
	stopCh  chan struct{}
	doneCh  chan struct{}
	running atomic.Bool
	stopMu  sync.Mutex
	logger  func(format string, args ...any)
}

// NewHoldCleanerWorker constructs a HoldCleanerWorker with the specified repository and configuration.
func NewHoldCleanerWorker(repo Repository, cfg CleanerConfig) *HoldCleanerWorker {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.BatchTimeout <= 0 {
		cfg.BatchTimeout = 5 * time.Second
	}
	return &HoldCleanerWorker{
		repo:   repo,
		clock:  realTimeProvider{},
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		logger: log.Printf,
	}
}

// NewHoldCleanerWorkerWithClock constructs a worker with custom clock and logger for deterministic testing.
func NewHoldCleanerWorkerWithClock(repo Repository, clock TimeProvider, cfg CleanerConfig, logger func(format string, args ...any)) *HoldCleanerWorker {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.BatchTimeout <= 0 {
		cfg.BatchTimeout = 5 * time.Second
	}
	if clock == nil {
		clock = realTimeProvider{}
	}
	if logger == nil {
		logger = func(string, ...any) {}
	}
	return &HoldCleanerWorker{
		repo:   repo,
		clock:  clock,
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		logger: logger,
	}
}

// IsRunning reports whether the cleaner background goroutine is actively running.
func (w *HoldCleanerWorker) IsRunning() bool {
	return w.running.Load()
}

// Start launches the cleaner background goroutine.
// Calling Start on an already running worker is a no-op.
func (w *HoldCleanerWorker) Start() {
	if !w.running.CompareAndSwap(false, true) {
		return // already running
	}
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	go w.loop()
}

// Stop signals the background cleaner to terminate and blocks until the loop has exited.
// Calling Stop on a stopped worker is a safe no-op.
func (w *HoldCleanerWorker) Stop() {
	w.stopMu.Lock()
	defer w.stopMu.Unlock()

	if !w.running.CompareAndSwap(true, false) {
		return // already stopped
	}
	close(w.stopCh)
	<-w.doneCh
}

// RunOnce executes a single hold cleanup pass.
// Exported for manual invocation and deterministic integration verification.
func (w *HoldCleanerWorker) RunOnce(ctx context.Context) (int64, error) {
	now := w.clock.Now()
	cleaned, err := w.repo.CleanExpiredHolds(ctx, now)
	if err != nil {
		return 0, err
	}
	return cleaned, nil
}

func (w *HoldCleanerWorker) loop() {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), w.cfg.BatchTimeout)
			cleaned, err := w.RunOnce(ctx)
			cancel()
			if err != nil {
				w.logger("[WARN] Padel hold cleaner sweep failed: %v", err)
			} else if cleaned > 0 {
				w.logger("[INFO] Padel hold cleaner released %d expired hold(s) to AVAILABLE", cleaned)
			}
		}
	}
}
