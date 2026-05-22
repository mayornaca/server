package tests

import (
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/deviceauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/tests"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type MobileController struct {
	base.Handler

	testsSvc *tests.Service
}

func NewMobileController(
	testsSvc *tests.Service,
	logger *zap.Logger,
	validator *validator.Validate,
) *MobileController {
	return &MobileController{
		Handler: base.Handler{
			Logger:    logger,
			Validator: validator,
		},
		testsSvc: testsSvc,
	}
}

func (h *MobileController) get(device models.Device, c *fiber.Ctx) error {
	items, err := h.testsSvc.Select(device.UserID, tests.WithDeviceID(device.ID))
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	return c.JSON(items)
}

type reportResponse struct {
	ID       string `json:"id"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// post ingests a batch of test results from a device. Per-test outcomes are
// returned so the device knows which entries were committed vs. silently
// dropped (e.g. orphan postId from a previous user). gesvial.16 changed the
// behaviour from "fail the whole batch on first orphan" to "skip orphans,
// commit the rest" — see tests.Service.Report for the rationale.
func (h *MobileController) post(device models.Device, c *fiber.Ctx) error {
	var items []*tests.TestResult

	if err := c.BodyParser(&items); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// Validate each item
	for _, item := range items {
		if err := h.ValidateStruct(item); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	}

	_, outcomes, err := h.testsSvc.Report(device.UserID, device.ID, items)
	if err != nil {
		return fmt.Errorf("failed to report tests: %w", err)
	}

	responses := make([]reportResponse, len(outcomes))
	for i, o := range outcomes {
		responses[i] = reportResponse{
			ID:       o.ID,
			Accepted: o.Accepted,
			Reason:   o.Reason,
		}
	}

	return c.Status(fiber.StatusCreated).JSON(responses)
}

func (h *MobileController) Register(router fiber.Router) {
	router.Get("", deviceauth.WithDevice(h.get))
	router.Post("", deviceauth.WithDevice(h.post))
}
