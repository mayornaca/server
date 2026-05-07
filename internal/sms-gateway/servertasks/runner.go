// Package servertasks runs periodic background tasks within the server process.
//
// The worker binary is the canonical home for cleanup tasks, but tasks that
// need full access to the server's service graph (events, schedules,
// posts, tests) live here to avoid module duplication.
//
// Coordination across replicas uses MySQL GET_LOCK via locker.Locker with a
// "server:" prefix to avoid collisions with worker keys.
package servertasks

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/posts"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/schedules"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/tests"
	"github.com/android-sms-gateway/server/internal/worker/locker"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	lockerPrefix    = "server:"
	lockerTimeout   = 5 * time.Second
	tickInterval    = 1 * time.Minute
	cronDispatchLK  = "tests:cron_dispatcher"
	expirePendLK    = "tests:expire_pending"
	retryFailedLK   = "tests:retry_failed"
)

type RunnerParams struct {
	fx.In

	DB *sql.DB

	SchedulesSvc *schedules.Service
	TestsSvc     *tests.Service
	PostsSvc     *posts.Service
	RetryConfig  tests.RetryConfig

	Logger *zap.Logger
}

type Runner struct {
	locker locker.Locker

	schedulesSvc *schedules.Service
	testsSvc     *tests.Service
	postsSvc     *posts.Service
	retryCfg     tests.RetryConfig

	logger *zap.Logger

	// lastEval tracks the previous dispatcher tick boundary so the cron
	// evaluator can find schedules whose cron would have fired in the
	// interval (lastEval, now].
	lastEval time.Time

	// lastRetryRun tracks when the retry sweep last ran, used to gate the
	// task to the configured Interval (which is independent of tickInterval=1m).
	lastRetryRun time.Time
}

func NewRunner(p RunnerParams) *Runner {
	return &Runner{
		locker:       locker.NewMySQLLocker(p.DB, lockerPrefix, lockerTimeout),
		schedulesSvc: p.SchedulesSvc,
		testsSvc:     p.TestsSvc,
		postsSvc:     p.PostsSvc,
		retryCfg:     p.RetryConfig,
		logger:       p.Logger.Named("servertasks"),
		lastEval:     time.Now().Truncate(time.Minute),
	}
}

// Run starts the periodic loop. Blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	r.logger.Info("server tasks runner started", zap.Duration("interval", tickInterval))
	defer r.logger.Info("server tasks runner stopped")
	defer func() { _ = r.locker.Close() }()

	// Initial tick almost immediately so first run does not wait a full minute
	// on boot.
	initial := time.NewTimer(5 * time.Second)
	select {
	case <-ctx.Done():
		initial.Stop()
		return
	case <-initial.C:
	}

	r.tick(ctx)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	now := time.Now().Truncate(time.Minute)

	// cloud-gesvial.19.1 M-MED-2: only advance lastEval when this replica
	// actually held the lock. Pre-fix a replica that lost the lock to a
	// peer still bumped its own clock; on the next tick where it DOES hold
	// the lock, FetchDue would skip the just-evaluated interval (it was
	// already advanced). Single-replica deployments are unaffected (the
	// lock always succeeds), but multi-replica setups would starve.
	if r.runWithLock(ctx, cronDispatchLK, func(ctx context.Context) {
		r.runCronDispatcher(ctx, r.lastEval, now)
	}) {
		r.lastEval = now
	}

	r.runWithLock(ctx, expirePendLK, func(ctx context.Context) {
		r.runExpirePending(ctx)
	})

	// Retry sweep is gated by its own interval (default 15 min) so it doesn't
	// run on every 1-minute tick. Skip when disabled via config. Same
	// lock-aware bookkeeping as cronDispatch above.
	if r.retryCfg.Enabled && time.Since(r.lastRetryRun) >= r.retryCfg.Interval {
		if r.runWithLock(ctx, retryFailedLK, func(ctx context.Context) {
			r.runRetryFailed(ctx)
		}) {
			r.lastRetryRun = time.Now()
		}
	}
}

// runWithLock returns true when this replica held the lock for the duration
// of fn (whether fn panicked or not). False means another replica owned it
// or the lock backend errored — callers can use this to decide whether their
// state-bookkeeping (e.g. lastEval) should advance. cloud-gesvial.19.1.
func (r *Runner) runWithLock(ctx context.Context, key string, fn func(context.Context)) bool {
	if err := r.locker.AcquireLock(ctx, key); err != nil {
		if errors.Is(err, locker.ErrLockNotAcquired) {
			// Another replica is running this tick; skip silently.
			return false
		}
		r.logger.Warn("failed to acquire lock", zap.String("key", key), zap.Error(err))
		return false
	}
	defer func() {
		if err := r.locker.ReleaseLock(ctx, key); err != nil {
			r.logger.Warn("failed to release lock", zap.String("key", key), zap.Error(err))
		}
	}()

	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("task panicked", zap.String("key", key), zap.Any("recover", rec))
		}
	}()

	fn(ctx)
	return true
}
