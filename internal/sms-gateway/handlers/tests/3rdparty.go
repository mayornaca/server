package tests

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/apierrors"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/permissions"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/tests"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type thirdPartyControllerParams struct {
	fx.In

	TestsSvc *tests.Service

	Validator *validator.Validate
	Logger    *zap.Logger
}

type ThirdPartyController struct {
	base.Handler

	testsSvc *tests.Service
}

func NewThirdPartyController(params thirdPartyControllerParams) *ThirdPartyController {
	return &ThirdPartyController{
		Handler: base.Handler{
			Logger:    params.Logger,
			Validator: params.Validator,
		},
		testsSvc: params.TestsSvc,
	}
}

func (h *ThirdPartyController) list(userID string, c *fiber.Ctx) error {
	var filters []tests.SelectFilter

	postIDs := c.Context().QueryArgs().PeekMulti("postId")
	if len(postIDs) == 1 {
		filters = append(filters, tests.WithPostID(string(postIDs[0])))
	} else if len(postIDs) > 1 {
		ids := make([]string, len(postIDs))
		for i, v := range postIDs {
			ids[i] = string(v)
		}
		filters = append(filters, tests.WithPostIDs(ids))
	}
	if deviceID := c.Query("deviceId"); deviceID != "" {
		filters = append(filters, tests.WithDeviceID(deviceID))
	}
	if testType := c.Query("type"); testType != "" {
		filters = append(filters, tests.WithTestType(tests.TestType(testType)))
	}
	if status := c.Query("status"); status != "" {
		filters = append(filters, tests.WithStatus(tests.TestStatus(status)))
	}
	// cloud-gesvial.19.1 H-MED-1: pre-fix invalid `from`/`to` query params were
	// silently dropped, returning the default window without telling the
	// caller. Now we 400 so the panel surfaces the typo.
	if from := c.Query("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return apierrors.ErrInvalidDateFrom(err)
		}
		filters = append(filters, tests.WithFrom(t))
	}
	if to := c.Query("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return apierrors.ErrInvalidDateTo(err)
		}
		filters = append(filters, tests.WithTo(t))
	}
	if limit := c.Query("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n <= 0 {
			return apierrors.ErrInvalidLimit
		}
		filters = append(filters, tests.WithLimit(n))
	}
	if offset := c.Query("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			return apierrors.ErrInvalidOffset
		}
		filters = append(filters, tests.WithOffset(n))
	}

	effectiveUID := permissions.EffectiveUserID(c, userID)

	count, err := h.testsSvc.Count(effectiveUID, filters...)
	if err != nil {
		return fmt.Errorf("failed to count tests: %w", err)
	}
	c.Set("X-Total-Count", strconv.FormatInt(count, 10))

	items, err := h.testsSvc.Select(effectiveUID, filters...)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	return c.JSON(items)
}

func (h *ThirdPartyController) get(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)

	result, err := h.testsSvc.Get(effectiveUID, id)
	if err != nil {
		if errors.Is(err, tests.ErrNotFound) {
			return apierrors.ErrTestNotFound
		}
		return fmt.Errorf("failed to get test: %w", err)
	}

	return c.JSON(result)
}

type scheduleRequest struct {
	PostID   string `json:"postId"   validate:"required"`
	TestType string `json:"testType" validate:"required,oneof=SMS CONNECTIVITY AUDIO_MIC AUDIO_SPEAKER"`
	DeviceID string `json:"deviceId"` // optional; empty = broadcast to all user devices
	Force    bool   `json:"force"`    // cloud-gesvial.17: cancel any existing PENDING for the same post+testType, then re-schedule
}

type scheduleBatchRequest struct {
	PostIDs  []string `json:"postIds"  validate:"required,min=1,dive,required"`
	TestType string   `json:"testType" validate:"required,oneof=SMS CONNECTIVITY AUDIO_MIC AUDIO_SPEAKER"`
	DeviceID string   `json:"deviceId"`
	Force    bool     `json:"force"` // cloud-gesvial.17: cancel pendings before re-scheduling each post
}

