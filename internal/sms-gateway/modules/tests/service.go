package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/db"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/events"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/paneleventsbus"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/posts"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PendingTTL is the maximum age a PENDING test may live before being
// flagged as ERROR by `expirePending`. Also the window used for dedupe on
// new schedule. Raised from 10 min to 30 min in cloud-gesvial.17 because
// real Chilean carrier SMS replies were observed arriving 15+ min late
// during peak hours, leading to false-positive ERROR timeouts before the
// genuine response made it through.
const PendingTTL = 30 * time.Minute

// MaxBatchSize caps how many posts can be scheduled in a single batch request.
const MaxBatchSize = 50

// DefaultReportDedupWindow is the fallback window used by Service.Report
// when no explicit ReportConfig.DedupWindow is provided (e.g. unit tests
// constructing the service with a zero-value config). Set to 30 min in
// cloud-gesvial.20.2 — the original 10 min proved too short in production
// where Chilean carrier delivery receipts arrive 15+ min late during peak
// hours, defeating the merge path. The operator can override at runtime
// via TESTS__DEDUP_WINDOW_MINUTES.
const DefaultReportDedupWindow = 30 * time.Minute

// ReportConfig holds per-instance toggles for Service.Report. Provided via
// fx from the config module so the operator can flip behaviour without code
// changes (e.g. revert the cloud-led dedup if a regression appears).
// cloud-gesvial.20.
type ReportConfig struct {
	// DedupReconciliationEnabled controls whether Report tries to merge new
	// evidence into an existing finalised row before inserting. When false,
	// Report falls back to legacy behaviour (always insert; no merge). The
	// default is true via config.Default().
	DedupReconciliationEnabled bool

	// DedupWindow is how far back Service.Report looks for a finalised
	// (PASSED/FAILED/ERROR) test on the same (postId, testType) before
	// deciding whether a new incoming report is a duplicate to reconcile
	// or a fresh row to insert. Zero defaults to DefaultReportDedupWindow
	// (30 min). Operator-tunable via TESTS__DEDUP_WINDOW_MINUTES.
	// cloud-gesvial.20.2.
	DedupWindow time.Duration
}

// dedupWindow returns the effective dedup window for this Service instance,
// falling back to DefaultReportDedupWindow when the config didn't specify one.
func (s *Service) dedupWindow() time.Duration {
	if s.reportCfg.DedupWindow > 0 {
		return s.reportCfg.DedupWindow
	}
	return DefaultReportDedupWindow
}

// RetryConfig drives the automatic retry of FAILED/ERROR tests
// (cloud-gesvial.19+). Defaults are tuned for the gesvial operational rhythm:
// retry kicks in after a test ages out, repeats up to MaxAttempts times, and
// stops re-trying anything older than Lookback to avoid resurrecting
// ancient state on a server restart.
type RetryConfig struct {
	Enabled        bool          // master switch
	Interval       time.Duration // how often the cron task runs (e.g. 15m)
	MaxAttempts    uint8         // ceiling on per-test retry_count before giving up
	Lookback       time.Duration // ignore tests older than this
	IncludeFailed  bool          // when false, only ERROR (timeout) is retried; FAILED stays as-is
}

var (
	ErrNotFound             = errors.New("test result not found")
	ErrPostNotOwned         = errors.New("post does not belong to this user")
	ErrDeviceNotOwned       = errors.New("device does not belong to this user")
	ErrPendingAlreadyExists = errors.New("test PENDING already exists for this post and testType")
	ErrBatchTooLarge        = errors.New("batch exceeds maximum allowed size")
	ErrEmptyBatch           = errors.New("batch contains no post IDs")
	ErrCannotCancel         = errors.New("only PENDING tests can be cancelled")
)

// PendingExistsError carries the id of the existing PENDING test that blocked
// a new schedule request.
type PendingExistsError struct {
	ExistingTestID string
}

func (e *PendingExistsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrPendingAlreadyExists.Error(), e.ExistingTestID)
}

func (e *PendingExistsError) Unwrap() error {
	return ErrPendingAlreadyExists
}

type ServiceParams struct {
	fx.In

	IDGen db.IDGen

	Tests       *Repository
	PostsSvc    *posts.Service
	DevicesSvc  *devices.Service
	EventsSvc   *events.Service
	PanelBus    *paneleventsbus.Service

	// ReportCfg is optional: callers without an explicit config (e.g. tests)
	// see the zero value (DedupReconciliationEnabled=false), which preserves
	// legacy behaviour. The production wiring in config/module.go provides a
	// default with reconciliation enabled.
	ReportCfg ReportConfig `optional:"true"`

	Logger *zap.Logger
}

type Service struct {
	idgen db.IDGen

	tests      *Repository
	postsSvc   *posts.Service
	devicesSvc *devices.Service
	eventsSvc  *events.Service
	panelBus   *paneleventsbus.Service
	reportCfg  ReportConfig

	logger *zap.Logger
}

func NewService(params ServiceParams) *Service {
	return &Service{
		idgen:      params.IDGen,
		tests:      params.Tests,
		postsSvc:   params.PostsSvc,
		devicesSvc: params.DevicesSvc,
		eventsSvc:  params.EventsSvc,
		panelBus:   params.PanelBus,
		reportCfg:  params.ReportCfg,
		logger:     params.Logger,
	}
}

