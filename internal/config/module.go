package config

import (
	"os"
	"strings"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers"
	"github.com/android-sms-gateway/server/internal/sms-gateway/jwt"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/auth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/messages"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/sse"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/tests"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/webhooks"
	"github.com/android-sms-gateway/server/internal/sms-gateway/otp"
	"github.com/android-sms-gateway/server/internal/sms-gateway/pubsub"
	"github.com/capcom6/go-infra-fx/config"
	"github.com/capcom6/go-infra-fx/db"
	"github.com/capcom6/go-infra-fx/http"
	"github.com/go-core-fx/cachefx"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// resolvePushBackend returns the final push backend selection, honoring the
// modern GATEWAY__PUSH_BACKEND env first and falling back to the deprecated
// GATEWAY__MODE (public→fcm, private→upstream) with a warning.
func resolvePushBackend(cfg Config, log *zap.Logger) PushBackend {
	if cfg.Gateway.PushBackend != "" {
		return cfg.Gateway.PushBackend
	}
	switch cfg.Gateway.Mode {
	case GatewayModePublic:
		log.Warn("GATEWAY__MODE is deprecated; use GATEWAY__PUSH_BACKEND=fcm")
		return PushBackendFCM
	case GatewayModePrivate:
		log.Warn("GATEWAY__MODE is deprecated; use GATEWAY__PUSH_BACKEND=upstream. " +
			"Also verify GATEWAY__UPSTREAM_URL points to a gesvial-operated server; " +
			"the historical default api.sms-gate.app is NOT a validated destination.")
		return PushBackendUpstream
	default:
		return PushBackendFCM // safer default than upstream
	}
}

// resolveFCMCredentials prefers reading from CredentialsJSONFile (if set and
// readable) over the inline CredentialsJSON env. This allows mounting the
// service account JSON as a docker secret at /run/secrets/fcm_sa instead of
// injecting the whole JSON through environment.
func resolveFCMCredentials(cfg FCMConfig, log *zap.Logger) string {
	if cfg.CredentialsJSONFile != "" {
		data, err := os.ReadFile(cfg.CredentialsJSONFile)
		if err != nil {
			log.Error("failed to read FCM credentials file; falling back to FCM__CREDENTIALS_JSON if set",
				zap.String("path", cfg.CredentialsJSONFile),
				zap.Error(err))
		} else {
			return string(data)
		}
	}
	return cfg.CredentialsJSON
}

// looksLikeFCMCredentials is a minimal sanity check — enough to detect the
// default "{}" placeholder or empty strings without doing a full JSON+schema
// validation (which would belong in the FCM client itself).
func looksLikeFCMCredentials(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return false
	}
	// Service account JSONs always contain a "private_key" field; use that as
	// a cheap structural hint. Prevents users from pasting nonsense JSON.
	return strings.Contains(s, "\"private_key\"")
}

// pushBackendToMode maps the public config value to the internal push.Mode.
func pushBackendToMode(b PushBackend) push.Mode {
	switch b {
	case PushBackendFCM:
		return push.ModeFCM
	case PushBackendUpstream:
		return push.ModeUpstream
	case PushBackendDisabled:
		return push.ModeDisabled
	default:
		return push.ModeFCM
	}
}

