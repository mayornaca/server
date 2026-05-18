package posts

import (
	"errors"
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/apierrors"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/permissions"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/posts"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type thirdPartyControllerParams struct {
	fx.In

	PostsSvc *posts.Service

	Validator *validator.Validate
	Logger    *zap.Logger
}

type ThirdPartyController struct {
	base.Handler

	postsSvc *posts.Service
}

func NewThirdPartyController(params thirdPartyControllerParams) *ThirdPartyController {
	return &ThirdPartyController{
		Handler: base.Handler{
			Logger:    params.Logger,
			Validator: params.Validator,
		},
		postsSvc: params.PostsSvc,
	}
}

func (h *ThirdPartyController) list(userID string, c *fiber.Ctx) error {
	var filters []posts.SelectFilter
	effectiveUID := permissions.EffectiveUserID(c, userID)

	if status := c.Query("status"); status != "" {
		filters = append(filters, posts.WithStatus(posts.PostStatus(status)))
	}
	if search := c.Query("search"); search != "" {
		filters = append(filters, posts.WithSearch(search))
	}
	// cloud-gesvial.20.1: invert the disabled-posts opt-in to make the filter
	// transversal across the panel (Dashboard, Tests, Posts toggle all align).
	// New: default = exclude disabled; pass `?includeDisabled=true` to see
	// them. Legacy: `?onlyEnabled=true` still works as a no-op alias because
	// the new default already does what it used to mean. This is a breaking
	// change for unknown 3rd-party integrators that relied on the old default
	// of "all posts" — we accept the break because the only current consumer
	// is our own panel, and any custom integrator can pass the new flag.
	if c.Query("includeDisabled") != "true" {
		filters = append(filters, posts.WithOnlyEnabled())
	}

	items, err := h.postsSvc.Select(effectiveUID, filters...)
	if err != nil {
		return fmt.Errorf("failed to list posts: %w", err)
	}

	return c.JSON(items)
}

func (h *ThirdPartyController) get(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)

	post, err := h.postsSvc.Get(effectiveUID, id)
	if err != nil {
		if errors.Is(err, posts.ErrNotFound) {
			return apierrors.ErrPostNotFound
		}
		return fmt.Errorf("failed to get post: %w", err)
	}

	return c.JSON(post)
}

func (h *ThirdPartyController) post(userID string, c *fiber.Ctx) error {
	dto := new(posts.SosPost)

	if err := h.BodyParserValidator(c, dto); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// cloud-gesvial.22.1.1: usar `userID` REAL (sub del JWT), NO EffectiveUserID.
	// EffectiveUserID(admin) → sentinel "__ADMIN__" — válido para LECTURAS
	// (admin ve todos los users vía bypass del filtro), pero al CREAR un poste
	// el sentinel se persiste como `sos_posts.user_id="__ADMIN__"`. Eso rompe
	// downstream:
	//   - el cron dispatcher invoca `posts.Service.Select(schedule.UserID)` y
	//     se ahorra el filtro user_id sólo si recibe el sentinel — los postes
	//     reales con user_id="__ADMIN__" mezclados con los de owners legítimos
	//     producen ownership ambiguo.
	//   - los `devicesSvc.Exists(post.UserID, deviceID)` devuelven false porque
	//     el device está bajo `<real_user>`, no bajo el sentinel.
	// El admin sigue viendo / borrando postes ajenos vía EffectiveUserID en
	// list/get/put/delete (los filtros bypassean el WHERE para el sentinel).
	if err := h.postsSvc.Create(userID, dto); err != nil {
		return fmt.Errorf("failed to create post: %w", err)
	}

	return c.Status(fiber.StatusCreated).JSON(dto)
}

func (h *ThirdPartyController) put(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)
	dto := new(posts.SosPost)

	if err := h.BodyParserValidator(c, dto); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.postsSvc.Update(effectiveUID, id, dto); err != nil {
		if errors.Is(err, posts.ErrNotFound) {
			return apierrors.ErrPostNotFound
		}
		return fmt.Errorf("failed to update post: %w", err)
	}

	post, err := h.postsSvc.Get(effectiveUID, id)
	if err != nil {
		return fmt.Errorf("failed to re-fetch post after update: %w", err)
	}
	return c.JSON(post)
}

type patchPostRequest struct {
	Name        *string `json:"name,omitempty"        validate:"omitempty,max=128"`
	KmMarker    *string `json:"kmMarker,omitempty"    validate:"omitempty,max=32"`
	PhoneNumber *string `json:"phoneNumber,omitempty" validate:"omitempty,max=20"`
	Status      *string `json:"status,omitempty"      validate:"omitempty,oneof=OK FAIL UNKNOWN TESTING"`
	Disabled    *bool   `json:"disabled,omitempty"`
}

func (h *ThirdPartyController) patch(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)

	req := new(patchPostRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	fields := posts.PatchFields{
		Name:        req.Name,
		KmMarker:    req.KmMarker,
		PhoneNumber: req.PhoneNumber,
		Disabled:    req.Disabled,
	}
	if req.Status != nil {
		status := posts.PostStatus(*req.Status)
		fields.Status = &status
	}

	updated, err := h.postsSvc.Patch(effectiveUID, id, fields)
	if err != nil {
		if errors.Is(err, posts.ErrNotFound) {
			return apierrors.ErrPostNotFound
		}
		return fmt.Errorf("failed to patch post: %w", err)
	}

	return c.JSON(updated)
}

func (h *ThirdPartyController) delete(userID string, c *fiber.Ctx) error {
	id := c.Params("id")
	effectiveUID := permissions.EffectiveUserID(c, userID)

	if err := h.postsSvc.Delete(effectiveUID, id); err != nil {
		return fmt.Errorf("failed to delete post: %w", err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ThirdPartyController) summary(userID string, c *fiber.Ctx) error {
	effectiveUID := permissions.EffectiveUserID(c, userID)
	// cloud-gesvial.20.1: dashboard counts default to enabled-only so
	// Total = OK + FAIL + UNKNOWN + TESTING. Pass ?includeDisabled=true for
	// admin views that want disabled posts in the count.
	includeDisabled := c.Query("includeDisabled") == "true"
	summary, err := h.postsSvc.Summary(effectiveUID, includeDisabled)
	if err != nil {
		return fmt.Errorf("failed to get summary: %w", err)
	}
	return c.JSON(summary)
}

func (h *ThirdPartyController) Register(router fiber.Router) {
	router.Get("", permissions.RequireScope(ScopeList), userauth.WithUserID(h.list))
	router.Get("/summary", permissions.RequireScope(ScopeList), userauth.WithUserID(h.summary))
	router.Post("", permissions.RequireScope(ScopeWrite), userauth.WithUserID(h.post))
	router.Get("/:id", permissions.RequireScope(ScopeRead), userauth.WithUserID(h.get))
	router.Put("/:id", permissions.RequireScope(ScopeWrite), userauth.WithUserID(h.put))
	router.Patch("/:id", permissions.RequireScope(ScopeWrite), userauth.WithUserID(h.patch))
	router.Delete("/:id", permissions.RequireScope(ScopeDelete), userauth.WithUserID(h.delete))
}
