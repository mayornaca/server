package config

import "time"

type GatewayMode string

const (
	// Deprecated: kept for backward-compat of GATEWAY__MODE env var.
	// Maps public→fcm, private→upstream at load time. Remove after 2026-10-01.
	GatewayModePublic  GatewayMode = "public"
	GatewayModePrivate GatewayMode = "private"

	// DefaultUpstreamURL is intentionally empty in gesvial.14+.
	// The upstream mode is only valid with a gesvial-operated URL set explicitly
	// via GATEWAY__UPSTREAM_URL. The historical upstream `api.sms-gate.app`
	// is blocked by policy (see security/egress-allowlist.yaml).
	DefaultUpstreamURL = ""
)

// PushBackend selects how the server delivers push events to gateway devices.
// Replaces the ambiguous GatewayMode names since gesvial.14.
type PushBackend string

const (
	PushBackendFCM      PushBackend = "fcm"      // self-hosted FCM (fcm.googleapis.com) — requires FCM credentials
	PushBackendUpstream PushBackend = "upstream" // external hub at GATEWAY__UPSTREAM_URL
	PushBackendDisabled PushBackend = "disabled" // no push — relies on SSE only
)

type Config struct {
	Gateway  Gateway   `yaml:"gateway"`  // gateway config
	HTTP     HTTP      `yaml:"http"`     // http server config
	Database Database  `yaml:"database"` // database config
	FCM      FCMConfig `yaml:"fcm"`      // firebase cloud messaging config
	SSE      SSE       `yaml:"sse"`      // server-sent events config
	Messages Messages  `yaml:"messages"` // messages config
	Cache    Cache     `yaml:"cache"`    // cache (memory or redis) config
	PubSub   PubSub    `yaml:"pubsub"`   // pubsub (memory or redis) config
	JWT      JWT       `yaml:"jwt"`      // jwt config
	OTP      OTP       `yaml:"otp"`      // one-time password config
	Webhooks Webhooks  `yaml:"webhooks"` // server-side webhook dispatch config (gesvial.14)
	Tests    Tests     `yaml:"tests"`    // test execution / retry policy (gesvial.19)
}

type Gateway struct {
	// PushBackend is the preferred way to select the push delivery backend since gesvial.14.
	// Valid values: "fcm", "upstream", "disabled". If empty, falls back to Mode for backward-compat.
	PushBackend PushBackend `yaml:"push_backend"  envconfig:"GATEWAY__PUSH_BACKEND"`

	// Deprecated: use PushBackend. Accepted for backward-compat until 2026-10-01.
	// "public" maps to PushBackendFCM, "private" to PushBackendUpstream.
	Mode         GatewayMode `yaml:"mode"          envconfig:"GATEWAY__MODE"`
	PrivateToken string      `yaml:"private_token" envconfig:"GATEWAY__PRIVATE_TOKEN"` // device registration token
	UpstreamURL  string      `yaml:"upstream_url"  envconfig:"GATEWAY__UPSTREAM_URL"`  // upstream URL when PushBackend=upstream
}

type HTTP struct {
	Listen  string   `yaml:"listen"  envconfig:"HTTP__LISTEN"`  // listen address
	Proxies []string `yaml:"proxies" envconfig:"HTTP__PROXIES"` // proxies

	API     API     `yaml:"api"`
	OpenAPI OpenAPI `yaml:"openapi"`
}

type API struct {
	Host string `yaml:"host" envconfig:"HTTP__API__HOST"` // public API host
	Path string `yaml:"path" envconfig:"HTTP__API__PATH"` // public API path
}

type OpenAPI struct {
	Enabled bool `yaml:"enabled" envconfig:"HTTP__OPENAPI__ENABLED"` // openapi enabled
}

type Database struct {
	Host     string `yaml:"host"     envconfig:"DATABASE__HOST"`     // database host
	Port     int    `yaml:"port"     envconfig:"DATABASE__PORT"`     // database port
	User     string `yaml:"user"     envconfig:"DATABASE__USER"`     // database user
	Password string `yaml:"password" envconfig:"DATABASE__PASSWORD"` // database password
	Database string `yaml:"database" envconfig:"DATABASE__DATABASE"` // database name
	Timezone string `yaml:"timezone" envconfig:"DATABASE__TIMEZONE"` // database timezone
	Debug    bool   `yaml:"debug"    envconfig:"DATABASE__DEBUG"`    // debug mode

	MaxOpenConns int `yaml:"max_open_conns" envconfig:"DATABASE__MAX_OPEN_CONNS"` // max open connections
	MaxIdleConns int `yaml:"max_idle_conns" envconfig:"DATABASE__MAX_IDLE_CONNS"` // max idle connections
}