//nolint:funlen // long function
func Module() fx.Option {
	return fx.Module(
		"appconfig",
		fx.Provide(
			func(log *zap.Logger) Config {
				defaultConfig := Default()

				if err := config.LoadConfig(&defaultConfig); err != nil {
					log.Error("Error loading config", zap.Error(err))
				}

				return defaultConfig
			},
			fx.Private,
		),
		fx.Provide(func(cfg Config) http.Config {
			const writeTimeout = 30 * time.Minute

			return http.Config{
				Listen:  cfg.HTTP.Listen,
				Proxies: cfg.HTTP.Proxies,

				WriteTimeout: writeTimeout, // SSE requires longer timeout
			}
		}),
		fx.Provide(func(cfg Config) db.Config {
			return db.Config{
				Dialect:  db.DialectMySQL,
				Host:     cfg.Database.Host,
				Port:     cfg.Database.Port,
				User:     cfg.Database.User,
				Password: cfg.Database.Password,
				Database: cfg.Database.Database,
				Timezone: cfg.Database.Timezone,
				Debug:    cfg.Database.Debug,

				MaxOpenConns: cfg.Database.MaxOpenConns,
				MaxIdleConns: cfg.Database.MaxIdleConns,

				DSN:             "",
				ConnMaxIdleTime: 0,
				ConnMaxLifetime: 0,
			}
		}),
		fx.Provide(func(cfg Config, log *zap.Logger) push.Config {
			backend := resolvePushBackend(cfg, log)
			credentials := resolveFCMCredentials(cfg.FCM, log)

			// Safety net: if FCM is selected but credentials are absent/empty/obviously
			// invalid, fall back to ModeDisabled instead of crashing at boot. SSE still
			// delivers events; push is just muted until the admin provides the JSON.
			if backend == PushBackendFCM && !looksLikeFCMCredentials(credentials) {
				log.Warn("GATEWAY__PUSH_BACKEND=fcm but FCM credentials are missing or invalid; " +
					"push is disabled for this run. SSE will still deliver events. " +
					"Set FCM__CREDENTIALS_JSON_FILE (or FCM__CREDENTIALS_JSON) with the service account JSON to enable push.")
				backend = PushBackendDisabled
			}

			// Loud warning if someone ever deploys with the old upstream hub as target.
			// gesvial policy: only validated destinations. api.sms-gate.app is NOT validated.
			if backend == PushBackendUpstream && strings.Contains(cfg.Gateway.UpstreamURL, "sms-gate.app") {
				log.Warn("GATEWAY__UPSTREAM_URL points to api.sms-gate.app — this leaks push metadata to a third-party hub. " +
					"Set a gesvial-operated URL or switch GATEWAY__PUSH_BACKEND=fcm (with FCM credentials) or =disabled.")
			}

			return push.Config{
				Mode: pushBackendToMode(backend),
				ClientOptions: map[string]string{
					"credentials":       credentials,
					"upstream_base_url": cfg.Gateway.UpstreamURL,
				},
				Debounce: time.Duration(cfg.FCM.DebounceSeconds) * time.Second,
				Timeout:  time.Duration(cfg.FCM.TimeoutSeconds) * time.Second,
			}
		}),
		fx.Provide(func(cfg Config) auth.Config {
			// Preserve legacy semantics of auth.Mode (public/private) because auth
			// uses it to decide whether to accept PRIVATE_TOKEN for device registration.
			// Derive from the effective push backend if the legacy Mode is unset.
			legacyMode := cfg.Gateway.Mode
			if legacyMode == "" {
				switch cfg.Gateway.PushBackend {
				case PushBackendUpstream:
					legacyMode = GatewayModePrivate
				default:
					legacyMode = GatewayModePublic
				}
			}
			return auth.Config{
				Mode:         auth.Mode(legacyMode),
				PrivateToken: cfg.Gateway.PrivateToken,
			}
		}),
		fx.Provide(func(cfg Config) handlers.Config {
			// Default and normalize API path/host
			if cfg.HTTP.API.Host == "" {
				cfg.HTTP.API.Path = "/api"
			}
			// Ensure leading slash and trim trailing slash (except root)
			if !strings.HasPrefix(cfg.HTTP.API.Path, "/") {
				cfg.HTTP.API.Path = "/" + cfg.HTTP.API.Path
			}
			if cfg.HTTP.API.Path != "/" && strings.HasSuffix(cfg.HTTP.API.Path, "/") {
				cfg.HTTP.API.Path = strings.TrimRight(cfg.HTTP.API.Path, "/")
			}
			// Guard against misconfigured scheme in host (accept "host[:port]" only)
			cfg.HTTP.API.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.HTTP.API.Host, "https://"), "http://")

			// UpstreamEnabled reflects whether this server acts as a push upstream hub
			// (exposing /upstream/v1 for other gesvial deployments). It was tied to
			// Mode=public historically; keep that semantic: the handler enables only
			// when PushBackend=fcm (equivalent to the old "public" mode).
			backend := cfg.Gateway.PushBackend
			if backend == "" && cfg.Gateway.Mode == GatewayModePublic {
				backend = PushBackendFCM
			}
			return handlers.Config{
				PublicHost:      cfg.HTTP.API.Host,
				PublicPath:      cfg.HTTP.API.Path,
				UpstreamEnabled: backend == PushBackendFCM,
				OpenAPIEnabled:  cfg.HTTP.OpenAPI.Enabled,
			}
		}),
		fx.Provide(func(cfg Config) messages.Config {
			return messages.Config{
				CacheTTL:        time.Duration(cfg.Messages.CacheTTLSeconds) * time.Second,
				HashingInterval: time.Duration(cfg.Messages.HashingIntervalSeconds) * time.Second,
			}
		}),
		fx.Provide(func(_ Config) devices.Config {
			return devices.Config{}
		}),
		fx.Provide(func(cfg Config) sse.Config {
			return sse.NewConfig(
				sse.WithKeepAlivePeriod(time.Duration(cfg.SSE.KeepAlivePeriodSeconds) * time.Second),
			)
		}),
		fx.Provide(func(cfg Config) cachefx.Config {
			return cachefx.Config{
				URL: cfg.Cache.URL,
			}
		}),
		fx.Provide(func(cfg Config) pubsub.Config {
			return pubsub.Config{
				URL:        cfg.PubSub.URL,
				BufferSize: cfg.PubSub.BufferSize,
			}
		}),
		fx.Provide(func(cfg Config) jwt.Config {
			accessTTL := cfg.JWT.AccessTTL
			if cfg.JWT.TTL != 0 {
				accessTTL = cfg.JWT.TTL
			}

			return jwt.Config{
				Secret:     cfg.JWT.Secret,
				AccessTTL:  time.Duration(accessTTL),
				RefreshTTL: time.Duration(cfg.JWT.RefreshTTL),
				Issuer:     cfg.JWT.Issuer,
			}
		}),
		fx.Provide(func(cfg Config) otp.Config {
			return otp.Config{
				TTL:     time.Duration(cfg.OTP.TTL) * time.Second,
				Retries: int(cfg.OTP.Retries),
			}
		}),
		fx.Provide(func(cfg Config) webhooks.Config {
			return webhooks.Config{
				ServerSideEnabled: cfg.Webhooks.ServerSideEnabled,
				Timeout:           time.Duration(cfg.Webhooks.TimeoutSeconds) * time.Second,
				MaxRetries:        cfg.Webhooks.MaxRetries,
			}
		}),
		fx.Provide(func(cfg Config) tests.RetryConfig {
			return tests.RetryConfig{
				Enabled:       cfg.Tests.RetryEnabled,
				Interval:      time.Duration(cfg.Tests.RetryIntervalMinutes) * time.Minute,
				MaxAttempts:   cfg.Tests.RetryMaxAttempts,
				Lookback:      time.Duration(cfg.Tests.RetryLookbackHours) * time.Hour,
				IncludeFailed: cfg.Tests.RetryIncludeFailed,
			}
		}),
		fx.Provide(func(cfg Config) tests.ReportConfig {
			windowMin := cfg.Tests.DedupWindowMinutes
			if windowMin == 0 {
				// Defensive default for legacy YAML without the new key —
				// avoids a 0 → instant cutoff that would disable merges.
				windowMin = 30
			}
			return tests.ReportConfig{
				DedupReconciliationEnabled: cfg.Tests.DedupReconciliationEnabled,
				DedupWindow:                time.Duration(windowMin) * time.Minute,
			}
		}),
	)
}