func (s *Service) Select(userID string, filters ...SelectFilter) ([]*TestResult, error) {
	filters = append(filters, WithUserID(userID))
	items, err := s.tests.Select(filters...)
	if err != nil {
		return nil, fmt.Errorf("failed to select tests: %w", err)
	}
	return items, nil
}

func (s *Service) Count(userID string, filters ...SelectFilter) (int64, error) {
	filters = append(filters, WithUserID(userID))
	count, err := s.tests.Count(filters...)
	if err != nil {
		return 0, fmt.Errorf("failed to count tests: %w", err)
	}
	return count, nil
}

func (s *Service) Get(userID string, id string) (*TestResult, error) {
	result, err := s.tests.SelectOne(WithUserID(userID), WithID(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get test: %w", err)
	}
	return result, nil
}

// Cancel marks a PENDING test as ERROR with reason "cancelled by operator".
// Used by the operator from the panel when the 409 dedupe blocks a retry —
// instead of waiting for the 10-min TTL to expire, the operator can cancel
// the pending and immediately re-schedule. Returns ErrNotFound if the test
// does not belong to userID, ErrCannotCancel if not in PENDING state.
//
// We mark as ERROR (instead of soft-delete) so the audit trail is preserved
// and the dedupe check (which filters by status=PENDING) lets the next
// schedule through.
func (s *Service) Cancel(userID string, id string) (*TestResult, error) {
	result, err := s.tests.SelectOne(WithUserID(userID), WithID(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get test: %w", err)
	}
	if result.Status != TestStatusPending {
		return nil, ErrCannotCancel
	}

	now := time.Now()
	result.Status = TestStatusError
	result.Error = strPtr("cancelled by operator")
	result.CompletedAt = &now

	if err := s.tests.Update(result); err != nil {
		return nil, fmt.Errorf("failed to cancel test: %w", err)
	}
	return result, nil
}

// ReportOutcome captures the per-result acceptance status returned by Report.
type ReportOutcome struct {
	ID       string
	Accepted bool
	Reason   string // empty when Accepted=true
}

// Report ingests a batch of test results from a device. Tests whose PostID
// does not belong to the device's user are SKIPPED (logged + reported back
// as Accepted=false, Reason="post not owned"), but the rest of the batch is
// inserted. This is intentional: gateways accumulate offline history that
// may include zombie postIds (re-registration, post deletion, ownership
// transfer). Pre-gesvial.16 the endpoint aborted the whole batch on the
// first orphan, leaving the device stuck in a retry loop and losing every
// genuine result behind the bad one.
//
// cloud-gesvial.20 — dual-evidence classification & primary dedup. The cloud
// is now the source of truth for SMS test classification (the app is a
// replica). Each incoming result is classified by Classify() based on the
// responseText extracted from its `details` JSON; the result is enriched
// (delivery_confirmed/carrier/at, failure_kind) and then either:
//
//   1. INSERTED as a new row when no recent finalised twin exists, or
//   2. MERGED into an existing finalised row in the dedup window
//      (ReportDedupWindow), enriching the existing record with whatever new
//      evidence the incoming carries — without producing a duplicate.
//
// The merge path fixes the bug where the app emitted two reports per logical
// test (one per SMS received): pre-gesvial.20 the carrier receipt overwrote
// the post response, flipping the post status from OK to FAIL. Now the
// receipt arrives, gets classified as CARRIER_DELIVERY_RECEIPT, and is
// merged into the existing PASSED row as `delivery_confirmed=true,
// delivery_carrier=Entel` — the post status stays OK and the operator sees
// both layers in the panel.
//
// Returns the number of newly inserted rows (not counting merges) and a
// per-result outcome list aligned with the input order.
func (s *Service) Report(userID string, deviceID string, results []*TestResult) (int, []ReportOutcome, error) {
	outcomes := make([]ReportOutcome, len(results))
	toInsert := make([]*TestResult, 0, len(results))
	mergedTargets := make([]*TestResult, 0)

	for i, r := range results {
		// Pre-fill outcome ID even for skipped items so the caller can correlate.
		if r.ID == "" {
			r.ID = s.idgen()
		}
		outcomes[i].ID = r.ID

		if r.PostID != "" {
			if _, err := s.postsSvc.Get(userID, r.PostID); err != nil {
				s.logger.Warn("skipping report: post not owned by device's user",
					zap.String("postID", r.PostID),
					zap.String("userID", userID),
					zap.String("testID", r.ID),
					zap.Error(err))
				outcomes[i].Accepted = false
				outcomes[i].Reason = fmt.Sprintf("post %s: %s", r.PostID, ErrPostNotOwned.Error())
				continue
			}
		}

		// cloud-gesvial.20.4: si la app v16.1+ envió el contrato nuevo
		// (`windowClosedAt` poblado, con o sin `evidenceMessages`), procesarla
		// server-side ANTES de applyClassification. Esto deriva
		// status / delivery_* / failure_kind desde la lista — incluso cuando
		// la lista es vacía (ventana cerró sin SMS → FAILED + TIMEOUT).
		//
		// cloud-gesvial.20.5: el discriminador de contrato pasó de
		// `len(EvidenceMessages) > 0` a `WindowClosedAt != nil` para cubrir el
		// caso "ventana sin SMS" donde la app manda `evidenceMessages: []`.
		// Pre-fix esos tests llegaban con status="" y el panel los pintaba
		// como badge gris vacío.
		if r.WindowClosedAt != nil || len(r.EvidenceMessages) > 0 {
			s.applyEvidenceMessages(r)
		}

		// cloud-gesvial.20: cloud-led classification. Inspect the responseText
		// (when present), classify it, and enrich the row's evidence fields.
		// The classifier is authoritative: even if the app pre-classified the
		// row, the cloud's verdict wins and any disagreement is logged.
		s.applyClassification(r)

		// cloud-gesvial.20: primary dedup window. If a recent finalised twin
		// for (postId, testType) exists, merge the new evidence into it
		// instead of inserting a duplicate row. Window is operator-tunable
		// via TESTS__DEDUP_WINDOW_MINUTES (cloud-gesvial.20.2).
		if s.reportCfg.DedupReconciliationEnabled && r.PostID != "" && r.TestType != "" {
			cutoff := time.Now().Add(-s.dedupWindow())
			existing, findErr := s.tests.SelectMostRecentFinalised(r.PostID, r.TestType, cutoff)
			if findErr == nil && existing != nil {
				merged, reason := s.mergeEvidenceIntoExisting(existing, r)
				if merged {
					outcomes[i].Accepted = false
					outcomes[i].Reason = reason
					mergedTargets = append(mergedTargets, existing)
					s.logger.Info("merged duplicate report into existing test",
						zap.String("postID", r.PostID),
						zap.String("testType", string(r.TestType)),
						zap.String("existingID", existing.ID),
						zap.String("incomingID", r.ID),
						zap.String("reason", reason))
					continue
				}
			} else if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
				// Don't block the report path on a transient lookup error —
				// fall through to insert and let the legacy "duplicate row"
				// behaviour handle this rare case.
				s.logger.Warn("dedup lookup failed; falling back to insert",
					zap.String("postID", r.PostID),
					zap.Error(findErr))
			}
		}

		r.UserID = userID
		r.DeviceID = &deviceID
		toInsert = append(toInsert, r)
		outcomes[i].Accepted = true
	}

	if len(toInsert) == 0 && len(mergedTargets) == 0 {
		return 0, outcomes, nil
	}

	if len(toInsert) > 0 {
		if err := s.tests.InsertBatch(toInsert); err != nil {
			return 0, outcomes, fmt.Errorf("failed to report tests: %w", err)
		}
	}

	// cloud-gesvial.19.1: collapse per-post status updates so when the same
	// post appears multiple times in one batch (e.g. delayed redelivery from
	// the gateway), only the most-recently-completed test sets the post's
	// status. Pre-fix this was last-write-wins by array order, so an old
	// FAILED arriving alongside a fresh PASSED could leave the post FAILED.
	//
	// cloud-gesvial.20: the latestByPost map now also considers rows whose
	// status was just merged so the post status reflects the authoritative
	// record after merge (e.g. PASSED wins over FAILED when the post
	// response arrives after the carrier-only insert).
	latestByPost := make(map[string]*TestResult, len(toInsert)+len(mergedTargets))
	for _, r := range toInsert {
		if s.panelBus != nil {
			s.panelBus.Publish(paneleventsbus.Event{
				Type:       paneleventsbus.EventTypeTestCompleted,
				UserID:     userID,
				ResourceID: r.ID,
				Status:     string(r.Status),
			})
		}
		prev, seen := latestByPost[r.PostID]
		if !seen || testCompletedAfter(r, prev) {
			latestByPost[r.PostID] = r
		}
	}
	for _, m := range mergedTargets {
		// Republish so panel subscribers refresh and the dual badge picks up
		// the new delivery_confirmed/carrier metadata.
		if s.panelBus != nil {
			s.panelBus.Publish(paneleventsbus.Event{
				Type:       paneleventsbus.EventTypeTestCompleted,
				UserID:     userID,
				ResourceID: m.ID,
				Status:     string(m.Status),
			})
		}
		prev, seen := latestByPost[m.PostID]
		if !seen || testCompletedAfter(m, prev) {
			latestByPost[m.PostID] = m
		}
	}
	for _, r := range latestByPost {
		s.updatePostStatus(userID, r)
	}

	return len(toInsert), outcomes, nil
}

// applyClassification runs the cloud-side Classifier on the responseText
// extracted from r.Details and enriches r with the resulting evidence
// (DeliveryConfirmed / DeliveryCarrier / DeliveryAt / FailureKind / Status).
// cloud-gesvial.20.
//
// Cloud is authoritative: when the app pre-populated the same fields, the
// cloud reclassifies the body and overwrites the app's version. Disagreement
// is logged but not error'd (the panel still displays the cloud's verdict).
//
// Behaviour by classification:
//   - POST_RESPONSE: leave Status alone (trust whatever app reported as
//     PASSED/FAILED from its own logic). Infer DeliveryConfirmed=true since
//     a post couldn't have replied without delivery happening at the
//     transport layer.
//   - CARRIER_DELIVERY_RECEIPT: never demote a real PASSED. The receipt is
//     transport-layer metadata. If the row had Status=PASSED it stays
//     PASSED (and we set DeliveryConfirmed=true with the carrier name). If
//     Status was FAILED, the carrier confirms transport worked, so the
//     failure mode becomes POST_NOT_RESPONDING. If the app shipped this
//     row as PASSED but the body is purely carrier (no post tokens), the
//     row is demoted to FAILED with FailureKind=POST_NOT_RESPONDING — the
//     legacy gesvial.19 carrier-filter behaviour, kept as a safety net for
//     app versions older than gesvial.16.
//   - UNKNOWN: leave the row alone.
// applyEvidenceMessages procesa el contrato `app-gesvial.16.1` (cloud-gesvial.20.4):
// la app entregó la lista bruta de SMS recibidos. El cloud clasifica cada uno
// con tests.Classify y deriva status, delivery_*, failure_kind. Después de
// ejecutar, deja el `details` JSON con `responseText` poblado (si hubo un
// POST_RESPONSE) para mantener compatibilidad con applyClassification y con
// el panel actual que sabe leer ese campo.
//
// Reglas (resumen):
//   - hay POST_RESPONSE → status=PASSED, delivery_confirmed=true (inferido).
//   - sólo CARRIER_DELIVERY_RECEIPT → status=FAILED, failure_kind=POST_NOT_RESPONDING,
//     delivery_confirmed=true.
//   - lista vacía o sólo UNKNOWN → status=FAILED, failure_kind=TIMEOUT,
//     delivery_confirmed=false.
func (s *Service) applyEvidenceMessages(r *TestResult) {
	if r == nil || len(r.EvidenceMessages) == 0 {
		// Caso "ventana cerró sin SMS": status=FAILED + TIMEOUT.
		if r != nil && r.Status == "" {
			r.Status = TestStatusFailed
			fk := FailureKindTimeout
			r.FailureKind = &fk
			no := false
			r.DeliveryConfirmed = &no
			r.Error = strPtr("ventana cerró sin SMS recibidos del poste o del carrier")
		}
		return
	}

	var (
		firstPostResponse    *EvidenceMessage
		firstCarrierReceipt  *EvidenceMessage
		carrierName          string
	)

	for i := range r.EvidenceMessages {
		ev := &r.EvidenceMessages[i]
		c := Classify(ev.Sender, ev.Body)
		switch c.Type {
		case IncomingTypePostResponse:
			if firstPostResponse == nil {
				firstPostResponse = ev
			}
		case IncomingTypeCarrierReceipt:
			if firstCarrierReceipt == nil {
				firstCarrierReceipt = ev
				carrierName = c.Carrier
			}
		}
	}

	now := time.Now()
	completedAt := now
	if r.WindowClosedAt != nil {
		completedAt = *r.WindowClosedAt
	} else if r.CompletedAt != nil {
		completedAt = *r.CompletedAt
	}

	switch {
	case firstPostResponse != nil:
		// Hubo respuesta real del poste → PASSED.
		r.Status = TestStatusPassed
		r.Error = nil
		r.FailureKind = nil
		// Inferir delivery_confirmed desde el carrier receipt si vino, o
		// inferir true a partir de la respuesta del poste (no podemos
		// tener respuesta sin que el SMS haya llegado).
		yes := true
		r.DeliveryConfirmed = &yes
		if firstCarrierReceipt != nil {
			at := firstCarrierReceipt.ReceivedAt
			r.DeliveryAt = &at
			if carrierName != "" {
				c := carrierName
				r.DeliveryCarrier = &c
			}
		} else {
			at := firstPostResponse.ReceivedAt
			r.DeliveryAt = &at
		}
		// Llenar details JSON para compat con panel y con applyClassification.
		details := buildDetailsJSON(firstPostResponse.Sender, firstPostResponse.Body)
		r.Details = &details

	case firstCarrierReceipt != nil:
		// Solo llegó delivery receipt — el poste no respondió.
		r.Status = TestStatusFailed
		fk := FailureKindPostNotResponding
		r.FailureKind = &fk
		yes := true
		r.DeliveryConfirmed = &yes
		at := firstCarrierReceipt.ReceivedAt
		r.DeliveryAt = &at
		if carrierName != "" {
			c := carrierName
			r.DeliveryCarrier = &c
		}
		r.Error = strPtr("carrier confirmó entrega pero el poste no respondió")

	default:
		// Hubo SMS pero ninguno clasificable (todos UNKNOWN). Tratar como TIMEOUT.
		r.Status = TestStatusFailed
		fk := FailureKindTimeout
		r.FailureKind = &fk
		no := false
		r.DeliveryConfirmed = &no
		r.Error = strPtr("ventana cerró con SMS no clasificables")
	}

	// CompletedAt suele venir vacío en el contrato nuevo; rellenar con
	// windowClosedAt (o now) para que el panel muestre algo coherente.
	if r.CompletedAt == nil {
		r.CompletedAt = &completedAt
	}

	// Limpiar EvidenceMessages para que no inflen el JSON serializado al
	// panel; ya quedaron capturadas en `details.responseText` (post response)
	// y en delivery_* (carrier receipt). Los SMS originales viven en la tabla
	// `messages` del gateway si hay que auditarlos.
	r.EvidenceMessages = nil
}

// buildDetailsJSON arma el JSON que vivía en `details` cuando la app vieja
// reportaba: incluye phoneNumber/responseSender/responseText. cloud-gesvial.20.4.
func buildDetailsJSON(sender, body string) string {
	type details struct {
		PhoneNumber    string `json:"phoneNumber,omitempty"`
		ResponseSender string `json:"responseSender,omitempty"`
		ResponseText   string `json:"responseText,omitempty"`
	}
	d := details{
		PhoneNumber:    sender,
		ResponseSender: sender,
		ResponseText:   body,
	}
	out, err := json.Marshal(d)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func (s *Service) applyClassification(r *TestResult) {
	if r == nil || r.Details == nil || *r.Details == "" {
		return
	}
	sender, body := extractResponseFromDetails(*r.Details)
	if sender == "" && body == "" {
		return
	}
	c := Classify(sender, body)

	now := time.Now()

	switch c.Type {
	case IncomingTypePostResponse:
		// A real post response implies the SMS reached the SIM. We can't
		// tell which carrier delivered it from the body alone, so leave
		// DeliveryCarrier empty unless the app had filled it. DeliveryAt
		// defaults to CompletedAt or now.
		if r.DeliveryConfirmed == nil {
			yes := true
			r.DeliveryConfirmed = &yes
		}
		if r.DeliveryAt == nil {
			ts := now
			if r.CompletedAt != nil {
				ts = *r.CompletedAt
			}
			r.DeliveryAt = &ts
		}

	case IncomingTypeCarrierReceipt:
		yes := true
		r.DeliveryConfirmed = &yes
		if c.Carrier != "" && r.DeliveryCarrier == nil {
			carrier := c.Carrier
			r.DeliveryCarrier = &carrier
		}
		if r.DeliveryAt == nil {
			ts := now
			if r.CompletedAt != nil {
				ts = *r.CompletedAt
			}
			r.DeliveryAt = &ts
		}
		// The body carries no application evidence — only transport. If the
		// app erroneously marked this row PASSED (legacy bug fixed in
		// app-gesvial.16), demote it. Real post-response rows don't reach
		// this branch because POST_RESPONSE is checked first by Classify.
		if r.Status == TestStatusPassed {
			s.logger.Warn("carrier receipt arrived as PASSED — demoting (likely legacy app bug)",
				zap.String("postID", r.PostID),
				zap.String("testID", r.ID),
				zap.String("sender", sender),
				zap.String("carrier", c.Carrier))
			r.Status = TestStatusFailed
			r.Error = strPtr("carrier delivery receipt only — post did not respond")
			fk := FailureKindPostNotResponding
			r.FailureKind = &fk
		} else if r.Status == TestStatusFailed && r.FailureKind == nil {
			fk := FailureKindPostNotResponding
			r.FailureKind = &fk
		}

	case IncomingTypeUnknown:
		// Unrecognised body — leave the row alone. The app's classification
		// (or absence thereof) is taken at face value. When the row is
		// FAILED with no FailureKind, mark it TIMEOUT so the panel shows
		// "no evidence" instead of an empty cell.
		if r.Status == TestStatusFailed && r.FailureKind == nil {
			fk := FailureKindTimeout
			r.FailureKind = &fk
		}
	}
}

// mergeEvidenceIntoExisting copies any evidence fields from `incoming` that
// are absent (or strictly weaker) on `existing` and persists the patch via
// UpdateEvidence. Returns merged=true when at least one field was updated
// (or when the incoming was a no-op duplicate that should be silently
// dropped) and a human reason string for the per-result outcome.
//
// Merge rules (cloud-gesvial.20):
//
//   - DeliveryConfirmed: nil → set; false → upgrade to true if incoming
//     says true.
//   - DeliveryCarrier: nil → set with incoming's value when present.
//   - DeliveryAt: nil → set; otherwise leave (the first receipt wins).
//   - FailureKind: nil → set with incoming's value when status≠PASSED.
//   - Status: only ever upgrades (FAILED → PASSED) when the new evidence
//     is a real post response; never demotes here. Demotion paths live in
//     applyClassification on the incoming row.
//   - Error: cleared when status flips from FAILED to PASSED.
func (s *Service) mergeEvidenceIntoExisting(existing, incoming *TestResult) (bool, string) {
	if existing == nil || incoming == nil {
		return false, ""
	}
	patch := map[string]any{}
	reasons := make([]string, 0, 4)

	// DeliveryConfirmed: nil/false → true.
	if boolPtrTrue(incoming.DeliveryConfirmed) && !boolPtrTrue(existing.DeliveryConfirmed) {
		yes := true
		patch["delivery_confirmed"] = yes
		existing.DeliveryConfirmed = &yes
		reasons = append(reasons, "delivery_confirmed=true")
	}
	// DeliveryCarrier: fill when missing.
	if existing.DeliveryCarrier == nil && incoming.DeliveryCarrier != nil && *incoming.DeliveryCarrier != "" {
		v := *incoming.DeliveryCarrier
		patch["delivery_carrier"] = v
		existing.DeliveryCarrier = &v
		reasons = append(reasons, "delivery_carrier="+v)
	}
	// DeliveryAt: fill when missing.
	if existing.DeliveryAt == nil && incoming.DeliveryAt != nil {
		v := *incoming.DeliveryAt
		patch["delivery_at"] = v
		existing.DeliveryAt = &v
		reasons = append(reasons, "delivery_at set")
	}

	// Status upgrade: only if the new evidence is a genuine post response
	// (Classify already inferred DeliveryConfirmed via the POST_RESPONSE
	// branch + status PASSED).
	if existing.Status == TestStatusFailed && incoming.Status == TestStatusPassed {
		patch["status"] = string(TestStatusPassed)
		patch["error"] = nil
		patch["failure_kind"] = nil
		existing.Status = TestStatusPassed
		existing.Error = nil
		existing.FailureKind = nil
		reasons = append(reasons, "status FAILED→PASSED")
	} else if existing.FailureKind == nil && incoming.FailureKind != nil {
		v := *incoming.FailureKind
		patch["failure_kind"] = v
		existing.FailureKind = &v
		reasons = append(reasons, "failure_kind="+v)
	}

	if len(patch) == 0 {
		// Pure duplicate — nothing to enrich. Still report as merged so the
		// caller doesn't insert a fresh row.
		return true, "duplicate within dedup window — no new evidence"
	}

	if err := s.tests.UpdateEvidence(existing.ID, patch); err != nil {
		s.logger.Warn("failed to merge evidence into existing test",
			zap.String("existingID", existing.ID),
			zap.Error(err))
		// Surface as not-merged so the caller falls back to insert and we
		// don't lose the evidence.
		return false, ""
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "merged")
	}
	return true, "merged into " + existing.ID + ": " + strings.Join(reasons, "; ")
}

func boolPtrTrue(p *bool) bool {
	return p != nil && *p
}

// testCompletedAfter reports whether `a` finished strictly after `b`. Falls back
// to CreatedAt when CompletedAt is missing on either side.
func testCompletedAfter(a, b *TestResult) bool {
	at := a.CreatedAt
	if a.CompletedAt != nil {
		at = *a.CompletedAt
	}
	bt := b.CreatedAt
	if b.CompletedAt != nil {
		bt = *b.CompletedAt
	}
	return at.After(bt)
}

// ScheduleSMSTest schedules a single test for the given post/testType.
// Kept named "SMS" for backwards compatibility with the existing handler,
// but now accepts any TestType.
func (s *Service) ScheduleSMSTest(userID string, postID string, deviceID string) (*TestResult, error) {
	return s.ScheduleTest(userID, postID, string(TestTypeSMS), deviceID)
}

// ScheduleTestForce is ScheduleTest + cancellation of any existing PENDING
// for the same (post, testType) before scheduling. Used by:
//   - the panel's "Cancelar pendiente y reintentar" button (operator override)
//   - the cron dispatcher when the schedule has CancelPending=true
//
// Returns the new TestResult (always, never PendingExistsError because the
// dedupe is bypassed by design here). The cancelled test stays in the audit
// trail with status=ERROR, error="superseded by retry".
func (s *Service) ScheduleTestForce(userID string, postID string, testType string, deviceID string) (*TestResult, error) {
	// Find any PENDING within the dedupe window and flip it to ERROR with a
	// dedicated reason so the audit trail explains why it didn't complete.
	// We do not return on the lookup error — best-effort: if the cancel fails,
	// ScheduleTest below will still succeed unless the dedupe blocks it.
	post, err := s.postsSvc.Get(userID, postID)
	if err == nil {
		tt := TestType(testType)
		existing, findErr := s.tests.SelectOne(
			WithUserID(post.UserID),
			WithPostID(postID),
			WithTestType(tt),
			WithStatus(TestStatusPending),
			WithCreatedAfter(time.Now().Add(-PendingTTL)),
		)
		if findErr == nil && existing != nil {
			now := time.Now()
			existing.Status = TestStatusError
			existing.Error = strPtr("superseded by retry")
			existing.CompletedAt = &now
			if updErr := s.tests.Update(existing); updErr != nil {
				s.logger.Warn("force schedule: failed to cancel previous PENDING; falling back to dedupe error",
					zap.String("postID", postID),
					zap.String("existingTestID", existing.ID),
					zap.Error(updErr))
			}
		}
	}

	return s.ScheduleTest(userID, postID, testType, deviceID)
}

// ScheduleTest creates a PENDING TestResult for a post and emits a TestRequested
// SSE event to the gateway. If a PENDING test already exists for the same
// post+testType within PendingTTL, returns *PendingExistsError without creating
// a new row.
func (s *Service) ScheduleTest(userID string, postID string, testType string, deviceID string) (*TestResult, error) {
	post, err := s.postsSvc.Get(userID, postID)
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	// Validate device ownership against the POST's owner — not the caller.
	// When an admin selects a gateway from the panel to dispatch a test, that
	// gateway is registered under the post's owner, not the admin. Keying the
	// ownership check by `userID` (caller) rejected admin-initiated runs.
	var deviceIDPtr *string
	if deviceID != "" {
		exists, devErr := s.devicesSvc.Exists(post.UserID, devices.WithID(deviceID))
		if devErr != nil {
			return nil, fmt.Errorf("failed to check device: %w", devErr)
		}
		if !exists {
			return nil, ErrDeviceNotOwned
		}
		deviceIDPtr = &deviceID
	}

	// Dedupe: if a PENDING exists within the TTL window, surface it instead of
	// creating a duplicate. Older PENDINGs will be cleared by ExpirePendingTask.
	// Use post.UserID (real owner) — when an admin schedules over another
	// user's post, dedupe must hit pre-existing PENDINGs of that owner. If we
	// keyed by `userID` (caller), admin re-runs would slip past dedupe.
	tt := TestType(testType)
	existing, findErr := s.tests.SelectOne(
		WithUserID(post.UserID),
		WithPostID(postID),
		WithTestType(tt),
		WithStatus(TestStatusPending),
		WithCreatedAfter(time.Now().Add(-PendingTTL)),
	)
	if findErr == nil && existing != nil {
		return existing, &PendingExistsError{ExistingTestID: existing.ID}
	}
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing pending: %w", findErr)
	}

	now := time.Now()
	result := &TestResult{
		ID:        s.idgen(),
		UserID:    post.UserID,
		PostID:    post.ID,
		TestType:  tt,
		Status:    TestStatusPending,
		Details:   strPtr(fmt.Sprintf(`{"scheduled":true,"phoneNumber":"%s","message":"*#36#"}`, post.PhoneNumber)),
		StartedAt: &now,
	}

	s.logger.Info("scheduled test",
		zap.String("postID", post.ID),
		zap.String("phoneNumber", post.PhoneNumber),
		zap.String("testType", testType),
		zap.String("status", string(TestStatusPending)))

	if err := s.tests.Insert(result); err != nil {
		return nil, fmt.Errorf("failed to create scheduled test: %w", err)
	}

	// Publish to the panel event bus so any open admin tab refreshes its
	// tests list / dashboard without polling. Cheap, non-blocking, dropped
	// silently for slow consumers. cloud-gesvial.19+.
	if s.panelBus != nil {
		s.panelBus.Publish(paneleventsbus.Event{
			Type:       paneleventsbus.EventTypeTestScheduled,
			UserID:     post.UserID,
			ResourceID: result.ID,
			Status:     string(result.Status),
		})
	}

	// cloud-gesvial.18.3: bump the post's lastTestDate so the panel column
	// "Último test" reflects "last attempt" (any state), not only "last
	// completed". cloud-gesvial.19.1: switched from Get+Update (full-struct
	// Save) to UpdateLastTestDate (single-column UPDATE) to eliminate the
	// lost-update race against the operator's PATCH /posts/:id (Disabled,
	// Status, etc.) that ran concurrently. Best-effort: if it fails, the
	// timestamp stays stale but the test is already created.
	go func(ownerID, postID string) {
		if updErr := s.postsSvc.UpdateLastTestDate(ownerID, postID, time.Now()); updErr != nil {
			s.logger.Warn("failed to bump lastTestDate on schedule", zap.String("postID", postID), zap.Error(updErr))
		}
	}(post.UserID, post.ID)

	// Notify gateway via events service (async, best-effort).
	// Use post.UserID (the real owner of the post) instead of userID (the
	// caller). When an admin with admin:all schedules over another user's
	// post, the SSE/FCM event must reach the OWNER's gateways — not the
	// admin's (which often have stale/abandoned devices).
	go func(ownerID string, deviceIDPtr *string, result *TestResult) {
		event := events.NewTestRequestedEvent(result.ID, result.PostID, string(result.TestType))
		if ntfErr := s.eventsSvc.Notify(ownerID, deviceIDPtr, event); ntfErr != nil {
			s.logger.Warn("failed to notify gateway of scheduled test",
				zap.String("testResultID", result.ID),
				zap.String("postID", result.PostID),
				zap.String("userID", ownerID),
				zap.Error(ntfErr))
		}
	}(post.UserID, deviceIDPtr, result)

	return result, nil
}

