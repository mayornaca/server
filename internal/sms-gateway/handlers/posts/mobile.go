package posts

import (
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/deviceauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/posts"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type MobileController struct {
	base.Handler

	postsSvc *posts.Service
}

func NewMobileController(
	postsSvc *posts.Service,
	logger *zap.Logger,
	validator *validator.Validate,
) *MobileController {
	return &MobileController{
		Handler: base.Handler{
			Logger:    logger,
			Validator: validator,
		},
		postsSvc: postsSvc,
	}
}

func (h *MobileController) get(device models.Device, c *fiber.Ctx) error {
	items, err := h.postsSvc.Select(device.UserID)
	if err != nil {
		return fmt.Errorf("failed to list posts: %w", err)
	}

	return c.JSON(items)
}

func (h *MobileController) post(device models.Device, c *fiber.Ctx) error {
	var items []*posts.SosPost

	if err := c.BodyParser(&items); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	for _, item := range items {
		if err := h.ValidateStruct(item); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	}

	if err := h.postsSvc.Sync(device.UserID, items); err != nil {
		return fmt.Errorf("failed to sync posts: %w", err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"synced": len(items),
	})
}

func (h *MobileController) Register(router fiber.Router) {
	router.Get("", deviceauth.WithDevice(h.get))
	router.Post("", deviceauth.WithDevice(h.post))
}