func (h *ThirdPartyController) schedule(userID string, c *fiber.Ctx) error {
	req := new(scheduleRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	effectiveUID := permissions.EffectiveUserID(c, userID)
	scheduleFn := h.testsSvc.ScheduleTest
	if req.Force {
		scheduleFn = h.testsSvc.ScheduleTestForce
	}
	result, err := scheduleFn(effectiveUID, req.PostID, req.TestType, req.DeviceID)
	if err != nil {
		var pendingErr *tests.PendingExistsError
		if errors.As(err, &pendingErr) {
			// cloud-gesvial.19.1 H-MED-2: send a minimal projection of the
			// existing PENDING instead of the full TestResult. Pre-fix the
			// 409 echoed UserID and raw details JSON — fields the caller
			// shouldn't see for a record they don't own. Panel only needs
			// id/postId/testType/createdAt to render the conflict UI.
			existing := fiber.Map{
				"id":       pendingErr.ExistingTestID,
				"status":   "PENDING",
				"postId":   req.PostID,
				"testType": req.TestType,
			}
			if result != nil {
				existing["createdAt"] = result.CreatedAt
			}
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error":          tests.ErrPendingAlreadyExists.Error(),
				"testResultId":   pendingErr.ExistingTestID,
				"status":         "PENDING",
				"existingResult": existing,
			})
		}
		if errors.Is(err, tests.ErrDeviceNotOwned) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return fmt.Errorf("failed to schedule test: %w", err)
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *ThirdPartyController) scheduleBatch(userID string, c *fiber.Ctx) error {
	req := new(scheduleBatchRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	effectiveUID := permissions.EffectiveUserID(c, userID)
	results, err := h.testsSvc.ScheduleBatchTest(effectiveUID, req.PostIDs, req.TestType, req.DeviceID, req.Force)
	if err != nil {
		if errors.Is(err, tests.ErrBatchTooLarge) || errors.Is(err, tests.ErrEmptyBatch) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if errors.Is(err, tests.ErrDeviceNotOwned) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return fmt.Errorf("failed to schedule batch: %w", err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"results": results})
}

// history returns aggregated test outcomes for the given postId in the
// requested time window, grouped by day or hour. Endpoint:
//
//	GET /api/3rdparty/v1/tests/history?postId=X&from=ISO&to=ISO&bucket=day|hour
//
// Used by the panel's historic comparison page in cloud-gesvial.20.
// cloud-gesvial.19+.
func (h *ThirdPartyController) history(userID string, c *fiber.Ctx) error {
	postID := c.Query("postId")
	if postID == "" {
		return apierrors.ErrPostIDRequired
	}
	effectiveUID := permissions.EffectiveUserID(c, userID)

	gran := tests.HistoryGranularity(c.Query("bucket", "day"))
	if gran != tests.HistoryGranularityDay && gran != tests.HistoryGranularityHour {
		return apierrors.ErrInvalidBucket
	}

	from, err := parseTimeOrDefault(c.Query("from"), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return apierrors.ErrInvalidDateFrom(err)
	}
	to, err := parseTimeOrDefault(c.Query("to"), time.Now())
	if err != nil {
		return apierrors.ErrInvalidDateTo(err)
	}

	// cloud-gesvial.19.2 C1: pass the request context so the SQL aggregation
	// gets cancelled if the client disconnects (SSE keepalive drop, browser
	// nav away, panel reconnect after token rotate).
	buckets, err := h.testsSvc.History(c.UserContext(), effectiveUID, postID, from, to, gran)
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	var totals tests.HistoryBucket
	totals.Bucket = "totals"
	for _, b := range buckets {
		totals.Passed += b.Passed
		totals.Failed += b.Failed
		totals.Error += b.Error
		totals.Pending += b.Pending
	}

	return c.JSON(fiber.Map{
		"postId":  postID,
		"from":    from.Format(time.RFC3339),
		"to":      to.Format(time.RFC3339),
		"bucket":  string(gran),
		"buckets": buckets,
		"totals":  totals,
	})
}

// parseTimeOrDefault accepts RFC3339 (e.g. 2026-04-01T00:00:00Z) or bare
// YYYY-MM-DD; returns def when input is empty.
func parseTimeOrDefault(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD, got %q", s)
}

// cancel marks a PENDING test as ERROR("cancelled by operator"). Lets the
// operator break out of the dedupe (10-min TTL) when retrying flows like the
// "Ejecutar prueba" panel button — instead of waiting for the TTL to expire.
// Returns 404 if the test does not belong to the user, 409 if not in PENDING.
func (h *ThirdPartyController) cancel(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)

	result, err := h.testsSvc.Cancel(effectiveUID, id)
	if err != nil {
		if errors.Is(err, tests.ErrNotFound) {
			return apierrors.ErrTestNotFound
		}
		if errors.Is(err, tests.ErrCannotCancel) {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return fmt.Errorf("failed to cancel test: %w", err)
	}
	return c.JSON(result)
}

func (h *ThirdPartyController) Register(router fiber.Router) {
	router.Get("", permissions.RequireScope(ScopeList), userauth.WithUserID(h.list))
	router.Get("/history", permissions.RequireScope(ScopeRead), userauth.WithUserID(h.history))
	router.Get("/:id", permissions.RequireScope(ScopeRead), userauth.WithUserID(h.get))
	router.Post("/schedule", permissions.RequireScope(ScopeSchedule), userauth.WithUserID(h.schedule))
	router.Post("/schedule/batch", permissions.RequireScope(ScopeSchedule), userauth.WithUserID(h.scheduleBatch))
	router.Delete("/:id", permissions.RequireScope(ScopeCancel), userauth.WithUserID(h.cancel))
}