// BatchResultItem represents the outcome of scheduling one post in a batch.
type BatchResultItem struct {
	PostID       string `json:"postId"`
	TestResultID string `json:"testResultId"`
	Status       string `json:"status"`          // "PENDING" (new) | "PENDING_EXISTING" (dedupe) | "ERROR"
	Error        string `json:"error,omitempty"` // set when Status=ERROR
}

// ScheduleBatchTest schedules tests for a list of posts, returning per-post
// outcomes. Dedupe errors are surfaced as status=PENDING_EXISTING rather than
// aborting the whole batch. When force=true, any existing PENDING for the
// same (post, testType) is cancelled (ERROR "superseded by retry") before
// the new schedule is attempted — used by the cron dispatcher when the
// schedule is configured with CancelPending=true and by the panel's batch
// "force retry" button.
func (s *Service) ScheduleBatchTest(userID string, postIDs []string, testType string, deviceID string, force bool) ([]BatchResultItem, error) {
	if len(postIDs) == 0 {
		return nil, ErrEmptyBatch
	}
	if len(postIDs) > MaxBatchSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrBatchTooLarge, len(postIDs), MaxBatchSize)
	}

	scheduleOne := s.ScheduleTest
	if force {
		scheduleOne = s.ScheduleTestForce
	}

	results := make([]BatchResultItem, 0, len(postIDs))
	for _, postID := range postIDs {
		r, err := scheduleOne(userID, postID, testType, deviceID)
		switch {
		case err == nil:
			results = append(results, BatchResultItem{
				PostID:       postID,
				TestResultID: r.ID,
				Status:       "PENDING",
			})
		case errors.Is(err, ErrPendingAlreadyExists):
			results = append(results, BatchResultItem{
				PostID:       postID,
				TestResultID: r.ID, // existing PENDING id
				Status:       "PENDING_EXISTING",
			})
		default:
			results = append(results, BatchResultItem{
				PostID: postID,
				Status: "ERROR",
				Error:  err.Error(),
			})
		}
	}
	return results, nil
}

