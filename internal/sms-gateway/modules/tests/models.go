package tests

import (
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/users"
	"gorm.io/gorm"
)

type TestType string

const (
	TestTypeConnectivity TestType = "CONNECTIVITY"
	TestTypeSMS          TestType = "SMS"
	TestTypeAudioMic     TestType = "AUDIO_MIC"
	TestTypeAudioSpeaker TestType = "AUDIO_SPEAKER"
)

type TestStatus string

const (
	TestStatusPending TestStatus = "PENDING"
	TestStatusPassed  TestStatus = "PASSED"
	TestStatusFailed  TestStatus = "FAILED"
	TestStatusError   TestStatus = "ERROR"
)

type VerificationStatus string

const (
	VerificationPending  VerificationStatus = "PENDING"
	VerificationMatch    VerificationStatus = "MATCH"
	VerificationMismatch VerificationStatus = "MISMATCH"
	VerificationError    VerificationStatus = "ERROR"
)

type TestResult struct {
	models.SoftDeletableModel

	ID       string `json:"id"       gorm:"primaryKey;type:char(21)"`
	UserID   string `json:"-"        gorm:"<-:create;not null;type:varchar(32);index:idx_test_results_user"`
	DeviceID *string `json:"deviceId,omitempty" gorm:"column:device_id;type:char(21);index:idx_test_results_device"`
	PostID   string `json:"postId"   validate:"required" gorm:"column:post_id;not null;type:char(21);index:idx_test_results_post"`

	TestType TestType   `json:"testType" validate:"required" gorm:"column:test_type;not null;type:varchar(20)"`
	// Status era required hasta cloud-gesvial.20.4. Ahora es opcional en el
	// inbound JSON: la app v16.1+ NO envía status (sólo `evidenceMessages` con
	// los SMS recibidos), y el cloud lo deriva server-side. La validación
	// efectiva de "el status es legal" se hace en Service.Report / ApplyEvidence
	// que se asegura de poblarlo antes del Insert.
	Status   TestStatus `json:"status,omitempty" gorm:"not null;type:varchar(10)"`

	CallRecordID *string `json:"callRecordId,omitempty" gorm:"column:call_record_id;type:varchar(64)"`
	MessageID    *string `json:"messageId,omitempty"    gorm:"column:message_id;type:varchar(64)"`

	FFTAnalysisJSON *string `json:"fftAnalysisJson,omitempty" gorm:"column:fft_analysis_json;type:text"`
	Details         *string `json:"details,omitempty"         gorm:"type:text"`
	Error           *string `json:"error,omitempty"           gorm:"type:text"`

	CloudVerificationStatus *VerificationStatus `json:"cloudVerificationStatus,omitempty" gorm:"column:cloud_verification_status;type:varchar(20)"`
	CloudWhisperResultJSON  *string             `json:"cloudWhisperResultJson,omitempty"  gorm:"column:cloud_whisper_result_json;type:text"`

	StartedAt   *time.Time `json:"startedAt,omitempty"   gorm:"column:started_at;type:datetime(3)"`
	CompletedAt *time.Time `json:"completedAt,omitempty" gorm:"column:completed_at;type:datetime(3)"`

	// Autonomous marks tests executed by the android app in offline mode
	// (SSE connection stale > threshold). Useful to separate server-driven
	// tests from app-driven ones in dashboards and metrics.
	Autonomous bool `json:"autonomous" gorm:"not null;default:false;index:idx_test_results_autonomous"`

	// cloud-gesvial.19 retry policy fields. RetryCount counts how many times
	// THIS test was the seed for an automatic retry; SupersededByTestID points
	// to the next test result that replaced it (the retry attempt). Together
	// they form the audit trail "test X was retried 2 times, last attempt was Y".
	RetryCount         uint8   `json:"retryCount"                 gorm:"column:retry_count;not null;default:0"`
	SupersededByTestID *string `json:"supersededByTestId,omitempty" gorm:"column:superseded_by_test_id;type:char(21)"`

	// cloud-gesvial.20 dual-evidence classification. A test SMS may produce up
	// to two distinct asynchronous events: a carrier delivery receipt
	// (transport layer — "the SMS reached the SIM") and a post response
	// (application layer — "the firmware processed the SMS and replied").
	// These four fields separate them so the operator sees both states for
	// one logical test instead of two confusing rows.
	//
	// Pointers are intentional: NULL = unknown (legacy rows or test still
	// in-flight); explicit value = cloud has classified the evidence.
	//   - DeliveryConfirmed: false = no carrier receipt seen in window;
	//     true = carrier acknowledged delivery.
	//   - DeliveryAt: timestamp of the receipt (when DeliveryConfirmed=true).
	//   - DeliveryCarrier: name normalised by Classifier ("Entel", "Movistar"...).
	//   - FailureKind: subclassification when Status=FAILED. One of
	//     POST_NOT_RESPONDING / NETWORK_DELIVERY / TIMEOUT, NULL otherwise.
	// See `docs/manual-operador.md` section 6.3 for the operator matrix.
	DeliveryConfirmed *bool      `json:"deliveryConfirmed,omitempty" gorm:"column:delivery_confirmed"`
	DeliveryAt        *time.Time `json:"deliveryAt,omitempty"        gorm:"column:delivery_at;type:datetime(3)"`
	DeliveryCarrier   *string    `json:"deliveryCarrier,omitempty"   gorm:"column:delivery_carrier;type:varchar(32)"`
	FailureKind       *string    `json:"failureKind,omitempty"       gorm:"column:failure_kind;type:varchar(32)"`

	// EvidenceMessageIDs is an optional reference list to entries in the
	// gateway's `messages` table. Populated by app-gesvial.16+; older app
	// versions leave it nil. Not persisted directly — relayed only on the
	// inbound mobile DTO so Service.Report can resolve raw SMS bodies for
	// authoritative re-classification.
	EvidenceMessageIDs []string `json:"evidenceMessageIds,omitempty" gorm:"-"`

	// EvidenceMessages — contrato `app-gesvial.16.1` (cloud-gesvial.20.4):
	// la app SÓLO envía la lista bruta de SMS recibidos en la ventana del test
	// (sender + body + receivedAt). El cloud clasifica cada uno con
	// tests.Classify y deriva status, delivery_confirmed, delivery_carrier,
	// delivery_at y failure_kind. Cuando este campo viene poblado, el campo
	// inbound `status` se ignora aunque venga (la app no debería mandarlo,
	// pero el cloud es defensivo).
	//
	// No se persiste en BD directamente — el primer SMS clasificado como
	// POST_RESPONSE deja su body en `details` JSON para mantener compatibilidad
	// con el panel actual.
	EvidenceMessages []EvidenceMessage `json:"evidenceMessages,omitempty" gorm:"-"`
	WindowClosedAt   *time.Time        `json:"windowClosedAt,omitempty" gorm:"-"`

	User users.User `json:"-" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

// EvidenceMessage representa un SMS recibido por la app durante la ventana
// del test. La app entrega estos crudos al cloud y el cloud los clasifica.
// cloud-gesvial.20.4.
type EvidenceMessage struct {
	Sender     string    `json:"sender"`
	Body       string    `json:"body"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// FailureKind values stored in TestResult.FailureKind. Encoded as constants
// so Service.Report and the panel agree on the spelling.
const (
	FailureKindPostNotResponding = "POST_NOT_RESPONDING" // delivery confirmed, no app response
	FailureKindNetworkDelivery   = "NETWORK_DELIVERY"    // no delivery, no response — transport failure
	FailureKindTimeout           = "TIMEOUT"             // no evidence, window closed
)

func (TestResult) TableName() string {
	return "test_results"
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(new(TestResult)); err != nil {
		return fmt.Errorf("test_results migration failed: %w", err)
	}
	return nil
}