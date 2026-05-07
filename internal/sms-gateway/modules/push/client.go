package push

import (
	"errors"
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/fcm"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/noop"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/upstream"
	"go.uber.org/zap"
)

var ErrInvalidPushMode = errors.New("invalid push mode")

// newClient builds the underlying push client according to config.Mode. The
// logger is forwarded to backends that emit per-message log lines (FCM since
// cloud-gesvial.19.3) so the operator can trace which device received what
// notification in `docker compose logs server`.
func newClient(config Config, logger *zap.Logger) (client.Client, error) {
	var (
		c   client.Client
		err error
	)

	switch config.Mode {
	case ModeFCM:
		c, err = fcm.New(config.ClientOptions, logger)
	case ModeUpstream:
		c, err = upstream.New(config.ClientOptions)
	case ModeDisabled:
		c, err = noop.New(config.ClientOptions)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidPushMode, config.Mode)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}