type FCMConfig struct {
	CredentialsJSON string `yaml:"credentials_json"      envconfig:"FCM__CREDENTIALS_JSON"`      // firebase service account JSON (inline string)
	// CredentialsJSONFile is the filesystem path to the service account JSON.
	// Preferred over CredentialsJSON (inline) — use with docker-compose secrets mounted at /run/secrets/*.
	// If both are set, the file content wins.
	CredentialsJSONFile string `yaml:"credentials_json_file" envconfig:"FCM__CREDENTIALS_JSON_FILE"`
	DebounceSeconds     uint16 `yaml:"debounce_seconds"      envconfig:"FCM__DEBOUNCE_SECONDS"` // push notification debounce (>= 5s)
	TimeoutSeconds      uint16 `yaml:"timeout_seconds"       envconfig:"FCM__TIMEOUT_SECONDS"`  // push notification send timeout
}

type HashingTask struct {
	IntervalSeconds uint16 `yaml:"interval_seconds" envconfig:"TASKS__HASHING__INTERVAL_SECONDS"` // deprecated
}

type SSE struct {
	KeepAlivePeriodSeconds uint16 `yaml:"keep_alive_period_seconds" envconfig:"SSE__KEEP_ALIVE_PERIOD_SECONDS"` // keep alive period in seconds, 0 for no keep alive
}

type Messages struct {
	CacheTTLSeconds        uint16 `yaml:"cache_ttl_seconds"        envconfig:"MESSAGES__CACHE_TTL_SECONDS"`        // cache ttl in seconds
	HashingIntervalSeconds uint16 `yaml:"hashing_interval_seconds" envconfig:"MESSAGES__HASHING_INTERVAL_SECONDS"` // hashing interval in seconds
}

type Cache struct {
	URL string `yaml:"url" envconfig:"CACHE__URL"`
}

type PubSub struct {
	URL        string `yaml:"url"         envconfig:"PUBSUB__URL"`
	BufferSize uint   `yaml:"buffer_size" envconfig:"PUBSUB__BUFFER_SIZE"`
}

type JWT struct {
	Secret     string   `yaml:"secret"      envconfig:"JWT__SECRET"`
	AccessTTL  Duration `yaml:"access_ttl"  envconfig:"JWT__ACCESS_TTL"`
	RefreshTTL Duration `yaml:"refresh_ttl" envconfig:"JWT__REFRESH_TTL"`
	Issuer     string   `yaml:"issuer"      envconfig:"JWT__ISSUER"`

	TTL Duration `yaml:"ttl" envconfig:"JWT__TTL"` // deprecated, remove after 2027-03-01
}

type OTP struct {
	TTL     uint16 `yaml:"ttl"     envconfig:"OTP__TTL"`
	Retries uint8  `yaml:"retries" envconfig:"OTP__RETRIES"`
}

// Webhooks configures server-side webhook dispatch (gesvial.14+).
// When ServerSideEnabled=true, the server POSTs events to user-registered webhook URLs
// in parallel with the Android gateway's existing dispatch (homologation mode).
// Disabled by default: only the gateway dispatches. Flip to true to validate equivalence
// before deprecating the gateway-side handler in a future release.
type Webhooks struct {
	ServerSideEnabled bool   `yaml:"server_side_enabled" envconfig:"WEBHOOKS__SERVER_SIDE_ENABLED"`
	TimeoutSeconds    uint16 `yaml:"timeout_seconds"     envconfig:"WEBHOOKS__TIMEOUT_SECONDS"`
	MaxRetries        uint8  `yaml:"max_retries"         envconfig:"WEBHOOKS__MAX_RETRIES"`
}

