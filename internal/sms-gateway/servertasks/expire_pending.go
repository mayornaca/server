package servertasks

import (
	"context"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/tests"
	"go.uber.org/zap"
)

// runExpirePending marks PENDING test results older than tests.PendingTTL as
// ERROR with a timeout error message.
func (r *Runner) runExpirePending(ctx context.Context) {
	_, err := r.testsSvc.ExpirePending(ctx, tests.PendingTTL)
	if err != nil {
		r.logger.Warn("failed to expire pending tests", zap.Error(err))
	}
}
