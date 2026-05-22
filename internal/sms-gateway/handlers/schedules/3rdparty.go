package schedules

import (
	"errors"
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/apierrors"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/permissions"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/schedules"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type thirdPartyControllerParams struct {
	fx.In

	SchedulesSvc *schedules.Service

	Validator *validator.Validate
	Logger    *zap.Logger
}

type ThirdPartyController struct {
	base.Handler

	schedulesSvc *schedules.Service
}

func NewThirdPartyController(params thirdPartyControllerParams) *ThirdPartyController {
	return &ThirdPartyController{
		Handler: base.Handler{
			Logger:    params.Logger,
			Validator: params.Validator,
		},
		schedulesSvc: params.SchedulesSvc,
	}
}

type scheduleRequest struct {
	Name             string   `json:"name"             validate:"required,max=64"`
	CronExpression   string   `json:"cronExpression"   validate:"required,max=120"`
	TestType         string   `json:"testType"         validate:"required,oneof=SMS CONNECTIVITY AUDIO_MIC AUDIO_SPEAKER"`
	Enabled          bool     `json:"enabled"`
	OnlyEnabledPosts bool     `json:"onlyEnabledPosts"`
	FilterPostIDs    []string `json:"filterPostIds,omitempty"`
	DeviceID         *string  `json:"deviceId,omitempty"`
}

func (r *scheduleRequest) toModel() *schedules.TestSchedule {
	return &schedules.TestSchedule{
		Name:             r.Name,
		CronExpression:   r.CronExpression,
		TestType:         r.TestType,
		Enabled:          r.Enabled,
		OnlyEnabledPosts: r.OnlyEnabledPosts,
		FilterPostIDs:    r.FilterPostIDs,
		DeviceID:         r.DeviceID,
	}
}

// All handlers route through permissions.EffectiveUserID so an admin:all
// caller sees / mutates schedules of every user (sentinel __ADMIN__), while
// regular users stay scoped to their own. cloud-gesvial.19.1.
func (h *ThirdPartyController) list(userID string, c *fiber.Ctx) error {
	items, err := h.schedulesSvc.Select(permissions.EffectiveUserID(c, userID))
	if err != nil {
		return fmt.Errorf("failed to list schedules: %w", err)
	}
	return c.JSON(items)
}

func (h *ThirdPartyController) get(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	item, err := h.schedulesSvc.Get(permissions.EffectiveUserID(c, userID), id)
	if err != nil {
		if errors.Is(err, schedules.ErrNotFound) {
			return apierrors.ErrScheduleNotFound
		}
		return fmt.Errorf("failed to get schedule: %w", err)
	}
	return c.JSON(item)
}

func (h *ThirdPartyController) post(userID string, c *fiber.Ctx) error {
	req := new(scheduleRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	model := req.toModel()
	// cloud-gesvial.22.1.1: usar `userID` REAL (sub del JWT), NO EffectiveUserID.
	// EffectiveUserID(admin) → sentinel "__ADMIN__" para LECTURAS (admin ve todos
	// los users). Pero al CREAR un schedule, persistir el sentinel rompe el cron
	// dispatcher: `resolvePostIDs(sch.UserID="__ADMIN__")` filtra `posts WHERE
	// user_id="__ADMIN__"` y devuelve [], el cron loguea "schedule has no
	// matching posts" y no crea tests. El operador veía `lastRunAt` actualizado
	// (MarkRun corre en defer) pero 0 tests creados — ningún error visible.
	// El owner real del schedule debe ser el creador (usuario humano), no el
	// sentinel admin. El admin sigue pudiendo VER/EDITAR/BORRAR schedules ajenos
	// vía EffectiveUserID en list/get/put/delete.
	if err := h.schedulesSvc.Create(userID, model); err != nil {
		if errors.Is(err, schedules.ErrInvalidCronExpr) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return fmt.Errorf("failed to create schedule: %w", err)
	}
	return c.Status(fiber.StatusCreated).JSON(model)
}

func (h *ThirdPartyController) put(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	req := new(scheduleRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	updated, err := h.schedulesSvc.Update(permissions.EffectiveUserID(c, userID), id, req.toModel())
	if err != nil {
		if errors.Is(err, schedules.ErrNotFound) {
			return apierrors.ErrScheduleNotFound
		}
		if errors.Is(err, schedules.ErrInvalidCronExpr) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return fmt.Errorf("failed to update schedule: %w", err)
	}
	return c.JSON(updated)
}

func (h *ThirdPartyController) delete(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.schedulesSvc.Delete(permissions.EffectiveUserID(c, userID), id); err != nil {
		if errors.Is(err, schedules.ErrNotFound) {
			return apierrors.ErrScheduleNotFound
		}
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ThirdPartyController) Register(router fiber.Router) {
	router.Get("", permissions.RequireScope(ScopeList), userauth.WithUserID(h.list))
	router.Post("", permissions.RequireScope(ScopeWrite), userauth.WithUserID(h.post))
	router.Get("/:id", permissions.RequireScope(ScopeRead), userauth.WithUserID(h.get))
	router.Put("/:id", permissions.RequireScope(ScopeWrite), userauth.WithUserID(h.put))
	router.Delete("/:id", permissions.RequireScope(ScopeDelete), userauth.WithUserID(h.delete))
}