// Tests groups operational policy for the test execution / retry pipeline.
// cloud-gesvial.19+. RetryEnabled is ON by default per operator decision —
// failed/error tests are auto-retried up to MaxAttempts, separated by
// IntervalMinutes, ignoring anything older than LookbackHours to avoid
// resurrecting ancient state when the server restarts.
type Tests struct {
	RetryEnabled         bool   `yaml:"retry_enabled"          envconfig:"TESTS__RETRY_FAILED_ENABLED"`
	RetryIntervalMinutes uint16 `yaml:"retry_interval_minutes" envconfig:"TESTS__RETRY_FAILED_INTERVAL_MINUTES"`
	RetryMaxAttempts     uint8  `yaml:"retry_max_attempts"     envconfig:"TESTS__RETRY_FAILED_MAX_ATTEMPTS"`
	RetryLookbackHours   uint16 `yaml:"retry_lookback_hours"   envconfig:"TESTS__RETRY_FAILED_LOOKBACK_HOURS"`
	RetryIncludeFailed   bool   `yaml:"retry_include_failed"   envconfig:"TESTS__RETRY_FAILED_INCLUDE_FAILED"`

	// DedupReconciliationEnabled (cloud-gesvial.20) controls whether
	// Service.Report attempts to merge a new evidence-bearing report into an
	// existing finalised row for (postId, testType) inside the dedup
	// window, instead of inserting a duplicate. Default: true. Set to
	// `false` via TESTS__DEDUP_RECONCILIATION_ENABLED=false to revert to
	// legacy "always insert" behaviour without redeploying — used as a
	// kill-switch if the merge logic ever loses evidence in production.
	DedupReconciliationEnabled bool `yaml:"dedup_reconciliation_enabled" envconfig:"TESTS__DEDUP_RECONCILIATION_ENABLED"`

	// DedupWindowMinutes (cloud-gesvial.20.2) is the look-back window in
	// minutes that Service.Report uses to find a recent finalised twin
	// for `(postId, testType)` before deciding whether to merge or insert.
	// Default: 30. Raised from the original 10 because Chilean carrier
	// delivery receipts arrive 15+ min late during peak hours, defeating
	// the merge path with the smaller window. Operator-tunable via
	// TESTS__DEDUP_WINDOW_MINUTES.
	DedupWindowMinutes uint16 `yaml:"dedup_window_minutes" envconfig:"TESTS__DEDUP_WINDOW_MINUTES"`
}

func Default() Config {
	//nolint:exhaustruct,mnd // default values
	return Config{
		Gateway: Gateway{
			// Default since gesvial.14: fcm self-hosted.
			// UpstreamURL kept as historical default only for users that explicitly
			// switch to PushBackend=upstream (which requires a self-hosted hub, never capcom6's).
			PushBackend: PushBackendFCM,
			Mode:        "", // intentionally empty so PushBackend takes effect; legacy env still honored
			UpstreamURL: DefaultUpstreamURL,
		},
		HTTP: HTTP{
			Listen: ":3000",
		},
		Database: Database{
			Host:     "localhost",
			Port:     3306,
			User:     "sms",
			Password: "sms",
			Database: "sms",
			Timezone: "UTC",
		},
		FCM: FCMConfig{
			CredentialsJSON: "",
			// cloud-gesvial.18.4: pre-fix the default was 0s, which is what
			// `context.WithTimeout(ctx, 0)` interprets as "already cancelled".
			// 30s is enough for `SendEach` (HTTP/2 multiplex, 500 msgs/batch)
			// to talk to fcm.googleapis.com on a slow link. Override via
			// FCM__TIMEOUT_SECONDS if needed.
			TimeoutSeconds:  30,
			DebounceSeconds: 5,
		},
		SSE: SSE{
			KeepAlivePeriodSeconds: 15,
		},
		Messages: Messages{
			CacheTTLSeconds:        300, // 5 minutes
			HashingIntervalSeconds: 60,
		},
		Cache: Cache{
			URL: "memory://",
		},
		PubSub: PubSub{
			URL:        "memory://",
			BufferSize: 128,
		},
		JWT: JWT{
			AccessTTL:  Duration(time.Minute * 15),
			RefreshTTL: Duration(time.Hour * 24 * 30),
			Issuer:     "sms-gate.app",
		},
		OTP: OTP{
			TTL:     300,
			Retries: 3,
		},
		Webhooks: Webhooks{
			ServerSideEnabled: false, // default OFF — gesvial.14 framework, flip to true for homologation
			TimeoutSeconds:    10,    // 10s per request
			MaxRetries:        3,     // exponential backoff: 5s, 15s, 45s
		},
		Tests: Tests{
			RetryEnabled:         true, // default ON per operator request — no retry-less floor
			RetryIntervalMinutes: 15,   // sweep every 15 min
			RetryMaxAttempts:     3,    // give up after 3 retries
			RetryLookbackHours:   2,    // ignore tests older than 2h
			RetryIncludeFailed:   true, // include FAILED + ERROR in retry sweep

			// cloud-gesvial.20 default ON: cloud is authoritative for SMS
			// classification, dedup is primary. Operator can set
			// TESTS__DEDUP_RECONCILIATION_ENABLED=false to revert in case of
			// regression.
			DedupReconciliationEnabled: true,
			// cloud-gesvial.20.2 default 30 min: covers the realistic gap
			// between a post response and a delayed carrier delivery
			// receipt under Chilean peak-hour carriers. Override via
			// TESTS__DEDUP_WINDOW_MINUTES.
			DedupWindowMinutes: 30,
		},
	}
}