// ExpirePending marks as ERROR every PENDING test older than olderThan.
// Intended to be called periodically by the worker to reap orphaned tests
// (e.g. the gateway never picked up the SSE event).
func (s *Service) ExpirePending(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := s.tests.ExpirePending(ctx, cutoff, "timeout sin respuesta del gateway")
	if err != nil {
		return 0, fmt.Errorf("failed to expire pending tests: %w", err)
	}
	if rows > 0 {
		s.logger.Info("expired pending tests", zap.Int64("rows", rows), zap.Time("cutoff", cutoff))
	}
	return rows, nil
}

// RetryFailed sweeps recent FAILED/ERROR tests that haven't exhausted their
// retry budget and re-schedules them with `force=true`. The new test result
// gets a fresh ID and `retry_count = previous + 1`; the original is marked
// with `superseded_by_test_id = newID` for audit. cloud-gesvial.19+.
//
// Returns the number of tests successfully retried.
func (s *Service) RetryFailed(ctx context.Context, cfg RetryConfig) (int, error) {
	if !cfg.Enabled {
		return 0, nil
	}

	statuses := []TestStatus{TestStatusError}
	if cfg.IncludeFailed {
		statuses = append(statuses, TestStatusFailed)
	}

	cutoff := time.Now().Add(-cfg.Lookback)
	candidates, err := s.tests.SelectRetryCandidates(ctx, statuses, cfg.MaxAttempts, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to select retry candidates: %w", err)
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	retried := 0
	for _, original := range candidates {
		deviceID := ""
		if original.DeviceID != nil {
			deviceID = *original.DeviceID
		}

		// ScheduleTestForce cancels any in-flight PENDING for the same
		// (post, testType) — important because the cron may have created a
		// fresh PENDING already that we don't want to leave stale.
		newResult, err := s.ScheduleTestForce(original.UserID, original.PostID, string(original.TestType), deviceID)
		if err != nil {
			s.logger.Warn("retry failed: could not schedule new test",
				zap.String("originalTestID", original.ID),
				zap.String("postID", original.PostID),
				zap.Error(err))
			continue
		}

		// Mirror the parent's retry_count + 1 onto the new test, and link
		// the original to its successor.
		newResult.RetryCount = original.RetryCount + 1
		original.SupersededByTestID = strPtr(newResult.ID)
		if err := s.tests.UpdateRetryLink(ctx, original, newResult); err != nil {
			s.logger.Warn("retry succeeded but link update failed",
				zap.String("originalTestID", original.ID),
				zap.String("newTestID", newResult.ID),
				zap.Error(err))
		}

		retried++
		s.logger.Info("retrying failed test",
			zap.String("originalID", original.ID),
			zap.String("newID", newResult.ID),
			zap.String("postID", original.PostID),
			zap.String("originalStatus", string(original.Status)),
			zap.Uint8("attempt", newResult.RetryCount))
	}

	return retried, nil
}

// History returns aggregated test outcomes for one post within [from, to],
// grouped by day or hour. Used by the /posts/:id/history endpoint to feed
// the historic comparison UI in cloud-gesvial.20. Validates ownership
// (or admin) before querying. cloud-gesvial.19+. cloud-gesvial.19.2 C1:
// accept ctx so HTTP timeouts and client disconnects propagate to the
// SQL query — pre-fix `context.Background()` ignored both, leaving the
// SSE/HTTP request bound to whatever the GROUP BY took to finish.
func (s *Service) History(ctx context.Context, userID, postID string, from, to time.Time, gran HistoryGranularity) ([]HistoryBucket, error) {
	// Ownership check via posts service — `__ADMIN__` short-circuits inside
	// posts.Service so this works for any user / admin combination.
	if _, err := s.postsSvc.Get(userID, postID); err != nil {
		return nil, err
	}
	return s.tests.SelectHistory(ctx, userID, postID, from, to, gran)
}

func (s *Service) updatePostStatus(userID string, result *TestResult) {
	var status posts.PostStatus
	switch result.Status {
	case TestStatusPassed:
		status = posts.PostStatusOK
	case TestStatusFailed, TestStatusError:
		status = posts.PostStatusFail
	default:
		return
	}

	post, err := s.postsSvc.Get(userID, result.PostID)
	if err != nil {
		s.logger.Warn("failed to get post for status update", zap.Error(err))
		return
	}

	previous := post.Status

	// cloud-gesvial.19.1: completedAt prefers result.CompletedAt over time.Now()
	// so a late batch report doesn't claim the test finished "now". Falls back
	// to now() only when the gateway didn't include the timestamp.
	completedAt := time.Now()
	if result.CompletedAt != nil {
		completedAt = *result.CompletedAt
	}
	if err := s.postsSvc.UpdateStatus(userID, post.ID, status, completedAt); err != nil {
		s.logger.Warn("failed to update post status", zap.Error(err))
		return
	}

	// Only emit the panel event when the status actually transitioned, to
	// avoid spamming subscribers on long stretches of consistent results.
	if s.panelBus != nil && previous != status {
		s.panelBus.Publish(paneleventsbus.Event{
			Type:       paneleventsbus.EventTypePostStatusChanged,
			UserID:     post.UserID,
			ResourceID: post.ID,
			Status:     string(status),
		})
	}
}

func strPtr(s string) *string {
	return &s
}